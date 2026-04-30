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

func TestPoolPartitionPublishPolicyPublishesRuntimeSnapshot(t *testing.T) {
	t.Parallel()

	partition := newPartitionPolicyUpdatePartition(t, "alpha", "beta")
	before := partition.Snapshot()

	result, err := partition.PublishPolicy(partitionPolicyUpdatePolicy(2*KiB, false))
	if err != nil {
		t.Fatalf("PublishPolicy() error = %v", err)
	}
	after := partition.Snapshot()
	if !result.Published || !result.RuntimePublished {
		t.Fatalf("PublishPolicy() = %+v, want runtime publication", result)
	}
	if after.PolicyGeneration != result.Generation || !after.PolicyGeneration.After(before.PolicyGeneration) {
		t.Fatalf("PolicyGeneration after PublishPolicy = %s result = %s before = %s",
			after.PolicyGeneration, result.Generation, before.PolicyGeneration)
	}
	if after.Policy.Budget.MaxRetainedBytes != 2*KiB {
		t.Fatalf("published retained target = %s, want 2 KiB", after.Policy.Budget.MaxRetainedBytes)
	}
	if !result.BudgetPublication.Published {
		t.Fatalf("budget publication = %+v, want published", result.BudgetPublication)
	}
}

func TestPoolPartitionUpdatePolicyWrapperReturnsError(t *testing.T) {
	t.Parallel()

	partition := newPartitionPolicyUpdatePartition(t, "alpha")
	policy := PartitionPolicy{Trim: PartitionTrimPolicy{Enabled: true}}
	if err := partition.UpdatePolicy(policy); !errors.Is(err, ErrInvalidPolicy) {
		t.Fatalf("UpdatePolicy(invalid trim) error = %v, want %v", err, ErrInvalidPolicy)
	}
}

func TestPoolPartitionPublishPolicyRejectsClosedPartition(t *testing.T) {
	t.Parallel()

	partition := MustNewPoolPartition(partitionPolicyUpdateConfig("alpha"))
	before := partition.Snapshot()
	if err := partition.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	result, err := partition.PublishPolicy(partitionPolicyUpdatePolicy(KiB, false))
	if !errors.Is(err, ErrClosed) {
		t.Fatalf("PublishPolicy(closed) error = %v, want %v", err, ErrClosed)
	}
	if result.Published || result.RuntimePublished || result.FailureReason != policyUpdateFailureClosed {
		t.Fatalf("PublishPolicy(closed) = %+v, want closed rejection", result)
	}
	if after := partition.Snapshot(); after.PolicyGeneration != before.PolicyGeneration {
		t.Fatalf("PolicyGeneration changed after closed rejection: got %s want %s", after.PolicyGeneration, before.PolicyGeneration)
	}
}

func TestPoolPartitionPublishPolicyRejectsPoolShapeChange(t *testing.T) {
	t.Parallel()

	partition := newPartitionPolicyUpdatePartition(t, "alpha")
	partitionPolicyUpdateCorruptPoolPolicy(t, partition, "alpha", func(policy *Policy) {
		policy.Shards.ShardsPerClass = 2
	})

	result, err := partition.PublishPolicy(partitionPolicyUpdatePolicy(KiB, false))
	if !errors.Is(err, ErrInvalidPolicy) {
		t.Fatalf("PublishPolicy(pool shape) error = %v, want %v", err, ErrInvalidPolicy)
	}
	if result.FailureReason != policyUpdateFailureShapeChange || !result.Diff.ShapeChanged || result.RuntimePublished {
		t.Fatalf("PublishPolicy(pool shape) = %+v, want shape rejection before runtime publication", result)
	}
}

func TestPoolPartitionPublishPolicyRejectsOwnershipChange(t *testing.T) {
	t.Parallel()

	partition := newPartitionPolicyUpdatePartition(t, "alpha")
	partitionPolicyUpdateCorruptPoolPolicy(t, partition, "alpha", func(policy *Policy) {
		policy.Ownership.Mode = OwnershipModeAccounting
		policy.Ownership.TrackInUseBytes = true
		policy.Ownership.TrackInUseBuffers = true
	})

	result, err := partition.PublishPolicy(partitionPolicyUpdatePolicy(KiB, false))
	if !errors.Is(err, ErrInvalidPolicy) {
		t.Fatalf("PublishPolicy(ownership) error = %v, want %v", err, ErrInvalidPolicy)
	}
	if result.FailureReason != policyUpdateFailureOwnershipChange || !result.Diff.OwnershipChanged || result.RuntimePublished {
		t.Fatalf("PublishPolicy(ownership) = %+v, want ownership rejection before runtime publication", result)
	}
}

