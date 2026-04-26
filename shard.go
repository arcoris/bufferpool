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

import "sync"

// shard is a shard-local runtime unit for one size-class owner.
//
// A shard is the hot-path synchronization boundary for retained storage. It
// owns one local raw bucket, shard-local counters, and one shard-local credit
// gate. The class layer decides which shard belongs to which SizeClass. This
// type deliberately does not store ClassID, ClassSize, SizeClass, or class-table
// references.
//
// Responsibility boundary:
//
//   - class_table.go normalizes requested sizes to size classes;
//   - class.go describes SizeClass metadata;
//   - class runtime code owns the set of shards for a class;
//   - shard.go owns one shard-local bucket, counters, and credit gate;
//   - bucket.go stores already admitted retained buffers;
//   - bucket_trim.go describes trim and clear results;
//   - shard_counters.go records shard-local accounting facts;
//   - shard_credit.go evaluates local retention credit;
//   - class_admission.go protects class-level capacity compatibility;
//   - admission code maps local decisions to public drop reasons;
//   - trim planning decides which shards should be trimmed.
//
// Hot-path behavior:
//
//   - tryGet records a routed get, attempts LIFO reuse from the bucket, and
//     records hit or miss;
//   - tryRetain records a routed put, evaluates shard credit, attempts bucket
//     retention, and records retain or drop;
//   - recordAllocation records allocation capacity after an owner allocates a new
//     buffer on miss.
//
// The shard does not allocate buffers. Allocation size is a class-level decision
// because the class owner knows the normalized class capacity and the original
// requested size.
//
// Concurrency:
//
// shard is safe for concurrent operations through its mutex:
//
//   - shard.mu serializes raw bucket mutation;
//   - shard.mu serializes credit evaluation with current retained accounting;
//   - shardCounters uses atomics;
//   - shardCredit uses atomics.
//
// The shard mutex keeps credit evaluation, bucket mutation, and retained gauges
// in one strict local sequence for retain/get/trim/clear operations. This
// prevents concurrent retain calls from admitting more buffers than the observed
// shard credit allows.
//
// The full shard state snapshot is not globally atomic across bucket, counters,
// and credit. It is suitable for diagnostics, tests, metrics sampling, and
// control-plane observation.
//
// Copying:
//
// shard MUST NOT be copied after first use. It contains a mutex, a raw bucket,
// and multiple atomic values.
type shard struct {
	// mu serializes physical retained-storage mutations with retained-counter
	// updates and shard-credit usage checks.
	mu sync.Mutex

	// bucket stores retained buffers for this shard.
	bucket bucket

	// counters records shard-local workload and retained-storage facts.
	counters shardCounters

	// credit is the current local retention credit gate for this shard.
	credit shardCredit
}

// newShard returns an enabled shard with a bucket containing bucketSlotLimit
// retained-buffer slots.
//
// The returned shard has disabled credit by default. Retention will be rejected
// until the owner applies a non-zero shardCreditLimit through updateCredit.
// This is intentional: physical storage capacity and retention credit are
// separate concepts.
func newShard(bucketSlotLimit int) shard {
	return shard{
		bucket: newBucket(bucketSlotLimit),
	}
}

// tryGet attempts to reuse a retained buffer from this shard.
//
// requestedSize is the caller-requested size routed to this shard. It is recorded
// for workload accounting only. The shard does not reshape the returned buffer to
// requestedSize because request-to-class normalization belongs to the class
// layer.
//
// On hit:
//
//   - a buffer is removed from the bucket;
//   - hit counters are updated;
//   - current retained gauges are decremented by the reused buffer capacity.
//
// On miss:
//
//   - miss counters are updated;
//   - no allocation is performed by the shard.
func (s *shard) tryGet(requestedSize Size) shardGetResult {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.counters.recordGet(requestedSize.Bytes())

	buffer, ok := s.bucket.pop()
	if !ok {
		s.counters.recordMiss()

		return shardGetResult{}
	}

	capacity := shardBufferCapacity(buffer)
	s.counters.recordHit(capacity)

	return shardGetResult{
		Buffer:   buffer,
		Hit:      true,
		Capacity: capacity,
	}
}

