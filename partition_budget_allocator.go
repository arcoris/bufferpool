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

	// PoolScores are typed Pool score diagnostics used to choose target weights.
	PoolScores []PoolBudgetScoreReport

	// ClassReports summarize each Pool-to-class publication plan.
	ClassReports []PoolClassBudgetPublicationReport

	// Published reports whether all Pool and class targets were applied.
	Published bool

	// FailureReason is empty on success and stable diagnostic text otherwise.
	FailureReason string
}

// plannedPoolBudget is one already validated Pool budget publication.
//
// Planning resolves the partition-local Pool, computes the exact class targets,
// and records the active-registry index before any Pool is mutated.
type plannedPoolBudget struct {
	target PoolBudgetTarget
	pool   *Pool
	index  int
}

// plannedPoolBudgetBatch is the all-or-nothing apply unit for partition budget
// publication.
//
// The partition foreground gate protects owned-Pool cleanup, while each Pool
// control operation below protects against direct Pool.Close callers. Once every
// Pool operation is admitted, applying the planned class targets cannot return
// policy errors because all target identity and feasibility checks already ran.
type plannedPoolBudgetBatch struct {
	plans  []plannedPoolBudget
	report PoolPartitionBudgetPublicationReport
}

// partitionBudgetApplicationPlan is a prevalidated partition budget
// publication prepared for PoolGroup-owned policy contraction.
//
// The plan contains the exact partition runtime policy value and the nested
// Pool/class budget batch that will be applied after the group publishes its
// runtime snapshot. Planning performs every normal validation step without
// mutation; applying the plan is expected to be no-fail for policy feasibility
// and class-target identity.
//
// PoolGroup may ask PoolPartition to build and apply this plan, but ownership
// still flows downward only. PoolPartition may admit Pool control operations;
// Pool control paths never call back into PoolPartition or PoolGroup.
type partitionBudgetApplicationPlan struct {
	target          PartitionBudgetTarget
	generation      Generation
	policy          PartitionPolicy
	poolBudgetBatch plannedPoolBudgetBatch
	report          PoolPartitionBudgetPublicationReport
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

	if err := p.validatePartitionBudgetTarget(target); err != nil {
		return err
	}

	runtime := p.currentRuntimeSnapshot()
	policy := runtime.Policy
	policy.Budget.MaxRetainedBytes = target.RetainedBytes

	generation := budgetPublicationGeneration(runtime.Generation, target.Generation)
	p.publishRuntimeSnapshot(newPartitionRuntimeSnapshotWithPressure(generation, policy, runtime.Pressure))
	return nil
}

// planPartitionBudgetApplicationLocked prepares a partition target and its
// nested Pool/class budget publication without mutating runtime state.
//
// Callers must already hold the partition foreground gate. Group publication
// also holds controller.mu while calling this helper so direct partition
// TickInto or PublishPolicy cannot change policy/budget planning inputs between
// planning and planned application.
func (p *PoolPartition) planPartitionBudgetApplicationLocked(
	target PartitionBudgetTarget,
) (partitionBudgetApplicationPlan, error) {
	runtime := p.currentRuntimeSnapshot()
	generation := budgetPublicationGeneration(runtime.Generation, target.Generation)

	policy := runtime.Policy
	policy.Budget.MaxRetainedBytes = target.RetainedBytes

	plan := partitionBudgetApplicationPlan{
		target:     target,
		generation: generation,
		policy:     policy.Normalize(),
		report: PoolPartitionBudgetPublicationReport{
			Generation: generation,
			Allocation: BudgetAllocationDiagnostics{
				Feasible: true,
				Reason:   budgetAllocationReasonFeasible,
			},
		},
	}

	// Validate partition-local lifecycle and policy compatibility before
	// deriving nested Pool/class plans. A failure here leaves both partition
	// runtime state and owned Pools untouched.
	if !p.lifecycle.AllowsWork() {
		plan.report.FailureReason = policyUpdateFailureClosed
		return plan, newError(ErrClosed, errPartitionClosed)
	}
	if err := plan.policy.Validate(); err != nil {
		plan.report.FailureReason = policyUpdateFailureInvalid
		return plan, err
	}
	compatibility := newPoolPartitionPolicyPublicationResult(runtime, plan.policy)
	if err := p.validateOwnedPoolRuntimePoliciesForPolicyPublication(&compatibility); err != nil {
		plan.report.FailureReason = compatibility.FailureReason
		if plan.report.FailureReason == "" {
			plan.report.FailureReason = policyUpdateFailureInvalid
		}
		return plan, err
	}

	// Build the nested Pool/class publication plan without mutation. The caller
	// decides whether and when the parent group runtime snapshot is published.
	batch, err := p.planPartitionPolicyBudgetBatchLocked(generation, plan.policy)
	plan.poolBudgetBatch = batch
	plan.report = batch.report
	if err != nil {
		if plan.report.FailureReason == "" {
			plan.report.FailureReason = policyUpdateFailureReasonForError(err, policyUpdateFailureInvalid)
		}
		return plan, err
	}
	return plan, nil
}

