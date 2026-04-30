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

// classBudgetAllocationReport describes Pool-to-class target feasibility.
type classBudgetAllocationReport struct {
	// Targets are deterministic class targets derived from the allocation.
	Targets []ClassBudgetTarget

	// Allocation reports whether child minimums fit the parent Pool target.
	Allocation budgetAllocationReport
}

// PoolClassBudgetPublicationReport describes one Pool-to-class budget
// publication attempt.
//
// Published is true only after the complete class target batch has validated
// and been applied. Infeasible default class allocations or malformed explicit
// target batches are reported before mutation, preserving all-or-nothing Pool
// budget publication.
type PoolClassBudgetPublicationReport struct {
	// PoolName is filled by partition callers that know the partition-local name.
	PoolName string

	// Generation is the intended class budget publication generation.
	Generation Generation

	// Allocation summarizes Pool-to-class feasibility.
	Allocation BudgetAllocationDiagnostics

	// Targets are the class targets considered for publication.
	Targets []ClassBudgetTarget

	// Published reports whether the class budget batch was applied.
	Published bool

	// FailureReason is empty on success and stable diagnostic text otherwise.
	FailureReason string
}

// applyPoolBudget publishes a Pool-level budget target into class budgets.
//
// When target.ClassTargets is empty, the Pool computes class targets from
// target.RetainedBytes using static class caps and equal zero-score distribution.
// The method does not trim over-target retained storage and is never called from
// Pool.Get or Pool.Put.
func (p *Pool) applyPoolBudget(target PoolBudgetTarget) (Generation, error) {
	p.mustBeInitialized()

	report, err := p.planPoolBudget(target)
	if err != nil {
		return p.currentRuntimeSnapshot().Generation, err
	}
	if !report.Allocation.Feasible {
		return p.currentRuntimeSnapshot().Generation, newError(ErrInvalidPolicy, report.Allocation.Reason)
	}

	if err := p.beginPoolControlOperation(); err != nil {
		return p.currentRuntimeSnapshot().Generation, err
	}
	defer p.endOperation()

	p.controlMu.Lock()
	defer p.controlMu.Unlock()

	return p.applyPlannedClassBudgetTargetsAndPublishLocked(report.Targets), nil
}

// applyClassBudgets publishes class targets and derived shard credits.
//
// The method validates the complete target batch before applying any class so a
// malformed publication cannot partially update a Pool. It returns the latest
// Pool runtime generation published for the batch.
func (p *Pool) applyClassBudgets(targets []ClassBudgetTarget) (Generation, error) {
	p.mustBeInitialized()

	if len(targets) == 0 {
		return p.currentRuntimeSnapshot().Generation, nil
	}

	if err := p.validateClassBudgetTargets(targets); err != nil {
		return p.currentRuntimeSnapshot().Generation, err
	}

	if err := p.beginPoolControlOperation(); err != nil {
		return p.currentRuntimeSnapshot().Generation, err
	}
	defer p.endOperation()

	p.controlMu.Lock()
	defer p.controlMu.Unlock()

	return p.applyPlannedClassBudgetTargetsAndPublishLocked(targets), nil
}

// applyPlannedClassBudgetTargets applies a prevalidated class target batch and
// republishes the current runtime policy view with the resulting generation.
//
// Callers must already hold a Pool control operation admitted through
// beginPoolControlOperation. The method takes controlMu because class-budget
// publication changes the same immutable runtime snapshot pointer as pressure
// and live policy publication. The class table is immutable, and planning has
// already checked target identity and duplicate class IDs, so this apply phase
// cannot return policy errors after earlier classes were mutated. Pool.Get and
// Pool.Put never call this method.
func (p *Pool) applyPlannedClassBudgetTargets(targets []ClassBudgetTarget) Generation {
	p.controlMu.Lock()
	defer p.controlMu.Unlock()

	return p.applyPlannedClassBudgetTargetsAndPublishLocked(targets)
}

