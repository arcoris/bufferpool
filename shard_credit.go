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
	// errShardCreditPartialLimit is used when a credit limit contains only one
	// enabled dimension.
	//
	// Shard credit is intentionally two-dimensional. A disabled limit is
	// represented as 0 buffers and 0 bytes. An enabled limit must have both a
	// positive buffer-count credit and a positive byte credit.
	errShardCreditPartialLimit = "bufferpool.shardCredit: buffer and byte credit must be enabled or disabled together"

	// errShardCreditInconsistentLimit is used when an enabled credit limit cannot
	// possibly retain the configured number of buffers.
	//
	// Every retained buffer has positive capacity. Therefore an enabled credit
	// with TargetBytes < TargetBuffers is internally inconsistent: it allows more
	// buffer slots than the byte limit can ever fund, even for one-byte buffers.
	errShardCreditInconsistentLimit = "bufferpool.shardCredit: byte credit must be greater than or equal to buffer credit"
)

const (
	// maxShardCreditValue is the largest value representable by shard credit
	// arithmetic.
	maxShardCreditValue = ^uint64(0)
)

// shardCredit stores the current local retention credit for one shard.
//
// A shard credit is a hot-path admission gate. It is not the owner of budget
// allocation and it is not the authoritative retained-memory ledger.
//
// Responsibility boundary:
//
//   - budget allocation computes higher-level budget distribution;
//   - class_budget.go derives per-shard credit limits from class targets;
//   - shard_credit.go stores and evaluates one shard-local credit limit;
//   - admission code maps credit decisions to public drop reasons;
//   - bucket.go physically stores retained buffers;
//   - bucket_trim.go physically removes retained buffers;
//   - shard_counters.go records retained and removed accounting;
//   - trim planning corrects over-target retained memory on the cold path.
//
// shardCredit deliberately does not know ClassSize, ClassID, SizeClass,
// classTable, pool budget, partition budget, pressure policy, or trim victims.
// Those belong to higher layers.
//
// This separation is important:
//
//   - class_budget.go may use ClassSize to compute a shard-local limit;
//   - shard_credit.go receives the already-derived TargetBuffers and TargetBytes;
//   - shard_credit.go only answers whether a candidate capacity fits that limit.
//
// Credit dimensions:
//
//   - TargetBuffers limits retained buffer count;
//   - TargetBytes limits retained backing capacity.
//
// Both dimensions are required. Buffer count alone does not bound memory, and
// byte credit alone does not bound slot/metadata pressure.
//
// Concurrency:
//
// shardCredit is safe for concurrent hot-path reads and control-plane updates.
// The target fields are loaded independently. A reader may observe a mixed pair
// during a concurrent update. This is acceptable because inconsistent observed
// credit rejects retention, while higher-level budget state and corrective trim
// remain authoritative after target changes.
//
// Copying:
//
// shardCredit MUST NOT be copied after first use because it contains atomic
// values.
type shardCredit struct {
	// generation is advanced after every applied credit update.
	//
	// It gives snapshots a cheap publication marker for tests, diagnostics,
	// controller sampling and policy snapshot correlation.
	generation AtomicGeneration

	// targetBuffers is the current local retained-buffer credit.
	targetBuffers atomicx.Uint64Gauge

	// targetBytes is the current local retained-byte credit.
	targetBytes atomicx.Uint64Gauge
}

// update applies a new shard-local credit limit and returns the new generation.
//
// The supplied limit must be either disabled in both dimensions or enabled in
// both dimensions. Partial limits are rejected because admission must evaluate
// both buffer-count and byte-capacity credit consistently.
//
// The generation is advanced after the target fields are stored. A concurrent
// reader may still observe old, new, or mixed target values; inconsistent mixed
// target values reject retention.
func (c *shardCredit) update(limit shardCreditLimit) Generation {
	limit.validate()

	c.targetBuffers.Store(limit.TargetBuffers)
	c.targetBytes.Store(limit.TargetBytes)

	return c.generation.Advance()
}

// disable disables local retention credit and returns the new generation.
//
// Disabled credit means shard admission must reject new retention through the
// credit gate. It does not clear already retained buffers. Physical cleanup
// belongs to trim/clear paths.
func (c *shardCredit) disable() Generation {
	return c.update(shardCreditLimit{})
}

