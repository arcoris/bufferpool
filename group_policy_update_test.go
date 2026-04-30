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
	"os"
	"strings"
	"sync"
	"testing"
)

func TestPoolGroupPublishPolicyPublishesRuntimeSnapshot(t *testing.T) {
	group := newGroupPolicyUpdateGroup(t, "alpha")
	before := group.currentRuntimeSnapshot()
	policy := groupPolicyUpdatePolicy(2 * KiB)

	result, err := group.PublishPolicy(policy)
	requireGroupNoError(t, err)
	if !result.Published || !result.RuntimePublished || result.Generation.IsZero() {
		t.Fatalf("PublishPolicy() = %+v, want runtime publication", result)
	}
	if result.PreviousGeneration != before.Generation {
		t.Fatalf("PreviousGeneration = %s, want %s", result.PreviousGeneration, before.Generation)
	}
	if after := group.currentRuntimeSnapshot(); after.Generation != result.Generation || after.Policy.Budget.MaxRetainedBytes != 2*KiB {
		t.Fatalf("runtime after PublishPolicy = %+v, result = %+v", after, result)
	}
	if !result.BudgetPublication.Published {
		t.Fatalf("BudgetPublication = %+v, want published", result.BudgetPublication)
	}
}

func TestPoolGroupUpdatePolicyWrapperReturnsError(t *testing.T) {
	group := newGroupPolicyUpdateGroup(t, "alpha")
	policy := groupPolicyUpdatePolicy(KiB)
	policy.Coordinator.Enabled = true

	err := group.UpdatePolicy(policy)
	requireGroupErrorIs(t, err, ErrInvalidPolicy)
}

func TestPoolGroupPublishPolicyRejectsClosedGroup(t *testing.T) {
	group := newGroupPolicyUpdateGroup(t, "alpha")
	before := group.currentRuntimeSnapshot().Generation
	requireGroupNoError(t, group.Close())

	result, err := group.PublishPolicy(groupPolicyUpdatePolicy(KiB))
	requireGroupErrorIs(t, err, ErrClosed)
	if result.Published || result.RuntimePublished || result.Generation != NoGeneration {
		t.Fatalf("PublishPolicy(closed) = %+v, want closed rejection", result)
	}
	if after := group.currentRuntimeSnapshot().Generation; after != before {
		t.Fatalf("closed PublishPolicy changed runtime generation: got %s want %s", after, before)
	}
}

func TestPoolGroupPublishPolicyPreservesPressureSignal(t *testing.T) {
	group := newGroupPolicyUpdateGroup(t, "alpha")
	publication, err := group.PublishPressure(PressureLevelHigh)
	requireGroupNoError(t, err)
	if !publication.FullyApplied() {
		t.Fatalf("PublishPressure() = %+v, want fully applied", publication)
	}
	before := group.currentRuntimeSnapshot().Pressure
	policy := groupPolicyUpdatePolicy(2 * KiB)
	policy.Pressure = PartitionPressurePolicy{Enabled: true, HighOwnedBytes: KiB}

	result, err := group.PublishPolicy(policy)
	requireGroupNoError(t, err)
	if !result.PressurePreserved {
		t.Fatalf("PublishPolicy() = %+v, want pressure preserved", result)
	}
	if after := group.currentRuntimeSnapshot().Pressure; after != before {
		t.Fatalf("pressure after PublishPolicy = %+v, want %+v", after, before)
	}
}

func TestPoolGroupPublishPolicyContractsPartitionBudgets(t *testing.T) {
	group := newGroupPolicyUpdateGroup(t, "alpha", "beta")

	result, err := group.PublishPolicy(groupPolicyUpdatePolicy(2 * KiB))
	requireGroupNoError(t, err)
	if !result.Published || !result.Contracted || !result.BudgetPublication.Published {
		t.Fatalf("PublishPolicy(contract) = %+v, want contracted publication", result)
	}
	if len(result.BudgetPublication.Targets) != 2 {
		t.Fatalf("targets = %+v, want two partitions", result.BudgetPublication.Targets)
	}
	for _, target := range result.BudgetPublication.Targets {
		snapshot, ok := group.PartitionSnapshot(target.PartitionName)
		if !ok {
			t.Fatalf("PartitionSnapshot(%q) missing", target.PartitionName)
		}
		if snapshot.Policy.Budget.MaxRetainedBytes != target.RetainedBytes {
			t.Fatalf("%s retained budget = %s, want target %s",
				target.PartitionName, snapshot.Policy.Budget.MaxRetainedBytes, target.RetainedBytes)
		}
	}
}

