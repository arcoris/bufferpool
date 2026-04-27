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

// PoolSnapshot is an observational point-in-time view of a Pool.
//
// Snapshot is safe for diagnostics, tests, and external metrics projection. It
// is intentionally allocation-friendly for callers rather than allocation-free:
// it clones Policy and allocates public class/shard slices so the caller owns
// the returned data. Future controller ticks should use internal sampling
// helpers instead of depending on this public diagnostic shape.
//
// The view is not a globally atomic transaction across all classes and shards.
// Each class/shard snapshot is race-safe, but concurrent operations may make
// different classes represent slightly different instants.
type PoolSnapshot struct {
	// Name is diagnostic metadata copied from Pool construction config.
	Name string

	// Lifecycle is the lifecycle state observed while building the snapshot.
	Lifecycle LifecycleState

	// Generation is the runtime policy publication generation observed by Pool.
	Generation Generation

	// Policy is a defensive copy of the effective runtime Policy.
	Policy Policy

	// Classes contains one snapshot per configured size class.
	Classes []PoolClassSnapshot

	// Counters contains aggregate lifetime counters plus derived current usage.
	Counters PoolCountersSnapshot

	// CurrentRetainedBuffers is derived from class/shard snapshots.
	CurrentRetainedBuffers uint64

	// CurrentRetainedBytes is derived from class/shard snapshots.
	CurrentRetainedBytes uint64
}

// ClassCount returns the number of class snapshots.
func (s PoolSnapshot) ClassCount() int {
	return len(s.Classes)
}

// ShardCount returns the total number of shard snapshots across all classes.
func (s PoolSnapshot) ShardCount() int {
	var total int

	for _, class := range s.Classes {
		total += len(class.Shards)
	}

	return total
}

// IsEmpty reports whether the snapshot has no retained storage and no observed
// activity.
func (s PoolSnapshot) IsEmpty() bool {
	return s.CurrentRetainedBuffers == 0 &&
		s.CurrentRetainedBytes == 0 &&
		s.Counters.IsZero()
}

// PoolClassSnapshot is an observational view of one Pool-owned size class.
//
// Class current retained usage is derived from shard snapshots. classCounters
// remain lifetime/window counters and do not own exact current retained gauges.
type PoolClassSnapshot struct {
	// Class is the immutable class descriptor assigned by the Pool class table.
	Class SizeClass

	// Budget is the observed class budget publication.
	Budget PoolClassBudgetSnapshot

	// Counters contains class lifetime counters plus derived current usage.
	Counters PoolCountersSnapshot

	// Shards contains one locally consistent snapshot per shard in the class.
	Shards []PoolShardSnapshot

	// CurrentRetainedBuffers is derived from shard current retained gauges.
	CurrentRetainedBuffers uint64

	// CurrentRetainedBytes is derived from shard current retained gauges.
	CurrentRetainedBytes uint64
}

// ShardCount returns the number of shard snapshots in this class.
func (s PoolClassSnapshot) ShardCount() int {
	return len(s.Shards)
}

// IsOverBudget reports whether current retained usage exceeds the observed class
// budget in either buffer-count or byte dimension.
//
// A disabled budget treats any retained usage as over-budget.
func (s PoolClassSnapshot) IsOverBudget() bool {
	if !s.Budget.IsEffective() {
		return s.CurrentRetainedBuffers != 0 || s.CurrentRetainedBytes != 0
	}

	if s.CurrentRetainedBuffers > s.Budget.TargetBuffers {
		return true
	}

	return s.CurrentRetainedBytes > s.Budget.TargetBytes
}

