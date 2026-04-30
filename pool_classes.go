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
	// errPoolInvalidShardSelectionMode is used when the effective policy contains
	// a shard selection mode that standalone Pool cannot execute.
	errPoolInvalidShardSelectionMode = "bufferpool.Pool: unsupported shard selection mode"

	// errPoolClassIDOutOfRange is used when a class descriptor produced by the
	// class table cannot index the Pool-owned class-state slice.
	errPoolClassIDOutOfRange = "bufferpool.Pool: class id out of range"

	// errPoolClassDescriptorMismatch is used when a class id points to a class
	// state with a different descriptor.
	errPoolClassDescriptorMismatch = "bufferpool.Pool: class descriptor mismatch"
)

// ClassCount returns the number of enabled size classes.
//
// The value comes from the immutable class table built during Pool construction.
func (p *Pool) ClassCount() int {
	p.mustBeInitialized()

	return p.table.len()
}

// ClassSizes returns a copy of the configured class-size profile.
//
// Mutating the returned slice cannot affect the live Pool.
func (p *Pool) ClassSizes() []ClassSize {
	p.mustBeInitialized()

	return p.table.classSizes()
}

// originClassForAcquiredCapacity maps a Pool-returned backing capacity back to
// the immutable class descriptor that produced it.
//
// LeaseRegistry uses this helper during acquisition diagnostics instead of
// reaching into the Pool's class table directly. The helper keeps class-table
// ownership inside Pool while still letting the ownership layer record the
// origin class needed for strict release validation and active-byte accounting.
func (p *Pool) originClassForAcquiredCapacity(capacity uint64) (SizeClass, bool) {
	p.mustBeInitialized()
	if capacity == 0 {
		return SizeClass{}, false
	}

	return p.table.classForCapacity(SizeFromBytes(capacity))
}

// publishInitialBudgets publishes the Pool's initial static class budgets.
//
// Pool has no hidden adaptive controller. It therefore starts from a
// deterministic static distribution of SoftRetainedBytes across classes. The
// algorithm caps every class by policy-level class/shard limits and
// redistributes unused budget among classes that still have spare capacity.
//
// This is not the final adaptive-budget algorithm. PoolGroup/PoolPartition
// controllers may later compute workload-aware class targets and publish them
// through classState.updateBudget or updateBudgetLimit.
func (p *Pool) publishInitialBudgets() {
	if len(p.classes) == 0 {
		return
	}

	assignments := poolInitialBudgetAssignments(p.constructionPolicy, p.classes)
	for index := range p.classes {
		p.classes[index].updateBudget(SizeFromBytes(assignments[index]))
	}
}

// mustClassStateFor returns the Pool-owned classState for class.
//
// The class descriptor must come from this Pool's class table. Manufacturing
// arbitrary ClassID values outside class-table construction is invalid because
// ClassID is only an ordinal inside one table.
func (p *Pool) mustClassStateFor(class SizeClass) *classState {
	index := class.ID().Index()
	if index < 0 || index >= len(p.classes) {
		panic(errPoolClassIDOutOfRange)
	}

	state := &p.classes[index]
	if !state.descriptor().Equal(class) {
		panic(errPoolClassDescriptorMismatch)
	}

	return state
}

// newPoolClassStates builds class runtime states for every class-table entry.
//
// The returned slice is indexed by ClassID. Each class receives the same shard
// shape from the effective Pool policy.
func newPoolClassStates(table classTable, policy Policy) []classState {
	classes := table.classesCopy()
	states := make([]classState, len(classes))

	for index, class := range classes {
		states[index] = newClassState(
			class,
			policy.Shards.ShardsPerClass,
			policy.Shards.BucketSlotsPerShard,
			policy.Shards.BucketSegmentSlotsPerShard,
		)
	}

	return states
}

// newPoolShardSelectors constructs one shard selector per class.
//
// Selectors are owner-local. They are not shared across pools, groups, or
// partitions. Keeping sequence state per class avoids a single pool-wide
// contention point for sequence-based modes.
func newPoolShardSelectors(classCount int, mode ShardSelectionMode) ([]shardSelector, error) {
	selectors := make([]shardSelector, classCount)
	for index := range selectors {
		selector, err := newPoolShardSelector(mode, index)
		if err != nil {
			return nil, err
		}

		selectors[index] = selector
	}

	return selectors, nil
}

// newPoolShardSelector constructs one selector instance for a single class.
//
// Pool creates one selector per class so sequence-based modes do not contend on
// a single pool-wide atomic counter. The selector itself remains small and local
// to classState calls.
func newPoolShardSelector(mode ShardSelectionMode, classIndex int) (shardSelector, error) {
	switch mode {
	case ShardSelectionModeSingle:
		return singleShardSelector{}, nil

	case ShardSelectionModeRoundRobin:
		return &roundRobinShardSelector{}, nil

	case ShardSelectionModeRandom:
		return &poolRandomShardSelector{}, nil

	case ShardSelectionModeProcessorInspired:
		return newProcessorInspiredShardSelector(classIndex), nil

	case ShardSelectionModeAffinity:
		return nil, newError(ErrInvalidPolicy, errPoolUnsupportedAffinityShardSelection)

	default:
		return nil, newError(ErrInvalidPolicy, errPoolInvalidShardSelectionMode)
	}
}

