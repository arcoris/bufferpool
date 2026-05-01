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

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"sync"
	"testing"
)

func TestPoolGroupPublishPolicyPlansPartitionBudgetBeforeRuntimePublication(t *testing.T) {
	group := newGroupPolicyUpdateGroup(t, "alpha", "beta")
	corruptGroupPolicyUpdateChildPool(t, group, "alpha", "alpha-pool")
	before := group.currentRuntimeSnapshot().Generation

	result, err := group.PublishPolicy(groupPolicyUpdatePolicy(2 * KiB))
	requireGroupErrorIs(t, err, ErrInvalidPolicy)
	if result.RuntimePublished || result.Published || result.Generation != NoGeneration {
		t.Fatalf("PublishPolicy(child plan failure) = %+v, want no group runtime publication", result)
	}
	if after := group.currentRuntimeSnapshot().Generation; after != before {
		t.Fatalf("group runtime generation changed after child plan failure: got %s want %s", after, before)
	}
}

func TestPoolGroupPublishPolicyChildPlanFailureDoesNotPublishGroupRuntime(t *testing.T) {
	group := newGroupPolicyUpdateGroup(t, "alpha", "beta")
	corruptGroupPolicyUpdateChildPool(t, group, "alpha", "alpha-pool")
	before := group.currentRuntimeSnapshot()

	result, err := group.PublishPolicy(groupPolicyUpdatePolicy(2 * KiB))
	requireGroupErrorIs(t, err, ErrInvalidPolicy)
	if result.RuntimePublished || result.Published {
		t.Fatalf("PublishPolicy(child plan failure) = %+v, want unpublished rejection", result)
	}
	if after := group.currentRuntimeSnapshot(); after.Generation != before.Generation || after.Policy != before.Policy {
		t.Fatalf("group runtime changed after child plan failure: before=%+v after=%+v", before, after)
	}
}

func TestPoolGroupPublishPolicyChildPlanFailureDoesNotMutateAnyPartition(t *testing.T) {
	group := newGroupPolicyUpdateGroup(t, "alpha", "beta")
	corruptGroupPolicyUpdateChildPool(t, group, "alpha", "alpha-pool")
	alphaBefore := groupPolicyUpdatePartitionGeneration(t, group, "alpha")
	betaBefore := groupPolicyUpdatePartitionGeneration(t, group, "beta")

	result, err := group.PublishPolicy(groupPolicyUpdatePolicy(2 * KiB))
	requireGroupErrorIs(t, err, ErrInvalidPolicy)
	if result.RuntimePublished || result.BudgetPublication.Published {
		t.Fatalf("PublishPolicy(child plan failure) = %+v, want no runtime or budget publication", result)
	}
	if alphaAfter := groupPolicyUpdatePartitionGeneration(t, group, "alpha"); alphaAfter != alphaBefore {
		t.Fatalf("alpha partition generation changed: got %s want %s", alphaAfter, alphaBefore)
	}
	if betaAfter := groupPolicyUpdatePartitionGeneration(t, group, "beta"); betaAfter != betaBefore {
		t.Fatalf("beta partition generation changed: got %s want %s", betaAfter, betaBefore)
	}
}

func TestPoolGroupPublishPolicyApplyAfterPlanningIsNoFail(t *testing.T) {
	group := newGroupPolicyUpdateGroup(t, "alpha", "beta")

	result, err := group.PublishPolicy(groupPolicyUpdatePolicy(2 * KiB))
	requireGroupNoError(t, err)
	if !result.Published || !result.RuntimePublished || !result.BudgetPublication.Published || result.FailureReason != "" {
		t.Fatalf("PublishPolicy(success) = %+v, want fully published no-failure result", result)
	}
	if len(result.BudgetPublication.SkippedPartitions) != 0 {
		t.Fatalf("SkippedPartitions = %+v, want none", result.BudgetPublication.SkippedPartitions)
	}
}

func TestGroupPolicyPreplannedApplyCannotFailForNormalPolicyReasons(t *testing.T) {
	group := newGroupPolicyUpdateGroup(t, "alpha", "beta")

	result, err := group.PublishPolicy(groupPolicyUpdatePolicy(2 * KiB))
	requireGroupNoError(t, err)
	if result.RuntimePublished && !result.Published {
		t.Fatalf("PublishPolicy() = %+v, runtime publication must imply successful planned child apply", result)
	}
	if !result.BudgetPublication.Published || result.BudgetPublication.FailureReason != "" {
		t.Fatalf("BudgetPublication = %+v, want no-fail planned child apply", result.BudgetPublication)
	}
}