// snapshot returns a point-in-time view of the current shard credit.
//
// The snapshot is not a strict multi-field atomic publication. Individual fields
// are loaded atomically, but concurrent updates can produce a mixed pair. This is
// acceptable because shard code applies credit against retained usage under its
// own synchronization, and budget state remains the higher-level target source.
func (c *shardCredit) snapshot() shardCreditSnapshot {
	return shardCreditSnapshot{
		Generation:    c.generation.Load(),
		TargetBuffers: c.targetBuffers.Load(),
		TargetBytes:   c.targetBytes.Load(),
	}
}

// evaluateRetain evaluates whether a returned buffer may be retained under the
// current credit snapshot.
//
// usage must describe retained shard state before accepting the candidate.
// candidateCapacity is the returned buffer capacity that admission wants to
// retain.
func (c *shardCredit) evaluateRetain(usage shardCreditUsage, candidateCapacity uint64) shardCreditDecision {
	return c.snapshot().evaluateRetain(usage, candidateCapacity)
}

// allowsRetain reports whether a returned buffer may be retained under the
// current credit snapshot.
func (c *shardCredit) allowsRetain(usage shardCreditUsage, candidateCapacity uint64) bool {
	return c.evaluateRetain(usage, candidateCapacity).AllowsRetain()
}

// shardCreditLimit is the assigned local retention credit for one shard.
//
// Disabled credit is represented as:
//
//	TargetBuffers = 0
//	TargetBytes   = 0
//
// Enabled credit requires both fields to be greater than zero.
//
// shardCreditLimit does not compute itself from ClassSize or class target bytes.
// That computation belongs to class_budget.go. This type only represents the
// already-derived shard-local limit.
type shardCreditLimit struct {
	// TargetBuffers is the local retained-buffer credit.
	TargetBuffers uint64

	// TargetBytes is the local retained-byte credit.
	TargetBytes uint64
}

// newShardCreditLimit returns a validated shard-local credit limit.
//
// Passing 0, 0 creates a disabled limit. Passing only one zero dimension is
// rejected as a partial limit.
func newShardCreditLimit(targetBuffers, targetBytes uint64) shardCreditLimit {
	limit := shardCreditLimit{
		TargetBuffers: targetBuffers,
		TargetBytes:   targetBytes,
	}

	limit.validate()

	return limit
}

// validate verifies that the limit is either fully disabled or fully enabled.
func (l shardCreditLimit) validate() {
	if l.TargetBuffers == 0 && l.TargetBytes == 0 {
		return
	}

	if l.TargetBuffers == 0 || l.TargetBytes == 0 {
		panic(errShardCreditPartialLimit)
	}

	if l.TargetBytes < l.TargetBuffers {
		panic(errShardCreditInconsistentLimit)
	}
}

// IsEnabled reports whether this limit allows any retained storage.
func (l shardCreditLimit) IsEnabled() bool {
	return l.TargetBuffers > 0 && l.TargetBytes > 0
}

// IsZero reports whether this limit is disabled.
func (l shardCreditLimit) IsZero() bool {
	return l.TargetBuffers == 0 && l.TargetBytes == 0
}

// IsPartial reports whether this limit has only one enabled dimension.
//
// Partial limits are invalid for committed updates, but snapshots may observe a
// partial pair during concurrent independent atomic loads.
func (l shardCreditLimit) IsPartial() bool {
	return (l.TargetBuffers == 0) != (l.TargetBytes == 0)
}

// IsConsistent reports whether the limit is disabled or internally consistent.
//
// An enabled limit is consistent when it has both dimensions enabled and its byte
// credit can fund at least one byte per retained-buffer credit.
func (l shardCreditLimit) IsConsistent() bool {
	if l.IsZero() {
		return true
	}

	if l.IsPartial() {
		return false
	}

	return l.TargetBytes >= l.TargetBuffers
}

// shardCreditUsage describes current retained storage in one shard.
//
// The values should describe retained state before a candidate buffer is
// accepted. Usually they are derived from shard-local retained gauges,
// synchronized shard state, or bucket aggregation.
type shardCreditUsage struct {
	// RetainedBuffers is the number of buffers currently retained by the shard.
	RetainedBuffers uint64

	// RetainedBytes is the retained backing capacity currently held by the shard.
	RetainedBytes uint64
}

// IsZero reports whether the usage contains no retained storage.
func (u shardCreditUsage) IsZero() bool {
	return u.RetainedBuffers == 0 && u.RetainedBytes == 0
}

