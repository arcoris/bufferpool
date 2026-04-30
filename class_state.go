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

const (
	// errClassStateInvalidClass is used when class runtime state is constructed
	// for an invalid size-class descriptor.
	//
	// classState owns runtime state for one enabled SizeClass. A zero or invalid
	// descriptor would make routing, budget ownership, shard credit distribution,
	// and snapshots ambiguous.
	errClassStateInvalidClass = "bufferpool.classState: size class must be valid"

	// errClassStateInvalidShardCount is used when class runtime state is
	// constructed without shards.
	//
	// A class needs at least one shard because shard is the data-plane contention
	// boundary that owns bucket storage and shard-local accounting.
	errClassStateInvalidShardCount = "bufferpool.classState: shard count must be greater than zero"

	// errClassStateShardIndexOutOfRange is used when a class operation references
	// a shard index outside the class-owned shard slice.
	errClassStateShardIndexOutOfRange = "bufferpool.classState: shard index out of range"

	// errClassStateRequestExceedsClassSize is used when a request larger than the
	// normalized class size reaches classState.
	//
	// classState is called after class-table request routing. A larger request
	// means the caller selected the wrong class.
	errClassStateRequestExceedsClassSize = "bufferpool.classState: requested size exceeds class size"

	// errClassStateInvalidAllocationCapacity is used when allocation accounting
	// is asked to record a zero-capacity backing buffer.
	errClassStateInvalidAllocationCapacity = "bufferpool.classState: allocation capacity must be greater than zero"

	// errClassStateAllocationBelowClassSize is used when allocation accounting is
	// routed to a class that the allocated backing capacity cannot serve.
	errClassStateAllocationBelowClassSize = "bufferpool.classState: allocation capacity must be greater than or equal to class size"
)

// classState owns runtime state for one normalized size class.
//
// The class table decides which SizeClass can serve a requested Size. Once a
// request or returned buffer has been routed to a class, classState owns the
// class-local runtime structures used to execute operations:
//
//   - class-level counters;
//   - class-level budget;
//   - shard-local runtime units;
//   - per-shard credit publication derived from the class budget.
//
// Responsibility boundary:
//
//   - class_table.go performs request-size normalization;
//   - class.go describes SizeClass identity and capacity;
//   - class_state.go owns runtime state for one SizeClass;
//   - shard_selection.go chooses shard indexes;
//   - class_admission.go owns minimal class-local retain eligibility checks;
//   - shard.go executes shard-local storage operations;
//   - class_counters.go records class-scope workload facts;
//   - class_budget.go computes whole-buffer class targets and shard credit;
//   - callers own lifecycle, owner-side accounting, and coordination.
//
// classState deliberately does not contain class-table lookup logic. It is
// constructed after a SizeClass is already known.
//
// classState also does not allocate buffers. Allocation remains an owner-level
// responsibility because the owner knows the requested size, selected class size,
// owner-side accounting mode, and lifecycle/admission state. classState only
// records allocation facts when the caller reports them.
//
// Concurrency:
//
// classState is safe for concurrent operations through its components:
//
//   - ordinary get/retain operations are synchronized at shard level;
//   - classCounters records lifetime/window facts with atomics;
//   - classBudget uses atomics;
//   - shardCredit uses atomics.
//
// A full classState snapshot is observational and not globally atomic across
// class budget, class counters, and all shards. Current retained usage is
// derived from shard snapshots.
//
// Copying:
//
// classState MUST NOT be copied after first use because it contains shards,
// buckets, mutexes, and atomic values.
type classState struct {
	// class is the immutable descriptor for this runtime state.
	class SizeClass

	// counters records class-scope workload and retained-storage facts.
	counters classCounters

	// budget stores the class-level retained-memory target and derives shard
	// credit plans.
	budget classBudget

	// shards are the shard-local runtime units owned by this class.
	shards []shard
}