// PoolShardSnapshot is an observational view of one shard inside one class.
//
// Shard is the authoritative owner of current retained state. Bucket state,
// shard counters, and shard credit are sampled while shard.mu is held by
// shard.state, so this snapshot is locally consistent for one shard.
type PoolShardSnapshot struct {
	// Index is the shard index inside its owning class.
	Index int

	// Bucket is the physical retained-storage state.
	Bucket PoolBucketSnapshot

	// Credit is the shard-local retention credit observed with the bucket.
	Credit PoolShardCreditSnapshot

	// Counters contains shard lifetime counters and authoritative current
	// retained gauges.
	Counters PoolCountersSnapshot

	// CurrentRetainedBuffers mirrors Counters.CurrentRetainedBuffers for direct
	// access by diagnostics.
	CurrentRetainedBuffers uint64

	// CurrentRetainedBytes mirrors Counters.CurrentRetainedBytes for direct
	// access by diagnostics.
	CurrentRetainedBytes uint64
}

// IsOverCredit reports whether current retained usage exceeds the observed shard
// credit in either buffer-count or byte dimension.
//
// Disabled or inconsistent credit treats any retained usage as over-credit.
func (s PoolShardSnapshot) IsOverCredit() bool {
	if !s.Credit.IsConsistent() || !s.Credit.IsEnabled() {
		return s.CurrentRetainedBuffers != 0 || s.CurrentRetainedBytes != 0
	}

	if s.CurrentRetainedBuffers > s.Credit.TargetBuffers {
		return true
	}

	return s.CurrentRetainedBytes > s.Credit.TargetBytes
}

// PoolBucketSnapshot describes one shard bucket's physical retained-storage
// state.
//
// Bucket is raw no-lock storage. This public snapshot is produced only through
// shard-owned synchronization, not by reading bucket directly from Pool.
type PoolBucketSnapshot struct {
	// RetainedBuffers is the number of occupied retained-buffer slots.
	RetainedBuffers int

	// RetainedBytes is the sum of retained buffer capacities.
	RetainedBytes uint64

	// SlotLimit is the fixed bucket capacity in buffer slots.
	SlotLimit int

	// AvailableSlots is SlotLimit - RetainedBuffers.
	AvailableSlots int
}

// IsEmpty reports whether the bucket retains no buffers.
func (s PoolBucketSnapshot) IsEmpty() bool {
	return s.RetainedBuffers == 0
}

// IsFull reports whether the bucket has no free retained-buffer slots.
func (s PoolBucketSnapshot) IsFull() bool {
	return s.AvailableSlots == 0
}

// PoolClassBudgetSnapshot describes the observed class budget.
//
// A class budget is a target publication, not a current usage ledger. Current
// retained usage is derived from shard snapshots.
type PoolClassBudgetSnapshot struct {
	// Generation identifies the budget publication version.
	Generation Generation

	// ClassSize is the whole-buffer size used for buffer-count arithmetic.
	ClassSize ClassSize

	// AssignedBytes is the byte budget assigned to the class before
	// whole-buffer normalization.
	AssignedBytes uint64

	// TargetBuffers is the whole number of class-sized buffers funded.
	TargetBuffers uint64

	// TargetBytes is TargetBuffers * ClassSize.
	TargetBytes uint64

	// RemainderBytes is AssignedBytes not large enough to fund another buffer.
	RemainderBytes uint64
}

// IsZero reports whether the class budget has no assigned or effective target.
func (s PoolClassBudgetSnapshot) IsZero() bool {
	return s.AssignedBytes == 0 &&
		s.TargetBuffers == 0 &&
		s.TargetBytes == 0 &&
		s.RemainderBytes == 0
}

// IsEffective reports whether the class budget funds at least one whole
// class-size buffer.
func (s PoolClassBudgetSnapshot) IsEffective() bool {
	return s.TargetBuffers > 0 && s.TargetBytes > 0
}

// PoolShardCreditSnapshot describes the observed shard-local retention credit.
//
// Credit is a local admission gate, not a retained-memory ledger. Consistency is
// class-size-aware: byte credit must be able to fund the advertised number of
// class-sized buffers.
type PoolShardCreditSnapshot struct {
	// Generation identifies the credit publication version.
	Generation Generation

	// ClassSize is required to compare byte and buffer dimensions correctly.
	ClassSize ClassSize

	// TargetBuffers is the shard-local retained-buffer count target.
	TargetBuffers uint64

	// TargetBytes is the shard-local retained-capacity target.
	TargetBytes uint64
}

