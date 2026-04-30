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

import "errors"

// PoolGroupBudgetSnapshot describes aggregate group budget usage.
//
// The group currently evaluates budget pressure by projecting its aggregate
// partition sample through partition-shaped retained-byte policy fields. This
// type gives report-facing group APIs group vocabulary while keeping the shared
// internal math reusable and explicit.
type PoolGroupBudgetSnapshot PartitionBudgetSnapshot

// IsOverBudget reports whether the aggregate group sample exceeds any enabled
// retained-byte budget limit.
func (s PoolGroupBudgetSnapshot) IsOverBudget() bool {
	return PartitionBudgetSnapshot(s).IsOverBudget()
}

// PartitionBudgetTarget is a retained-memory budget publication for one
// group-owned partition.
//
// The target is a control-plane value. It does not execute trim, does not move
// retained buffers, and does not mutate Pools directly. Group code may publish
// it into a partition runtime policy boundary, and later partition code may
// derive PoolBudgetTarget values from it.
type PartitionBudgetTarget struct {
	// Generation identifies the budget target publication.
	Generation Generation

	// PartitionName is the group-local partition name.
	PartitionName string

	// RetainedBytes is the assigned retained-memory target.
	RetainedBytes Size

	// MinRetainedBytes is the lower clamp used while computing the target.
	MinRetainedBytes Size

	// MaxRetainedBytes is the upper clamp used while computing the target. A zero
	// value means the allocator treated the partition as unbounded above.
	MaxRetainedBytes Size
}

// partitionBudgetAllocationInput is one group-to-partition allocation input.
type partitionBudgetAllocationInput struct {
	PartitionName     string
	BaseRetainedBytes Size
	MinRetainedBytes  Size
	MaxRetainedBytes  Size
	Score             float64
}

// partitionBudgetAllocationReport describes group-to-partition target
// feasibility.
type partitionBudgetAllocationReport struct {
	// Targets are deterministic partition targets derived from the allocation.
	Targets []PartitionBudgetTarget

	// Allocation reports whether child minimums fit the parent group target.
	Allocation budgetAllocationReport
}

// computePartitionBudgetTargets computes group-to-partition budget targets.
func (g *PoolGroup) computePartitionBudgetTargets(
	generation Generation,
	retainedBytes Size,
	inputs []partitionBudgetAllocationInput,
) []PartitionBudgetTarget {
	g.mustBeInitialized()

	return allocatePartitionBudgetTargets(generation, retainedBytes, inputs)
}

// applyPartitionBudgetTargets publishes retained-byte targets into group-owned
// partitions.
//
// This is an internal manual publication hook. It is serialized with hard Close
// through runtimeMu, rejects closed groups, and publishes only partition policy
// targets. Pool/class distribution remains a PoolPartition TickInto
// responsibility.
func (g *PoolGroup) applyPartitionBudgetTargets(targets []PartitionBudgetTarget) error {
	g.mustBeInitialized()
	g.runtimeMu.RLock()
	defer g.runtimeMu.RUnlock()

	if !g.lifecycle.AllowsWork() {
		return newError(ErrClosed, errGroupClosed)
	}

	skipped, err := g.publishPartitionBudgetTargets(targets, nil)
	if err != nil {
		return err
	}
	if len(skipped) > 0 {
		return newError(ErrClosed, errGroupClosed)
	}

	return nil
}

// publishPartitionBudgetTargets publishes targets without taking runtimeMu.
//
// Callers must already hold the appropriate group foreground gate. Closed child
// partitions are recorded as skipped so applied group ticks can report partial
// publication without duplicating Partition close semantics.
func (g *PoolGroup) publishPartitionBudgetTargets(
	targets []PartitionBudgetTarget,
	skipped []PoolGroupSkippedPartition,
) ([]PoolGroupSkippedPartition, error) {
	for _, target := range targets {
		partition, ok := g.registry.partition(target.PartitionName)
		if !ok {
			return skipped, newError(ErrInvalidOptions, errGroupPartitionMissing+": "+target.PartitionName)
		}
		if err := partition.applyPartitionBudget(target); err != nil {
			if errors.Is(err, ErrClosed) {
				skipped = append(skipped, PoolGroupSkippedPartition{PartitionName: target.PartitionName, Reason: err.Error()})
				continue
			}
			return skipped, err
		}
	}

	return skipped, nil
}

// allocatePartitionBudgetTargets applies the common base-plus-adaptive allocator
// to group-local partitions.
func allocatePartitionBudgetTargets(
	generation Generation,
	retainedBytes Size,
	inputs []partitionBudgetAllocationInput,
) []PartitionBudgetTarget {
	return allocatePartitionBudgetTargetsReport(generation, retainedBytes, inputs).Targets
}

// allocatePartitionBudgetTargetsReport applies the common allocator and returns
// feasibility diagnostics for hard-budget callers.
func allocatePartitionBudgetTargetsReport(
	generation Generation,
	retainedBytes Size,
	inputs []partitionBudgetAllocationInput,
) partitionBudgetAllocationReport {
	allocationInputs := make([]budgetAllocationInput, len(inputs))
	for index, input := range inputs {
		allocationInputs[index] = budgetAllocationInput{
			BaseBytes: input.BaseRetainedBytes.Bytes(),
			MinBytes:  input.MinRetainedBytes.Bytes(),
			MaxBytes:  input.MaxRetainedBytes.Bytes(),
			Score:     input.Score,
		}
	}

	allocation := allocateBudgetTargetsReport(retainedBytes.Bytes(), allocationInputs)
	targets := make([]PartitionBudgetTarget, len(inputs))
	for index, result := range allocation.Results {
		targets[index] = PartitionBudgetTarget{
			Generation:       generation,
			PartitionName:    inputs[index].PartitionName,
			RetainedBytes:    SizeFromBytes(result.AssignedBytes),
			MinRetainedBytes: inputs[index].MinRetainedBytes,
			MaxRetainedBytes: inputs[index].MaxRetainedBytes,
		}
	}

	return partitionBudgetAllocationReport{Targets: targets, Allocation: allocation}
}
