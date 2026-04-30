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
	// partitionPolicyPublicationNoBudgetTarget reports a partition policy
	// publication that changed policy state but carried no retained budget to
	// project into owned Pools.
	partitionPolicyPublicationNoBudgetTarget = "no_budget_target"
)

// PoolPartitionPolicyPublicationResult reports one managed partition policy
// publication.
//
// PoolPartition policy publication is a managed foreground control operation.
// It is serialized with hard Close by the partition foreground gate and with
// controller ticks by controller.mu. A successful publication may store a new
// immutable partition runtime snapshot, project retained-budget contraction
// into owned Pools, apply prevalidated Pool/class budget targets, and execute
// bounded partition trim. It never rebuilds the partition registry, rebuilds
// owned Pools, calls Pool.Get or Pool.Put, calls PoolGroup, starts background
// scheduling, or forces active leases.
type PoolPartitionPolicyPublicationResult struct {
	// PreviousGeneration is the partition runtime-policy generation observed
	// before the publication attempt.
	PreviousGeneration Generation

	// Generation is the partition runtime-policy generation published by this
	// attempt. It remains NoGeneration when publication is rejected before the
	// runtime snapshot changes.
	Generation Generation

	// Published reports whether the partition accepted policy as its effective
	// runtime policy.
	Published bool

	// RuntimePublished reports whether a new immutable partition runtime
	// snapshot was stored.
	RuntimePublished bool

	// Contracted reports whether partition budget limits became more restrictive.
	//
	// Contraction restricts future retained targets and optional cleanup. It does
	// not force active leases or checked-out buffers to return.
	Contracted bool

	// TrimAttempted reports whether bounded partition trim was executed after a
	// successful retained-target contraction.
	TrimAttempted bool

	// FailureReason is empty on success and contains a stable diagnostic reason
	// on rejection.
	FailureReason string

	// Diff describes which live-updatable policy sections changed.
	Diff PolicyUpdateDiffDiagnostics

	// PreviousPolicy is the partition policy observed before publication.
	PreviousPolicy PartitionPolicy

	// Policy is the normalized candidate partition policy.
	Policy PartitionPolicy

	// BudgetPublication reports retained-budget projection into owned Pools and
	// nested class-budget publication.
	BudgetPublication PoolPartitionBudgetPublicationReport

	// TrimPlan is the bounded trim request derived from Policy when
	// TrimAttempted is true.
	TrimPlan PartitionTrimPlan

	// TrimResult reports retained-buffer cleanup. Active leases are not included
	// because trim only removes already-retained Pool storage.
	TrimResult PartitionTrimResult

	// PressurePreserved reports whether the pressure signal observed before
	// publication was preserved in the new partition runtime snapshot.
	PressurePreserved bool
}