// IsEnabled reports whether shard credit permits any retained storage.
func (s PoolShardCreditSnapshot) IsEnabled() bool {
	return s.TargetBuffers > 0 && s.TargetBytes > 0
}

// IsZero reports whether shard credit is disabled.
func (s PoolShardCreditSnapshot) IsZero() bool {
	return s.TargetBuffers == 0 && s.TargetBytes == 0
}

// IsPartial reports whether only one credit dimension is enabled.
func (s PoolShardCreditSnapshot) IsPartial() bool {
	return (s.TargetBuffers == 0) != (s.TargetBytes == 0)
}

// IsConsistent reports whether credit is disabled or internally consistent.
func (s PoolShardCreditSnapshot) IsConsistent() bool {
	if s.IsZero() {
		return true
	}

	if s.IsPartial() {
		return false
	}

	if s.ClassSize.IsZero() {
		return false
	}

	return s.TargetBytes >= poolSaturatingProduct(s.TargetBuffers, s.ClassSize.Bytes())
}

// PoolCountersSnapshot is a public aggregate counter projection.
//
// Lifetime fields are monotonic counters. CurrentRetainedBuffers and
// CurrentRetainedBytes are gauges derived from the snapshot level that produced
// this value. Pool aggregate counters include owner-side Put/drop counters plus
// class/shard counters without double counting owner-side and lower-layer drops.
type PoolCountersSnapshot struct {
	// Gets is the number of acquisition attempts that reached classState.
	Gets uint64

	// RequestedBytes is the sum of requested logical lengths for Gets.
	RequestedBytes uint64

	// Hits is the number of acquisition attempts served from retained storage.
	Hits uint64

	// HitBytes is the sum of returned retained buffer capacities on hits.
	HitBytes uint64

	// Misses is the number of acquisition attempts that allocated.
	Misses uint64

	// Allocations is the number of miss-path backing array allocations recorded.
	Allocations uint64

	// AllocatedBytes is the sum of capacities allocated on misses.
	AllocatedBytes uint64

	// Puts is the number of valid public Put attempts at Pool aggregate level,
	// or class/shard return attempts at lower snapshot levels.
	//
	// At Pool aggregate level, Puts can exceed Retains + Drops because a valid
	// Put rejected by lifecycle in strict closed mode is still a public Put
	// attempt but has no retained/dropped outcome.
	Puts uint64

	// ReturnedBytes is the sum of returned buffer capacities.
	ReturnedBytes uint64

	// Retains is the number of returned buffers accepted into retained storage.
	Retains uint64

	// RetainedBytes is the sum of capacities accepted into retained storage.
	RetainedBytes uint64

	// Drops is the number of returned buffers not retained.
	Drops uint64

	// DroppedBytes is the sum of capacities not retained.
	DroppedBytes uint64

	// DropReasons contains owner-side drop reasons for Pool aggregate snapshots.
	DropReasons PoolDropReasonCounters

	// TrimOperations is the number of trim calls recorded.
	TrimOperations uint64

	// TrimmedBuffers is the number of retained buffers removed by trim.
	TrimmedBuffers uint64

	// TrimmedBytes is the retained capacity removed by trim.
	TrimmedBytes uint64

	// ClearOperations is the number of clear calls recorded.
	ClearOperations uint64

	// ClearedBuffers is the number of retained buffers removed by clear.
	ClearedBuffers uint64

	// ClearedBytes is the retained capacity removed by clear.
	ClearedBytes uint64

	// CurrentRetainedBuffers is the current retained-buffer gauge.
	CurrentRetainedBuffers uint64

	// CurrentRetainedBytes is the current retained-capacity gauge.
	CurrentRetainedBytes uint64
}

