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

// shardCounters stores atomic workload and retention counters for one shard.
//
// A shard is a data-plane contention boundary inside one size class. Counters in
// this type describe what happened inside that shard after higher-level routing
// has selected it. They are intentionally internal and should not be treated as
// the public metrics contract.
//
// Responsibility boundary:
//
//   - shard code owns call ordering;
//   - bucket code owns synchronized retained-buffer storage;
//   - class_admission.go owns minimal class-local retain eligibility checks;
//   - admission code owns policy/drop-reason mapping;
//   - trim planning owns victim and target selection;
//   - workload-window code computes recent deltas;
//   - workload scoring code consumes deltas;
//   - metrics code owns public aggregation and export;
//   - shardCounters only records facts that already happened.
//
// The project context uses workload signals such as gets, puts, hits, misses,
// allocations, drops, requested bytes, allocated bytes, returned capacity, and
// retained bytes. This file follows that terminology. The names are accounting
// terms, not a commitment to final public API method names.
//
// Counter categories:
//
//   - monotonic lifetime counters record cumulative events and byte totals;
//   - current retained gauges record retained buffers and retained bytes now.
//
// Retained byte accounting uses buffer capacity, not slice length, because the
// backing array capacity is what keeps memory reachable.
//
// In-use bytes and reserved bytes are intentionally not represented here. Those
// require ownership-aware accounting that can track checked-out buffers. This
// file only tracks retained storage and shard-local workload outcomes.
//
// Correctness:
//
// recordHit, recordTrim, and recordClear subtract from retained gauges. If shard
// code records a removal that was not previously recorded as retained, the
// underlying atomic gauges panic on underflow. That is deliberate: retained
// accounting corruption must fail fast in tests and development.
//
// Concurrency:
//
// shardCounters is safe for concurrent use through the atomic primitives it
// contains. It MUST NOT be copied after first use because it contains atomic
// values.
type shardCounters struct {
	// gets counts buffer requests routed to this shard.
	//
	// A get is counted after higher-level code has selected this shard. Requests
	// rejected before shard routing are not shard workload.
	gets atomicx.Uint64Counter

	// requestedBytes sums caller-requested byte sizes for gets routed to this
	// shard.
	//
	// This is requested size, not class size and not retained/allocated capacity.
	requestedBytes atomicx.Uint64Counter

	// hits counts gets satisfied from retained bucket storage.
	hits atomicx.Uint64Counter

	// hitBytes sums capacities removed from retained storage to satisfy hits.
	hitBytes atomicx.Uint64Counter

	// misses counts gets that did not find a retained buffer in this shard.
	misses atomicx.Uint64Counter

	// allocations counts new backing-buffer allocations associated with this
	// shard.
	//
	// Allocation is separate from miss because owner paths may allocate for
	// reasons other than a direct retained-storage miss.
	allocations atomicx.Uint64Counter

	// allocatedBytes sums capacities of newly allocated backing buffers.
	allocatedBytes atomicx.Uint64Counter

	// puts counts returned buffers routed to this shard for possible retention or
	// discard.
	puts atomicx.Uint64Counter

	// returnedBytes sums capacities of buffers returned to this shard.
	returnedBytes atomicx.Uint64Counter

	// retains counts returned buffers successfully retained by bucket storage.
	retains atomicx.Uint64Counter

	// retainedBytes sums capacities of returned buffers successfully retained.
	retainedBytes atomicx.Uint64Counter

	// drops counts returned buffers not retained by this shard.
	drops atomicx.Uint64Counter

	// droppedBytes sums capacities of returned buffers not retained.
	droppedBytes atomicx.Uint64Counter

	// trimOperations counts local trim operations recorded for shard buckets.
	//
	// The operation is counted even when it removes no buffers. Removed capacity
	// is tracked separately by trimmedBuffers and trimmedBytes.
	trimOperations atomicx.Uint64Counter

	// trimmedBuffers counts buffers removed by trim operations.
	trimmedBuffers atomicx.Uint64Counter

	// trimmedBytes sums capacities removed by trim operations.
	trimmedBytes atomicx.Uint64Counter

	// clearOperations counts hard clear operations recorded for shard buckets.
	//
	// Clear is tracked separately from trim because close/reset/policy-change
	// cleanup is not necessarily the same as adaptive pressure trimming.
	clearOperations atomicx.Uint64Counter

	// clearedBuffers counts buffers removed by clear operations.
	clearedBuffers atomicx.Uint64Counter

	// clearedBytes sums capacities removed by clear operations.
	clearedBytes atomicx.Uint64Counter

	// currentRetainedBuffers is the current number of buffers retained by this
	// shard.
	currentRetainedBuffers atomicx.Uint64Gauge

	// currentRetainedBytes is the current retained backing capacity in bytes.
	currentRetainedBytes atomicx.Uint64Gauge
}