// applyPlannedClassBudgetTargetsAndPublishLocked applies targets while controlMu
// is already held and republishes the current runtime policy snapshot.
func (p *Pool) applyPlannedClassBudgetTargetsAndPublishLocked(targets []ClassBudgetTarget) Generation {
	generation := p.applyPlannedClassBudgetTargetsLocked(targets)
	runtime := p.currentRuntimeSnapshot()
	poolGeneration := budgetPublicationGeneration(runtime.Generation, generation)
	p.publishRuntimeSnapshot(newPoolRuntimeSnapshotWithPressure(poolGeneration, runtime.Policy, runtime.Pressure))
	return poolGeneration
}

// applyPlannedClassBudgetTargetsLocked applies prevalidated targets without
// publishing a Pool runtime snapshot.
//
// Live policy publication uses this helper to apply candidate-policy class
// budgets before atomically publishing the candidate runtime snapshot. Other
// control paths call applyPlannedClassBudgetTargetsAndPublishLocked so budget
// changes remain visible through the Pool runtime generation.
func (p *Pool) applyPlannedClassBudgetTargetsLocked(targets []ClassBudgetTarget) Generation {
	var generation Generation
	for _, target := range targets {
		state := &p.classes[target.ClassID.Index()]
		targetGeneration := budgetPublicationGeneration(state.budgetSnapshot().Generation, target.Generation)
		classGeneration := state.updateBudgetAtLeast(target.TargetBytes, targetGeneration)
		if generation.Before(classGeneration) {
			generation = classGeneration
		}
	}

	return generation
}

// defaultClassBudgetTargets computes class targets for a Pool-level retained
// target when the caller did not provide explicit class targets.
func (p *Pool) defaultClassBudgetTargets(generation Generation, retainedBytes Size) []ClassBudgetTarget {
	return p.defaultClassBudgetTargetsReport(generation, retainedBytes).Targets
}

// defaultClassBudgetTargetsReport computes default class targets and keeps the
// allocation feasibility report for applied budget publication.
func (p *Pool) defaultClassBudgetTargetsReport(generation Generation, retainedBytes Size) classBudgetAllocationReport {
	return p.defaultClassBudgetTargetsReportForPolicy(generation, retainedBytes, p.currentRuntimeSnapshot().Policy)
}

// defaultClassBudgetTargetsReportForPolicy computes class targets using policy
// instead of the currently published runtime policy.
//
// Pool.PublishPolicy uses this during the planning phase so rejected candidate
// policies do not mutate runtime snapshots before budget feasibility is known.
func (p *Pool) defaultClassBudgetTargetsReportForPolicy(generation Generation, retainedBytes Size, policy Policy) classBudgetAllocationReport {
	inputs := make([]classBudgetAllocationInput, len(p.classes))
	for index := range p.classes {
		inputs[index] = classBudgetAllocationInput{
			ClassID:        p.classes[index].classID(),
			MaxTargetBytes: SizeFromBytes(poolClassStaticBudgetCap(policy, p.classes[index].classSize())),
		}
	}

	return allocateClassBudgetTargetsReport(generation, retainedBytes, inputs)
}

// validatePoolBudgetTarget validates the full class target batch implied by one
// Pool target before publication.
func (p *Pool) validatePoolBudgetTarget(target PoolBudgetTarget) error {
	report, err := p.planPoolBudget(target)
	if err != nil {
		return err
	}
	if !report.Allocation.Feasible {
		return newError(ErrInvalidPolicy, report.Allocation.Reason)
	}
	return nil
}

// planPoolBudget validates a PoolBudgetTarget and returns the full class target
// batch without mutating Pool runtime state.
func (p *Pool) planPoolBudget(target PoolBudgetTarget) (classBudgetAllocationReport, error) {
	return p.planPoolBudgetForPolicy(target, p.currentRuntimeSnapshot().Policy)
}

// planPoolBudgetForPolicy validates a PoolBudgetTarget against an explicit
// policy value without mutating Pool runtime state.
func (p *Pool) planPoolBudgetForPolicy(target PoolBudgetTarget, policy Policy) (classBudgetAllocationReport, error) {
	if len(target.ClassTargets) == 0 {
		report := p.defaultClassBudgetTargetsReportForPolicy(target.Generation, target.RetainedBytes, policy)
		if err := p.validateClassBudgetTargets(report.Targets); err != nil {
			return report, err
		}
		return report, nil
	}
	if err := p.validateClassBudgetTargets(target.ClassTargets); err != nil {
		return classBudgetAllocationReport{Targets: target.ClassTargets}, err
	}
	return classBudgetAllocationReportFromTargets(target.Generation, target.RetainedBytes, target.ClassTargets), nil
}