func TestPartitionPolicyPreplannedApplyCannotFailForNormalPolicyReasons(t *testing.T) {
	partition := newPartitionPolicyUpdatePartition(t, "alpha", "beta")

	result, err := partition.PublishPolicy(partitionPolicyUpdatePolicy(2*KiB, false))
	requirePartitionNoError(t, err)
	if result.RuntimePublished && !result.Published {
		t.Fatalf("PublishPolicy() = %+v, runtime publication must imply successful planned Pool apply", result)
	}
	if !result.BudgetPublication.Published || result.BudgetPublication.FailureReason != "" {
		t.Fatalf("BudgetPublication = %+v, want no-fail planned Pool apply", result.BudgetPublication)
	}
	for _, classReport := range result.BudgetPublication.ClassReports {
		if !classReport.Published || classReport.FailureReason != "" {
			t.Fatalf("ClassReport = %+v, want no-fail planned class apply", classReport)
		}
	}
}

func TestPoolPolicyPreplannedClassApplyCannotFailForNormalPolicyReasons(t *testing.T) {
	pool := MustNew(PoolConfig{Policy: poolTestSmallSingleShardPolicy()})
	defer closePoolForTest(t, pool)

	result, err := pool.PublishPolicy(poolPolicyUpdateContractedPolicy(false))
	requirePoolPolicyNoError(t, err)
	if result.RuntimePublished && !result.Published {
		t.Fatalf("PublishPolicy() = %+v, runtime publication must imply planned class apply", result)
	}
	if !result.ClassBudgetPublication.Published || result.ClassBudgetPublication.FailureReason != "" {
		t.Fatalf("ClassBudgetPublication = %+v, want no-fail planned class apply", result.ClassBudgetPublication)
	}
}

func TestPoolGroupPublishPolicyRuntimePublishedImpliesPartitionTargetsApplied(t *testing.T) {
	group := newGroupPolicyUpdateGroup(t, "alpha", "beta")

	result, err := group.PublishPolicy(groupPolicyUpdatePolicy(2 * KiB))
	requireGroupNoError(t, err)
	if !result.RuntimePublished {
		t.Fatalf("PublishPolicy() = %+v, want runtime publication", result)
	}
	for _, target := range result.BudgetPublication.Targets {
		snapshot, ok := group.PartitionSnapshot(target.PartitionName)
		if !ok {
			t.Fatalf("PartitionSnapshot(%q) missing", target.PartitionName)
		}
		if snapshot.Policy.Budget.MaxRetainedBytes != target.RetainedBytes {
			t.Fatalf("%s retained budget = %s, want %s",
				target.PartitionName, snapshot.Policy.Budget.MaxRetainedBytes, target.RetainedBytes)
		}
		if len(snapshot.Pools) == 0 {
			t.Fatalf("%s has no Pool snapshots after target publication", target.PartitionName)
		}
		if assigned := sumPoolClassBudgetTargets(snapshot.Pools[0].Pool.Classes); assigned > target.RetainedBytes.Bytes() {
			t.Fatalf("%s class targets = %d, want <= partition target %d",
				target.PartitionName, assigned, target.RetainedBytes.Bytes())
		}
	}
}

func TestPoolGroupPublishPolicyReportFieldsForChildPlanFailure(t *testing.T) {
	group := newGroupPolicyUpdateGroup(t, "alpha", "beta")
	corruptGroupPolicyUpdateChildPool(t, group, "alpha", "alpha-pool")

	result, err := group.PublishPolicy(groupPolicyUpdatePolicy(2 * KiB))
	requireGroupErrorIs(t, err, ErrInvalidPolicy)
	if result.FailureReason != policyUpdateFailureShapeChange {
		t.Fatalf("FailureReason = %q, want %q", result.FailureReason, policyUpdateFailureShapeChange)
	}
	if result.BudgetPublication.Published || result.BudgetPublication.FailureReason != policyUpdateFailureShapeChange {
		t.Fatalf("BudgetPublication = %+v, want unpublished shape-change failure", result.BudgetPublication)
	}
	if len(result.SkippedPartitions) != 1 || result.SkippedPartitions[0].Reason != policyUpdateFailureShapeChange {
		t.Fatalf("SkippedPartitions = %+v, want alpha shape-change skip", result.SkippedPartitions)
	}
}

func TestPoolTrimClosedReasonIsStable(t *testing.T) {
	pool := closedPolicyPublicationPool(t)

	result := pool.Trim(PoolTrimPlan{MaxBuffers: 1, MaxBytes: KiB})
	if result.Attempted || result.Executed || result.Reason != errPoolTrimClosed {
		t.Fatalf("Trim(closed) = %+v, want stable closed trim reason", result)
	}
}

func TestPoolTrimClassClosedReasonIsStable(t *testing.T) {
	pool := closedPolicyPublicationPool(t)

	result := pool.TrimClass(ClassID(0), 1, KiB)
	if result.Attempted || result.Executed || result.Reason != errPoolTrimClosed {
		t.Fatalf("TrimClass(closed) = %+v, want stable closed trim reason", result)
	}
}