// PublishPolicy publishes a live PoolPartition runtime policy update.
//
// The operation validates the candidate partition policy, validates that owned
// Pool runtime policies have not drifted into unsupported shape or ownership
// changes, plans Pool/class budgets before mutation, publishes the partition
// runtime snapshot only after planning succeeds, and then applies the
// prevalidated Pool budget batch. If retained targets shrink and
// TrimOnPolicyShrink is enabled, it executes one bounded partition trim cycle.
//
// PublishPolicy is intentionally cold control-plane work. It does not call
// Pool.Get or Pool.Put, does not rebuild owned Pools or the partition registry,
// does not call PoolGroup, and does not force active leases.
func (p *PoolPartition) PublishPolicy(policy PartitionPolicy) (PoolPartitionPolicyPublicationResult, error) {
	p.mustBeInitialized()

	normalized := policy.Normalize()
	initial := p.currentRuntimeSnapshot()
	result := newPoolPartitionPolicyPublicationResult(initial, normalized)
	if err := validatePartitionPolicyPublicationCandidate(normalized, &result); err != nil {
		return result, err
	}

	if err := p.beginForegroundOperation(); err != nil {
		result.FailureReason = policyUpdateFailureClosed
		return result, err
	}
	defer p.endForegroundOperation()

	p.controller.mu.Lock()
	defer p.controller.mu.Unlock()

	runtime := p.currentRuntimeSnapshot()
	result = newPoolPartitionPolicyPublicationResult(runtime, normalized)
	if err := validatePartitionPolicyPublicationCandidate(normalized, &result); err != nil {
		return result, err
	}
	if err := p.validateOwnedPoolRuntimePoliciesForPolicyPublication(&result); err != nil {
		return result, err
	}

	generation := runtime.Generation.Next()
	budgetBatch, budgetErr := p.planPartitionPolicyBudgetBatchLocked(generation, normalized)
	result.BudgetPublication = budgetBatch.report
	if budgetErr != nil {
		result.FailureReason = policyUpdateFailureInvalid
		return result, budgetErr
	}

	shouldPublishBudget := partitionPolicyHasRetainedTarget(normalized)
	if shouldPublishBudget && !budgetBatch.report.CanPublish() {
		result.FailureReason = policyUpdateFailureInfeasibleBudget
		if result.BudgetPublication.FailureReason == "" {
			result.BudgetPublication.FailureReason = policyUpdateFailureInfeasibleBudget
		}
		return result, newError(ErrInvalidPolicy, result.BudgetPublication.FailureReason)
	}

	if shouldPublishBudget {
		if err := budgetBatch.beginPoolControlOperations(); err != nil {
			result.BudgetPublication.FailureReason = err.Error()
			result.FailureReason = policyUpdateFailureClosed
			return result, err
		}
		defer budgetBatch.endPoolControlOperations()
	}

	p.publishRuntimeSnapshot(newPartitionRuntimeSnapshotWithPressure(generation, normalized, runtime.Pressure))

	result.Generation = generation
	result.Published = true
	result.RuntimePublished = true
	result.PressurePreserved = p.currentRuntimeSnapshot().Pressure == runtime.Pressure

	if shouldPublishBudget {
		result.BudgetPublication = p.applyPlannedPoolBudgetBatchLocked(&budgetBatch)
	}

	if partitionPolicyContractsRetainedTarget(runtime.Policy, normalized) &&
		normalized.Trim.Enabled &&
		normalized.Trim.TrimOnPolicyShrink {
		result.TrimPlan = newPartitionPolicyShrinkTrimPlan(normalized.Trim, runtime.Pressure.Level, len(p.registry.entries))
		if result.TrimPlan.Enabled {
			result.TrimAttempted = true
			result.TrimResult = p.executeTrimPlan(result.TrimPlan)
		}
	}

	return result, nil
}

// UpdatePolicy publishes policy and returns only the operation error.
func (p *PoolPartition) UpdatePolicy(policy PartitionPolicy) error {
	_, err := p.PublishPolicy(policy)
	return err
}

// newPoolPartitionPolicyPublicationResult builds a stable report skeleton for
// both successful and rejected partition policy publications.
func newPoolPartitionPolicyPublicationResult(
	runtime *partitionRuntimeSnapshot,
	candidate PartitionPolicy,
) PoolPartitionPolicyPublicationResult {
	var previousPolicy PartitionPolicy
	var previousGeneration Generation
	if runtime != nil {
		previousPolicy = runtime.Policy.Normalize()
		previousGeneration = runtime.Generation
	}
	diff := classifyPartitionPolicyUpdate(previousPolicy, candidate)
	return PoolPartitionPolicyPublicationResult{
		PreviousGeneration: previousGeneration,
		Contracted:         diff.RetentionContracted,
		Diff:               diff,
		PreviousPolicy:     previousPolicy,
		Policy:             candidate.Normalize(),
	}
}

// validatePartitionPolicyPublicationCandidate validates the candidate partition
// policy before any runtime mutation is allowed.
func validatePartitionPolicyPublicationCandidate(
	policy PartitionPolicy,
	result *PoolPartitionPolicyPublicationResult,
) error {
	if err := policy.Validate(); err != nil {
		result.FailureReason = policyUpdateFailureInvalid
		return err
	}
	return nil
}