func TestPoolPartitionPublishPolicyPreservesPressureSignal(t *testing.T) {
	t.Parallel()

	partition := newPartitionPolicyUpdatePartition(t, "alpha")
	signal := PressureSignal{Level: PressureLevelHigh, Source: PressureSourceManual, Generation: Generation(33)}
	if err := partition.applyPressure(signal); err != nil {
		t.Fatalf("applyPressure() error = %v", err)
	}

	policy := partitionPolicyUpdatePolicy(KiB, false)
	policy.Pressure = PartitionPressurePolicy{
		Enabled:            true,
		MediumOwnedBytes:   KiB,
		HighOwnedBytes:     2 * KiB,
		CriticalOwnedBytes: 4 * KiB,
	}
	result, err := partition.PublishPolicy(policy)
	if err != nil {
		t.Fatalf("PublishPolicy() error = %v", err)
	}
	if !result.PressurePreserved {
		t.Fatalf("PublishPolicy() = %+v, want pressure preserved", result)
	}
	if got := partition.currentRuntimeSnapshot().Pressure; got != signal {
		t.Fatalf("partition pressure after PublishPolicy = %+v, want %+v", got, signal)
	}
}

func TestPoolPartitionPublishPolicyContractsPoolBudgets(t *testing.T) {
	t.Parallel()

	partition := newPartitionPolicyUpdatePartition(t, "alpha", "beta")
	result, err := partition.PublishPolicy(partitionPolicyUpdatePolicy(KiB, false))
	if err != nil {
		t.Fatalf("PublishPolicy(contract) error = %v", err)
	}
	if !result.Contracted || !result.BudgetPublication.Published {
		t.Fatalf("PublishPolicy(contract) = %+v, want contracted budget publication", result)
	}
	var assigned uint64
	for _, target := range result.BudgetPublication.Targets {
		assigned = poolSaturatingAdd(assigned, target.RetainedBytes.Bytes())
	}
	if assigned > KiB.Bytes() {
		t.Fatalf("assigned Pool budgets = %d, want <= %d", assigned, KiB.Bytes())
	}
}

func TestPoolPartitionPublishPolicyContractsClassBudgets(t *testing.T) {
	t.Parallel()

	partition := newPartitionPolicyUpdatePartition(t, "alpha", "beta")
	result, err := partition.PublishPolicy(partitionPolicyUpdatePolicy(KiB, false))
	if err != nil {
		t.Fatalf("PublishPolicy(contract) error = %v", err)
	}
	if !result.BudgetPublication.Published {
		t.Fatalf("budget publication = %+v, want published", result.BudgetPublication)
	}
	var assigned uint64
	for _, poolSnapshot := range partition.Snapshot().Pools {
		assigned = poolSaturatingAdd(assigned, sumPoolClassBudgetTargets(poolSnapshot.Pool.Classes))
	}
	if assigned > KiB.Bytes() {
		t.Fatalf("assigned class budgets = %d, want <= %d", assigned, KiB.Bytes())
	}
}

func TestPoolPartitionPublishPolicyDoesNotCommitOnInfeasibleBudget(t *testing.T) {
	t.Parallel()

	partition := newPartitionPolicyUpdatePartition(t, "alpha", "beta")
	before := partition.Snapshot()
	beforeAlpha := before.Pools[0].Pool.Generation

	result, err := partition.PublishPolicy(partitionPolicyUpdatePolicy(SizeFromBytes(512), false))
	if !errors.Is(err, ErrInvalidPolicy) {
		t.Fatalf("PublishPolicy(infeasible) error = %v, want %v", err, ErrInvalidPolicy)
	}
	if result.Published || result.RuntimePublished || result.FailureReason != policyUpdateFailureInfeasibleBudget {
		t.Fatalf("PublishPolicy(infeasible) = %+v, want rejected infeasible budget", result)
	}
	after := partition.Snapshot()
	if after.PolicyGeneration != before.PolicyGeneration {
		t.Fatalf("partition policy generation changed after infeasible budget: got %s want %s", after.PolicyGeneration, before.PolicyGeneration)
	}
	if after.Pools[0].Pool.Generation != beforeAlpha {
		t.Fatalf("pool generation changed after infeasible budget: got %s want %s", after.Pools[0].Pool.Generation, beforeAlpha)
	}
}