func TestPoolTrimShardClosedReasonIsStable(t *testing.T) {
	pool := closedPolicyPublicationPool(t)

	result := pool.TrimShard(ClassID(0), 0, 1, KiB)
	if result.Attempted || result.Executed || result.Reason != errPoolTrimClosed {
		t.Fatalf("TrimShard(closed) = %+v, want stable closed trim reason", result)
	}
}

func TestPoolTrimClosedDoesNotInspectStorage(t *testing.T) {
	pool := closedPolicyPublicationPool(t)

	classResult := pool.TrimClass(ClassID(999), 1, KiB)
	if classResult.Reason != errPoolTrimClosed || classResult.Attempted {
		t.Fatalf("TrimClass(closed invalid class) = %+v, want lifecycle rejection before class inspection", classResult)
	}
	shardResult := pool.TrimShard(ClassID(999), 999, 1, KiB)
	if shardResult.Reason != errPoolTrimClosed || shardResult.Attempted {
		t.Fatalf("TrimShard(closed invalid shard) = %+v, want lifecycle rejection before shard inspection", shardResult)
	}
}

func TestPoolTrimClosedDoesNotMutateRetainedCounters(t *testing.T) {
	pool := closedPolicyPublicationPool(t)
	before := pool.Metrics()

	result := pool.Trim(PoolTrimPlan{MaxBuffers: 1, MaxBytes: KiB})
	after := pool.Metrics()
	if result.Attempted || result.Executed || result.Reason != errPoolTrimClosed {
		t.Fatalf("Trim(closed) = %+v, want closed no-op", result)
	}
	if after.CurrentRetainedBytes != before.CurrentRetainedBytes || after.CurrentRetainedBuffers != before.CurrentRetainedBuffers {
		t.Fatalf("closed trim mutated retained counters: before=%+v after=%+v", before, after)
	}
}

func TestPoolPublishPolicyClosedReasonIsStable(t *testing.T) {
	pool := closedPolicyPublicationPool(t)

	result, err := pool.PublishPolicy(poolPolicyUpdateExpandedPolicy())
	requirePoolPolicyErrorIs(t, err, ErrClosed)
	if result.FailureReason != policyUpdateFailureClosed || result.RuntimePublished || result.Published {
		t.Fatalf("PublishPolicy(closed) = %+v, want stable closed policy reason", result)
	}
}

func TestPoolPublishPolicyResultFieldInvariants(t *testing.T) {
	pool := MustNew(PoolConfig{Policy: poolTestSmallSingleShardPolicy()})
	defer closePoolForTest(t, pool)
	before := pool.currentRuntimeSnapshot()

	result, err := pool.PublishPolicy(poolPolicyUpdateExpandedPolicy())
	requirePoolPolicyNoError(t, err)
	assertPolicyPublicationSuccessInvariants(t, result.Published, result.RuntimePublished, result.Generation, result.FailureReason)
	if result.PreviousGeneration != before.Generation {
		t.Fatalf("PreviousGeneration = %s, want %s", result.PreviousGeneration, before.Generation)
	}
	if result.Contracted != result.Diff.RetentionContracted {
		t.Fatalf("Contracted = %v Diff.RetentionContracted = %v", result.Contracted, result.Diff.RetentionContracted)
	}
	if !result.PressurePreserved {
		t.Fatalf("PressurePreserved = false on successful Pool publication")
	}
}

func TestPartitionPublishPolicyResultFieldInvariants(t *testing.T) {
	partition := newPartitionPolicyUpdatePartition(t, "alpha")
	before := partition.currentRuntimeSnapshot()

	result, err := partition.PublishPolicy(partitionPolicyUpdatePolicy(KiB, false))
	requirePartitionNoError(t, err)
	assertPolicyPublicationSuccessInvariants(t, result.Published, result.RuntimePublished, result.Generation, result.FailureReason)
	if result.PreviousGeneration != before.Generation {
		t.Fatalf("PreviousGeneration = %s, want %s", result.PreviousGeneration, before.Generation)
	}
	if result.Contracted != result.Diff.RetentionContracted {
		t.Fatalf("Contracted = %v Diff.RetentionContracted = %v", result.Contracted, result.Diff.RetentionContracted)
	}
	if !result.PressurePreserved {
		t.Fatalf("PressurePreserved = false on successful partition publication")
	}
}