// validateClassBudgetTargets validates class target identity before
// publication.
func (p *Pool) validateClassBudgetTargets(targets []ClassBudgetTarget) error {
	seen := make(map[ClassID]struct{}, len(targets))
	for _, target := range targets {
		index := target.ClassID.Index()
		if index < 0 || index >= len(p.classes) {
			return newError(ErrInvalidOptions, errPoolBudgetTargetClassMissing)
		}
		if p.classes[index].classID() != target.ClassID {
			return newError(ErrInvalidOptions, errPoolBudgetTargetClassMissing)
		}
		if _, ok := seen[target.ClassID]; ok {
			return newError(ErrInvalidOptions, errPoolBudgetTargetDuplicateClass)
		}
		seen[target.ClassID] = struct{}{}
	}
	return nil
}

// classBudgetAllocationReportFromTargets summarizes explicit class targets.
//
// Explicit class targets are already chosen by a higher controller phase. This
// helper preserves feasibility visibility by checking whether the explicit
// target sum exceeds the Pool-level parent target before Pool mutation starts.
func classBudgetAllocationReportFromTargets(
	generation Generation,
	retainedBytes Size,
	targets []ClassBudgetTarget,
) classBudgetAllocationReport {
	copied := append([]ClassBudgetTarget(nil), targets...)
	results := make([]budgetAllocationResult, len(copied))
	var assigned uint64
	for index := range copied {
		if copied[index].Generation.IsZero() {
			copied[index].Generation = generation
		}
		targetBytes := copied[index].TargetBytes.Bytes()
		assigned = poolSaturatingAdd(assigned, targetBytes)
		results[index] = budgetAllocationResult{AssignedBytes: targetBytes}
	}

	requested := retainedBytes.Bytes()
	if requested == 0 {
		requested = assigned
	}
	allocation := budgetAllocationReport{
		Results:        results,
		Feasible:       true,
		RequestedBytes: requested,
		AssignedBytes:  assigned,
		Reason:         budgetAllocationReasonFeasible,
	}
	if assigned > requested {
		allocation.Feasible = false
		allocation.OvercommittedBytes = assigned - requested
		allocation.Reason = budgetAllocationReasonMinimumsExceedParent
	}
	return classBudgetAllocationReport{Targets: copied, Allocation: allocation}
}

// allocateClassBudgetTargets applies the common base-plus-adaptive allocator to
// Pool-local classes.
func allocateClassBudgetTargets(
	generation Generation,
	retainedBytes Size,
	inputs []classBudgetAllocationInput,
) []ClassBudgetTarget {
	return allocateClassBudgetTargetsReport(generation, retainedBytes, inputs).Targets
}

// allocateClassBudgetTargetsReport applies the common allocator and returns
// feasibility diagnostics for hard-budget callers.
func allocateClassBudgetTargetsReport(
	generation Generation,
	retainedBytes Size,
	inputs []classBudgetAllocationInput,
) classBudgetAllocationReport {
	allocationInputs := make([]budgetAllocationInput, len(inputs))
	for index, input := range inputs {
		allocationInputs[index] = budgetAllocationInput{
			BaseBytes: input.BaseTargetBytes.Bytes(),
			MinBytes:  input.MinTargetBytes.Bytes(),
			MaxBytes:  input.MaxTargetBytes.Bytes(),
			Score:     input.Score,
		}
	}

	allocation := allocateBudgetTargetsReport(retainedBytes.Bytes(), allocationInputs)
	targets := make([]ClassBudgetTarget, len(inputs))
	for index, result := range allocation.Results {
		targets[index] = ClassBudgetTarget{
			Generation:  generation,
			ClassID:     inputs[index].ClassID,
			TargetBytes: SizeFromBytes(result.AssignedBytes),
		}
	}

	return classBudgetAllocationReport{Targets: targets, Allocation: allocation}
}
