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
	// groupPolicyPublicationNoBudgetTarget reports a group policy publication
	// that changed group policy state but carried no retained parent target to
	// publish into child partitions.
	groupPolicyPublicationNoBudgetTarget = "no_budget_target"
)

// PoolGroupPolicyPublicationResult reports one group-level live policy
// publication.
//
// PoolGroup policy publication is a foreground group control operation. It is
// serialized with hard Close by runtimeMu, preserves the current pressure
// signal, may publish a new immutable group runtime policy snapshot, and may
// publish retained-budget targets into owned PoolPartitions. It never scans Pool
// shards, never executes Pool trim, never calls Pool.Get or Pool.Put, never
// computes class EWMA, and never starts background work.
type PoolGroupPolicyPublicationResult struct {
	// PreviousGeneration is the group runtime-policy generation observed before
	// the publication attempt.
	PreviousGeneration Generation

	// Generation is the group runtime-policy generation published by this
	// attempt. It remains NoGeneration when validation, feasibility, lifecycle,
	// or child precheck rejects the publication before the runtime snapshot
	// changes.
	Generation Generation

	// Published reports whether the group accepted the candidate policy and all
	// required child partition budget targets were accepted.
	Published bool

	// RuntimePublished reports whether a new immutable group runtime snapshot was
	// stored.
	RuntimePublished bool

	// Contracted reports whether group retained-budget limits became more
	// restrictive.
	//
	// Contraction lowers future partition targets only. It does not scan shards,
	// trim Pool storage, or force active leases.
	Contracted bool

	// FailureReason is empty on success and contains stable diagnostic text on
	// rejection or observable partial publication.
	FailureReason string

	// Diff describes which group policy sections changed.
	Diff PolicyUpdateDiffDiagnostics

	// PreviousPolicy is the normalized group policy observed before publication.
	PreviousPolicy PoolGroupPolicy

	// Policy is the normalized candidate group policy.
	Policy PoolGroupPolicy

	// BudgetPublication reports group-to-partition retained-budget feasibility
	// and publication status.
	BudgetPublication PoolGroupBudgetPublicationReport

	// SkippedPartitions records child partitions that could not accept target
	// publication.
	SkippedPartitions []PoolGroupSkippedPartition

	// PressurePreserved reports whether the pressure signal observed before
	// publication was preserved in the new group runtime snapshot.
	PressurePreserved bool
}

// PublishPolicy publishes a live PoolGroup runtime policy update.
//
// The operation validates the candidate policy, plans group-to-partition budget
// targets when a retained parent target exists, prevalidates and admits child
// partitions before storing group runtime state, publishes the group runtime
// snapshot with the previous pressure signal, and then applies the prevalidated
// partition targets. UpdatePolicy is the error-only compatibility wrapper.
func (g *PoolGroup) PublishPolicy(policy PoolGroupPolicy) (PoolGroupPolicyPublicationResult, error) {
	g.mustBeInitialized()

	normalized := policy.Normalize()
	initial := g.currentRuntimeSnapshot()
	result := newPoolGroupPolicyPublicationResult(initial, normalized)
	if err := validateGroupPolicyPublicationCandidate(normalized, &result); err != nil {
		return result, err
	}

	g.runtimeMu.Lock()
	defer g.runtimeMu.Unlock()

	if !g.lifecycle.AllowsWork() {
		result.FailureReason = policyUpdateFailureClosed
		return result, newError(ErrClosed, errGroupClosed)
	}

	runtime := g.currentRuntimeSnapshot()
	result = newPoolGroupPolicyPublicationResult(runtime, normalized)
	if err := validateGroupPolicyPublicationCandidate(normalized, &result); err != nil {
		return result, err
	}

	generation := g.generation.Load().Next()
	budgetBatch, budgetErr := g.planGroupPolicyBudgetPublicationBatchLocked(generation, normalized)
	result.BudgetPublication = budgetBatch.report
	if budgetErr != nil {
		result.FailureReason = policyUpdateFailureInvalid
		return result, budgetErr
	}

	shouldPublishBudget := groupPolicyHasRetainedTarget(normalized)
	if shouldPublishBudget && !budgetBatch.report.CanPublish() {
		if !budgetBatch.report.Allocation.Feasible {
			result.FailureReason = policyUpdateFailureInfeasibleBudget
			if result.BudgetPublication.FailureReason == "" {
				result.BudgetPublication.FailureReason = policyUpdateFailureInfeasibleBudget
			}
			return result, newError(ErrInvalidPolicy, result.BudgetPublication.FailureReason)
		}
		if len(budgetBatch.report.SkippedPartitions) > 0 {
			result.SkippedPartitions = append(result.SkippedPartitions[:0], budgetBatch.report.SkippedPartitions...)
			result.FailureReason = policyUpdateFailureSkippedChild
			return result, newError(ErrClosed, policyUpdateFailureSkippedChild)
		}
		result.FailureReason = policyUpdateFailureInvalid
		if result.BudgetPublication.FailureReason == "" {
			result.BudgetPublication.FailureReason = policyUpdateFailureInvalid
		}
		return result, newError(ErrInvalidPolicy, result.BudgetPublication.FailureReason)
	}

	if shouldPublishBudget {
		if err := budgetBatch.beginPartitionForegroundOperations(); err != nil {
			result.SkippedPartitions = append(result.SkippedPartitions[:0], budgetBatch.report.SkippedPartitions...)
			result.BudgetPublication = budgetBatch.report
			result.FailureReason = policyUpdateFailureSkippedChild
			return result, err
		}
		defer budgetBatch.endPartitionForegroundOperations()
	}

	g.generation.Store(generation)
	g.publishRuntimeSnapshot(newGroupRuntimeSnapshotWithPressure(generation, normalized, runtime.Pressure))

	result.Generation = generation
	result.RuntimePublished = true
	result.PressurePreserved = g.currentRuntimeSnapshot().Pressure == runtime.Pressure

	if shouldPublishBudget {
		result.BudgetPublication = g.applyPlannedPartitionBudgetBatchLocked(&budgetBatch)
		result.SkippedPartitions = append(result.SkippedPartitions[:0], result.BudgetPublication.SkippedPartitions...)
		if !result.BudgetPublication.Published {
			result.FailureReason = policyUpdateFailureSkippedChild
			return result, newError(ErrClosed, policyUpdateFailureSkippedChild)
		}
	} else {
		result.BudgetPublication = budgetBatch.report
	}

	result.Published = true
	return result, nil
}