// recordGet records a buffer request routed to this shard.
//
// requestedSize is the caller-requested size. It is not necessarily the size
// class, the reused buffer capacity, or the allocated capacity.
func (c *shardCounters) recordGet(requestedSize uint64) {
	c.gets.Inc()
	c.requestedBytes.Add(requestedSize)
}

// recordHit records that a get was satisfied from retained bucket storage.
//
// capacity is the capacity of the retained buffer removed for reuse. Removing a
// retained buffer decreases current retained gauges.
func (c *shardCounters) recordHit(capacity uint64) {
	c.hits.Inc()
	c.hitBytes.Add(capacity)

	c.currentRetainedBuffers.Dec()
	c.currentRetainedBytes.Sub(capacity)
}

// recordMiss records that a get did not find a retained buffer in this shard.
func (c *shardCounters) recordMiss() {
	c.misses.Inc()
}

// recordAllocation records a newly allocated backing-buffer capacity.
//
// Allocation is tracked separately from miss because not every miss necessarily
// allocates in the same place, and owner paths may allocate for reasons other
// than a bucket miss.
func (c *shardCounters) recordAllocation(capacity uint64) {
	c.allocations.Inc()
	c.allocatedBytes.Add(capacity)
}

// recordPut records a returned buffer routed to this shard.
//
// capacity is the returned buffer capacity. The buffer may later be retained or
// dropped. Outcome-specific accounting must be recorded through recordRetain or
// recordDrop.
func (c *shardCounters) recordPut(capacity uint64) {
	c.puts.Inc()
	c.returnedBytes.Add(capacity)
}

// recordRetain records that a returned buffer was successfully retained.
//
// capacity is the retained backing capacity. Retaining increases current
// retained gauges and lifetime retain counters.
func (c *shardCounters) recordRetain(capacity uint64) {
	c.retains.Inc()
	c.retainedBytes.Add(capacity)

	c.currentRetainedBuffers.Inc()
	c.currentRetainedBytes.Add(capacity)
}

// recordDrop records that a returned buffer was not retained.
//
// Drop reasons are intentionally not represented here. Admission, pressure,
// ownership, full-bucket, class, and policy-specific reasons belong to
// higher-level accounting or reason-specific counters.
func (c *shardCounters) recordDrop(capacity uint64) {
	c.drops.Inc()
	c.droppedBytes.Add(capacity)
}

// recordTrim records a completed bucket-level trim operation.
//
// The operation count is incremented even when result removed no buffers. Current
// retained gauges are decremented only by the actually removed amount.
func (c *shardCounters) recordTrim(result bucketTrimResult) {
	c.trimOperations.Inc()

	if result.RemovedBuffers == 0 && result.RemovedBytes == 0 {
		return
	}

	removedBuffers := uint64(result.RemovedBuffers)

	c.trimmedBuffers.Add(removedBuffers)
	c.trimmedBytes.Add(result.RemovedBytes)

	c.currentRetainedBuffers.Sub(removedBuffers)
	c.currentRetainedBytes.Sub(result.RemovedBytes)
}