// newClassState returns runtime state for one size class.
//
// shardCount MUST be greater than zero. bucketSlotLimit is the total physical
// bucket capacity for each shard. bucketSegmentSlotLimit optionally overrides
// the lazy segment size used by each shard bucket; tests may omit it and use the
// package default segment size clamped to the bucket size.
//
// The returned class state starts with zero assigned class budget, which means
// shard credit is disabled until the owner publishes a budget through
// updateBudget or updateBudgetLimit.
func newClassState(class SizeClass, shardCount int, bucketSlotLimit int, bucketSegmentSlotLimit ...int) classState {
	if !class.IsValid() {
		panic(errClassStateInvalidClass)
	}

	if shardCount <= 0 {
		panic(errClassStateInvalidShardCount)
	}

	shards := make([]shard, shardCount)
	for index := range shards {
		shards[index] = newShard(bucketSlotLimit, bucketSegmentSlotLimit...)
	}

	return classState{
		class:  class,
		budget: newClassBudget(class.Size()),
		shards: shards,
	}
}

// descriptor returns the size-class descriptor owned by this runtime state.
func (s *classState) descriptor() SizeClass {
	return s.class
}

// classID returns the ordinal class identifier.
func (s *classState) classID() ClassID {
	return s.class.ID()
}

// classSize returns the normalized reusable capacity of this class.
func (s *classState) classSize() ClassSize {
	return s.class.Size()
}

// shardCount returns the number of shards owned by this class.
func (s *classState) shardCount() int {
	return len(s.shards)
}

// mustShardAt returns a class-owned shard for internal classState operations.
//
// Ordinary get/retain paths MUST go through classState so class admission and
// class lifetime counters are preserved. Ordinary class-level get/retain paths
// MUST NOT call shard methods directly. Tests that need retained-storage state
// should prefer classState.state() unless direct shard access is explicitly
// testing shard-local behavior.
func (s *classState) mustShardAt(shardIndex int) *shard {
	if shardIndex < 0 || shardIndex >= len(s.shards) {
		panic(errClassStateShardIndexOutOfRange)
	}

	return &s.shards[shardIndex]
}

// updateBudget computes and publishes a new class budget from an assigned byte
// target.
//
// The method also derives the per-shard credit plan and applies the resulting
// shardCreditLimit values to all class-owned shards.
// Publication is shard-by-shard, not an atomic class-wide cutover. Hot-path
// retain uses the local shard credit it observes.
//
// The returned generation is the class-budget generation, not a shard-credit
// generation.
func (s *classState) updateBudget(assignedBytes Size) Generation {
	generation := s.budget.updateAssignedBytes(assignedBytes)
	s.applyShardCreditPlan(s.budget.shardCreditPlan(len(s.shards)))

	return generation
}

// updateBudgetLimit publishes an already computed class budget limit.
//
// The limit must belong to this class size. This method is useful when a higher
// layer computes classBudgetLimit values before applying them to class states.
// Publication is shard-by-shard, not an atomic class-wide cutover.
func (s *classState) updateBudgetLimit(limit classBudgetLimit) Generation {
	generation := s.budget.update(limit)
	s.applyShardCreditPlan(s.budget.shardCreditPlan(len(s.shards)))

	return generation
}

// disableBudget disables the class effective retained target and disables
// shard-local credit for all class-owned shards.
//
// Disabling budget does not physically clear already retained buffers. Physical
// cleanup belongs to trim or clear paths.
func (s *classState) disableBudget() Generation {
	generation := s.budget.disable()
	s.applyShardCreditPlan(s.budget.shardCreditPlan(len(s.shards)))

	return generation
}

// applyShardCreditPlan applies a class-derived shard credit plan to all shards.
//
// The plan MUST have the same shard count as the class state.
func (s *classState) applyShardCreditPlan(plan classShardCreditPlan) {
	if plan.ShardCount != len(s.shards) {
		panic(errClassStateInvalidShardCount)
	}

	for index := range s.shards {
		s.shards[index].updateCredit(plan.creditForShard(index))
	}
}

// tryGet attempts to reuse a retained buffer from one selected shard.
//
// requestedSize is the caller-requested size before class normalization. The
// class has already been selected by the caller. requestedSize MUST be less than
// or equal to the class size.
//
// The method records both shard-scope and class-scope accounting:
//
//   - shard.tryGet records shard counters;
//   - classState records class counters from the observed shard result.
func (s *classState) tryGet(shardIndex int, requestedSize Size) classGetResult {
	result := s.tryGetShard(shardIndex, requestedSize)
	s.counters.recordGetResult(requestedSize, result.Shard)

	return result
}