// shardCreditSnapshot is an immutable view of a shard-local credit assignment.
//
// Admission code can evaluate multiple candidates against one snapshot without
// repeatedly loading atomics. The snapshot represents a local shard gate, not a
// globally consistent budget publication.
//
// Because target fields are loaded independently, a snapshot can temporarily
// contain a mixed pair during concurrent update. Evaluation treats such a pair as
// inconsistent credit and rejects retention rather than accepting memory under an
// ambiguous limit.
type shardCreditSnapshot struct {
	// Generation identifies the credit publication observed by this snapshot.
	Generation Generation

	// TargetBuffers is the local retained-buffer credit.
	TargetBuffers uint64

	// TargetBytes is the local retained-byte credit.
	TargetBytes uint64
}

// Limit returns the credit limit represented by this snapshot.
//
// The returned limit is not validated because a concurrent update can make a
// snapshot observe a mixed pair. Callers that need to reason about validity
// should use IsConsistent, IsEnabled, or IsZero on the snapshot.
func (s shardCreditSnapshot) Limit() shardCreditLimit {
	return shardCreditLimit{
		TargetBuffers: s.TargetBuffers,
		TargetBytes:   s.TargetBytes,
	}
}

// IsEnabled reports whether this snapshot allows any retained storage.
func (s shardCreditSnapshot) IsEnabled() bool {
	return s.TargetBuffers > 0 && s.TargetBytes > 0
}

// IsZero reports whether this snapshot represents disabled credit.
func (s shardCreditSnapshot) IsZero() bool {
	return s.TargetBuffers == 0 && s.TargetBytes == 0
}

// IsPartial reports whether this snapshot contains only one enabled dimension.
//
// This should not be produced by a committed shardCreditLimit, but it can be
// observed transiently because target fields are loaded independently.
func (s shardCreditSnapshot) IsPartial() bool {
	return s.Limit().IsPartial()
}

// IsConsistent reports whether this snapshot is disabled or internally
// consistent.
//
// Inconsistent snapshots reject retention. This is conservative and appropriate
// for a hot-path credit gate.
func (s shardCreditSnapshot) IsConsistent() bool {
	return s.Limit().IsConsistent()
}

// evaluateRetain evaluates whether candidateCapacity fits this credit snapshot
// given current retained shard usage.
//
// The method performs no mutation. It is suitable for hot-path admission where
// the caller already has current retained usage.
func (s shardCreditSnapshot) evaluateRetain(usage shardCreditUsage, candidateCapacity uint64) shardCreditDecision {
	if candidateCapacity == 0 {
		return shardCreditRejectInvalidCapacity
	}

	if !s.IsConsistent() {
		return shardCreditRejectInconsistentCredit
	}

	if !s.IsEnabled() {
		return shardCreditRejectNoCredit
	}

	if usage.RetainedBuffers >= s.TargetBuffers {
		return shardCreditRejectBufferCreditExhausted
	}

	if usage.RetainedBytes >= s.TargetBytes {
		return shardCreditRejectByteCreditExhausted
	}

	if candidateCapacity > maxShardCreditValue-usage.RetainedBytes {
		return shardCreditRejectByteAccountingOverflow
	}

	if usage.RetainedBytes+candidateCapacity > s.TargetBytes {
		return shardCreditRejectByteCreditExhausted
	}

	return shardCreditAccept
}

// allowsRetain reports whether candidateCapacity fits this credit snapshot.
func (s shardCreditSnapshot) allowsRetain(usage shardCreditUsage, candidateCapacity uint64) bool {
	return s.evaluateRetain(usage, candidateCapacity).AllowsRetain()
}

// remaining returns the remaining local shard credit before accepting another
// candidate buffer.
//
// Remaining values are saturated at zero when current usage already exceeds the
// target. This can happen transiently after credit contraction before cold trim
// has physically removed over-target retained buffers.
//
// Inconsistent or disabled snapshots report no remaining credit.
func (s shardCreditSnapshot) remaining(usage shardCreditUsage) shardCreditRemaining {
	if !s.IsConsistent() || !s.IsEnabled() {
		return shardCreditRemaining{}
	}

	var buffers uint64
	if usage.RetainedBuffers < s.TargetBuffers {
		buffers = s.TargetBuffers - usage.RetainedBuffers
	}

	var bytes uint64
	if usage.RetainedBytes < s.TargetBytes {
		bytes = s.TargetBytes - usage.RetainedBytes
	}

	return shardCreditRemaining{
		Buffers: buffers,
		Bytes:   bytes,
	}
}

