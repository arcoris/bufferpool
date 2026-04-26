/*
  Copyright 2026 The ARCORIS Authors

  Licensed under the Apache License, Version 2.0 (the "License");
  you may not use this file except in compliance with the License.
  You may obtain a copy of the License at

      http://www.apache.org/licenses/LICENSE-2.0

  Unless required by applicable law or agreed to in writing, software
  distributed under the License is distributed on an "AS IS" BASIS,
  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
  See the License for the specific language governing permissions and
  limitations under the License.
*/

package bufferpool

import "arcoris.dev/bufferpool/internal/atomicx"

const (
	// errClassBudgetZeroClassSize is used when class-budget construction receives
	// a zero class size.
	//
	// A class budget converts byte targets into whole buffers of one normalized
	// class size. A zero class size would make that conversion undefined.
	errClassBudgetZeroClassSize = "bufferpool.classBudget: class size must be greater than zero"

	// errClassBudgetClassSizeMismatch is used when a budget owned by one class is
	// updated with a limit computed for another class size.
	//
	// Class budget state is class-local. Applying a limit produced for a different
	// class size would corrupt shard-credit distribution.
	errClassBudgetClassSizeMismatch = "bufferpool.classBudget: class size mismatch"

	// errClassBudgetInvalidLimit is used when a manually constructed class budget
	// limit violates whole-buffer arithmetic invariants.
	//
	// Limits produced by newClassBudgetLimit are valid by construction. This guard
	// exists for tests, defensive checks, and caller code paths that may build
	// limits outside the constructor.
	errClassBudgetInvalidLimit = "bufferpool.classBudget: invalid budget limit"

	// errClassBudgetShardCountInvalid is used when shard-credit planning receives
	// a non-positive shard count.
	//
	// A class target can be distributed only across at least one shard.
	errClassBudgetShardCountInvalid = "bufferpool.classBudget: shard count must be greater than zero"

	// errClassBudgetShardIndexOutOfRange is used when a shard-credit plan is
	// queried with an invalid shard index.
	errClassBudgetShardIndexOutOfRange = "bufferpool.classBudget: shard index out of range"
)

const (
	// maxClassBudgetValue is the largest value representable by class-budget
	// arithmetic.
	maxClassBudgetValue = ^uint64(0)
)

// classBudget stores the current class-level retained-memory target for one
// normalized size class.
//
// The class budget is a class-local target value. It does not store buffers,
// does not record workload counters, and does not decide admission outcomes
// directly. Its job is to translate an assigned byte target into:
//
//   - how many whole buffers of this ClassSize the class may retain;
//   - how many aligned bytes that whole-buffer target represents;
//   - how that target should be distributed into shardCreditLimit values.
//
// Responsibility boundary:
//
//   - budget allocation assigns target bytes to each class;
//   - class_budget.go normalizes that byte target to whole class buffers;
//   - class_budget.go builds a deterministic per-shard credit plan;
//   - shard_credit.go evaluates one already-derived shardCreditLimit;
//   - shard.go applies credit and stores retained buffers through bucket;
//   - class_counters.go records class-scope retained accounting;
//   - trim planning corrects over-target retained storage after contractions.
//
// Class budget uses whole-buffer arithmetic. If a class has size 4 KiB and the
// allocator assigns 10 KiB, the effective target is 2 buffers = 8 KiB, with 2 KiB
// left as unaligned remainder. The class cannot retain half of a buffer.
//
// Source of truth:
//
// assignedBytes is the only mutable budget source of truth. TargetBuffers,
// TargetBytes, and RemainderBytes are derived from assignedBytes during snapshot
// construction. This avoids mixed snapshots during concurrent updates.
//
// Concurrency:
//
// classBudget is safe for concurrent readers and control-plane updates through
// atomic fields. Snapshot arithmetic fields are internally consistent because
// they are derived from one assignedBytes load. Generation is loaded separately
// and is only an observational publication marker. classState publishes the
// derived shard-credit plan shard by shard, so budget updates are not an atomic
// class-wide cutover across all shards.
//
// Copying:
//
// classBudget MUST NOT be copied after first use because it contains atomic
// values.
type classBudget struct {
	// classSize is immutable after construction.
	classSize ClassSize

	// generation is advanced after every applied budget update.
	generation AtomicGeneration

	// assignedBytes is the raw byte target assigned to this class by a higher
	// level allocator.
	//
	// This is the only mutable budget source of truth. Derived fields are not
	// stored as independent atomics because doing so can produce inconsistent
	// snapshots under concurrent updates.
	assignedBytes atomicx.Uint64Gauge
}