// tryGetShard performs one shard-local get and returns its class-shaped result
// without updating class-level counters.
//
// Ordinary single-shard get paths use tryGet, which records the class outcome
// immediately. Fallback probing needs several shard-local attempts to collapse
// into one logical class get, so it uses this helper and records exactly one
// class-level hit or miss after the final outcome is known.
func (s *classState) tryGetShard(shardIndex int, requestedSize Size) classGetResult {
	if requestedSize > s.class.ByteSize() {
		panic(errClassStateRequestExceedsClassSize)
	}

	shardResult := s.mustShardAt(shardIndex).tryGet(requestedSize)
	return classGetResult{
		Class:      s.class,
		ShardIndex: shardIndex,
		Shard:      shardResult,
	}
}

// tryGetSelected attempts to reuse a retained buffer through a selected shard.
func (s *classState) tryGetSelected(selector shardSelector, requestedSize Size) classGetResult {
	return s.tryGet(s.selectShard(selector), requestedSize)
}

// tryGetSelectedWithFallback attempts the primary selected shard first and then
// probes a bounded sequence of neighboring shards on miss.
//
// The configured fallback count is clamped to shardCount-1 so callers may keep
// one default policy across single-shard and multi-shard runtimes. Allocation on
// all-miss paths remains associated with the primary shard.
//
// Shard counters observe every physical probe. Class counters observe one
// logical Get: a fallback hit records one class hit, and an all-miss fallback
// records one class miss. This keeps aggregate hit ratio aligned with public Get
// calls while still making probe behavior visible in shard diagnostics.
func (s *classState) tryGetSelectedWithFallback(selector shardSelector, requestedSize Size, fallbackShards int) classGetResult {
	primaryShardIndex := s.selectShard(selector)
	primaryResult := s.tryGetShard(primaryShardIndex, requestedSize)
	if primaryResult.Hit() {
		s.counters.recordGetResult(requestedSize, primaryResult.Shard)
		return primaryResult
	}

	shardCount := len(s.shards)
	for offset := 1; offset <= boundedFallbackShardCount(fallbackShards, shardCount); offset++ {
		result := s.tryGetShard(fallbackShardIndex(primaryShardIndex, offset, shardCount), requestedSize)
		if result.Hit() {
			s.counters.recordGetResult(requestedSize, result.Shard)
			return result
		}
	}

	s.counters.recordGetResult(requestedSize, primaryResult.Shard)
	return primaryResult
}

// recordAllocation records an allocation associated with one selected shard and
// this class.
//
// The class does not allocate buffers itself. The owner should call this after a
// miss or bypass path allocates a new backing buffer for this class.
func (s *classState) recordAllocation(shardIndex int, capacity uint64) {
	selectedShard := s.mustShardAt(shardIndex)
	s.validateAllocationCapacity(capacity)

	selectedShard.recordAllocation(capacity)
	s.counters.recordAllocation(capacity)
}

// recordAllocatedBuffer records the capacity of an allocated buffer associated
// with one selected shard and this class.
func (s *classState) recordAllocatedBuffer(shardIndex int, buffer []byte) {
	s.recordAllocation(shardIndex, uint64(cap(buffer)))
}

// tryRetain attempts to retain a returned buffer through one selected shard.
//
// The method records both shard-scope and class-scope accounting:
//
//   - class admission rejects buffers that cannot serve this class;
//   - shard.tryRetain records shard counters only after class acceptance;
//   - classState records class counters from the observed class/shard result.
//
// Callers must perform any lifecycle, owner-side accounting, or additional
// retention-limit checks before or around this method.
func (s *classState) tryRetain(shardIndex int, buffer []byte) classRetainResult {
	return s.tryRetainWithOptions(shardIndex, buffer, classRetainOptions{})
}