func TestGroupPublishPolicyResultFieldInvariants(t *testing.T) {
	group := newGroupPolicyUpdateGroup(t, "alpha")
	before := group.currentRuntimeSnapshot()

	result, err := group.PublishPolicy(groupPolicyUpdatePolicy(KiB))
	requireGroupNoError(t, err)
	assertPolicyPublicationSuccessInvariants(t, result.Published, result.RuntimePublished, result.Generation, result.FailureReason)
	if result.PreviousGeneration != before.Generation {
		t.Fatalf("PreviousGeneration = %s, want %s", result.PreviousGeneration, before.Generation)
	}
	if result.Contracted != result.Diff.RetentionContracted {
		t.Fatalf("Contracted = %v Diff.RetentionContracted = %v", result.Contracted, result.Diff.RetentionContracted)
	}
	if !result.PressurePreserved {
		t.Fatalf("PressurePreserved = false on successful group publication")
	}
}

func TestGroupPublishPolicyNoRuntimePublicationOnChildPlanFailure(t *testing.T) {
	group := newGroupPolicyUpdateGroup(t, "alpha")
	corruptGroupPolicyUpdateChildPool(t, group, "alpha", "alpha-pool")
	before := group.currentRuntimeSnapshot().Generation

	result, err := group.PublishPolicy(groupPolicyUpdatePolicy(KiB))
	requireGroupErrorIs(t, err, ErrInvalidPolicy)
	if result.RuntimePublished || result.Published || result.Generation != NoGeneration {
		t.Fatalf("PublishPolicy(child plan failure) = %+v, want no runtime publication", result)
	}
	if after := group.currentRuntimeSnapshot().Generation; after != before {
		t.Fatalf("group runtime generation changed: got %s want %s", after, before)
	}
}

func TestPolicyPublicationFailureReasonIsStable(t *testing.T) {
	pool := closedPolicyPublicationPool(t)
	poolResult, err := pool.PublishPolicy(poolPolicyUpdateExpandedPolicy())
	requirePoolPolicyErrorIs(t, err, ErrClosed)
	if poolResult.FailureReason != policyUpdateFailureClosed {
		t.Fatalf("Pool FailureReason = %q, want %q", poolResult.FailureReason, policyUpdateFailureClosed)
	}
	if trim := pool.Trim(PoolTrimPlan{MaxBuffers: 1, MaxBytes: KiB}); trim.Reason != errPoolTrimClosed {
		t.Fatalf("Trim Reason = %q, want %q", trim.Reason, errPoolTrimClosed)
	}
	group := newGroupPolicyUpdateGroup(t, "alpha")
	corruptGroupPolicyUpdateChildPool(t, group, "alpha", "alpha-pool")
	groupResult, err := group.PublishPolicy(groupPolicyUpdatePolicy(KiB))
	requireGroupErrorIs(t, err, ErrInvalidPolicy)
	if groupResult.FailureReason != policyUpdateFailureShapeChange {
		t.Fatalf("Group FailureReason = %q, want %q", groupResult.FailureReason, policyUpdateFailureShapeChange)
	}
}

func TestPolicyPublicationFailureReasonsDoNotExposeRawErrorText(t *testing.T) {
	raw := newError(ErrInvalidPolicy, "bufferpool: raw validation detail")
	report := PoolPartitionBudgetPublicationReport{
		Allocation: BudgetAllocationDiagnostics{Feasible: true},
	}

	reason := groupPolicyPartitionPlanFailureReason(raw, report)
	if reason != policyUpdateFailureInvalid {
		t.Fatalf("failure reason = %q, want stable %q", reason, policyUpdateFailureInvalid)
	}
	if reason == raw.Error() {
		t.Fatalf("failure reason exposed raw error text %q", reason)
	}
}

func TestGroupPolicyPartitionPlanFailureReasonMapsInvalidPolicy(t *testing.T) {
	err := newError(ErrInvalidPolicy, "bufferpool: detailed policy validation text")
	report := PoolPartitionBudgetPublicationReport{Allocation: BudgetAllocationDiagnostics{Feasible: true}}
	if got := groupPolicyPartitionPlanFailureReason(err, report); got != policyUpdateFailureInvalid {
		t.Fatalf("reason = %q, want %q", got, policyUpdateFailureInvalid)
	}
}

func TestGroupPolicyPartitionPlanFailureReasonMapsInvalidOptions(t *testing.T) {
	err := newError(ErrInvalidOptions, "bufferpool: detailed options validation text")
	report := PoolPartitionBudgetPublicationReport{Allocation: BudgetAllocationDiagnostics{Feasible: true}}
	if got := groupPolicyPartitionPlanFailureReason(err, report); got != policyUpdateFailureInvalid {
		t.Fatalf("reason = %q, want %q", got, policyUpdateFailureInvalid)
	}
}

func TestGroupPolicyPartitionPlanFailureReasonMapsClosed(t *testing.T) {
	err := newError(ErrClosed, "bufferpool: detailed close text")
	report := PoolPartitionBudgetPublicationReport{Allocation: BudgetAllocationDiagnostics{Feasible: true}}
	if got := groupPolicyPartitionPlanFailureReason(err, report); got != policyUpdateFailureClosed {
		t.Fatalf("reason = %q, want %q", got, policyUpdateFailureClosed)
	}
}