// recordAllocation records an allocation associated with this shard.
//
// The shard does not allocate buffers itself. The class or pool owner records
// allocation here after it allocates a new backing buffer because a shard reuse
// attempt missed or because a higher-level path intentionally bypassed reuse.
func (s *shard) recordAllocation(capacity uint64) {
	s.counters.recordAllocation(capacity)
}

// recordAllocatedBuffer records the capacity of an allocated buffer.
//
// This helper is convenient for call sites that already have the allocated
// buffer and do not need to compute cap(buffer) themselves.
func (s *shard) recordAllocatedBuffer(buffer []byte) {
	s.recordAllocation(shardBufferCapacity(buffer))
}

// tryRetain attempts to retain a returned buffer in this shard.
//
// The method represents the shard-local part of return-path retention. It does
// not perform all admission checks. Owner layers must still handle lifecycle,
// ownership, max retained capacity, pressure, origin-class growth, and public
// drop-reason mapping.
//
// The method performs only shard-local work:
//
//   - record the routed put;
//   - evaluate current shard credit against current retained usage;
//   - push into the bucket when credit allows it;
//   - record retain or drop counters.
//
// A positive credit decision does not guarantee physical retention. The bucket
// may still reject the buffer if its local slot storage is full. This distinction
// is preserved in shardRetainResult: CreditDecision may be accept while Retained
// is false.
func (s *shard) tryRetain(buffer []byte) shardRetainResult {
	s.mu.Lock()
	defer s.mu.Unlock()

	capacity := shardBufferCapacity(buffer)

	s.counters.recordPut(capacity)

	decision := s.credit.evaluateRetain(s.creditUsage(), capacity)
	if !decision.AllowsRetain() {
		s.counters.recordDrop(capacity)

		return shardRetainResult{
			Capacity:       capacity,
			CreditDecision: decision,
		}
	}

	if !s.bucket.push(buffer) {
		s.counters.recordDrop(capacity)

		return shardRetainResult{
			Capacity:       capacity,
			CreditDecision: decision,
		}
	}

	s.counters.recordRetain(capacity)

	return shardRetainResult{
		Capacity:       capacity,
		Retained:       true,
		CreditDecision: decision,
	}
}

// trim removes up to maxBuffers retained buffers from this shard.
//
// The physical removal is delegated to bucket.trim. The shard records trim
// counters after the bucket operation completes. Trim planning, victim selection,
// and pressure policy belong to higher layers.
func (s *shard) trim(maxBuffers int) bucketTrimResult {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := s.bucket.trim(maxBuffers)
	s.counters.recordTrim(result)

	return result
}

// clear removes all retained buffers from this shard.
//
// clear is intended for shutdown, hard cleanup, policy invalidation, or tests.
// It physically clears bucket storage and records clear counters. It does not
// disable credit; credit publication remains a separate control-plane operation.
func (s *shard) clear() bucketTrimResult {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := s.bucket.clear()
	s.counters.recordClear(result)

	return result
}

// updateCredit applies a new shard-local retention credit limit.
func (s *shard) updateCredit(limit shardCreditLimit) Generation {
	return s.credit.update(limit)
}

// disableCredit disables shard-local retention credit.
//
// Disabling credit prevents new retention through tryRetain but does not clear
// already retained buffers. Physical cleanup belongs to trim or clear.
func (s *shard) disableCredit() Generation {
	return s.credit.disable()
}

// state returns an observational snapshot of this shard.
//
// The returned state is not globally atomic across bucket, counters, and credit.
// It is intended for diagnostics, tests, metrics aggregation, and control-plane
// sampling.
func (s *shard) state() shardState {
	s.mu.Lock()
	defer s.mu.Unlock()

	return shardState{
		Bucket:   s.bucket.state(),
		Counters: s.counters.snapshot(),
		Credit:   s.credit.snapshot(),
	}
}