// tryRetainWithOptions attempts to retain a returned buffer through one
// selected shard with class-level publication options.
//
// Class admission still runs before shard admission. Options are forwarded only
// after the buffer is known to be compatible with this class.
func (s *classState) tryRetainWithOptions(shardIndex int, buffer []byte, options classRetainOptions) classRetainResult {
	selectedShard := s.mustShardAt(shardIndex)
	capacity := uint64(cap(buffer))
	classDecision := evaluateClassRetain(s.class, buffer)
	if !classDecision.AllowsShardRetain() {
		s.counters.recordRejectedPut(capacity)

		return classRetainResult{
			Class:            s.class,
			ShardIndex:       shardIndex,
			ClassDecision:    classDecision,
			ReturnedCapacity: capacity,
		}
	}

	shardResult := selectedShard.tryRetainWithOptions(buffer, shardRetainOptions{
		ZeroBeforeRetain: options.ZeroBeforeRetain,
	})
	s.counters.recordRetainResult(shardResult)

	return classRetainResult{
		Class:            s.class,
		ShardIndex:       shardIndex,
		ClassDecision:    classDecision,
		Shard:            shardResult,
		ReturnedCapacity: shardResult.Capacity,
	}
}

// tryRetainSelected attempts to retain a returned buffer through a selected
// shard.
func (s *classState) tryRetainSelected(selector shardSelector, buffer []byte) classRetainResult {
	return s.tryRetain(s.selectShard(selector), buffer)
}

// tryRetainSelectedWithOptions attempts to retain a returned buffer through a
// selected shard with class-level publication options.
func (s *classState) tryRetainSelectedWithOptions(selector shardSelector, buffer []byte, options classRetainOptions) classRetainResult {
	return s.tryRetainWithOptions(s.selectShard(selector), buffer, options)
}

// classRetainOptions configures class-to-shard retain publication behavior.
type classRetainOptions struct {
	// ZeroBeforeRetain clears the returned buffer before shard publication when
	// the shard accepts retention.
	ZeroBeforeRetain bool
}

// trimShard removes up to maxBuffers retained buffers from one shard owned by
// this class.
//
// The shard records shard-scope trim counters. classState records class-scope
// trim counters from the observed physical removal result.
func (s *classState) trimShard(shardIndex int, maxBuffers int) bucketTrimResult {
	result := s.mustShardAt(shardIndex).trim(maxBuffers)
	s.counters.recordTrim(result)

	return result
}

// clearShard removes all retained buffers from one shard owned by this class.
//
// The shard records shard-scope clear counters. classState records class-scope
// clear counters from the observed physical removal result.
func (s *classState) clearShard(shardIndex int) bucketTrimResult {
	result := s.mustShardAt(shardIndex).clear()
	s.counters.recordClear(result)

	return result
}

// clear removes retained buffers from all shards owned by this class.
//
// clear is an internal class-wide storage reduction helper, not a transactional
// global barrier. It clears shards one by one, synchronized by each shard. A
// caller that needs strong shutdown semantics must stop new get/retain
// operations at the owner lifecycle level before calling clear.
//
// The returned result is the aggregate physical removal observed by this call.
func (s *classState) clear() classStorageReductionResult {
	var total classStorageReductionResult

	for index := range s.shards {
		result := s.shards[index].clear()
		total.addBucketTrimResult(result)
	}

	s.counters.recordClearAmount(total.RemovedBuffers, total.RemovedBytes)

	return total
}

// selectShard chooses and validates a shard index for this class state.
func (s *classState) selectShard(selector shardSelector) int {
	return selectShardIndex(selector, len(s.shards))
}

// boundedFallbackShardCount returns the number of additional shards that may be
// probed after the primary shard.
//
// The function intentionally clamps instead of rejecting counts larger than the
// local shard set. This lets defaults enable one fallback probe while still
// constructing valid single-shard pools. Negative configured values should be
// rejected by policy validation, but the hot-path helper treats them as disabled
// defensively.
func boundedFallbackShardCount(configured int, shardCount int) int {
	if configured <= 0 || shardCount <= 1 {
		return 0
	}

	maxFallback := shardCount - 1
	if configured > maxFallback {
		return maxFallback
	}

	return configured
}