// IsZero reports whether the counter snapshot contains no observed activity and
// no retained usage.
func (s PoolCountersSnapshot) IsZero() bool {
	return s.Gets == 0 &&
		s.RequestedBytes == 0 &&
		s.Hits == 0 &&
		s.HitBytes == 0 &&
		s.Misses == 0 &&
		s.Allocations == 0 &&
		s.AllocatedBytes == 0 &&
		s.Puts == 0 &&
		s.ReturnedBytes == 0 &&
		s.Retains == 0 &&
		s.RetainedBytes == 0 &&
		s.Drops == 0 &&
		s.DroppedBytes == 0 &&
		s.DropReasons.IsZero() &&
		s.TrimOperations == 0 &&
		s.TrimmedBuffers == 0 &&
		s.TrimmedBytes == 0 &&
		s.ClearOperations == 0 &&
		s.ClearedBuffers == 0 &&
		s.ClearedBytes == 0 &&
		s.CurrentRetainedBuffers == 0 &&
		s.CurrentRetainedBytes == 0
}

// ReuseAttempts returns hit + miss attempts.
func (s PoolCountersSnapshot) ReuseAttempts() uint64 {
	return s.Hits + s.Misses
}

// PutOutcomes returns retain + drop outcomes.
//
// This is not guaranteed to equal Puts. Valid Put calls rejected by lifecycle in
// strict closed mode are counted as Puts but not as retention outcomes.
func (s PoolCountersSnapshot) PutOutcomes() uint64 {
	return s.Retains + s.Drops
}

// RemovalOperations returns trim + clear operation attempts.
func (s PoolCountersSnapshot) RemovalOperations() uint64 {
	return s.TrimOperations + s.ClearOperations
}

// RemovedBuffers returns buffers removed by trim and clear.
func (s PoolCountersSnapshot) RemovedBuffers() uint64 {
	return s.TrimmedBuffers + s.ClearedBuffers
}

// RemovedBytes returns retained capacity removed by trim and clear.
func (s PoolCountersSnapshot) RemovedBytes() uint64 {
	return s.TrimmedBytes + s.ClearedBytes
}

// Snapshot returns an observational Pool snapshot.
//
// The returned Policy and class slices are independent from Pool-owned mutable
// storage. Mutating the snapshot cannot affect the live Pool.
func (p *Pool) Snapshot() PoolSnapshot {
	p.mustBeInitialized()

	runtime := p.currentRuntimeSnapshot()
	classes := make([]PoolClassSnapshot, len(p.classes))

	var counters PoolCountersSnapshot
	var currentRetainedBuffers uint64
	var currentRetainedBytes uint64

	for index := range p.classes {
		classSnapshot := newPoolClassSnapshot(p.classes[index].state())
		classes[index] = classSnapshot

		poolCountersAdd(&counters, classSnapshot.Counters)

		currentRetainedBuffers += classSnapshot.CurrentRetainedBuffers
		currentRetainedBytes += classSnapshot.CurrentRetainedBytes
	}

	counters.CurrentRetainedBuffers = currentRetainedBuffers
	counters.CurrentRetainedBytes = currentRetainedBytes
	poolCountersApplyOwner(&counters, p.ownerCounters.snapshot())

	return PoolSnapshot{
		Name:                   p.name,
		Lifecycle:              p.lifecycle.Load(),
		Generation:             runtime.Generation,
		Policy:                 runtime.clonePolicy(),
		Classes:                classes,
		Counters:               counters,
		CurrentRetainedBuffers: currentRetainedBuffers,
		CurrentRetainedBytes:   currentRetainedBytes,
	}
}