// recordClear records a completed bucket-level clear operation.
//
// Clear operations are tracked separately from trim operations so close/reset
// cleanup does not get mixed with adaptive pressure trimming.
func (c *shardCounters) recordClear(result bucketTrimResult) {
	c.clearOperations.Inc()

	if result.RemovedBuffers == 0 && result.RemovedBytes == 0 {
		return
	}

	removedBuffers := uint64(result.RemovedBuffers)

	c.clearedBuffers.Add(removedBuffers)
	c.clearedBytes.Add(result.RemovedBytes)

	c.currentRetainedBuffers.Sub(removedBuffers)
	c.currentRetainedBytes.Sub(result.RemovedBytes)
}

// snapshot returns an immutable point-in-time view of shard counters.
//
// The snapshot is not globally atomic across all fields. Individual fields are
// loaded atomically, but concurrent updates may make different fields represent
// slightly different instants. This is acceptable for metrics, workload windows,
// pressure heuristics, and controller sampling. Strongly consistent
// accounting, if needed, must be performed by the owner under its own
// synchronization.
func (c *shardCounters) snapshot() shardCountersSnapshot {
	return shardCountersSnapshot{
		Gets:           c.gets.Load(),
		RequestedBytes: c.requestedBytes.Load(),

		Hits:     c.hits.Load(),
		HitBytes: c.hitBytes.Load(),
		Misses:   c.misses.Load(),

		Allocations:    c.allocations.Load(),
		AllocatedBytes: c.allocatedBytes.Load(),

		Puts:          c.puts.Load(),
		ReturnedBytes: c.returnedBytes.Load(),

		Retains:       c.retains.Load(),
		RetainedBytes: c.retainedBytes.Load(),

		Drops:        c.drops.Load(),
		DroppedBytes: c.droppedBytes.Load(),

		TrimOperations: c.trimOperations.Load(),
		TrimmedBuffers: c.trimmedBuffers.Load(),
		TrimmedBytes:   c.trimmedBytes.Load(),

		ClearOperations: c.clearOperations.Load(),
		ClearedBuffers:  c.clearedBuffers.Load(),
		ClearedBytes:    c.clearedBytes.Load(),

		CurrentRetainedBuffers: c.currentRetainedBuffers.Load(),
		CurrentRetainedBytes:   c.currentRetainedBytes.Load(),
	}
}

// shardCountersSnapshot is an immutable point-in-time view of one shard's
// counters.
//
// The type is internal. Public metrics should be modeled separately so internal
// accounting can evolve without freezing the public API.
//
// Lifetime fields are monotonic counters. CurrentRetainedBuffers and
// CurrentRetainedBytes are gauges.
type shardCountersSnapshot struct {
	// Gets is the total number of buffer requests routed to this shard.
	Gets uint64

	// RequestedBytes is the cumulative caller-requested size observed by gets.
	RequestedBytes uint64

	// Hits is the number of gets served from retained storage.
	Hits uint64

	// HitBytes is the cumulative-retained capacity removed for hits.
	HitBytes uint64

	// Misses is the number of gets that did not find retained storage.
	Misses uint64

	// Allocations is the total number of new allocations recorded by this shard.
	Allocations uint64

	// AllocatedBytes is the cumulative capacity of newly allocated buffers.
	AllocatedBytes uint64

	// Puts is the number of returned buffers routed to this shard.
	Puts uint64

	// ReturnedBytes is the cumulative capacity of returned buffers.
	ReturnedBytes uint64

	// Retains is the number of returned buffers retained by this shard.
	Retains uint64

	// RetainedBytes is the cumulative capacity retained by this shard.
	RetainedBytes uint64

	// Drops is the number of returned buffers not retained by this shard.
	Drops uint64

	// DroppedBytes is the cumulative capacity of returned buffers not retained.
	DroppedBytes uint64

	// TrimOperations is the number of bucket trim operations recorded by this
	// shard.
	TrimOperations uint64

	// TrimmedBuffers is the number of buffers removed by trim operations.
	TrimmedBuffers uint64

	// TrimmedBytes is the cumulative capacity removed by trim operations.
	TrimmedBytes uint64

	// ClearOperations is the number of bucket clear operations recorded by this
	// shard.
	ClearOperations uint64

	// ClearedBuffers is the number of buffers removed by clear operations.
	ClearedBuffers uint64

	// ClearedBytes is the cumulative capacity removed by clear operations.
	ClearedBytes uint64

	// CurrentRetainedBuffers is the current number of buffers retained by this
	// shard.
	CurrentRetainedBuffers uint64

	// CurrentRetainedBytes is the current retained backing capacity in bytes.
	CurrentRetainedBytes uint64
}