// overage returns retained storage above this credit snapshot.
//
// Overage is saturated at zero when usage is within target. It is useful for
// diagnostics and trim planning, but it does not perform physical trim.
//
// Disabled credit treats all retained usage as over-target. This is intentional:
// disabling credit stops new retention, but already retained memory still needs
// cold-path trim if the owner wants to release it.
func (s shardCreditSnapshot) overage(usage shardCreditUsage) shardCreditAmount {
	if !s.IsConsistent() || s.IsZero() {
		return shardCreditAmount{
			Buffers: usage.RetainedBuffers,
			Bytes:   usage.RetainedBytes,
		}
	}

	var buffers uint64
	if usage.RetainedBuffers > s.TargetBuffers {
		buffers = usage.RetainedBuffers - s.TargetBuffers
	}

	var bytes uint64
	if usage.RetainedBytes > s.TargetBytes {
		bytes = usage.RetainedBytes - s.TargetBytes
	}

	return shardCreditAmount{
		Buffers: buffers,
		Bytes:   bytes,
	}
}

// isOverTarget reports whether usage exceeds this credit snapshot in either
// dimension.
//
// Disabled or inconsistent credit treats any retained usage as over-target.
func (s shardCreditSnapshot) isOverTarget(usage shardCreditUsage) bool {
	return !s.overage(usage).IsZero()
}

// shardCreditRemaining describes remaining local shard credit.
type shardCreditRemaining struct {
	// Buffers is remaining retained-buffer credit.
	Buffers uint64

	// Bytes is remaining retained-byte credit.
	Bytes uint64
}

// IsZero reports whether no buffer and no byte credit remains.
func (r shardCreditRemaining) IsZero() bool {
	return r.Buffers == 0 && r.Bytes == 0
}

// shardCreditAmount describes a buffer-count and byte amount.
//
// It is used for over-target diagnostics and trim planning helpers.
type shardCreditAmount struct {
	// Buffers is a buffer-count amount.
	Buffers uint64

	// Bytes is a retained-byte amount.
	Bytes uint64
}

// IsZero reports whether no buffer and no byte amount is present.
func (a shardCreditAmount) IsZero() bool {
	return a.Buffers == 0 && a.Bytes == 0
}

// shardCreditDecision describes a shard-credit admission result or the absence
// of shard-credit evaluation when class admission rejected earlier.
//
// This is intentionally internal. Public drop reasons, if exposed later, should
// be modeled separately so internal credit mechanics can evolve.
type shardCreditDecision uint8

const (
	// shardCreditAccept means the candidate fits the current shard credit.
	shardCreditAccept shardCreditDecision = iota

	// shardCreditRejectInvalidCapacity means the candidate has zero retained
	// capacity and cannot be useful retained storage.
	shardCreditRejectInvalidCapacity

	// shardCreditRejectNoCredit means the shard currently has no enabled local
	// credit.
	shardCreditRejectNoCredit

	// shardCreditRejectInconsistentCredit means the observed credit snapshot is
	// internally inconsistent. This can happen transiently during concurrent
	// independent atomic loads, or permanently if a caller bypasses validation.
	shardCreditRejectInconsistentCredit

	// shardCreditRejectBufferCreditExhausted means retaining another buffer would
	// exceed the shard's buffer-count credit.
	shardCreditRejectBufferCreditExhausted

	// shardCreditRejectByteCreditExhausted means retaining the candidate would
	// exceed the shard's retained-byte credit, or the shard is already at or over
	// its byte target.
	shardCreditRejectByteCreditExhausted

	// shardCreditRejectByteAccountingOverflow means adding the candidate capacity
	// to current retained bytes would overflow uint64.
	shardCreditRejectByteAccountingOverflow

	// shardCreditNotEvaluated means class-level admission rejected retention
	// before shard credit was evaluated.
	shardCreditNotEvaluated
)

// AllowsRetain reports whether this decision accepts the candidate.
func (d shardCreditDecision) AllowsRetain() bool {
	return d == shardCreditAccept
}

// String returns the stable diagnostic name of the decision.
func (d shardCreditDecision) String() string {
	switch d {
	case shardCreditAccept:
		return "accept"
	case shardCreditRejectInvalidCapacity:
		return "reject_invalid_capacity"
	case shardCreditRejectNoCredit:
		return "reject_no_credit"
	case shardCreditRejectInconsistentCredit:
		return "reject_inconsistent_credit"
	case shardCreditRejectBufferCreditExhausted:
		return "reject_buffer_credit_exhausted"
	case shardCreditRejectByteCreditExhausted:
		return "reject_byte_credit_exhausted"
	case shardCreditRejectByteAccountingOverflow:
		return "reject_byte_accounting_overflow"
	case shardCreditNotEvaluated:
		return "not_evaluated"
	default:
		return "unknown"
	}
}