// validateOwnedPoolRuntimePoliciesForPolicyPublication verifies that every
// owned Pool still has a runtime policy that can be controlled by this
// partition.
//
// Partition policy publication does not carry Pool construction shape. If a
// Pool runtime snapshot somehow contains a shape or ownership transition from
// its construction policy, the partition rejects publication before projecting
// budgets. That preserves the model boundary: PoolPartition may publish
// retained-budget targets into already-built Pools, but it may not rebuild Pool
// classes, shards, selectors, buckets, or lease accounting.
func (p *PoolPartition) validateOwnedPoolRuntimePoliciesForPolicyPublication(result *PoolPartitionPolicyPublicationResult) error {
	for _, entry := range p.registry.entries {
		runtime := entry.pool.currentRuntimeSnapshot()
		compatibility := checkLivePolicyUpdateCompatibility(
			entry.pool.constructionPolicy,
			runtime.Policy,
			poolConstructionModePartitionOwned,
		)
		if compatibility.Compatible {
			continue
		}

		result.Diff.ShapeChanged = result.Diff.ShapeChanged || compatibility.Diff.Diagnostics.ShapeChanged
		result.Diff.OwnershipChanged = result.Diff.OwnershipChanged || compatibility.Diff.Diagnostics.OwnershipChanged
		reason := compatibility.FailureReason
		if reason == "" {
			reason = policyUpdateFailureInvalid
		}
		result.FailureReason = reason
		return newError(ErrInvalidPolicy, reason)
	}
	return nil
}

// planPartitionPolicyBudgetBatchLocked computes a full retained-budget
// publication batch from the candidate partition policy without mutating Pools.
func (p *PoolPartition) planPartitionPolicyBudgetBatchLocked(
	generation Generation,
	policy PartitionPolicy,
) (plannedPoolBudgetBatch, error) {
	report := PoolPartitionBudgetPublicationReport{
		Generation: generation,
		Allocation: BudgetAllocationDiagnostics{
			Feasible:    true,
			Reason:      budgetAllocationReasonFeasible,
			TargetCount: len(p.registry.entries),
		},
		FailureReason: partitionPolicyPublicationNoBudgetTarget,
	}
	batch := plannedPoolBudgetBatch{
		plans:  make([]plannedPoolBudget, 0, len(p.registry.entries)),
		report: report,
	}
	if !partitionPolicyHasRetainedTarget(policy) {
		return batch, nil
	}

	inputs := make([]poolBudgetAllocationInput, 0, len(p.registry.entries))
	for _, entry := range p.registry.entries {
		inputs = append(inputs, poolBudgetAllocationInput{
			PoolName:          entry.name,
			BaseRetainedBytes: SizeFromBytes(poolCurrentBudgetAssignedBytes(entry.pool)),
			MinRetainedBytes:  SizeFromBytes(poolMinimumRetainedBudget(entry.pool)),
			MaxRetainedBytes:  SizeFromBytes(poolControllerRetainedBudget(entry.pool)),
		})
	}

	allocation := allocatePoolBudgetTargetsReport(generation, policy.Budget.MaxRetainedBytes, inputs)
	batch.report.Allocation = newBudgetAllocationDiagnostics(allocation.Allocation)
	batch.report.Targets = append([]PoolBudgetTarget(nil), allocation.Targets...)
	batch.report.FailureReason = ""
	if !allocation.Allocation.Feasible {
		batch.report.FailureReason = allocation.Allocation.Reason
		return batch, nil
	}

	planned, err := p.planPoolBudgetTargetsLocked(allocation.Targets)
	planned.report.Generation = generation
	planned.report.Allocation = newBudgetAllocationDiagnostics(allocation.Allocation)
	if planned.report.FailureReason == "" {
		planned.report.FailureReason = batch.report.FailureReason
	}
	return planned, err
}

// poolCurrentBudgetAssignedBytes returns the current Pool-to-class assignment
// total for budget planning.
func poolCurrentBudgetAssignedBytes(pool *Pool) uint64 {
	var total uint64
	for index := range pool.classes {
		total = poolSaturatingAdd(total, pool.classes[index].budgetSnapshot().AssignedBytes)
	}
	return total
}

