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

// partition_budget_allocator.go owns pure partition-to-Pool retained-budget
// allocation. These helpers return target plans and feasibility diagnostics
// only; partition_budget_publication.go consumes the plans and mutates runtime
// state under owner gates.

// poolBudgetAllocationInput is one partition-to-Pool allocation input.
type poolBudgetAllocationInput struct {
	PoolName          string
	BaseRetainedBytes Size
	MinRetainedBytes  Size
	MaxRetainedBytes  Size
	Score             float64
}

// poolBudgetAllocationReport describes partition-to-Pool target feasibility.
type poolBudgetAllocationReport struct {
	// Targets are deterministic Pool targets derived from the allocation.
	Targets []PoolBudgetTarget

	// Allocation reports whether child minimums fit the parent partition target.
	Allocation budgetAllocationReport
}

// allocatePoolBudgetTargets applies the common base-plus-adaptive allocator to
// partition-local Pools.
func allocatePoolBudgetTargets(
	generation Generation,
	retainedBytes Size,
	inputs []poolBudgetAllocationInput,
) []PoolBudgetTarget {
	return allocatePoolBudgetTargetsReport(generation, retainedBytes, inputs).Targets
}

// allocatePoolBudgetTargetsReport applies the common allocator and returns
// feasibility diagnostics for hard-budget callers.
func allocatePoolBudgetTargetsReport(
	generation Generation,
	retainedBytes Size,
	inputs []poolBudgetAllocationInput,
) poolBudgetAllocationReport {
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
	targets := make([]PoolBudgetTarget, len(inputs))
	for index, result := range allocation.Results {
		targets[index] = PoolBudgetTarget{
			Generation:    generation,
			PoolName:      inputs[index].PoolName,
			RetainedBytes: SizeFromBytes(result.AssignedBytes),
		}
	}

	return poolBudgetAllocationReport{Targets: targets, Allocation: allocation}
}