// applyPlannedPartitionBudgetApplicationLocked applies a partition plan created
// by planPartitionBudgetApplicationLocked.
//
// The caller must hold the partition foreground gate, hold controller.mu, and
// have admitted every Pool control operation in plan.poolBudgetBatch. The method
// does not allocate new targets, validate policy, or return normal policy
// errors; those concerns belong to planning before the parent group runtime
// snapshot is published.
func (p *PoolPartition) applyPlannedPartitionBudgetApplicationLocked(
	plan *partitionBudgetApplicationPlan,
) PoolPartitionBudgetPublicationReport {
	runtime := p.currentRuntimeSnapshot()
	p.publishRuntimeSnapshot(newPartitionRuntimeSnapshotWithPressure(plan.generation, plan.policy, runtime.Pressure))
	plan.report = p.applyPlannedPoolBudgetBatchLocked(&plan.poolBudgetBatch)
	return plan.report
}

// beginPlannedPartitionBudgetPoolControlOperations admits the owned Pool gates
// needed by a preplanned partition budget application.
//
// PoolGroup calls this partition-owned wrapper before publishing group runtime
// state. The wrapper keeps Pool control admission behind the PoolPartition
// boundary while still letting the group prove that successful runtime
// publication will not later fail because an owned Pool closed.
func (p *PoolPartition) beginPlannedPartitionBudgetPoolControlOperations(
	plan *partitionBudgetApplicationPlan,
) error {
	return plan.poolBudgetBatch.beginPoolControlOperations()
}

// endPlannedPartitionBudgetPoolControlOperations releases Pool gates admitted
// by beginPlannedPartitionBudgetPoolControlOperations.
func (p *PoolPartition) endPlannedPartitionBudgetPoolControlOperations(
	plan *partitionBudgetApplicationPlan,
) {
	plan.poolBudgetBatch.endPoolControlOperations()
}

// validatePartitionBudgetTarget checks whether target can become the partition
// retained budget without mutating runtime state.
func (p *PoolPartition) validatePartitionBudgetTarget(target PartitionBudgetTarget) error {
	runtime := p.currentRuntimeSnapshot()
	policy := runtime.Policy
	policy.Budget.MaxRetainedBytes = target.RetainedBytes
	return policy.Validate()
}

// applyPoolBudgetTargets publishes retained targets into partition-owned Pools.
//
// This is an internal manual publication hook for later partition controllers.
// It delegates to Pool.applyPoolBudget so Pool remains the owner of class budget
// and shard-credit publication.
func (p *PoolPartition) applyPoolBudgetTargets(targets []PoolBudgetTarget) error {
	p.mustBeInitialized()
	if len(targets) == 0 {
		return nil
	}
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
	batch, err := p.planPoolBudgetTargetsLocked(targets)
	if err != nil || !batch.report.CanPublish() {
		return batch.report, err
	}

	if err := batch.beginPoolControlOperations(); err != nil {
		batch.report.FailureReason = policyUpdateFailureReasonForError(err, policyUpdateFailureClosed)
		return batch.report, err
	}
	defer batch.endPoolControlOperations()

	return p.applyPlannedPoolBudgetBatchLocked(&batch), nil
}