// creditUsage returns retained usage for shard credit evaluation.
//
// The values come from shard retained gauges rather than from bucket.state().
// shard.mu keeps those gauges in the same local sequence as credit checks and
// bucket mutation.
func (s *shard) creditUsage() shardCreditUsage {
	return shardCreditUsage{
		RetainedBuffers: s.counters.currentRetainedBuffers.Load(),
		RetainedBytes:   s.counters.currentRetainedBytes.Load(),
	}
}

// bucketState returns the current local bucket state.
func (s *shard) bucketState() bucketState {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.bucket.state()
}

// countersSnapshot returns the current shard counter snapshot.
func (s *shard) countersSnapshot() shardCountersSnapshot {
	return s.counters.snapshot()
}

// creditSnapshot returns the current shard credit snapshot.
func (s *shard) creditSnapshot() shardCreditSnapshot {
	return s.credit.snapshot()
}

// shardGetResult describes one shard-local get/reuse attempt.
type shardGetResult struct {
	// Buffer is the reused buffer returned by the shard.
	//
	// It is nil when Hit is false. On hit, the buffer has len == 0 and preserves
	// its backing capacity.
	Buffer []byte

	// Hit reports whether the get was satisfied from retained bucket storage.
	Hit bool

	// Capacity is cap(Buffer) on hit, otherwise zero.
	Capacity uint64
}

// Miss reports whether the get attempt did not find retained storage.
func (r shardGetResult) Miss() bool {
	return !r.Hit
}

// shardRetainResult describes one shard-local retain attempt.
type shardRetainResult struct {
	// Capacity is cap(buffer) for the returned buffer evaluated by the shard.
	Capacity uint64

	// Retained reports whether the buffer was physically stored in the bucket.
	Retained bool

	// CreditDecision is the result of the shard credit evaluation.
	//
	// When CreditDecision rejects, Retained is always false. When CreditDecision
	// accepts, Retained may still be false if the bucket rejects physical storage,
	// for example because its local slot storage is full.
	CreditDecision shardCreditDecision
}

// Dropped reports whether the retain attempt did not physically store the
// returned buffer.
func (r shardRetainResult) Dropped() bool {
	return !r.Retained
}

// RejectedByCredit reports whether the shard credit gate rejected retention.
func (r shardRetainResult) RejectedByCredit() bool {
	return !r.CreditDecision.AllowsRetain()
}

// RejectedByBucket reports whether credit allowed retention but physical bucket
// storage rejected the buffer.
func (r shardRetainResult) RejectedByBucket() bool {
	return r.CreditDecision.AllowsRetain() && !r.Retained
}

// shardState is an observational view of one shard.
//
// It is intentionally internal. Public snapshots should be modeled separately so
// runtime internals can evolve without freezing public API structure.
type shardState struct {
	// Bucket is the current bucket storage state.
	Bucket bucketState

	// Counters is the current shard counter snapshot.
	Counters shardCountersSnapshot

	// Credit is the current shard credit snapshot.
	Credit shardCreditSnapshot
}

// RetainedUsage returns retained usage derived from the shard counter snapshot.
func (s shardState) RetainedUsage() shardCreditUsage {
	return shardCreditUsage{
		RetainedBuffers: s.Counters.CurrentRetainedBuffers,
		RetainedBytes:   s.Counters.CurrentRetainedBytes,
	}
}

// IsOverCredit reports whether retained usage exceeds the observed credit
// snapshot.
func (s shardState) IsOverCredit() bool {
	return s.Credit.isOverTarget(s.RetainedUsage())
}

// shardBufferCapacity returns the retained-memory capacity represented by
// buffer.
//
// Bucket storage still performs its own validation and accounting.
func shardBufferCapacity(buffer []byte) uint64 {
	return uint64(cap(buffer))
}