func TestPoolGroupPublishPolicyReportsInfeasiblePartitionBudget(t *testing.T) {
	group := newGroupPolicyUpdateGroup(t, "alpha", "beta")

	result, err := group.PublishPolicy(groupPolicyUpdatePolicy(SizeFromBytes(512)))
	requireGroupErrorIs(t, err, ErrInvalidPolicy)
	if result.Published || result.RuntimePublished {
		t.Fatalf("PublishPolicy(infeasible) = %+v, want rejected before runtime publication", result)
	}
	if result.BudgetPublication.Allocation.Feasible {
		t.Fatalf("BudgetPublication.Allocation = %+v, want infeasible", result.BudgetPublication.Allocation)
	}
	if result.BudgetPublication.FailureReason == "" {
		t.Fatalf("BudgetPublication = %+v, want failure reason", result.BudgetPublication)
	}
}

func TestPoolGroupPublishPolicyDoesNotPublishOnInfeasibleBudget(t *testing.T) {
	group := newGroupPolicyUpdateGroup(t, "alpha", "beta")
	before := group.currentRuntimeSnapshot().Generation
	alphaBefore := groupPolicyUpdatePartitionGeneration(t, group, "alpha")

	result, err := group.PublishPolicy(groupPolicyUpdatePolicy(SizeFromBytes(512)))
	requireGroupErrorIs(t, err, ErrInvalidPolicy)
	if result.RuntimePublished {
		t.Fatalf("PublishPolicy(infeasible) = %+v, want no runtime publication", result)
	}
	if after := group.currentRuntimeSnapshot().Generation; after != before {
		t.Fatalf("runtime generation changed on infeasible budget: got %s want %s", after, before)
	}
	if alphaAfter := groupPolicyUpdatePartitionGeneration(t, group, "alpha"); alphaAfter != alphaBefore {
		t.Fatalf("alpha policy generation changed: got %s want %s", alphaAfter, alphaBefore)
	}
}

func TestPoolGroupPublishPolicyReportsSkippedClosedPartition(t *testing.T) {
	group := newGroupPolicyUpdateGroup(t, "alpha", "beta")
	beta, ok := group.partition("beta")
	if !ok {
		t.Fatal("partition beta missing")
	}
	requirePartitionNoError(t, beta.Close())

	result, err := group.PublishPolicy(groupPolicyUpdatePolicy(2 * KiB))
	requireGroupErrorIs(t, err, ErrClosed)
	if result.Published || result.RuntimePublished {
		t.Fatalf("PublishPolicy(closed child) = %+v, want no group runtime publication", result)
	}
	if len(result.SkippedPartitions) != 1 || result.SkippedPartitions[0].PartitionName != "beta" {
		t.Fatalf("SkippedPartitions = %+v, want beta", result.SkippedPartitions)
	}
	if len(result.BudgetPublication.SkippedPartitions) != 1 {
		t.Fatalf("BudgetPublication = %+v, want skipped child", result.BudgetPublication)
	}
}

func TestPoolGroupPublishPolicyDoesNotPartiallyPublishOnClosedChild(t *testing.T) {
	group := newGroupPolicyUpdateGroup(t, "alpha", "beta")
	alphaBefore := groupPolicyUpdatePartitionGeneration(t, group, "alpha")
	groupBefore := group.currentRuntimeSnapshot().Generation
	beta, ok := group.partition("beta")
	if !ok {
		t.Fatal("partition beta missing")
	}
	requirePartitionNoError(t, beta.Close())

	result, err := group.PublishPolicy(groupPolicyUpdatePolicy(2 * KiB))
	requireGroupErrorIs(t, err, ErrClosed)
	if result.RuntimePublished || result.Published {
		t.Fatalf("PublishPolicy(closed child) = %+v, want no publication", result)
	}
	if groupAfter := group.currentRuntimeSnapshot().Generation; groupAfter != groupBefore {
		t.Fatalf("group runtime generation changed: got %s want %s", groupAfter, groupBefore)
	}
	if alphaAfter := groupPolicyUpdatePartitionGeneration(t, group, "alpha"); alphaAfter != alphaBefore {
		t.Fatalf("alpha partition generation changed: got %s want %s", alphaAfter, alphaBefore)
	}
}

func TestPoolGroupPublishPolicyNoBudgetTargetPublishesRuntimeOnly(t *testing.T) {
	group := newGroupPolicyUpdateGroup(t, "alpha")
	alphaBefore := groupPolicyUpdatePartitionGeneration(t, group, "alpha")
	policy := DefaultPoolGroupPolicy()
	policy.Pressure = PartitionPressurePolicy{Enabled: true, MediumOwnedBytes: KiB}

	result, err := group.PublishPolicy(policy)
	requireGroupNoError(t, err)
	if !result.Published || !result.RuntimePublished {
		t.Fatalf("PublishPolicy(no budget) = %+v, want runtime-only publication", result)
	}
	if result.BudgetPublication.Published {
		t.Fatalf("BudgetPublication = %+v, want unpublished no-budget report", result.BudgetPublication)
	}
	if result.BudgetPublication.FailureReason != groupPolicyPublicationNoBudgetTarget {
		t.Fatalf("BudgetPublication.FailureReason = %q, want %q",
			result.BudgetPublication.FailureReason, groupPolicyPublicationNoBudgetTarget)
	}
	if alphaAfter := groupPolicyUpdatePartitionGeneration(t, group, "alpha"); alphaAfter != alphaBefore {
		t.Fatalf("partition generation changed without budget target: got %s want %s", alphaAfter, alphaBefore)
	}
}

