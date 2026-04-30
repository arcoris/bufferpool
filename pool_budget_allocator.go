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
	// errPoolBudgetTargetClassMissing reports a class budget target whose
	// ClassID cannot index this Pool's immutable class table.
	errPoolBudgetTargetClassMissing = "bufferpool.Pool: class budget target class not found"

	// errPoolBudgetTargetDuplicateClass reports two targets for the same class in
	// one publication batch.
	errPoolBudgetTargetDuplicateClass = "bufferpool.Pool: duplicate class budget target"
)

// ClassBudgetTarget is a retained-memory budget publication for one Pool size
// class.
//
// TargetBytes is an assigned byte target. classBudget normalizes it to whole
// class-size buffers before deriving shard-local credit.
type ClassBudgetTarget struct {
	// Generation identifies the budget target publication.
	Generation Generation

	// ClassID identifies the Pool-local class receiving the target.
	ClassID ClassID

	// TargetBytes is the assigned retained-byte target for the class.
	TargetBytes Size
}

// classBudgetAllocationInput is one Pool-to-class allocation input.
type classBudgetAllocationInput struct {
	ClassID         ClassID
	BaseTargetBytes Size
	MinTargetBytes  Size
	MaxTargetBytes  Size
	Score           float64
}

// applyPoolBudget publishes a Pool-level budget target into class budgets.
//
// When target.ClassTargets is empty, the Pool computes class targets from
// target.RetainedBytes using static class caps and equal zero-score distribution.
// The method does not trim over-target retained storage and is never called from
// Pool.Get or Pool.Put.
func (p *Pool) applyPoolBudget(target PoolBudgetTarget) Generation {
	p.mustBeInitialized()

	classTargets := target.ClassTargets
	if len(classTargets) == 0 {
		classTargets = p.defaultClassBudgetTargets(target.Generation, target.RetainedBytes)
	}

	return p.applyClassBudgets(classTargets)
}

// applyClassBudgets publishes class targets and derived shard credits.
//
// The method validates the complete target batch before applying any class so a
// malformed publication cannot partially update a Pool. It returns the latest
// Pool runtime generation published for the batch.
func (p *Pool) applyClassBudgets(targets []ClassBudgetTarget) Generation {
	p.mustBeInitialized()

	if len(targets) == 0 {
		return p.currentRuntimeSnapshot().Generation
	}

	p.mustValidateClassBudgetTargets(targets)

	var generation Generation
	for _, target := range targets {
		state := &p.classes[target.ClassID.Index()]
		targetGeneration := budgetPublicationGeneration(state.budgetSnapshot().Generation, target.Generation)
		classGeneration := state.updateBudgetAtLeast(target.TargetBytes, targetGeneration)
		if generation.Before(classGeneration) {
			generation = classGeneration
		}
	}

	runtime := p.currentRuntimeSnapshot()
	poolGeneration := budgetPublicationGeneration(runtime.Generation, generation)
	p.publishRuntimeSnapshot(newPoolRuntimeSnapshotWithPressure(poolGeneration, runtime.Policy, runtime.Pressure))
	return poolGeneration
}

// defaultClassBudgetTargets computes class targets for a Pool-level retained
// target when the caller did not provide explicit class targets.
func (p *Pool) defaultClassBudgetTargets(generation Generation, retainedBytes Size) []ClassBudgetTarget {
	inputs := make([]classBudgetAllocationInput, len(p.classes))
	for index := range p.classes {
		inputs[index] = classBudgetAllocationInput{
			ClassID:        p.classes[index].classID(),
			MaxTargetBytes: SizeFromBytes(poolClassStaticBudgetCap(p.currentRuntimeSnapshot().Policy, p.classes[index].classSize())),
		}
	}

	return allocateClassBudgetTargets(generation, retainedBytes, inputs)
}

// mustValidateClassBudgetTargets validates class target identity before
// publication.
func (p *Pool) mustValidateClassBudgetTargets(targets []ClassBudgetTarget) {
	seen := make(map[ClassID]struct{}, len(targets))
	for _, target := range targets {
		index := target.ClassID.Index()
		if index < 0 || index >= len(p.classes) {
			panic(errPoolBudgetTargetClassMissing)
		}
		if p.classes[index].classID() != target.ClassID {
			panic(errPoolBudgetTargetClassMissing)
		}
		if _, ok := seen[target.ClassID]; ok {
			panic(errPoolBudgetTargetDuplicateClass)
		}
		seen[target.ClassID] = struct{}{}
	}
}

// allocateClassBudgetTargets applies the common base-plus-adaptive allocator to
// Pool-local classes.
func allocateClassBudgetTargets(
	generation Generation,
	retainedBytes Size,
	inputs []classBudgetAllocationInput,
) []ClassBudgetTarget {
	allocationInputs := make([]budgetAllocationInput, len(inputs))
	for index, input := range inputs {
		allocationInputs[index] = budgetAllocationInput{
			BaseBytes: input.BaseTargetBytes.Bytes(),
			MinBytes:  input.MinTargetBytes.Bytes(),
			MaxBytes:  input.MaxTargetBytes.Bytes(),
			Score:     input.Score,
		}
	}

	results := allocateBudgetTargets(retainedBytes.Bytes(), allocationInputs)
	targets := make([]ClassBudgetTarget, len(inputs))
	for index, result := range results {
		targets[index] = ClassBudgetTarget{
			Generation:  generation,
			ClassID:     inputs[index].ClassID,
			TargetBytes: SizeFromBytes(result.AssignedBytes),
		}
	}

	return targets
}