// IsZero reports whether the snapshot contains no observed activity and no
// current retained storage.
func (s shardCountersSnapshot) IsZero() bool {
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
		s.TrimOperations == 0 &&
		s.TrimmedBuffers == 0 &&
		s.TrimmedBytes == 0 &&
		s.ClearOperations == 0 &&
		s.ClearedBuffers == 0 &&
		s.ClearedBytes == 0 &&
		s.CurrentRetainedBuffers == 0 &&
		s.CurrentRetainedBytes == 0
}

// ReuseAttempts returns the number of reuse decisions observed by this snapshot.
//
// In ordinary paths this should equal Hits + Misses. It may be lower than Gets
// if some gets are rejected or handled before bucket reuse is attempted.
func (s shardCountersSnapshot) ReuseAttempts() uint64 {
	return s.Hits + s.Misses
}

// PutOutcomes returns the number of put attempts that reached a retain or drop
// outcome.
//
// In ordinary paths this should equal Retains + Drops. It may be lower than Puts
// if some puts fail validation before admission.
func (s shardCountersSnapshot) PutOutcomes() uint64 {
	return s.Retains + s.Drops
}

// RemovalOperations returns the number of local storage-reduction operations.
//
// This counts operation attempts, not removed buffers.
func (s shardCountersSnapshot) RemovalOperations() uint64 {
	return s.TrimOperations + s.ClearOperations
}

// RemovedBuffers returns buffers removed by trim and clear operations.
func (s shardCountersSnapshot) RemovedBuffers() uint64 {
	return s.TrimmedBuffers + s.ClearedBuffers
}

// RemovedBytes returns retained capacity removed by trim and clear operations.
func (s shardCountersSnapshot) RemovedBytes() uint64 {
	return s.TrimmedBytes + s.ClearedBytes
}

// deltaSince returns a wrap-aware monotonic delta from previous to s.
//
// Lifetime counters are converted to deltas. Current retained gauges are copied
// from the newer snapshot because gauges are not lifetime counters and should not
// be interpreted as deltas.
func (s shardCountersSnapshot) deltaSince(previous shardCountersSnapshot) shardCountersDelta {
	return shardCountersDelta{
		Gets:           shardCounterDelta(previous.Gets, s.Gets),
		RequestedBytes: shardCounterDelta(previous.RequestedBytes, s.RequestedBytes),

		Hits:     shardCounterDelta(previous.Hits, s.Hits),
		HitBytes: shardCounterDelta(previous.HitBytes, s.HitBytes),
		Misses:   shardCounterDelta(previous.Misses, s.Misses),

		Allocations:    shardCounterDelta(previous.Allocations, s.Allocations),
		AllocatedBytes: shardCounterDelta(previous.AllocatedBytes, s.AllocatedBytes),

		Puts:          shardCounterDelta(previous.Puts, s.Puts),
		ReturnedBytes: shardCounterDelta(previous.ReturnedBytes, s.ReturnedBytes),

		Retains:       shardCounterDelta(previous.Retains, s.Retains),
		RetainedBytes: shardCounterDelta(previous.RetainedBytes, s.RetainedBytes),

		Drops:        shardCounterDelta(previous.Drops, s.Drops),
		DroppedBytes: shardCounterDelta(previous.DroppedBytes, s.DroppedBytes),

		TrimOperations: shardCounterDelta(previous.TrimOperations, s.TrimOperations),
		TrimmedBuffers: shardCounterDelta(previous.TrimmedBuffers, s.TrimmedBuffers),
		TrimmedBytes:   shardCounterDelta(previous.TrimmedBytes, s.TrimmedBytes),

		ClearOperations: shardCounterDelta(previous.ClearOperations, s.ClearOperations),
		ClearedBuffers:  shardCounterDelta(previous.ClearedBuffers, s.ClearedBuffers),
		ClearedBytes:    shardCounterDelta(previous.ClearedBytes, s.ClearedBytes),

		CurrentRetainedBuffers: s.CurrentRetainedBuffers,
		CurrentRetainedBytes:   s.CurrentRetainedBytes,
	}
}

