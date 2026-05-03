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

// group_budget_allocator.go owns pure group-to-partition retained-budget
// allocation. These helpers compute deterministic targets and feasibility
// diagnostics only; publication helpers consume the targets later.

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

// computePartitionBudgetTargets computes group-to-partition budget targets for
// advisory callers that do not publish runtime state.
//
// Applied coordinator code must use computePartitionBudgetTargetsReport so
// feasibility does not disappear before hard-budget publication.
func (g *PoolGroup) computePartitionBudgetTargets(
	generation Generation,
	retainedBytes Size,
	inputs []partitionBudgetAllocationInput,
) []PartitionBudgetTarget {
	g.mustBeInitialized()

	return allocatePartitionBudgetTargets(generation, retainedBytes, inputs)
}

// computePartitionBudgetTargetsReport computes group-to-partition budget targets
// together with feasibility diagnostics for applied coordinator paths.
func (g *PoolGroup) computePartitionBudgetTargetsReport(
	generation Generation,
	retainedBytes Size,
	inputs []partitionBudgetAllocationInput,
) partitionBudgetAllocationReport {
	g.mustBeInitialized()

	return allocatePartitionBudgetTargetsReport(generation, retainedBytes, inputs)
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