// applyPlannedPoolBudgetBatchLocked applies a prevalidated partition-to-Pool
// budget batch.
//
// The caller must already hold the partition foreground gate and must have
// admitted every target Pool through beginPoolControlOperations. Under those
// preconditions the apply phase cannot return policy validation errors after a
// subset of Pools has been mutated: every Pool name, class target, and
// feasibility report was resolved during planning, and Pool.Close is blocked by
// the admitted Pool control operations.
func (p *PoolPartition) applyPlannedPoolBudgetBatchLocked(batch *plannedPoolBudgetBatch) PoolPartitionBudgetPublicationReport {
	for _, plan := range batch.plans {
		_ = plan.pool.applyPlannedClassBudgetTargets(plan.target.ClassTargets)
		for index := range batch.report.ClassReports {
			if batch.report.ClassReports[index].PoolName == plan.target.PoolName {
				batch.report.ClassReports[index].Published = true
			}
		}
		if plan.index >= 0 {
			_ = p.activeRegistry.markDirtyIndex(plan.index)
		}
	}

	batch.report.Published = len(batch.plans) > 0
	return batch.report
}

// planPoolBudgetTargetsLocked validates a full partition-to-Pool publication
// batch without mutating any Pool.
func (p *PoolPartition) planPoolBudgetTargetsLocked(targets []PoolBudgetTarget) (plannedPoolBudgetBatch, error) {
	if !p.lifecycle.AllowsWork() {
		return plannedPoolBudgetBatch{}, newError(ErrClosed, errPartitionClosed)
	}

	// Planning resolves every Pool name and class target before any Pool is
	// mutated. The report is populated as the exact batch that apply will later
	// consume.
	report := PoolPartitionBudgetPublicationReport{
		Allocation: BudgetAllocationDiagnostics{
			Feasible:    true,
			Reason:      budgetAllocationReasonFeasible,
			TargetCount: len(targets),
		},
	}
	if len(targets) > 0 {
		report.Generation = targets[0].Generation
	}

	batch := plannedPoolBudgetBatch{
		plans:  make([]plannedPoolBudget, 0, len(targets)),
		report: report,
	}

	for _, target := range targets {
		pool, ok := p.registry.pool(target.PoolName)
		if !ok {
			return batch, newError(ErrInvalidOptions, errPartitionPoolMissing+": "+target.PoolName)
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
			classReport.FailureReason = policyUpdateFailureReasonForError(err, policyUpdateFailureInvalid)
			batch.report.ClassReports = append(batch.report.ClassReports, classReport)
			return batch, err
		}

		if !classAllocation.Allocation.Feasible {
			classReport.FailureReason = classAllocation.Allocation.Reason
			batch.report.ClassReports = append(batch.report.ClassReports, classReport)
			batch.report.FailureReason = classAllocation.Allocation.Reason
			return batch, nil
		}

		batch.report.ClassReports = append(batch.report.ClassReports, classReport)

		index, _ := p.registry.poolIndex(target.PoolName)
		target.ClassTargets = classAllocation.Targets
		batch.report.Targets = append(batch.report.Targets, target)
		batch.plans = append(batch.plans, plannedPoolBudget{target: target, pool: pool, index: index})
	}

	return batch, nil
}

// beginPoolControlOperations admits every target Pool before mutation starts.
func (b *plannedPoolBudgetBatch) beginPoolControlOperations() error {
	for index, plan := range b.plans {
		if err := plan.pool.beginPoolControlOperation(); err != nil {
			for admitted := 0; admitted < index; admitted++ {
				b.plans[admitted].pool.endOperation()
			}
			return err
		}
	}
	return nil
}

// endPoolControlOperations releases Pool operation gates admitted for a batch.
func (b *plannedPoolBudgetBatch) endPoolControlOperations() {
	for _, plan := range b.plans {
		plan.pool.endOperation()
	}
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
