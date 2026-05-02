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

// PoolPolicyPublicationResult reports one Pool-local live policy publication.
//
// The result is value-shaped so callers can inspect successful, rejected, and
// partially planned outcomes without matching error strings. Pool policy
// publication is a foreground control operation: it may publish a new immutable
// runtime policy snapshot, apply class budgets and shard credits, and optionally
// execute bounded retained-buffer trim. It never rebuilds class tables, shard
// arrays, shard selectors, or bucket metadata, and it never calls PoolPartition,
// PoolGroup, controllers, EWMA code, or schedulers.
type PoolPolicyPublicationResult struct {
	// PreviousGeneration is the runtime policy generation observed before the
	// publication attempt that produced this result.
	PreviousGeneration Generation

	// Generation is the runtime generation that was published. It remains
	// NoGeneration when the update is rejected before runtime publication.
	Generation Generation

	// Published reports whether the Pool accepted the candidate policy as its
	// effective runtime policy.
	Published bool

	// RuntimePublished reports whether a new immutable Pool runtime snapshot was
	// stored.
	RuntimePublished bool

	// Contracted reports whether the candidate reduced future retention limits.
	//
	// Contraction restricts new retention and class/shard credit. It does not
	// force checked-out buffers to be reclaimed.
	Contracted bool

	// TrimAttempted reports whether bounded Pool trim was executed because the
	// policy contracted retention and TrimOnPolicyShrink was enabled.
	TrimAttempted bool

	// FailureReason is empty on success and contains a stable diagnostic reason
	// on rejection.
	FailureReason string

	// Diff describes which policy sections changed between the previously
	// published policy and the candidate.
	Diff PolicyUpdateDiffDiagnostics

	// PreviousPolicy is a defensive copy of the policy observed before the
	// publication attempt.
	PreviousPolicy Policy

	// Policy is a defensive copy of the candidate policy after Pool construction
	// defaults were applied.
	Policy Policy

	// ClassBudgetPublication describes the planned and applied Pool-to-class
	// budget publication used to update class budgets and shard credits.
	ClassBudgetPublication PoolClassBudgetPublicationReport

	// TrimPlan is the bounded trim request derived from the candidate policy when
	// TrimAttempted is true.
	TrimPlan PoolTrimPlan

	// TrimResult reports bounded physical retained-buffer removal.
	TrimResult PoolTrimResult

	// PressurePreserved reports whether the runtime pressure signal observed
	// before publication was preserved in the new runtime snapshot.
	PressurePreserved bool
}

// PublishPolicy publishes a live Pool-local runtime policy update.
//
// PublishPolicy is a control-plane operation owned by Pool. It validates that
// policy can be applied without rebuilding construction shape, enters the Pool
// lifecycle operation gate, serializes with other Pool-local control mutations,
// plans class budgets from the candidate policy, publishes class/shard credits,
// stores a new immutable runtime snapshot while preserving the current pressure
// signal, and optionally performs bounded trim after retention contraction. The
// budget-before-snapshot ordering is deliberate: Pool never exposes a new
// effective policy without the corresponding local class budgets and shard
// credits already in force.
//
// The method intentionally does not change Pool.Get or Pool.Put semantics. Hot
// paths continue to load the immutable runtime snapshot they already consume.
// PublishPolicy does not call PoolPartition, PoolGroup, controllers, EWMA, or
// background scheduling. Rejected shape, ownership, invalid-policy, closed-pool,
// or infeasible-budget updates do not publish a new runtime snapshot.
func (p *Pool) PublishPolicy(policy Policy) (PoolPolicyPublicationResult, error) {
	p.mustBeInitialized()

	// Validate the value before entering the Pool control gate; the same checks
	// are repeated under controlMu against the latest runtime snapshot.
	normalized := effectivePoolConfigPolicy(policy)
	initial := p.currentRuntimeSnapshot()
	result := newPoolPolicyPublicationResult(initial, normalized)
	if err := p.validatePoolPolicyPublicationCandidate(normalized, &result); err != nil {
		return result, err
	}

	if err := p.validatePoolPolicyPublication(initial.Policy, normalized, &result); err != nil {
		return result, err
	}

	// Pool-local publication is serialized with Close and other control-plane
	// mutations, but it does not add locks to Pool.Get or Pool.Put.
	if err := p.beginPoolControlOperation(); err != nil {
		result.FailureReason = policyUpdateFailureClosed
		return result, err
	}
	defer p.endOperation()

	p.controlMu.Lock()
	defer p.controlMu.Unlock()

	runtime := p.currentRuntimeSnapshot()
	result = newPoolPolicyPublicationResult(runtime, normalized)
	if err := p.validatePoolPolicyPublicationCandidate(normalized, &result); err != nil {
		return result, err
	}

	if err := p.validatePoolPolicyPublication(runtime.Policy, normalized, &result); err != nil {
		return result, err
	}

	// Plan class budgets from the candidate policy before any runtime snapshot
	// is published. Infeasible class allocation rejects the whole update.
	attemptGeneration := runtime.Generation.Next()
	target := PoolBudgetTarget{
		Generation:    attemptGeneration,
		PoolName:      p.name,
		RetainedBytes: normalized.Retention.SoftRetainedBytes,
	}
	classAllocation, err := p.planPoolBudgetForPolicy(target, normalized)
	result.ClassBudgetPublication = poolClassBudgetPublicationReportFromAllocation(p.name, attemptGeneration, classAllocation)
	if err != nil {
		result.ClassBudgetPublication.FailureReason = policyUpdateFailureReasonForError(err, policyUpdateFailureInvalid)
		result.FailureReason = policyUpdateFailureInvalid
		return result, err
	}

	if !classAllocation.Allocation.Feasible {
		result.ClassBudgetPublication.FailureReason = classAllocation.Allocation.Reason
		result.FailureReason = policyUpdateFailureInfeasibleBudget
		return result, newError(ErrInvalidPolicy, policyUpdateFailureInfeasibleBudget)
	}

	// Apply class/shard budget state first, then publish the candidate runtime
	// policy with the prior pressure signal preserved.
	classGeneration := p.applyPlannedClassBudgetTargetsLocked(classAllocation.Targets)
	generation := budgetPublicationGeneration(runtime.Generation, classGeneration)
	p.publishRuntimeSnapshot(newPoolRuntimeSnapshotWithPressure(generation, normalized, runtime.Pressure))

	result.Generation = generation
	result.Published = true
	result.RuntimePublished = true
	result.PressurePreserved = p.currentRuntimeSnapshot().Pressure == runtime.Pressure
	result.ClassBudgetPublication.Generation = generation
	result.ClassBudgetPublication.Published = true

	// Optional cleanup is bounded by TrimPolicy and removes only retained
	// buffers already stored in Pool buckets.
	if result.Contracted && normalized.Trim.Enabled && normalized.Trim.TrimOnPolicyShrink {
		result.TrimPlan = poolTrimPlanFromPolicy(normalized.Trim)
		result.TrimAttempted = true
		result.TrimResult = p.trimLocked(result.TrimPlan)
	}

	return result, nil
}