// newClassBudget returns a budget owner for one normalized class size.
//
// classSize MUST be greater than zero.
func newClassBudget(classSize ClassSize) classBudget {
	if classSize.IsZero() {
		panic(errClassBudgetZeroClassSize)
	}

	return classBudget{
		classSize: classSize,
	}
}

// updateAssignedBytes computes and applies a new class budget from an assigned
// byte target.
//
// assignedBytes is the byte target produced by a higher-level allocator for this
// class. The value may be zero, which disables the effective retained target for
// the class without changing the class size.
func (b *classBudget) updateAssignedBytes(assignedBytes Size) Generation {
	if b.classSize.IsZero() {
		panic(errClassBudgetZeroClassSize)
	}

	return b.update(newClassBudgetLimit(b.classSize, assignedBytes))
}

// update applies an already computed class budget limit.
//
// The limit must belong to the same class size as this budget owner. Only the raw
// assigned byte target is stored; all whole-buffer fields are recomputed by
// snapshot readers.
func (b *classBudget) update(limit classBudgetLimit) Generation {
	limit.validate()

	if b.classSize.IsZero() {
		panic(errClassBudgetZeroClassSize)
	}

	if limit.ClassSize != b.classSize {
		panic(errClassBudgetClassSizeMismatch)
	}

	b.assignedBytes.Store(limit.AssignedBytes)

	return b.generation.Advance()
}

// disable disables the effective retained target for this class and returns the
// new generation.
//
// Disabling class budget does not physically clear retained buffers. Existing
// retained memory must be released by trim or clear paths.
func (b *classBudget) disable() Generation {
	return b.updateAssignedBytes(0)
}

// snapshot returns a point-in-time view of this class budget.
//
// Budget arithmetic fields are internally consistent because TargetBuffers,
// TargetBytes, and RemainderBytes are derived from a single assignedBytes load.
// Generation is loaded independently and is an observational publication marker;
// concurrent readers may observe a generation that is slightly older or newer
// than the assignedBytes value. That is acceptable for diagnostics, sampling,
// and shard-credit planning.
func (b *classBudget) snapshot() classBudgetSnapshot {
	if b.classSize.IsZero() {
		panic(errClassBudgetZeroClassSize)
	}

	assignedBytes := SizeFromBytes(b.assignedBytes.Load())
	limit := newClassBudgetLimit(b.classSize, assignedBytes)

	return classBudgetSnapshot{
		Generation:     b.generation.Load(),
		ClassSize:      limit.ClassSize,
		AssignedBytes:  limit.AssignedBytes,
		TargetBuffers:  limit.TargetBuffers,
		TargetBytes:    limit.TargetBytes,
		RemainderBytes: limit.RemainderBytes,
	}
}

// shardCreditPlan returns a deterministic per-shard credit plan for the current
// class budget snapshot.
func (b *classBudget) shardCreditPlan(shardCount int) classShardCreditPlan {
	return b.snapshot().shardCreditPlan(shardCount)
}

// classBudgetLimit is the effective whole-buffer budget for one size class.
//
// AssignedBytes is the raw byte target assigned by a higher-level allocator.
// TargetBuffers is floor(AssignedBytes / ClassSize). TargetBytes is
// TargetBuffers * ClassSize. RemainderBytes is AssignedBytes - TargetBytes.
//
// A zero AssignedBytes value is valid and produces a disabled effective target.
type classBudgetLimit struct {
	// ClassSize is the normalized buffer capacity of the class this limit belongs
	// to.
	ClassSize ClassSize

	// AssignedBytes is the raw class byte target assigned by a higher-level
	// allocator.
	AssignedBytes uint64

	// TargetBuffers is the number of whole class-size buffers funded by
	// AssignedBytes.
	TargetBuffers uint64

	// TargetBytes is TargetBuffers * ClassSize.
	TargetBytes uint64

	// RemainderBytes is AssignedBytes - TargetBytes.
	RemainderBytes uint64
}