// shardSelectorFor returns the selector associated with class.
//
// ClassID values are table-local ordinals. The descriptor must come from this
// Pool's class table, and invalid ids panic because using a selector for the
// wrong class would corrupt routing and accounting.
func (p *Pool) shardSelectorFor(class SizeClass) shardSelector {
	index := class.ID().Index()
	if index < 0 || index >= len(p.selectors) {
		panic(errPoolClassIDOutOfRange)
	}

	return p.selectors[index]
}

// poolInitialBudgetAssignments computes deterministic static class budgets.
//
// The algorithm starts from SoftRetainedBytes, caps every class by the policy's
// byte and buffer limits, and redistributes unused bytes among classes that
// still have spare capacity. It is intentionally static: future controllers may
// publish workload-aware budgets later, but Pool construction must not hide an
// adaptive control loop.
func poolInitialBudgetAssignments(policy Policy, classes []classState) []uint64 {
	assignments := make([]uint64, len(classes))
	if len(classes) == 0 {
		return assignments
	}

	caps := make([]uint64, len(classes))
	var totalCap uint64
	for index := range classes {
		capBytes := poolClassStaticBudgetCap(policy, classes[index].classSize())
		caps[index] = capBytes
		totalCap = poolSaturatingAdd(totalCap, capBytes)
	}

	remaining := poolMinUint64(policy.Retention.SoftRetainedBytes.Bytes(), totalCap)
	for remaining > 0 {
		eligible := poolBudgetEligibleClasses(assignments, caps)
		if eligible == 0 {
			return assignments
		}

		share := remaining / uint64(eligible)
		if share == 0 {
			share = 1
		}

		for index := range assignments {
			if assignments[index] >= caps[index] {
				continue
			}

			room := caps[index] - assignments[index]
			assigned := poolMinUint64(share, room)
			assigned = poolMinUint64(assigned, remaining)

			assignments[index] += assigned
			remaining -= assigned
			if remaining == 0 {
				return assignments
			}
		}
	}

	return assignments
}

// poolBudgetEligibleClasses counts classes that can still receive budget.
//
// A class is eligible when its current assignment is below its static cap. The
// count is recomputed each redistribution pass so capped classes stop receiving
// shares immediately.
func poolBudgetEligibleClasses(assignments []uint64, caps []uint64) int {
	var eligible int
	for index := range assignments {
		if assignments[index] < caps[index] {
			eligible++
		}
	}

	return eligible
}

// poolClassStaticBudgetCap returns the maximum byte target this Pool may assign
// to one class under static standalone budget publication.
//
// The cap combines class-level and shard-level byte/buffer limits:
//
//   - MaxClassRetainedBytes;
//   - MaxClassRetainedBuffers * classSize;
//   - ShardsPerClass * MaxShardRetainedBytes;
//   - ShardsPerClass * MaxShardRetainedBuffers * classSize.
//
// The result is a byte target, not a current usage value.
func poolClassStaticBudgetCap(policy Policy, classSize ClassSize) uint64 {
	classBytes := classSize.Bytes()

	byClassBytes := policy.Retention.MaxClassRetainedBytes.Bytes()
	byClassBuffers := poolSaturatingProduct(policy.Retention.MaxClassRetainedBuffers, classBytes)

	byShardBytes := poolSaturatingProduct(
		uint64(policy.Shards.ShardsPerClass),
		policy.Retention.MaxShardRetainedBytes.Bytes(),
	)

	byShardBuffers := poolSaturatingProduct(
		uint64(policy.Shards.ShardsPerClass),
		poolSaturatingProduct(policy.Retention.MaxShardRetainedBuffers, classBytes),
	)

	return poolMinUint64(
		byClassBytes,
		poolMinUint64(
			byClassBuffers,
			poolMinUint64(byShardBytes, byShardBuffers),
		),
	)
}

// poolSaturatingProduct returns left * right or max uint64 on overflow.
//
// Budget cap calculation should be conservative under overflow. Saturating to
// max keeps the overflowing dimension from accidentally becoming a small cap.
func poolSaturatingProduct(left, right uint64) uint64 {
	if left == 0 || right == 0 {
		return 0
	}

	max := ^uint64(0)
	if left > max/right {
		return max
	}

	return left * right
}

// poolSaturatingAdd returns left + right or max uint64 on overflow.
//
// Static budget aggregation should never wrap. Saturating keeps overflow from
// turning a large total cap into a small value that would underfund classes.
func poolSaturatingAdd(left, right uint64) uint64 {
	max := ^uint64(0)
	if left > max-right {
		return max
	}

	return left + right
}

// poolMinUint64 returns the smaller of left and right.
//
// The helper keeps budget arithmetic readable without introducing a generic
// dependency for one unsigned integer operation.
func poolMinUint64(left, right uint64) uint64 {
	if left < right {
		return left
	}

	return right
}