// poolMinimumRetainedBudget returns the smallest meaningful positive retained
// assignment for an owned Pool during partition policy publication.
//
// A zero partition retained target means "do not publish Pool budgets". Once a
// positive parent target is configured, the publication model gives every owned
// Pool enough target to retain at least one smallest-class buffer. If the parent
// budget cannot fund those minimums, publication reports infeasibility instead
// of silently overcommitting the hard retained target.
func poolMinimumRetainedBudget(pool *Pool) uint64 {
	if len(pool.classes) == 0 {
		return 0
	}
	return pool.classes[0].classSize().Bytes()
}

// minimumRetainedBudget returns the smallest meaningful positive retained
// assignment for all Pools owned by the partition.
//
// PoolGroup uses this partition-owned view when it validates group-to-partition
// retained-budget feasibility. The group still does not inspect Pool shards or
// Pool buckets; the partition boundary summarizes the minimum budget needed for
// its owned Pools to receive one smallest-class retained buffer each.
func (p *PoolPartition) minimumRetainedBudget() uint64 {
	var total uint64
	for _, entry := range p.registry.entries {
		total = poolSaturatingAdd(total, poolMinimumRetainedBudget(entry.pool))
	}
	return total
}

// classifyPartitionPolicyUpdate maps partition policy differences into the
// shared policy publication diagnostics used by Pool and hierarchy reports.
func classifyPartitionPolicyUpdate(previous PartitionPolicy, next PartitionPolicy) PolicyUpdateDiffDiagnostics {
	previous = previous.Normalize()
	next = next.Normalize()
	diagnostics := PolicyUpdateDiffDiagnostics{
		RetentionChanged:    previous.Budget != next.Budget,
		RetentionContracted: partitionBudgetContracts(previous.Budget, next.Budget),
		RetentionExpanded:   partitionBudgetExpands(previous.Budget, next.Budget),
		PressureChanged:     previous.Pressure != next.Pressure,
		TrimChanged:         previous.Trim != next.Trim,
	}
	diagnostics.NeedsTrim = partitionPolicyContractsRetainedTarget(previous, next) &&
		next.Trim.Enabled &&
		next.Trim.TrimOnPolicyShrink
	return diagnostics
}

func partitionPolicyHasRetainedTarget(policy PartitionPolicy) bool {
	return !policy.Budget.MaxRetainedBytes.IsZero()
}

func partitionPolicyContractsRetainedTarget(previous PartitionPolicy, next PartitionPolicy) bool {
	return partitionSizeLimitDecreased(previous.Budget.MaxRetainedBytes, next.Budget.MaxRetainedBytes)
}

func partitionBudgetContracts(previous PartitionBudgetPolicy, next PartitionBudgetPolicy) bool {
	return partitionSizeLimitDecreased(previous.MaxRetainedBytes, next.MaxRetainedBytes) ||
		partitionSizeLimitDecreased(previous.MaxActiveBytes, next.MaxActiveBytes) ||
		partitionSizeLimitDecreased(previous.MaxOwnedBytes, next.MaxOwnedBytes)
}

func partitionBudgetExpands(previous PartitionBudgetPolicy, next PartitionBudgetPolicy) bool {
	return partitionSizeLimitIncreased(previous.MaxRetainedBytes, next.MaxRetainedBytes) ||
		partitionSizeLimitIncreased(previous.MaxActiveBytes, next.MaxActiveBytes) ||
		partitionSizeLimitIncreased(previous.MaxOwnedBytes, next.MaxOwnedBytes)
}

// partitionSizeLimitDecreased compares Size limits where zero means "unbounded".
func partitionSizeLimitDecreased(previous Size, next Size) bool {
	if previous.IsZero() {
		return !next.IsZero()
	}
	if next.IsZero() {
		return false
	}
	return next < previous
}

// partitionSizeLimitIncreased compares Size limits where zero means
// "unbounded".
func partitionSizeLimitIncreased(previous Size, next Size) bool {
	if previous.IsZero() {
		return false
	}
	if next.IsZero() {
		return true
	}
	return next > previous
}