// newClassBudgetLimit returns a whole-buffer class budget limit.
//
// classSize MUST be greater than zero. assignedBytes may be zero.
func newClassBudgetLimit(classSize ClassSize, assignedBytes Size) classBudgetLimit {
	if classSize.IsZero() {
		panic(errClassBudgetZeroClassSize)
	}

	classBytes := classSize.Bytes()
	assigned := assignedBytes.Bytes()

	targetBuffers := assigned / classBytes
	targetBytes := targetBuffers * classBytes
	remainderBytes := assigned - targetBytes

	return classBudgetLimit{
		ClassSize:      classSize,
		AssignedBytes:  assigned,
		TargetBuffers:  targetBuffers,
		TargetBytes:    targetBytes,
		RemainderBytes: remainderBytes,
	}
}

// validate verifies the internal arithmetic invariants of the class budget
// limit.
//
// The constructor always produces a valid limit. This method exists because tests
// and caller code may manually construct limits. Invalid arithmetic is treated
// as a programming error.
func (l classBudgetLimit) validate() {
	if l.ClassSize.IsZero() {
		panic(errClassBudgetZeroClassSize)
	}

	classBytes := l.ClassSize.Bytes()

	if l.TargetBuffers > maxClassBudgetValue/classBytes {
		panic(errClassBudgetInvalidLimit)
	}

	expectedTargetBytes := l.TargetBuffers * classBytes
	if l.TargetBytes != expectedTargetBytes {
		panic(errClassBudgetInvalidLimit)
	}

	if l.RemainderBytes >= classBytes {
		panic(errClassBudgetInvalidLimit)
	}

	if l.TargetBytes > maxClassBudgetValue-l.RemainderBytes {
		panic(errClassBudgetInvalidLimit)
	}

	expectedAssignedBytes := l.TargetBytes + l.RemainderBytes
	if l.AssignedBytes != expectedAssignedBytes {
		panic(errClassBudgetInvalidLimit)
	}
}

// IsZero reports whether this limit has no effective retained target and no
// assigned bytes.
func (l classBudgetLimit) IsZero() bool {
	return l.AssignedBytes == 0 &&
		l.TargetBuffers == 0 &&
		l.TargetBytes == 0 &&
		l.RemainderBytes == 0
}

// IsEffective reports whether this limit funds at least one whole class-size
// buffer.
func (l classBudgetLimit) IsEffective() bool {
	return l.TargetBuffers > 0 && l.TargetBytes > 0
}

// Limit returns l as a shard credit only when the entire class budget is treated
// as one shard.
//
// Most class owners should use shardCreditPlan instead.
func (l classBudgetLimit) Limit() shardCreditLimit {
	l.validate()

	if !l.IsEffective() {
		return shardCreditLimit{}
	}

	return newShardCreditLimit(l.TargetBuffers, l.TargetBytes)
}

// shardCreditPlan returns a deterministic distribution of this class budget
// limit across shardCount shards.
func (l classBudgetLimit) shardCreditPlan(shardCount int) classShardCreditPlan {
	l.validate()

	return newClassShardCreditPlan(l, shardCount)
}

// classBudgetSnapshot is an immutable view of one class budget owner.
//
// The snapshot is internal. Public policy or metrics snapshots should be modeled
// separately so internal budget mechanics can evolve.
//
// Snapshot arithmetic fields are expected to be internally consistent when the
// snapshot comes from classBudget.snapshot. Manually constructed snapshots can be
// invalid; shardCreditPlan validates through Limit().
type classBudgetSnapshot struct {
	// Generation identifies the observed budget publication.
	Generation Generation

	// ClassSize is the normalized capacity of this class.
	ClassSize ClassSize

	// AssignedBytes is the raw byte target assigned to this class.
	AssignedBytes uint64

	// TargetBuffers is the number of whole class-size buffers funded by the
	// assigned target.
	TargetBuffers uint64

	// TargetBytes is TargetBuffers * ClassSize.
	TargetBytes uint64

	// RemainderBytes is AssignedBytes - TargetBytes.
	RemainderBytes uint64
}

// Limit returns the budget limit represented by this snapshot.
func (s classBudgetSnapshot) Limit() classBudgetLimit {
	return classBudgetLimit{
		ClassSize:      s.ClassSize,
		AssignedBytes:  s.AssignedBytes,
		TargetBuffers:  s.TargetBuffers,
		TargetBytes:    s.TargetBytes,
		RemainderBytes: s.RemainderBytes,
	}
}

// IsZero reports whether the snapshot contains no assigned or effective budget.
func (s classBudgetSnapshot) IsZero() bool {
	return s.AssignedBytes == 0 &&
		s.TargetBuffers == 0 &&
		s.TargetBytes == 0 &&
		s.RemainderBytes == 0
}