// newPoolClassSnapshot converts an internal classState snapshot to the public
// Pool class DTO.
//
// The conversion allocates shard snapshots because public Snapshot returns data
// that callers can keep independently from Pool internals.
func newPoolClassSnapshot(snapshot classStateSnapshot) PoolClassSnapshot {
	shards := make([]PoolShardSnapshot, len(snapshot.Shards))

	for index, shard := range snapshot.Shards {
		shards[index] = newPoolShardSnapshot(index, snapshot.Class, shard)
	}

	return PoolClassSnapshot{
		Class: snapshot.Class,
		Budget: PoolClassBudgetSnapshot{
			Generation:     snapshot.Budget.Generation,
			ClassSize:      snapshot.Budget.ClassSize,
			AssignedBytes:  snapshot.Budget.AssignedBytes,
			TargetBuffers:  snapshot.Budget.TargetBuffers,
			TargetBytes:    snapshot.Budget.TargetBytes,
			RemainderBytes: snapshot.Budget.RemainderBytes,
		},
		Counters:               newPoolCountersFromClass(snapshot.Counters, snapshot.CurrentRetainedBuffers, snapshot.CurrentRetainedBytes),
		Shards:                 shards,
		CurrentRetainedBuffers: snapshot.CurrentRetainedBuffers,
		CurrentRetainedBytes:   snapshot.CurrentRetainedBytes,
	}
}

// newPoolShardSnapshot converts one internal shard state to the public shard
// DTO.
//
// The owning class descriptor supplies ClassSize so credit consistency can be
// checked across byte and buffer dimensions without comparing unrelated units.
func newPoolShardSnapshot(index int, class SizeClass, snapshot shardState) PoolShardSnapshot {
	counters := newPoolCountersFromShard(snapshot.Counters)

	return PoolShardSnapshot{
		Index: index,
		Bucket: PoolBucketSnapshot{
			RetainedBuffers: snapshot.Bucket.RetainedBuffers,
			RetainedBytes:   snapshot.Bucket.RetainedBytes,
			SlotLimit:       snapshot.Bucket.SlotLimit,
			AvailableSlots:  snapshot.Bucket.AvailableSlots,
		},
		Credit: PoolShardCreditSnapshot{
			Generation:    snapshot.Credit.Generation,
			ClassSize:     class.Size(),
			TargetBuffers: snapshot.Credit.TargetBuffers,
			TargetBytes:   snapshot.Credit.TargetBytes,
		},
		Counters:               counters,
		CurrentRetainedBuffers: counters.CurrentRetainedBuffers,
		CurrentRetainedBytes:   counters.CurrentRetainedBytes,
	}
}

// newPoolCountersFromClass maps class lifetime counters into the public counter
// projection.
//
// classCounters do not own current retained gauges. The caller supplies the
// derived class current usage from classStateSnapshot.
func newPoolCountersFromClass(snapshot classCountersSnapshot, currentBuffers uint64, currentBytes uint64) PoolCountersSnapshot {
	return PoolCountersSnapshot{
		Gets:           snapshot.Gets,
		RequestedBytes: snapshot.RequestedBytes,

		Hits:     snapshot.Hits,
		HitBytes: snapshot.HitBytes,
		Misses:   snapshot.Misses,

		Allocations:    snapshot.Allocations,
		AllocatedBytes: snapshot.AllocatedBytes,

		Puts:          snapshot.Puts,
		ReturnedBytes: snapshot.ReturnedBytes,

		Retains:       snapshot.Retains,
		RetainedBytes: snapshot.RetainedBytes,

		Drops:        snapshot.Drops,
		DroppedBytes: snapshot.DroppedBytes,

		TrimOperations: snapshot.TrimOperations,
		TrimmedBuffers: snapshot.TrimmedBuffers,
		TrimmedBytes:   snapshot.TrimmedBytes,

		ClearOperations: snapshot.ClearOperations,
		ClearedBuffers:  snapshot.ClearedBuffers,
		ClearedBytes:    snapshot.ClearedBytes,

		CurrentRetainedBuffers: currentBuffers,
		CurrentRetainedBytes:   currentBytes,
	}
}