// shardCountersDelta describes activity observed between two shard counter
// snapshots.
//
// All event fields are deltas over monotonic lifetime counters. Current retained
// fields represent the current gauge values from the newer snapshot.
type shardCountersDelta struct {
	Gets           uint64
	RequestedBytes uint64

	Hits     uint64
	HitBytes uint64
	Misses   uint64

	Allocations    uint64
	AllocatedBytes uint64

	Puts          uint64
	ReturnedBytes uint64

	Retains       uint64
	RetainedBytes uint64

	Drops        uint64
	DroppedBytes uint64

	TrimOperations uint64
	TrimmedBuffers uint64
	TrimmedBytes   uint64

	ClearOperations uint64
	ClearedBuffers  uint64
	ClearedBytes    uint64

	CurrentRetainedBuffers uint64
	CurrentRetainedBytes   uint64
}

// IsZero reports whether the delta contains no observed activity and no current
// retained storage.
func (d shardCountersDelta) IsZero() bool {
	return d.Gets == 0 &&
		d.RequestedBytes == 0 &&
		d.Hits == 0 &&
		d.HitBytes == 0 &&
		d.Misses == 0 &&
		d.Allocations == 0 &&
		d.AllocatedBytes == 0 &&
		d.Puts == 0 &&
		d.ReturnedBytes == 0 &&
		d.Retains == 0 &&
		d.RetainedBytes == 0 &&
		d.Drops == 0 &&
		d.DroppedBytes == 0 &&
		d.TrimOperations == 0 &&
		d.TrimmedBuffers == 0 &&
		d.TrimmedBytes == 0 &&
		d.ClearOperations == 0 &&
		d.ClearedBuffers == 0 &&
		d.ClearedBytes == 0 &&
		d.CurrentRetainedBuffers == 0 &&
		d.CurrentRetainedBytes == 0
}

// ReuseAttempts returns the number of reuse decisions observed in this delta.
func (d shardCountersDelta) ReuseAttempts() uint64 {
	return d.Hits + d.Misses
}

// PutOutcomes returns the number of put outcomes observed in this delta.
func (d shardCountersDelta) PutOutcomes() uint64 {
	return d.Retains + d.Drops
}

// RemovalOperations returns the number of trim and clear operations observed in
// this delta.
func (d shardCountersDelta) RemovalOperations() uint64 {
	return d.TrimOperations + d.ClearOperations
}

// RemovedBuffers returns buffers removed by trim and clear operations in this
// delta.
func (d shardCountersDelta) RemovedBuffers() uint64 {
	return d.TrimmedBuffers + d.ClearedBuffers
}

// RemovedBytes returns retained capacity removed by trim and clear operations in
// this delta.
func (d shardCountersDelta) RemovedBytes() uint64 {
	return d.TrimmedBytes + d.ClearedBytes
}

// shardCounterDelta returns the wrap-aware delta between two monotonic uint64
// counters.
//
// uint64 subtraction has the desired modulo behavior for a single wrap. This is
// the same arithmetic used by atomicx.Uint64CounterDelta.
func shardCounterDelta(previous, current uint64) uint64 {
	return atomicx.NewUint64CounterDelta(previous, current).Value
}