// UpdatePolicy publishes policy and returns only the operation error.
func (p *Pool) UpdatePolicy(policy Policy) error {
	_, err := p.PublishPolicy(policy)
	return err
}

// newPoolPolicyPublicationResult builds the stable report skeleton shared by
// successful and rejected publication paths.
func newPoolPolicyPublicationResult(runtime *poolRuntimeSnapshot, candidate Policy) PoolPolicyPublicationResult {
	var previousPolicy Policy
	var previousGeneration Generation
	if runtime != nil {
		previousPolicy = runtime.clonePolicy()
		previousGeneration = runtime.Generation
	}

	diff := classifyPolicyUpdate(previousPolicy, candidate)
	return PoolPolicyPublicationResult{
		PreviousGeneration: previousGeneration,
		Contracted:         diff.Diagnostics.RetentionContracted,
		Diff:               diff.Diagnostics,
		PreviousPolicy:     previousPolicy,
		Policy:             clonePoolPolicy(candidate),
	}
}

// validatePoolPolicyPublicationCandidate runs full policy and context support
// validation before any live publication is allowed to mutate Pool state.
//
// Compatibility checks answer "can this already-built Pool transition from the
// previous value to the next value?". Candidate validation answers the earlier
// question: "is the next value itself a coherent policy that this Pool owner
// mode can enforce?". Keeping both checks prevents invalid pressure, trim,
// admission, or ownership values from reaching the runtime snapshot just because
// they did not change construction shape.
func (p *Pool) validatePoolPolicyPublicationCandidate(policy Policy, result *PoolPolicyPublicationResult) error {
	if err := policy.Validate(); err != nil {
		result.FailureReason = policyUpdateFailureInvalid
		return err
	}

	if err := validatePoolSupportedPolicy(policy, p.constructionMode); err != nil {
		result.FailureReason = policyUpdateFailureInvalid
		return err
	}
	return nil
}

// validatePoolPolicyPublication validates value compatibility before mutation.
func (p *Pool) validatePoolPolicyPublication(previous Policy, next Policy, result *PoolPolicyPublicationResult) error {
	compatibility := checkLivePolicyUpdateCompatibility(previous, next, p.constructionMode)
	result.Diff = compatibility.Diff.Diagnostics
	result.Contracted = compatibility.Diff.Diagnostics.RetentionContracted
	if compatibility.Compatible {
		return nil
	}

	reason := compatibility.FailureReason
	if reason == "" {
		reason = policyUpdateFailureInvalid
	}

	result.FailureReason = reason
	return newError(ErrInvalidPolicy, reason)
}

// poolClassBudgetPublicationReportFromAllocation converts a planned allocation
// into the report shape returned by PublishPolicy.
func poolClassBudgetPublicationReportFromAllocation(
	poolName string,
	generation Generation,
	report classBudgetAllocationReport,
) PoolClassBudgetPublicationReport {
	return PoolClassBudgetPublicationReport{
		PoolName:      poolName,
		Generation:    generation,
		Allocation:    newBudgetAllocationDiagnostics(report.Allocation),
		Targets:       append([]ClassBudgetTarget(nil), report.Targets...),
		Published:     false,
		FailureReason: "",
	}
}

// poolTrimPlanFromPolicy converts TrimPolicy cycle bounds into a Pool-local
// trim plan.
func poolTrimPlanFromPolicy(policy TrimPolicy) PoolTrimPlan {
	return PoolTrimPlan{
		MaxBuffers:        poolClampUint64ToInt(policy.MaxBuffersPerCycle),
		MaxBytes:          policy.MaxBytesPerCycle,
		MaxClasses:        policy.MaxClassesPerPoolPerCycle,
		MaxShardsPerClass: policy.MaxShardsPerClassPerCycle,
	}
}

// poolClampUint64ToInt clamps count to the largest int on the current platform.
//
// Trim policy stores buffer counts as uint64 because higher-level partition and
// group policy use unsigned budget math. PoolTrimPlan uses int because local
// bucket loops count concrete removals. Clamping avoids architecture-dependent
// truncation when a very large policy value reaches the Pool boundary.
func poolClampUint64ToInt(count uint64) int {
	maxInt := int(^uint(0) >> 1)
	if count > uint64(maxInt) {
		return maxInt
	}
	return int(count)
}