func TestPolicyPublicationFailureReasonStillReturnsWrappedError(t *testing.T) {
	pool := MustNew(PoolConfig{Policy: poolTestSmallSingleShardPolicy()})
	defer closePoolForTest(t, pool)

	policy := poolPolicyUpdateExpandedPolicy()
	policy.Retention.SoftRetainedBytes = 8 * KiB
	policy.Retention.HardRetainedBytes = 4 * KiB
	result, err := pool.PublishPolicy(policy)
	requirePoolPolicyErrorIs(t, err, ErrInvalidPolicy)
	if result.FailureReason != policyUpdateFailureInvalid {
		t.Fatalf("FailureReason = %q, want %q", result.FailureReason, policyUpdateFailureInvalid)
	}
	if err == nil || err.Error() == result.FailureReason {
		t.Fatalf("returned error = %v, want detailed wrapped error separate from stable report reason", err)
	}
}

func TestPoolPublishPolicyNoBudgetOrNoChangeResultFields(t *testing.T) {
	pool := MustNew(PoolConfig{Policy: poolTestSmallSingleShardPolicy()})
	defer closePoolForTest(t, pool)

	before := pool.currentRuntimeSnapshot()
	result, err := pool.PublishPolicy(before.clonePolicy())
	requirePoolPolicyNoError(t, err)
	assertPolicyPublicationSuccessInvariants(t, result.Published, result.RuntimePublished, result.Generation, result.FailureReason)
	if result.Contracted || result.TrimAttempted || result.PreviousGeneration != before.Generation {
		t.Fatalf("PublishPolicy(no change) = %+v, want non-contracted publication fields", result)
	}
}

func TestPartitionPublishPolicyNoBudgetOrNoChangeResultFields(t *testing.T) {
	partition := newPartitionPolicyUpdatePartition(t, "alpha")
	before := partition.currentRuntimeSnapshot()
	policy := DefaultPartitionPolicy()
	policy.Pressure = PartitionPressurePolicy{Enabled: true, HighOwnedBytes: KiB}

	result, err := partition.PublishPolicy(policy)
	requirePartitionNoError(t, err)
	assertPolicyPublicationSuccessInvariants(t, result.Published, result.RuntimePublished, result.Generation, result.FailureReason)
	if result.Contracted || result.TrimAttempted || result.PreviousGeneration != before.Generation {
		t.Fatalf("PublishPolicy(no budget) = %+v, want runtime-only publication fields", result)
	}
	if result.BudgetPublication.Published || result.BudgetPublication.FailureReason != partitionPolicyPublicationNoBudgetTarget {
		t.Fatalf("BudgetPublication = %+v, want no-budget report", result.BudgetPublication)
	}
}

func TestGroupPublishPolicyNoBudgetTargetResultFields(t *testing.T) {
	group := newGroupPolicyUpdateGroup(t, "alpha")
	before := group.currentRuntimeSnapshot()
	policy := DefaultPoolGroupPolicy()
	policy.Pressure = PartitionPressurePolicy{Enabled: true, HighOwnedBytes: KiB}

	result, err := group.PublishPolicy(policy)
	requireGroupNoError(t, err)
	assertPolicyPublicationSuccessInvariants(t, result.Published, result.RuntimePublished, result.Generation, result.FailureReason)
	if result.Contracted || result.PreviousGeneration != before.Generation {
		t.Fatalf("PublishPolicy(no budget) = %+v, want runtime-only group fields", result)
	}
	if result.BudgetPublication.Published || result.BudgetPublication.FailureReason != groupPolicyPublicationNoBudgetTarget {
		t.Fatalf("BudgetPublication = %+v, want no-budget report", result.BudgetPublication)
	}
}

func TestPoolPolicyPublicationRejectedResultFields(t *testing.T) {
	pool := MustNew(PoolConfig{Policy: poolTestSmallSingleShardPolicy()})
	defer closePoolForTest(t, pool)
	policy := pool.Snapshot().Policy
	policy.Shards.ShardsPerClass++

	result, err := pool.PublishPolicy(policy)
	requirePoolPolicyErrorIs(t, err, ErrInvalidPolicy)
	assertPolicyPublicationRejectedInvariants(t, result.Published, result.RuntimePublished, result.Generation, result.FailureReason)
	if result.FailureReason != policyUpdateFailureShapeChange {
		t.Fatalf("FailureReason = %q, want %q", result.FailureReason, policyUpdateFailureShapeChange)
	}
}