// UpdatePolicy publishes policy and returns only the operation error.
func (g *PoolGroup) UpdatePolicy(policy PoolGroupPolicy) error {
	_, err := g.PublishPolicy(policy)
	return err
}

// newPoolGroupPolicyPublicationResult builds a stable report skeleton shared by
// successful and rejected publication paths.
func newPoolGroupPolicyPublicationResult(
	runtime *groupRuntimeSnapshot,
	candidate PoolGroupPolicy,
) PoolGroupPolicyPublicationResult {
	var previousPolicy PoolGroupPolicy
	var previousGeneration Generation
	if runtime != nil {
		previousPolicy = runtime.Policy.Normalize()
		previousGeneration = runtime.Generation
	}
	diff := classifyGroupPolicyUpdate(previousPolicy, candidate)
	return PoolGroupPolicyPublicationResult{
		PreviousGeneration: previousGeneration,
		Contracted:         diff.RetentionContracted,
		Diff:               diff,
		PreviousPolicy:     previousPolicy,
		Policy:             candidate.Normalize(),
	}
}

// validateGroupPolicyPublicationCandidate validates the candidate before any
// group runtime mutation is allowed.
func validateGroupPolicyPublicationCandidate(
	policy PoolGroupPolicy,
	result *PoolGroupPolicyPublicationResult,
) error {
	if err := policy.Validate(); err != nil {
		result.FailureReason = policyUpdateFailureInvalid
		return err
	}
	return nil
}

// groupPolicyPartitionBudgetBatch is the all-or-observable group publication
// unit.
//
// Planning validates every target partition and every derived partition policy
// before the group runtime snapshot is published. Admission then holds each
// partition foreground gate until targets are applied, so direct partition Close
// cannot slip between group runtime publication and child target mutation.
type groupPolicyPartitionBudgetBatch struct {
	plans  []groupPolicyPartitionBudgetPlan
	report PoolGroupBudgetPublicationReport
}

// groupPolicyPartitionBudgetPlan is one prevalidated partition target.
type groupPolicyPartitionBudgetPlan struct {
	target    PartitionBudgetTarget
	partition *PoolPartition
	admitted  bool
}