// fallbackShardIndex maps a primary shard and one-based fallback offset to the
// next shard candidate.
//
// Power-of-two shard counts use a mask, matching the default resolver shape and
// avoiding division. Non-power-of-two counts are still supported for explicit
// policies and use modulo. The caller must pass an offset in
// [1, boundedFallbackShardCount].
func fallbackShardIndex(primaryShardIndex int, offset int, shardCount int) int {
	candidate := primaryShardIndex + offset
	if shardCount > 0 && shardCount&(shardCount-1) == 0 {
		return candidate & (shardCount - 1)
	}

	return candidate % shardCount
}

func (s *classState) validateAllocationCapacity(capacity uint64) {
	if capacity == 0 {
		panic(errClassStateInvalidAllocationCapacity)
	}

	if capacity < s.class.Bytes() {
		panic(errClassStateAllocationBelowClassSize)
	}
}

// state returns an observational snapshot of this class runtime state.
//
// The returned state is not globally atomic across budget, counters, and shards.
// Current retained usage is derived from shard snapshots. Without concurrent
// mutation this derived state is exactly consistent; with concurrent mutation it
// is race-safe but may observe different shards at different instants.
func (s *classState) state() classStateSnapshot {
	shards := make([]shardState, len(s.shards))
	var currentRetainedBuffers uint64
	var currentRetainedBytes uint64

	for index := range s.shards {
		shardState := s.shards[index].state()
		shards[index] = shardState
		currentRetainedBuffers += shardState.Counters.CurrentRetainedBuffers
		currentRetainedBytes += shardState.Counters.CurrentRetainedBytes
	}

	return classStateSnapshot{
		Class:                  s.class,
		Budget:                 s.budget.snapshot(),
		Counters:               s.counters.snapshot(),
		Shards:                 shards,
		CurrentRetainedBuffers: currentRetainedBuffers,
		CurrentRetainedBytes:   currentRetainedBytes,
	}
}

// countersSnapshot returns the class-scope counter snapshot.
func (s *classState) countersSnapshot() classCountersSnapshot {
	return s.counters.snapshot()
}

// budgetSnapshot returns the class budget snapshot.
func (s *classState) budgetSnapshot() classBudgetSnapshot {
	return s.budget.snapshot()
}

// classGetResult describes one class-level get/reuse attempt.
//
// It preserves the class descriptor and shard index used for the operation while
// delegating hit/miss details to shardGetResult.
type classGetResult struct {
	// Class is the size class that handled the get attempt.
	Class SizeClass

	// ShardIndex is the selected shard inside Class.
	ShardIndex int

	// Shard is the shard-local get result.
	Shard shardGetResult
}

// Hit reports whether the get was satisfied from retained storage.
func (r classGetResult) Hit() bool {
	return r.Shard.Hit
}

// Miss reports whether the get did not find retained storage.
func (r classGetResult) Miss() bool {
	return r.Shard.Miss()
}

// Buffer returns the reused buffer.
func (r classGetResult) Buffer() []byte {
	return r.Shard.Buffer
}

// Capacity returns the capacity of the reused buffer on hit.
func (r classGetResult) Capacity() uint64 {
	return r.Shard.Capacity
}

// classRetainResult describes one class-level retain attempt.
//
// It preserves the class descriptor, selected shard index, class-level decision,
// and shard-local outcome for the operation.
type classRetainResult struct {
	// Class is the size class that handled the retain attempt.
	Class SizeClass

	// ShardIndex is the selected shard inside Class.
	ShardIndex int

	// ClassDecision is the class-local retain eligibility decision.
	ClassDecision classRetainDecision

	// Shard is the shard-local retain result.
	Shard shardRetainResult

	// ReturnedCapacity is cap(buffer) as observed before any class or shard
	// retention decision.
	ReturnedCapacity uint64
}

// Retained reports whether the returned buffer was physically retained.
func (r classRetainResult) Retained() bool {
	return r.ClassDecision.AllowsShardRetain() && r.Shard.Retained
}

// Dropped reports whether the returned buffer was not physically retained.
func (r classRetainResult) Dropped() bool {
	return !r.Retained()
}