// newPoolCountersFromShard maps shard counters into the public counter
// projection.
//
// Shard current retained gauges are authoritative, so they can be copied
// directly from the shard counter snapshot.
func newPoolCountersFromShard(snapshot shardCountersSnapshot) PoolCountersSnapshot {
	return PoolCountersSnapshot{
		Gets:           snapshot.Gets,
		RequestedBytes: snapshot.RequestedBytes,

		Hits:     snapshot.Hits,
		HitBytes: snapshot.HitBytes,
		Misses:   snapshot.Misses,

		Allocations:    snapshot.Allocations,
		AllocatedBytes: snapshot.AllocatedBytes,

		Puts:          snapshot.Puts,
		ReturnedBytes: snapshot.ReturnedBytes,

		Retains:       snapshot.Retains,
		RetainedBytes: snapshot.RetainedBytes,

		Drops:        snapshot.Drops,
		DroppedBytes: snapshot.DroppedBytes,

		TrimOperations: snapshot.TrimOperations,
		TrimmedBuffers: snapshot.TrimmedBuffers,
		TrimmedBytes:   snapshot.TrimmedBytes,

		ClearOperations: snapshot.ClearOperations,
		ClearedBuffers:  snapshot.ClearedBuffers,
		ClearedBytes:    snapshot.ClearedBytes,

		CurrentRetainedBuffers: snapshot.CurrentRetainedBuffers,
		CurrentRetainedBytes:   snapshot.CurrentRetainedBytes,
	}
}

// poolCountersAdd adds src into dst.
//
// This helper is used for observational aggregation. It does not perform
// overflow checks because counters are monotonic uint64 runtime counters; a
// wrap would already mean the sampled counter stream exceeded its representable
// diagnostic range.
func poolCountersAdd(dst *PoolCountersSnapshot, src PoolCountersSnapshot) {
	dst.Gets += src.Gets
	dst.RequestedBytes += src.RequestedBytes

	dst.Hits += src.Hits
	dst.HitBytes += src.HitBytes
	dst.Misses += src.Misses

	dst.Allocations += src.Allocations
	dst.AllocatedBytes += src.AllocatedBytes

	dst.Puts += src.Puts
	dst.ReturnedBytes += src.ReturnedBytes

	dst.Retains += src.Retains
	dst.RetainedBytes += src.RetainedBytes

	dst.Drops += src.Drops
	dst.DroppedBytes += src.DroppedBytes
	dst.DropReasons = poolDropReasonCountersAdd(dst.DropReasons, src.DropReasons)

	dst.TrimOperations += src.TrimOperations
	dst.TrimmedBuffers += src.TrimmedBuffers
	dst.TrimmedBytes += src.TrimmedBytes

	dst.ClearOperations += src.ClearOperations
	dst.ClearedBuffers += src.ClearedBuffers
	dst.ClearedBytes += src.ClearedBytes

	dst.CurrentRetainedBuffers += src.CurrentRetainedBuffers
	dst.CurrentRetainedBytes += src.CurrentRetainedBytes
}

// poolCountersApplyOwner merges Pool owner-side counters into an aggregate
// counter snapshot.
//
// Owner Puts/ReturnedBytes replace the class sum because they represent every
// valid public Put attempt, including attempts that never reached classState.
// Owner Drops are added to lower-layer drops because they describe distinct
// no-retain outcomes.
func poolCountersApplyOwner(dst *PoolCountersSnapshot, owner poolOwnerCountersSnapshot) {
	dst.Puts = owner.Puts
	dst.ReturnedBytes = owner.ReturnedBytes
	dst.Drops += owner.Drops
	dst.DroppedBytes += owner.DroppedBytes
	dst.DropReasons = poolDropReasonCountersAdd(dst.DropReasons, owner.DropReasons)
}

// poolDropReasonCountersAdd returns the field-wise sum of two owner-side reason
// counter sets.
//
// Reason counters are intentionally separate from lower-layer drop counters, so
// this helper is only for merging owner-side reason projections.
func poolDropReasonCountersAdd(left, right PoolDropReasonCounters) PoolDropReasonCounters {
	return PoolDropReasonCounters{
		ClosedPool:              left.ClosedPool + right.ClosedPool,
		ReturnedBuffersDisabled: left.ReturnedBuffersDisabled + right.ReturnedBuffersDisabled,
		Oversized:               left.Oversized + right.Oversized,
		UnsupportedClass:        left.UnsupportedClass + right.UnsupportedClass,
		InvalidPolicy:           left.InvalidPolicy + right.InvalidPolicy,
	}
}