func TestPoolPartitionPublishPolicyDoesNotForceActiveLeases(t *testing.T) {
	t.Parallel()

	partition := newPartitionPolicyUpdatePartition(t, "alpha")
	lease, err := partition.Acquire("alpha", 512)
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}

	result, err := partition.PublishPolicy(partitionPolicyUpdatePolicy(SizeFromBytes(512), true))
	if err != nil {
		t.Fatalf("PublishPolicy(contract) error = %v", err)
	}
	if result.TrimAttempted && result.TrimResult.TrimmedBuffers > 0 {
		t.Fatalf("trim removed retained buffers while only active lease existed: %+v", result.TrimResult)
	}
	if active := partition.Snapshot().Leases.ActiveCount(); active != 1 {
		t.Fatalf("active leases after PublishPolicy = %d, want 1", active)
	}
	requirePartitionNoError(t, partition.Release(lease, lease.Buffer()))
}

func TestPoolPartitionPublishPolicyTrimOnShrinkIsBounded(t *testing.T) {
	t.Parallel()

	partition := newPartitionPolicyUpdatePartition(t, "alpha")
	pool, ok := partition.pool("alpha")
	if !ok {
		t.Fatal("alpha pool missing")
	}
	seedPoolRetainedBuffers(t, pool, 4, 512)

	result, err := partition.PublishPolicy(partitionPolicyUpdatePolicy(SizeFromBytes(512), true))
	if err != nil {
		t.Fatalf("PublishPolicy(contract+trim) error = %v", err)
	}
	if !result.TrimAttempted || !result.TrimResult.Attempted {
		t.Fatalf("PublishPolicy(contract+trim) = %+v, want trim attempt", result)
	}
	if result.TrimResult.TrimmedBuffers > 1 || result.TrimResult.TrimmedBytes > 512 {
		t.Fatalf("trim result = %+v, want bounded to one 512-byte buffer", result.TrimResult)
	}
}

func TestPoolPartitionPublishPolicyWithoutTrimLeavesOverTargetRetained(t *testing.T) {
	t.Parallel()

	partition := newPartitionPolicyUpdatePartition(t, "alpha")
	pool, ok := partition.pool("alpha")
	if !ok {
		t.Fatal("alpha pool missing")
	}
	seedPoolRetainedBuffers(t, pool, 4, 512)
	before := pool.Metrics()

	result, err := partition.PublishPolicy(partitionPolicyUpdatePolicy(SizeFromBytes(512), false))
	if err != nil {
		t.Fatalf("PublishPolicy(contract without trim) error = %v", err)
	}
	after := pool.Metrics()
	if result.TrimAttempted {
		t.Fatalf("PublishPolicy(contract without trim) = %+v, want no trim", result)
	}
	if after.CurrentRetainedBytes != before.CurrentRetainedBytes || after.CurrentRetainedBuffers != before.CurrentRetainedBuffers {
		t.Fatalf("retained changed without trim: before=%+v after=%+v", before, after)
	}
}

func TestPoolPartitionPublishPolicyConcurrentWithClose(t *testing.T) {
	partition := MustNewPoolPartition(partitionPolicyUpdateConfig("alpha", "beta"))
	start := make(chan struct{})
	var wg sync.WaitGroup
	for index := 0; index < 8; index++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			policy := partitionPolicyUpdatePolicy(KiB, index%2 == 0)
			_, err := partition.PublishPolicy(policy)
			if err != nil && !errors.Is(err, ErrClosed) {
				t.Errorf("PublishPolicy concurrent error = %v, want nil or %v", err, ErrClosed)
			}
		}(index)
	}
	close(start)
	if err := partition.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	wg.Wait()
}