// planGroupPolicyBudgetPublicationBatchLocked computes and prevalidates a
// group-to-partition retained-budget batch without mutating group or partition
// runtime state.
func (g *PoolGroup) planGroupPolicyBudgetPublicationBatchLocked(
	generation Generation,
	policy PoolGroupPolicy,
) (groupPolicyPartitionBudgetBatch, error) {
	report := PoolGroupBudgetPublicationReport{
		Generation: generation,
		Allocation: BudgetAllocationDiagnostics{
			Feasible:    true,
			Reason:      budgetAllocationReasonFeasible,
			TargetCount: g.registry.len(),
		},
		FailureReason: groupPolicyPublicationNoBudgetTarget,
	}
	batch := groupPolicyPartitionBudgetBatch{
		plans:  make([]groupPolicyPartitionBudgetPlan, 0, g.registry.len()),
		report: report,
	}
	if !groupPolicyHasRetainedTarget(policy) {
		return batch, nil
	}

	inputs := make([]partitionBudgetAllocationInput, 0, g.registry.len())
	for _, entry := range g.registry.entries {
		inputs = append(inputs, partitionBudgetAllocationInput{
			PartitionName:     entry.name,
			BaseRetainedBytes: entry.partition.currentRuntimeSnapshot().Policy.Budget.MaxRetainedBytes,
			MinRetainedBytes:  SizeFromBytes(entry.partition.minimumRetainedBudget()),
		})
	}

	allocation := g.computePartitionBudgetTargetsReport(generation, policy.Budget.MaxRetainedBytes, inputs)
	batch.report.Allocation = newBudgetAllocationDiagnostics(allocation.Allocation)
	batch.report.Targets = append([]PartitionBudgetTarget(nil), allocation.Targets...)
	batch.report.FailureReason = ""
	if !allocation.Allocation.Feasible {
		batch.report.FailureReason = allocation.Allocation.Reason
		return batch, nil
	}

	for _, target := range allocation.Targets {
		partition, ok := g.registry.partition(target.PartitionName)
		if !ok {
			return batch, newError(ErrInvalidOptions, errGroupPartitionMissing+": "+target.PartitionName)
		}
		if !partition.lifecycle.AllowsWork() {
			skipped := PoolGroupSkippedPartition{PartitionName: target.PartitionName, Reason: errPartitionClosed}
			batch.report.SkippedPartitions = append(batch.report.SkippedPartitions, skipped)
			batch.report.FailureReason = policyUpdateFailureSkippedChild
			continue
		}
		if err := partition.validatePartitionBudgetTarget(target); err != nil {
			return batch, err
		}
		batch.plans = append(batch.plans, groupPolicyPartitionBudgetPlan{target: target, partition: partition})
	}
	if len(batch.report.SkippedPartitions) > 0 {
		return batch, nil
	}
	return batch, nil
}

// beginPartitionForegroundOperations admits every target partition before group
// runtime state is published.
func (b *groupPolicyPartitionBudgetBatch) beginPartitionForegroundOperations() error {
	for index := range b.plans {
		plan := &b.plans[index]
		if err := plan.partition.beginForegroundOperation(); err != nil {
			for admitted := 0; admitted < index; admitted++ {
				if b.plans[admitted].admitted {
					b.plans[admitted].partition.endForegroundOperation()
					b.plans[admitted].admitted = false
				}
			}
			b.report.SkippedPartitions = append(b.report.SkippedPartitions, PoolGroupSkippedPartition{
				PartitionName: plan.target.PartitionName,
				Reason:        err.Error(),
			})
			b.report.FailureReason = policyUpdateFailureSkippedChild
			return err
		}
		plan.admitted = true
	}
	return nil
}

// endPartitionForegroundOperations releases partition operation gates admitted
// for a budget batch.
func (b *groupPolicyPartitionBudgetBatch) endPartitionForegroundOperations() {
	for index := range b.plans {
		if !b.plans[index].admitted {
			continue
		}
		b.plans[index].partition.endForegroundOperation()
		b.plans[index].admitted = false
	}
}

// applyPlannedPartitionBudgetBatchLocked applies already prevalidated partition
// targets. It does not call Pool.Get, Pool.Put, Pool.Trim, or inspect Pool
// shards.
func (g *PoolGroup) applyPlannedPartitionBudgetBatchLocked(
	batch *groupPolicyPartitionBudgetBatch,
) PoolGroupBudgetPublicationReport {
	for _, plan := range batch.plans {
		if err := plan.partition.applyPartitionBudgetLocked(plan.target); err != nil {
			batch.report.SkippedPartitions = append(batch.report.SkippedPartitions, PoolGroupSkippedPartition{
				PartitionName: plan.target.PartitionName,
				Reason:        err.Error(),
			})
			batch.report.FailureReason = policyUpdateFailureSkippedChild
			batch.report.Published = false
			return batch.report
		}
	}
	batch.report.Published = len(batch.plans) > 0
	return batch.report
}

// classifyGroupPolicyUpdate maps group policy differences into shared
// publication diagnostics.
func classifyGroupPolicyUpdate(previous PoolGroupPolicy, next PoolGroupPolicy) PolicyUpdateDiffDiagnostics {
	previous = previous.Normalize()
	next = next.Normalize()
	return PolicyUpdateDiffDiagnostics{
		RetentionChanged:    previous.Budget != next.Budget,
		RetentionContracted: partitionBudgetContracts(previous.Budget, next.Budget),
		RetentionExpanded:   partitionBudgetExpands(previous.Budget, next.Budget),
		PressureChanged:     previous.Pressure != next.Pressure,
	}
}

func groupPolicyHasRetainedTarget(policy PoolGroupPolicy) bool {
	return !policy.Budget.MaxRetainedBytes.IsZero()
}
