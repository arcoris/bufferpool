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

// poolBudgetAllocationReport describes partition-to-Pool target feasibility.
type poolBudgetAllocationReport struct {
	// Targets are deterministic Pool targets derived from the allocation.
	Targets []PoolBudgetTarget

	// Allocation reports whether child minimums fit the parent partition target.
	Allocation budgetAllocationReport
}

// PoolPartitionBudgetPublicationReport describes one partition-to-Pool budget
// publication attempt.
//
// Published is true only after every Pool target and every nested class target
// has validated and accepted publication. Infeasible hard-budget allocations
// leave Published=false and do not mutate earlier Pools.
type PoolPartitionBudgetPublicationReport struct {
	// Generation is the intended Pool budget publication generation.
	Generation Generation

	// Allocation summarizes partition-to-Pool feasibility.
	Allocation BudgetAllocationDiagnostics

	// Targets are the Pool targets considered for publication.
	Targets []PoolBudgetTarget

	// ClassReports summarize each Pool-to-class publication plan.
	ClassReports []PoolClassBudgetPublicationReport

	// Published reports whether all Pool and class targets were applied.
	Published bool

	// FailureReason is empty on success and stable diagnostic text otherwise.
	FailureReason string
}

// CanPublish reports whether this publication report is both feasible and free
// of child class failures.
func (r PoolPartitionBudgetPublicationReport) CanPublish() bool {
	if len(r.Targets) == 0 {
		return false
	}
	if !r.Allocation.Feasible {
		return false
	}
	for _, classReport := range r.ClassReports {
		if !classReport.Allocation.Feasible || classReport.FailureReason != "" {
			return false
		}
	}
	return true
}

// applyPartitionBudget publishes the retained target into the partition policy
// stream.
//
// This method updates only partition runtime policy. It does not derive Pool
// budgets, publish class targets, execute trim, or start controller work.
func (p *PoolPartition) applyPartitionBudget(target PartitionBudgetTarget) error {
	p.mustBeInitialized()
	if err := p.beginForegroundOperation(); err != nil {
		return err
	}
	defer p.endForegroundOperation()

	return p.applyPartitionBudgetLocked(target)
}

// applyPartitionBudgetLocked publishes a partition target while the foreground
// gate is already held.
func (p *PoolPartition) applyPartitionBudgetLocked(target PartitionBudgetTarget) error {
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
	if err := p.beginForegroundOperation(); err != nil {
		return err
	}
	defer p.endForegroundOperation()

	report, err := p.applyPoolBudgetTargetsLocked(targets)
	if err != nil {
		return err
	}
	if !report.Published {
		return newError(ErrInvalidPolicy, report.FailureReason)
	}
	return nil
}

// applyPoolBudgetTargetsLocked validates and publishes Pool targets while the
// partition foreground gate is already held.
func (p *PoolPartition) applyPoolBudgetTargetsLocked(targets []PoolBudgetTarget) (PoolPartitionBudgetPublicationReport, error) {
	if !p.lifecycle.AllowsWork() {
		return PoolPartitionBudgetPublicationReport{}, newError(ErrClosed, errPartitionClosed)
	}

	report := PoolPartitionBudgetPublicationReport{
		Targets: append([]PoolBudgetTarget(nil), targets...),
		Allocation: BudgetAllocationDiagnostics{
			Feasible:    true,
			Reason:      budgetAllocationReasonFeasible,
			TargetCount: len(targets),
		},
	}
	if len(targets) > 0 {
		report.Generation = targets[0].Generation
	}
	prepared := make([]struct {
		target PoolBudgetTarget
		pool   *Pool
		index  int
	}, 0, len(targets))
	for _, target := range targets {
		pool, ok := p.registry.pool(target.PoolName)
		if !ok {
			return report, newError(ErrInvalidOptions, errPartitionPoolMissing+": "+target.PoolName)
		}
		classAllocation, err := pool.planPoolBudget(target)
		classReport := PoolClassBudgetPublicationReport{
			PoolName:   target.PoolName,
			Generation: target.Generation,
			Allocation: newBudgetAllocationDiagnostics(classAllocation.Allocation),
			Targets:    classAllocation.Targets,
			Published:  false,
		}
		if err != nil {
			classReport.FailureReason = err.Error()
			report.ClassReports = append(report.ClassReports, classReport)
			return report, err
		}
		if !classAllocation.Allocation.Feasible {
			classReport.FailureReason = classAllocation.Allocation.Reason
			report.ClassReports = append(report.ClassReports, classReport)
			report.FailureReason = classAllocation.Allocation.Reason
			return report, nil
		}
		report.ClassReports = append(report.ClassReports, classReport)
		index, _ := p.registry.poolIndex(target.PoolName)
		target.ClassTargets = classAllocation.Targets
		prepared = append(prepared, struct {
			target PoolBudgetTarget
			pool   *Pool
			index  int
		}{target: target, pool: pool, index: index})
	}

	for _, item := range prepared {
		if _, err := item.pool.applyPoolBudget(item.target); err != nil {
			report.FailureReason = err.Error()
			return report, err
		}
		for index := range report.ClassReports {
			if report.ClassReports[index].PoolName == item.target.PoolName {
				report.ClassReports[index].Published = true
			}
		}
		if item.index >= 0 {
			_ = p.activeRegistry.markDirtyIndex(item.index)
		}
	}

	report.Targets = append(report.Targets[:0], targets...)
	report.Published = len(targets) > 0
	return report, nil
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