func TestPoolPartitionPublishPolicyConcurrentWithAcquireRelease(t *testing.T) {
	partition := newPartitionPolicyUpdatePartition(t, "alpha", "beta")
	var wg sync.WaitGroup
	errs := make(chan error, 32)
	for index := 0; index < 8; index++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			for iteration := 0; iteration < 16; iteration++ {
				name := "alpha"
				if (index+iteration)%2 == 1 {
					name = "beta"
				}
				lease, err := partition.Acquire(name, 512)
				if err != nil {
					errs <- err
					return
				}
				if err := partition.Release(lease, lease.Buffer()); err != nil {
					errs <- err
					return
				}
			}
		}(index)
	}
	for iteration := 0; iteration < 16; iteration++ {
		if _, err := partition.PublishPolicy(partitionPolicyUpdatePolicy(KiB, iteration%2 == 0)); err != nil {
			t.Fatalf("PublishPolicy() error = %v", err)
		}
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("Acquire/Release concurrent error = %v", err)
	}
}

func TestPoolPartitionPublishPolicyConcurrentWithTickInto(t *testing.T) {
	partition := newPartitionPolicyUpdatePartition(t, "alpha", "beta")
	var wg sync.WaitGroup
	errs := make(chan error, 32)
	for index := 0; index < 4; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var report PartitionControllerReport
			for iteration := 0; iteration < 16; iteration++ {
				if err := partition.TickInto(&report); err != nil {
					errs <- err
					return
				}
			}
		}()
	}
	for iteration := 0; iteration < 16; iteration++ {
		if _, err := partition.PublishPolicy(partitionPolicyUpdatePolicy(KiB, iteration%2 == 0)); err != nil {
			t.Fatalf("PublishPolicy() error = %v", err)
		}
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("TickInto concurrent error = %v", err)
	}
}

func TestPoolPartitionPublishPolicyDoesNotCallPoolGetPut(t *testing.T) {
	t.Parallel()

	content, err := os.ReadFile("partition_policy_update.go")
	if err != nil {
		t.Fatalf("read partition_policy_update.go: %v", err)
	}
	source := string(content)
	for _, forbidden := range []string{".Get(", ".GetSize(", ".Put("} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("partition_policy_update.go contains data-plane call %q", forbidden)
		}
	}
}

func newPartitionPolicyUpdatePartition(t *testing.T, poolNames ...string) *PoolPartition {
	t.Helper()

	partition := MustNewPoolPartition(partitionPolicyUpdateConfig(poolNames...))
	t.Cleanup(func() {
		if err := partition.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	})
	return partition
}

func partitionPolicyUpdateConfig(poolNames ...string) PoolPartitionConfig {
	config := testPartitionConfig(poolNames...)
	for index := range config.Pools {
		config.Pools[index].Config.Policy = poolTestSmallSingleShardPolicy()
	}
	return config
}

func partitionPolicyUpdatePolicy(retained Size, trimOnShrink bool) PartitionPolicy {
	return PartitionPolicy{
		Budget: PartitionBudgetPolicy{
			MaxRetainedBytes: retained,
		},
		Trim: partitionPolicyUpdateTrimPolicy(trimOnShrink),
	}
}

func partitionPolicyUpdateTrimPolicy(trimOnShrink bool) PartitionTrimPolicy {
	return PartitionTrimPolicy{
		Enabled:                   true,
		MaxPoolsPerCycle:          1,
		MaxBuffersPerCycle:        1,
		MaxBytesPerCycle:          SizeFromBytes(512),
		MaxClassesPerPoolPerCycle: 1,
		MaxShardsPerClassPerCycle: 1,
		TrimOnPolicyShrink:        trimOnShrink,
	}
}

func partitionPolicyUpdateCorruptPoolPolicy(
	t *testing.T,
	partition *PoolPartition,
	poolName string,
	mutate func(*Policy),
) {
	t.Helper()

	pool, ok := partition.pool(poolName)
	if !ok {
		t.Fatalf("%s pool missing", poolName)
	}
	runtime := pool.currentRuntimeSnapshot()
	policy := runtime.clonePolicy()
	mutate(&policy)
	pool.runtimeSnapshot.Store(newPoolRuntimeSnapshotWithPressure(runtime.Generation.Next(), policy, runtime.Pressure))
}