// IsEffective reports whether the snapshot funds at least one whole class-size
// buffer.
func (s classBudgetSnapshot) IsEffective() bool {
	return s.TargetBuffers > 0 && s.TargetBytes > 0
}

// shardCreditPlan returns a deterministic per-shard credit plan for this
// snapshot.
func (s classBudgetSnapshot) shardCreditPlan(shardCount int) classShardCreditPlan {
	return s.Limit().shardCreditPlan(shardCount)
}

// classShardCreditPlan describes how one class budget is distributed across
// shards.
//
// Distribution is done in whole buffers, not raw bytes. If target buffers do not
// divide evenly across shards, the first ExtraShards shards receive one extra
// buffer. This gives deterministic, stable distribution without requiring a
// separate balancing data structure.
//
// Example:
//
//	TargetBuffers = 10
//	ShardCount    = 4
//
//	Result:
//	shard 0 -> 3 buffers
//	shard 1 -> 3 buffers
//	shard 2 -> 2 buffers
//	shard 3 -> 2 buffers
type classShardCreditPlan struct {
	// ClassSize is the normalized capacity of this class.
	ClassSize ClassSize

	// ShardCount is the number of shards receiving credit.
	ShardCount int

	// TargetBuffers is the total number of class buffers distributed by this
	// plan.
	TargetBuffers uint64

	// TargetBytes is TargetBuffers * ClassSize.
	TargetBytes uint64

	// BaseBuffersPerShard is the number of buffers assigned to every shard before
	// remainder distribution.
	BaseBuffersPerShard uint64

	// ExtraShards is the number of lowest-index shards receiving one additional
	// buffer.
	ExtraShards int
}

// newClassShardCreditPlan returns a deterministic shard-credit plan.
func newClassShardCreditPlan(limit classBudgetLimit, shardCount int) classShardCreditPlan {
	limit.validate()

	if shardCount <= 0 {
		panic(errClassBudgetShardCountInvalid)
	}

	baseBuffers := limit.TargetBuffers / uint64(shardCount)
	extraShards := int(limit.TargetBuffers % uint64(shardCount))

	return classShardCreditPlan{
		ClassSize:           limit.ClassSize,
		ShardCount:          shardCount,
		TargetBuffers:       limit.TargetBuffers,
		TargetBytes:         limit.TargetBytes,
		BaseBuffersPerShard: baseBuffers,
		ExtraShards:         extraShards,
	}
}

// IsZero reports whether this plan distributes no effective shard credit.
func (p classShardCreditPlan) IsZero() bool {
	return p.TargetBuffers == 0 && p.TargetBytes == 0
}

// creditForShard returns the shard-local credit assigned to shardIndex.
//
// shardIndex MUST be in the range [0, ShardCount).
func (p classShardCreditPlan) creditForShard(shardIndex int) shardCreditLimit {
	if shardIndex < 0 || shardIndex >= p.ShardCount {
		panic(errClassBudgetShardIndexOutOfRange)
	}

	if p.IsZero() {
		return shardCreditLimit{}
	}

	buffers := p.BaseBuffersPerShard
	if shardIndex < p.ExtraShards {
		buffers++
	}

	if buffers == 0 {
		return shardCreditLimit{}
	}

	bytes := buffers * p.ClassSize.Bytes()

	return newShardCreditLimit(buffers, bytes)
}

// credits returns shard-local credit limits for all shards in this plan.
//
// The returned slice is new and may be modified by the caller.
func (p classShardCreditPlan) credits() []shardCreditLimit {
	credits := make([]shardCreditLimit, p.ShardCount)
	for index := range credits {
		credits[index] = p.creditForShard(index)
	}

	return credits
}

// totalCredit returns the total buffer and byte credit represented by all shard
// limits in this plan.
func (p classShardCreditPlan) totalCredit() classShardCreditTotal {
	credits := p.credits()

	var buffers uint64
	var bytes uint64

	for _, credit := range credits {
		buffers += credit.TargetBuffers
		bytes += credit.TargetBytes
	}

	return classShardCreditTotal{
		Buffers: buffers,
		Bytes:   bytes,
	}
}

// classShardCreditTotal describes the sum of shard-local credit limits.
type classShardCreditTotal struct {
	// Buffers is total retained-buffer credit.
	Buffers uint64

	// Bytes is total retained-byte credit.
	Bytes uint64
}

// IsZero reports whether this total contains no buffer or byte credit.
func (t classShardCreditTotal) IsZero() bool {
	return t.Buffers == 0 && t.Bytes == 0
}
