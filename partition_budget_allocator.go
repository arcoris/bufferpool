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

// PoolBudgetTarget is a retained-memory budget publication for one
// partition-owned Pool.
//
// The optional ClassTargets field carries precomputed class-level targets for
// the Pool. When ClassTargets is empty, Pool.applyPoolBudget computes an equal
// class distribution from RetainedBytes and the Pool's static class caps.
type PoolBudgetTarget struct {
	// Generation identifies the budget target publication.
	Generation Generation

	// PoolName is the partition-local Pool name.
	PoolName string

	// RetainedBytes is the assigned retained-memory target for the Pool.
	RetainedBytes Size

	// ClassTargets optionally contains explicit class targets for the Pool.
	ClassTargets []ClassBudgetTarget
}

// poolBudgetAllocationInput is one partition-to-Pool allocation input.
type poolBudgetAllocationInput struct {
	PoolName          string
	BaseRetainedBytes Size
	MinRetainedBytes  Size
	MaxRetainedBytes  Size
	Score             float64
}

// applyPartitionBudget publishes the retained target into the partition policy
// stream.
//
// This method updates only partition runtime policy. It does not derive Pool
// budgets, publish class targets, execute trim, or start controller work.
func (p *PoolPartition) applyPartitionBudget(target PartitionBudgetTarget) error {
	p.mustBeInitialized()
	if !p.lifecycle.AllowsWork() {
		return newError(ErrClosed, errPartitionClosed)
	}

	runtime := p.currentRuntimeSnapshot()
	policy := runtime.Policy
	policy.Budget.MaxRetainedBytes = target.RetainedBytes
	if err := policy.Validate(); err != nil {
		return err
	}

	generation := budgetPublicationGeneration(runtime.Generation, target.Generation)
	p.publishRuntimeSnapshot(newPartitionRuntimeSnapshotWithPressure(generation, policy, runtime.Pressure))
	return nil
}

// applyPoolBudgetTargets publishes retained targets into partition-owned Pools.
//
// This is an internal manual publication hook for later partition controllers.
// It delegates to Pool.applyPoolBudget so Pool remains the owner of class budget
// and shard-credit publication.
func (p *PoolPartition) applyPoolBudgetTargets(targets []PoolBudgetTarget) error {
	p.mustBeInitialized()
	if !p.lifecycle.AllowsWork() {
		return newError(ErrClosed, errPartitionClosed)
	}

	for _, target := range targets {
		pool, ok := p.registry.pool(target.PoolName)
		if !ok {
			return newError(ErrInvalidOptions, errPartitionPoolMissing+": "+target.PoolName)
		}

		_ = pool.applyPoolBudget(target)
		if index, ok := p.registry.poolIndex(target.PoolName); ok {
			_ = p.activeRegistry.markDirtyIndex(index)
		}
	}

	return nil
}

// allocatePoolBudgetTargets applies the common base-plus-adaptive allocator to
// partition-local Pools.
func allocatePoolBudgetTargets(
	generation Generation,
	retainedBytes Size,
	inputs []poolBudgetAllocationInput,
) []PoolBudgetTarget {
	allocationInputs := make([]budgetAllocationInput, len(inputs))
	for index, input := range inputs {
		allocationInputs[index] = budgetAllocationInput{
			BaseBytes: input.BaseRetainedBytes.Bytes(),
			MinBytes:  input.MinRetainedBytes.Bytes(),
			MaxBytes:  input.MaxRetainedBytes.Bytes(),
			Score:     input.Score,
		}
	}

	results := allocateBudgetTargets(retainedBytes.Bytes(), allocationInputs)
	targets := make([]PoolBudgetTarget, len(inputs))
	for index, result := range results {
		targets[index] = PoolBudgetTarget{
			Generation:    generation,
			PoolName:      inputs[index].PoolName,
			RetainedBytes: SizeFromBytes(result.AssignedBytes),
		}
	}

	return targets
}