func TestPartitionPolicyPublicationRejectedResultFields(t *testing.T) {
	partition := newPartitionPolicyUpdatePartition(t, "alpha")
	partitionPolicyUpdateCorruptPoolPolicy(t, partition, "alpha", func(policy *Policy) {
		policy.Shards.ShardsPerClass++
	})

	result, err := partition.PublishPolicy(partitionPolicyUpdatePolicy(KiB, false))
	requirePartitionErrorIs(t, err, ErrInvalidPolicy)
	assertPolicyPublicationRejectedInvariants(t, result.Published, result.RuntimePublished, result.Generation, result.FailureReason)
	if result.FailureReason != policyUpdateFailureShapeChange {
		t.Fatalf("FailureReason = %q, want %q", result.FailureReason, policyUpdateFailureShapeChange)
	}
}

func TestGroupPolicyPublicationRejectedResultFields(t *testing.T) {
	group := newGroupPolicyUpdateGroup(t, "alpha")
	corruptGroupPolicyUpdateChildPool(t, group, "alpha", "alpha-pool")

	result, err := group.PublishPolicy(groupPolicyUpdatePolicy(KiB))
	requireGroupErrorIs(t, err, ErrInvalidPolicy)
	assertPolicyPublicationRejectedInvariants(t, result.Published, result.RuntimePublished, result.Generation, result.FailureReason)
	if result.FailureReason != policyUpdateFailureShapeChange {
		t.Fatalf("FailureReason = %q, want %q", result.FailureReason, policyUpdateFailureShapeChange)
	}
}

func TestPolicyPublicationSuccessResultFields(t *testing.T) {
	pool := MustNew(PoolConfig{Policy: poolTestSmallSingleShardPolicy()})
	defer closePoolForTest(t, pool)

	result, err := pool.PublishPolicy(poolPolicyUpdateExpandedPolicy())
	requirePoolPolicyNoError(t, err)
	assertPolicyPublicationSuccessInvariants(t, result.Published, result.RuntimePublished, result.Generation, result.FailureReason)
	if result.Published && !result.RuntimePublished {
		t.Fatalf("PublishPolicy() = %+v, published must imply runtime published", result)
	}
}

func TestPolicyUpdateDoesNotEnterPoolHotPathAST(t *testing.T) {
	for _, file := range []string{"pool_get.go", "pool_put.go"} {
		facts := parsePolicyPublicationASTFacts(t, file)
		for _, forbidden := range []string{
			"PublishPolicy",
			"UpdatePolicy",
			"policyUpdate",
			"PoolPartition",
			"PoolGroup",
			"partitionController",
			"groupCoordinator",
			"ExecuteTrim",
			"PlanTrim",
			"activeRegistry",
		} {
			if facts.hasIdentifier(forbidden) || facts.hasCall(forbidden) || facts.hasSelector(forbidden) {
				t.Fatalf("%s AST references %q; Pool hot path files must not enter policy-update/control-plane code", file, forbidden)
			}
		}
	}
}

func TestGroupPolicyUpdateDoesNotCallPoolTrimDirectlyAST(t *testing.T) {
	facts := parsePolicyPublicationASTFacts(t, "group_policy_update.go")
	for _, forbidden := range []string{"Trim", "TrimClass", "TrimShard", "Get", "Put"} {
		if facts.hasCall(forbidden) {
			t.Fatalf("group_policy_update.go calls %q; group policy update must not call Pool data-plane/trim APIs", forbidden)
		}
	}
}

func TestGroupPolicyUpdateDoesNotCallPoolTrimOrHotPathAST(t *testing.T) {
	facts := parsePolicyPublicationASTFacts(t, "group_policy_update.go")
	for _, forbidden := range []string{"Trim", "TrimClass", "TrimShard", "Get", "Put"} {
		if facts.hasCall(forbidden) {
			t.Fatalf("group_policy_update.go calls %q; group policy update must not call Pool data-plane/trim APIs", forbidden)
		}
	}
}

func TestGroupPolicyUpdateDoesNotScanPoolShardInternalsAST(t *testing.T) {
	facts := parsePolicyPublicationASTFacts(t, "group_policy_update.go")
	for _, forbidden := range []string{"shards", "mustClassStateFor", "PoolShard", "ClassState", "classState", "shard", "bucket", "PoolTrimPlan"} {
		if facts.hasIdentifier(forbidden) || facts.hasSelector(forbidden) || facts.hasCall(forbidden) {
			t.Fatalf("group_policy_update.go AST references %q; group policy update must not scan shard/class internals", forbidden)
		}
	}
}