// RejectedByClass reports whether class-local admission rejected retention
// before shard storage was attempted.
func (r classRetainResult) RejectedByClass() bool {
	return !r.ClassDecision.AllowsShardRetain()
}

// RejectedByCredit reports whether shard credit rejected retention.
func (r classRetainResult) RejectedByCredit() bool {
	return r.ClassDecision.AllowsShardRetain() && r.Shard.RejectedByCredit()
}

// RejectedByBucket reports whether credit accepted retention but bucket storage
// rejected the buffer.
func (r classRetainResult) RejectedByBucket() bool {
	return r.ClassDecision.AllowsShardRetain() && r.Shard.RejectedByBucket()
}

// Capacity returns the returned buffer capacity evaluated by the class.
func (r classRetainResult) Capacity() uint64 {
	return r.ReturnedCapacity
}

// CreditDecision returns the shard-credit decision observed by this class-level
// retain attempt. If class-local admission rejected the buffer before shard
// credit was evaluated, it returns shardCreditNotEvaluated.
func (r classRetainResult) CreditDecision() shardCreditDecision {
	if !r.ClassDecision.AllowsShardRetain() {
		return shardCreditNotEvaluated
	}

	return r.Shard.CreditDecision
}

// ClassRetainDecision returns the class-local retain eligibility decision.
func (r classRetainResult) ClassRetainDecision() classRetainDecision {
	return r.ClassDecision
}

// classStateSnapshot is an observational view of one class runtime state.
//
// It is intentionally internal. Public snapshots should be modeled separately so
// runtime internals can evolve without freezing public API structure.
type classStateSnapshot struct {
	// Class is the size-class descriptor represented by this state.
	Class SizeClass

	// Budget is the current class budget snapshot.
	Budget classBudgetSnapshot

	// Counters is the current class counter snapshot.
	Counters classCountersSnapshot

	// Shards are observational snapshots of class-owned shards.
	Shards []shardState

	// CurrentRetainedBuffers is derived by summing shard current retained
	// counters in this snapshot.
	CurrentRetainedBuffers uint64

	// CurrentRetainedBytes is derived by summing shard current retained byte
	// counters in this snapshot.
	CurrentRetainedBytes uint64
}

// ShardCount returns the number of shard snapshots.
func (s classStateSnapshot) ShardCount() int {
	return len(s.Shards)
}

// RetainedUsage returns retained usage derived from shard snapshots.
func (s classStateSnapshot) RetainedUsage() shardCreditUsage {
	return shardCreditUsage{
		RetainedBuffers: s.CurrentRetainedBuffers,
		RetainedBytes:   s.CurrentRetainedBytes,
	}
}

// IsOverBudget reports whether class retained usage exceeds the class budget in
// either retained-buffer or retained-byte dimension.
//
// A zero or ineffective budget treats any retained usage as over-budget. This is
// useful after budget contraction: disabling budget prevents new retention but
// does not physically clear existing retained buffers.
func (s classStateSnapshot) IsOverBudget() bool {
	usage := s.RetainedUsage()

	if !s.Budget.IsEffective() {
		return !usage.IsZero()
	}

	if usage.RetainedBuffers > s.Budget.TargetBuffers {
		return true
	}

	return usage.RetainedBytes > s.Budget.TargetBytes
}

// classStorageReductionResult is an aggregate physical storage-reduction result
// across one or more class-owned shards.
type classStorageReductionResult struct {
	// RemovedBuffers is the total number of physically removed retained buffers.
	RemovedBuffers int

	// RemovedBytes is the total retained backing capacity removed in bytes.
	RemovedBytes uint64
}

// addBucketTrimResult adds one shard/bucket trim result to this aggregate.
func (r *classStorageReductionResult) addBucketTrimResult(result bucketTrimResult) {
	r.RemovedBuffers += result.RemovedBuffers
	r.RemovedBytes += result.RemovedBytes
}

// IsZero reports whether no retained storage was removed.
func (r classStorageReductionResult) IsZero() bool {
	return r.RemovedBuffers == 0 && r.RemovedBytes == 0
}