func TestPoolGroupPublishPolicyDoesNotScanShards(t *testing.T) {
	source := groupPolicyUpdateSource(t)
	for _, disallowed := range []string{".shards", "mustClassStateFor", "PoolPartition.TickInto"} {
		if strings.Contains(source, disallowed) {
			t.Fatalf("group_policy_update.go contains %q; group publication must not scan shard/class internals", disallowed)
		}
	}
}

func TestPoolGroupPublishPolicyDoesNotExecutePoolTrimDirectly(t *testing.T) {
	source := groupPolicyUpdateSource(t)
	for _, disallowed := range []string{".Trim(", ".TrimClass(", ".TrimShard(", ".Get(", ".Put("} {
		if strings.Contains(source, disallowed) {
			t.Fatalf("group_policy_update.go contains %q; group publication must not call Pool data-plane/trim APIs", disallowed)
		}
	}
}

func TestPoolGroupPublishPolicyConcurrentWithClose(t *testing.T) {
	group := newGroupPolicyUpdateGroup(t, "alpha")
	policy := groupPolicyUpdatePolicy(KiB)
	start := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		<-start
		_, err := group.PublishPolicy(policy)
		done <- err
	}()

	close(start)
	closeErr := group.Close()
	publishErr := <-done
	if closeErr != nil && !errors.Is(closeErr, ErrClosed) {
		t.Fatalf("Close() error = %v", closeErr)
	}
	if publishErr != nil && !errors.Is(publishErr, ErrClosed) {
		t.Fatalf("PublishPolicy concurrent error = %v, want nil or %v", publishErr, ErrClosed)
	}
}

func TestPoolGroupPublishPolicyConcurrentWithTickInto(t *testing.T) {
	group := newGroupPolicyUpdateGroup(t, "alpha", "beta")
	var wg sync.WaitGroup
	errs := make(chan error, 40)
	for index := 0; index < 20; index++ {
		wg.Add(2)
		go func(iteration int) {
			defer wg.Done()
			_, err := group.PublishPolicy(groupPolicyUpdatePolicy(SizeFromBytes(uint64(iteration%2+2) * 1024)))
			errs <- err
		}(index)
		go func() {
			defer wg.Done()
			var report PoolGroupCoordinatorReport
			errs <- group.TickInto(&report)
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		requireGroupNoError(t, err)
	}
}

func TestPoolGroupPublishPolicyConcurrentWithAcquireRelease(t *testing.T) {
	group := newGroupPolicyUpdateGroup(t, "alpha")
	var wg sync.WaitGroup
	errs := make(chan error, 40)
	for index := 0; index < 20; index++ {
		wg.Add(2)
		go func(iteration int) {
			defer wg.Done()
			_, err := group.PublishPolicy(groupPolicyUpdatePolicy(SizeFromBytes(uint64(iteration%2+1) * 1024)))
			errs <- err
		}(index)
		go func() {
			defer wg.Done()
			lease, err := group.Acquire("alpha-pool", 300)
			if err != nil {
				errs <- err
				return
			}
			errs <- group.Release(lease, lease.Buffer())
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		requireGroupNoError(t, err)
	}
}

func newGroupPolicyUpdateGroup(t *testing.T, partitionNames ...string) *PoolGroup {
	t.Helper()

	group := MustNewPoolGroup(groupPolicyUpdateConfig(partitionNames...))
	t.Cleanup(func() {
		if err := group.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	})
	return group
}

func groupPolicyUpdateConfig(partitionNames ...string) PoolGroupConfig {
	config := testGroupConfig(partitionNames...)
	for partitionIndex := range config.Partitions {
		for poolIndex := range config.Partitions[partitionIndex].Config.Pools {
			config.Partitions[partitionIndex].Config.Pools[poolIndex].Config.Policy = poolTestSmallSingleShardPolicy()
		}
	}
	return config
}

func groupPolicyUpdatePolicy(retained Size) PoolGroupPolicy {
	policy := DefaultPoolGroupPolicy()
	policy.Budget.MaxRetainedBytes = retained
	return policy
}

func groupPolicyUpdatePartitionGeneration(t *testing.T, group *PoolGroup, partitionName string) Generation {
	t.Helper()
	snapshot, ok := group.PartitionSnapshot(partitionName)
	if !ok {
		t.Fatalf("PartitionSnapshot(%q) missing", partitionName)
	}
	return snapshot.PolicyGeneration
}

func groupPolicyUpdateSource(t *testing.T) string {
	t.Helper()
	source, err := os.ReadFile("group_policy_update.go")
	if err != nil {
		t.Fatalf("ReadFile(group_policy_update.go) error = %v", err)
	}
	return string(source)
}