func TestPartitionPolicyUpdateDoesNotReferencePoolGroupAST(t *testing.T) {
	facts := parsePolicyPublicationASTFacts(t, "partition_policy_update.go")
	if facts.hasIdentifier("PoolGroup") || facts.hasSelector("PoolGroup") {
		t.Fatalf("partition_policy_update.go AST references PoolGroup; partition policy update must stay partition-local")
	}
	for _, required := range []string{"planPartitionPolicyBudgetBatchLocked", "applyPlannedPoolBudgetBatchLocked", "executeTrimPlan"} {
		if !facts.hasIdentifier(required) && !facts.hasCall(required) && !facts.hasSelector(required) {
			t.Fatalf("partition_policy_update.go AST missing %q; partition policy update must own managed Pool control calls", required)
		}
	}
}

func TestPoolPolicyUpdateDoesNotReferencePartitionOrGroupAST(t *testing.T) {
	facts := parsePolicyPublicationASTFacts(t, "pool_policy_update.go")
	for _, forbidden := range []string{"PoolGroup", "PoolPartition", "groupCoordinator", "partitionController"} {
		if facts.hasIdentifier(forbidden) || facts.hasSelector(forbidden) || facts.hasCall(forbidden) {
			t.Fatalf("pool_policy_update.go AST references %q; Pool policy update must stay Pool-local", forbidden)
		}
	}
}

func TestPoolPublishPolicyFinalInterleavings(t *testing.T) {
	runPoolPublishPolicyInterleavings(t)
}

func TestPoolPartitionPublishPolicyFinalInterleavings(t *testing.T) {
	runPartitionPublishPolicyInterleavings(t)
}

func TestPoolGroupPublishPolicyFinalInterleavings(t *testing.T) {
	runGroupPublishPolicyInterleavings(t)
}

func TestPoolPublishPolicyRaceStyleInterleavings(t *testing.T) {
	runPoolPublishPolicyInterleavings(t)
}

func runPoolPublishPolicyInterleavings(t *testing.T) {
	t.Helper()

	pool := MustNew(PoolConfig{Policy: poolTestSmallSingleShardPolicy()})
	start := make(chan struct{})
	errs := make(chan error, 96)
	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		<-start
		for iteration := 0; iteration < 32; iteration++ {
			buffer, err := pool.Get(300)
			if err != nil {
				if !errors.Is(err, ErrClosed) {
					errs <- err
				}
				return
			}
			if err := pool.Put(buffer); err != nil && !errors.Is(err, ErrClosed) {
				errs <- err
				return
			}
		}
	}()
	go func() {
		defer wg.Done()
		<-start
		for iteration := 0; iteration < 32; iteration++ {
			policy := poolPolicyUpdateExpandedPolicy()
			if iteration%2 == 1 {
				policy = poolTestSmallSingleShardPolicy()
			}
			if _, err := pool.PublishPolicy(policy); err != nil {
				if !errors.Is(err, ErrClosed) {
					errs <- err
				}
				return
			}
		}
	}()
	go func() {
		defer wg.Done()
		<-start
		errs <- pool.Close()
	}()
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		requirePoolPolicyNoError(t, err)
	}
}

func TestPartitionPublishPolicyRaceStyleInterleavings(t *testing.T) {
	runPartitionPublishPolicyInterleavings(t)
}

func runPartitionPublishPolicyInterleavings(t *testing.T) {
	t.Helper()

	partition := MustNewPoolPartition(partitionPolicyUpdateConfig("alpha"))
	start := make(chan struct{})
	errs := make(chan error, 128)
	var wg sync.WaitGroup
	wg.Add(4)
	go func() {
		defer wg.Done()
		<-start
		for iteration := 0; iteration < 24; iteration++ {
			lease, err := partition.Acquire("alpha", 300)
			if err != nil {
				if !errors.Is(err, ErrClosed) {
					errs <- err
				}
				return
			}
			if err := partition.Release(lease, lease.Buffer()); err != nil && !errors.Is(err, ErrClosed) {
				errs <- err
				return
			}
		}
	}()
	go func() {
		defer wg.Done()
		<-start
		for iteration := 0; iteration < 24; iteration++ {
			if _, err := partition.PublishPolicy(partitionPolicyUpdatePolicy(KiB, iteration%2 == 0)); err != nil {
				if !errors.Is(err, ErrClosed) {
					errs <- err
				}
				return
			}
		}
	}()
	go func() {
		defer wg.Done()
		<-start
		var report PartitionControllerReport
		for iteration := 0; iteration < 24; iteration++ {
			if err := partition.TickInto(&report); err != nil {
				if !errors.Is(err, ErrClosed) {
					errs <- err
				}
				return
			}
		}
	}()
	go func() {
		defer wg.Done()
		<-start
		errs <- partition.Close()
	}()
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		requirePartitionNoError(t, err)
	}
}

func TestGroupPublishPolicyRaceStyleInterleavings(t *testing.T) {
	runGroupPublishPolicyInterleavings(t)
}

func runGroupPublishPolicyInterleavings(t *testing.T) {
	t.Helper()

	group := MustNewPoolGroup(groupPolicyUpdateConfig("alpha"))
	start := make(chan struct{})
	errs := make(chan error, 128)
	var wg sync.WaitGroup
	wg.Add(4)
	go func() {
		defer wg.Done()
		<-start
		for iteration := 0; iteration < 24; iteration++ {
			lease, err := group.Acquire("alpha-pool", 300)
			if err != nil {
				if !errors.Is(err, ErrClosed) {
					errs <- err
				}
				return
			}
			if err := group.Release(lease, lease.Buffer()); err != nil && !errors.Is(err, ErrClosed) {
				errs <- err
				return
			}
		}
	}()
	go func() {
		defer wg.Done()
		<-start
		for iteration := 0; iteration < 24; iteration++ {
			if _, err := group.PublishPolicy(groupPolicyUpdatePolicy(KiB)); err != nil {
				if !errors.Is(err, ErrClosed) {
					errs <- err
				}
				return
			}
		}
	}()
	go func() {
		defer wg.Done()
		<-start
		var report PoolGroupCoordinatorReport
		for iteration := 0; iteration < 24; iteration++ {
			if err := group.TickInto(&report); err != nil {
				if !errors.Is(err, ErrClosed) {
					errs <- err
				}
				return
			}
		}
	}()
	go func() {
		defer wg.Done()
		<-start
		errs <- group.Close()
	}()
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		requireGroupNoError(t, err)
	}
}

func corruptGroupPolicyUpdateChildPool(t *testing.T, group *PoolGroup, partitionName string, poolName string) {
	t.Helper()
	partition, ok := group.partition(partitionName)
	if !ok {
		t.Fatalf("partition %q missing", partitionName)
	}
	partitionPolicyUpdateCorruptPoolPolicy(t, partition, poolName, func(policy *Policy) {
		policy.Shards.ShardsPerClass++
	})
}

func closedPolicyPublicationPool(t *testing.T) *Pool {
	t.Helper()
	pool := MustNew(PoolConfig{Policy: poolTestSmallSingleShardPolicy()})
	seedPoolRetainedBuffers(t, pool, 1, 512)
	if err := pool.Close(); err != nil {
		t.Fatalf("Pool.Close() error = %v", err)
	}
	return pool
}

func assertPolicyPublicationSuccessInvariants(
	t *testing.T,
	published bool,
	runtimePublished bool,
	generation Generation,
	failureReason string,
) {
	t.Helper()
	if !published || !runtimePublished {
		t.Fatalf("Published=%v RuntimePublished=%v, want both true", published, runtimePublished)
	}
	if generation == NoGeneration {
		t.Fatalf("Generation = %s, want published generation", generation)
	}
	if failureReason != "" {
		t.Fatalf("FailureReason = %q, want empty on success", failureReason)
	}
}

func assertPolicyPublicationRejectedInvariants(
	t *testing.T,
	published bool,
	runtimePublished bool,
	generation Generation,
	failureReason string,
) {
	t.Helper()
	if published || runtimePublished {
		t.Fatalf("Published=%v RuntimePublished=%v, want both false", published, runtimePublished)
	}
	if generation != NoGeneration {
		t.Fatalf("Generation = %s, want NoGeneration on rejected publication", generation)
	}
	if failureReason == "" {
		t.Fatal("FailureReason empty on rejected publication")
	}
}

type policyPublicationASTFacts struct {
	identifiers map[string]struct{}
	selectors   map[string]struct{}
	calls       map[string]struct{}
}

func parsePolicyPublicationASTFacts(t *testing.T, file string) policyPublicationASTFacts {
	t.Helper()
	parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
	if err != nil {
		t.Fatalf("ParseFile(%s) error = %v", file, err)
	}
	facts := policyPublicationASTFacts{
		identifiers: make(map[string]struct{}),
		selectors:   make(map[string]struct{}),
		calls:       make(map[string]struct{}),
	}
	ast.Inspect(parsed, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.Ident:
			facts.identifiers[n.Name] = struct{}{}
		case *ast.SelectorExpr:
			facts.selectors[n.Sel.Name] = struct{}{}
		case *ast.CallExpr:
			switch fun := n.Fun.(type) {
			case *ast.Ident:
				facts.calls[fun.Name] = struct{}{}
			case *ast.SelectorExpr:
				facts.calls[fun.Sel.Name] = struct{}{}
			}
		}
		return true
	})
	return facts
}

func (f policyPublicationASTFacts) hasIdentifier(name string) bool {
	_, ok := f.identifiers[name]
	return ok
}

func (f policyPublicationASTFacts) hasSelector(name string) bool {
	_, ok := f.selectors[name]
	return ok
}

func (f policyPublicationASTFacts) hasCall(name string) bool {
	_, ok := f.calls[name]
	return ok
}
