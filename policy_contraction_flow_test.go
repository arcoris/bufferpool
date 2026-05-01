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
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestPoolGroupPublishPolicyEndToEndContraction(t *testing.T) {
	group := newGroupPolicyUpdateGroup(t, "alpha", "beta")
	retainGroupBuffers(t, group, "alpha-pool", 4, 300)
	retainGroupBuffers(t, group, "beta-pool", 4, 300)
	before := group.Metrics()
	if before.CurrentRetainedBytes == 0 {
		t.Fatalf("test setup retained bytes = 0")
	}

	result, err := group.PublishPolicy(groupPolicyUpdatePolicy(KiB))
	requireGroupNoError(t, err)
	if !result.Published || !result.BudgetPublication.Published {
		t.Fatalf("PublishPolicy() = %+v, want group-to-partition budget publication", result)
	}
	if len(result.BudgetPublication.Targets) != 2 {
		t.Fatalf("targets = %+v, want two partition targets", result.BudgetPublication.Targets)
	}

	tickAllGroupPartitions(t, group)
	for _, partitionName := range group.PartitionNames() {
		partition, ok := group.partition(partitionName)
		if !ok {
			t.Fatalf("partition %q missing", partitionName)
		}
		snapshot := partition.Snapshot()
		if snapshot.Policy.Budget.MaxRetainedBytes > SizeFromBytes(512) {
			t.Fatalf("%s retained budget = %s, want <= 512 B", partitionName, snapshot.Policy.Budget.MaxRetainedBytes)
		}
		if len(snapshot.Pools) != 1 {
			t.Fatalf("%s pool snapshots = %d, want 1", partitionName, len(snapshot.Pools))
		}
		poolSnapshot := snapshot.Pools[0].Pool
		if assigned := sumPoolClassBudgetTargets(poolSnapshot.Classes); assigned > 512 {
			t.Fatalf("%s class budget bytes = %d, want <= 512", snapshot.Pools[0].Name, assigned)
		}
		if credit := poolSnapshot.Classes[0].Shards[0].Credit.TargetBytes; credit > 512 {
			t.Fatalf("%s shard credit bytes = %d, want <= 512", snapshot.Pools[0].Name, credit)
		}
	}

	alpha, ok := group.partition("alpha")
	if !ok {
		t.Fatal("alpha partition missing")
	}
	alphaPool, ok := alpha.pool("alpha-pool")
	if !ok {
		t.Fatal("alpha pool missing")
	}
	retainedBefore := alphaPool.Metrics().CurrentRetainedBytes
	lease, err := group.Acquire("alpha-pool", 300)
	requireGroupNoError(t, err)
	requireGroupNoError(t, group.Release(lease, lease.Buffer()))
	retainedAfter := alphaPool.Metrics().CurrentRetainedBytes
	if retainedAfter > retainedBefore {
		t.Fatalf("future return grew retained bytes after contraction: before=%d after=%d", retainedBefore, retainedAfter)
	}
}

func TestPolicyContractionDoesNotForceActiveLease(t *testing.T) {
	partition := newPartitionPolicyUpdatePartition(t, "alpha")
	lease, err := partition.Acquire("alpha", 900)
	requirePartitionNoError(t, err)

	result, err := partition.PublishPolicy(partitionPolicyUpdatePolicy(SizeFromBytes(512), true))
	requirePartitionNoError(t, err)
	if !result.Published || !result.Contracted {
		t.Fatalf("PublishPolicy() = %+v, want contracted publication", result)
	}
	if active := partition.Snapshot().Leases.ActiveCount(); active != 1 {
		t.Fatalf("active leases after contraction = %d, want 1", active)
	}

	requirePartitionNoError(t, partition.Release(lease, lease.Buffer()))
}

func TestPolicyContractionLateReleaseCompletesOwnership(t *testing.T) {
	partition := newPartitionPolicyUpdatePartition(t, "alpha")
	lease, err := partition.Acquire("alpha", 900)
	requirePartitionNoError(t, err)
	result, err := partition.PublishPolicy(partitionPolicyUpdatePolicy(SizeFromBytes(512), false))
	requirePartitionNoError(t, err)
	if !result.Published || !result.Contracted {
		t.Fatalf("PublishPolicy() = %+v, want contracted publication", result)
	}

	requirePartitionNoError(t, partition.Release(lease, lease.Buffer()))
	if active := partition.Snapshot().Leases.ActiveCount(); active != 0 {
		t.Fatalf("active leases after late release = %d, want 0", active)
	}
}

func TestPolicyContractionReturnedBufferFollowsNewPolicy(t *testing.T) {
	partition := newPartitionPolicyUpdatePartition(t, "alpha")
	pool, ok := partition.pool("alpha")
	if !ok {
		t.Fatal("alpha pool missing")
	}
	lease, err := partition.Acquire("alpha", 900)
	requirePartitionNoError(t, err)

	signal := PressureSignal{Level: PressureLevelHigh, Source: PressureSourceManual, Generation: Generation(103)}
	requirePoolPolicyNoError(t, pool.applyPressure(signal))
	policy := poolPolicyUpdateContractedPolicy(false)
	policy.Pressure = poolTestPressurePolicy()
	policy.Pressure.High.DisableRetention = true
	result, err := pool.PublishPolicy(policy)
	requirePoolPolicyNoError(t, err)
	if !result.Published || !result.Contracted || !result.PressurePreserved {
		t.Fatalf("Pool.PublishPolicy() = %+v, want contracted publication preserving pressure", result)
	}

	beforeDrops := pool.Metrics().Drops
	beforeRetained := pool.Metrics().CurrentRetainedBytes
	requirePartitionNoError(t, partition.Release(lease, lease.Buffer()))
	after := partition.Snapshot()
	if after.Leases.ActiveCount() != 0 {
		t.Fatalf("active leases after release = %d, want 0", after.Leases.ActiveCount())
	}
	poolMetrics := pool.Metrics()
	if poolMetrics.Drops <= beforeDrops {
		t.Fatalf("Pool drops after pressure-policy release = %d before = %d, want dropped return", poolMetrics.Drops, beforeDrops)
	}
	if poolMetrics.CurrentRetainedBytes != beforeRetained {
		t.Fatalf("retained bytes after pressure-policy release = %d before = %d, want no retained growth",
			poolMetrics.CurrentRetainedBytes, beforeRetained)
	}
}

func TestPoolPolicyContractionTrimOnShrinkEndToEnd(t *testing.T) {
	pool := MustNew(PoolConfig{Policy: poolTestSmallSingleShardPolicy()})
	defer closePoolForTest(t, pool)
	seedPoolRetainedBuffers(t, pool, 4, 512)

	result, err := pool.PublishPolicy(poolPolicyUpdateContractedPolicy(true))
	requirePoolPolicyNoError(t, err)
	if !result.TrimAttempted || result.TrimResult.TrimmedBuffers != 1 || result.TrimResult.TrimmedBytes > 512 {
		t.Fatalf("PublishPolicy(trim) = %+v, want one bounded trim", result)
	}
}

// TestPolicyContractionWithAdaptiveTrimStillBounded verifies live contraction
// uses the same scored Pool trim path as ordinary corrective trim while still
// respecting per-cycle limits and exposing candidate diagnostics.
func TestPolicyContractionWithAdaptiveTrimStillBounded(t *testing.T) {
	pool := MustNew(PoolConfig{Policy: poolTestSmallSingleShardPolicy()})
	defer closePoolForTest(t, pool)
	seedPoolRetainedBuffers(t, pool, 4, 512)

	result, err := pool.PublishPolicy(poolPolicyUpdateContractedPolicy(true))
	requirePoolPolicyNoError(t, err)
	if !result.TrimAttempted || !result.TrimResult.Executed {
		t.Fatalf("PublishPolicy(trim) = %+v, want adaptive trim-on-shrink execution", result)
	}
	if result.TrimResult.TrimmedBuffers > 1 || result.TrimResult.TrimmedBytes > 512 {
		t.Fatalf("TrimResult = %+v, want contraction trim bounded by policy limits", result.TrimResult)
	}
	if len(result.TrimResult.CandidateClasses) == 0 {
		t.Fatalf("TrimResult = %+v, want candidate score diagnostics", result.TrimResult)
	}
	assertScoreValueFiniteAndClamped(t, result.TrimResult.CandidateClasses[0].Score.Value)
}

func TestPartitionPolicyContractionTrimOnShrinkEndToEnd(t *testing.T) {
	partition := newPartitionPolicyUpdatePartition(t, "alpha")
	pool, ok := partition.pool("alpha")
	if !ok {
		t.Fatal("alpha pool missing")
	}
	seedPoolRetainedBuffers(t, pool, 4, 512)

	result, err := partition.PublishPolicy(partitionPolicyUpdatePolicy(SizeFromBytes(512), true))
	requirePartitionNoError(t, err)
	if !result.TrimAttempted || result.TrimResult.TrimmedBuffers != 1 || result.TrimResult.TrimmedBytes > 512 {
		t.Fatalf("PublishPolicy(trim) = %+v, want one bounded partition trim", result)
	}
}

func TestGroupPolicyContractionTrimOnShrinkThroughPartitions(t *testing.T) {
	config := groupPolicyUpdateConfig("alpha")
	config.Partitions[0].Config.Policy.Trim = PartitionTrimPolicy{
		Enabled:                   true,
		MaxPoolsPerCycle:          1,
		MaxBuffersPerCycle:        1,
		MaxBytesPerCycle:          SizeFromBytes(512),
		MaxClassesPerPoolPerCycle: 1,
		MaxShardsPerClassPerCycle: 1,
	}
	group := MustNewPoolGroup(config)
	t.Cleanup(func() { requireGroupNoError(t, group.Close()) })
	retainGroupBuffers(t, group, "alpha-pool", 4, 300)
	before := group.Metrics().CurrentRetainedBytes

	result, err := group.PublishPolicy(groupPolicyUpdatePolicy(SizeFromBytes(512)))
	requireGroupNoError(t, err)
	if !result.Published || !result.BudgetPublication.Published {
		t.Fatalf("PublishPolicy() = %+v, want group contraction", result)
	}
	if afterGroup := group.Metrics().CurrentRetainedBytes; afterGroup != before {
		t.Fatalf("group PublishPolicy trimmed directly: before=%d after=%d", before, afterGroup)
	}

	partition, ok := group.partition("alpha")
	if !ok {
		t.Fatal("alpha partition missing")
	}
	trim := partition.ExecuteTrim()
	if !trim.Executed || trim.TrimmedBuffers != 1 || trim.TrimmedBytes > 512 {
		t.Fatalf("ExecuteTrim() = %+v, want one bounded partition trim", trim)
	}
}

func TestPolicyContractionWithoutTrimLeavesRetainedOverTarget(t *testing.T) {
	pool := MustNew(PoolConfig{Policy: poolTestSmallSingleShardPolicy()})
	defer closePoolForTest(t, pool)
	seedPoolRetainedBuffers(t, pool, 4, 512)
	before := pool.Metrics().CurrentRetainedBytes

	result, err := pool.PublishPolicy(poolPolicyUpdateContractedPolicy(false))
	requirePoolPolicyNoError(t, err)
	if result.TrimAttempted {
		t.Fatalf("PublishPolicy(no trim) = %+v, want no trim", result)
	}
	if after := pool.Metrics().CurrentRetainedBytes; after != before {
		t.Fatalf("retained changed without trim: before=%d after=%d", before, after)
	}
}

func TestPolicyContractionWithoutTrimRejectsFutureOverTargetRetention(t *testing.T) {
	pool := MustNew(PoolConfig{Policy: poolTestSmallSingleShardPolicy()})
	defer closePoolForTest(t, pool)
	seedPoolRetainedBuffers(t, pool, 4, 512)
	result, err := pool.PublishPolicy(poolPolicyUpdateContractedPolicy(false))
	requirePoolPolicyNoError(t, err)
	if !result.Published || !result.Contracted {
		t.Fatalf("PublishPolicy() = %+v, want contracted publication", result)
	}
	before := pool.Metrics().CurrentRetainedBytes

	if err := pool.Put(make([]byte, 0, 512)); err != nil {
		t.Fatalf("Put() after no-trim contraction error = %v", err)
	}
	if after := pool.Metrics().CurrentRetainedBytes; after != before {
		t.Fatalf("future return changed retained bytes over target: before=%d after=%d", before, after)
	}
}

func TestPoolPolicyUpdateRejectsShapeMutation(t *testing.T) {
	pool := MustNew(PoolConfig{Policy: poolTestSmallSingleShardPolicy()})
	defer closePoolForTest(t, pool)

	tests := []struct {
		name   string
		mutate func(*Policy)
	}{
		{name: "class size", mutate: func(policy *Policy) {
			policy.Classes.Sizes = []ClassSize{ClassSizeFromBytes(256), ClassSizeFromBytes(512)}
		}},
		{name: "shard count", mutate: func(policy *Policy) { policy.Shards.ShardsPerClass = 2 }},
		{name: "bucket segment", mutate: func(policy *Policy) { policy.Shards.BucketSegmentSlotsPerShard++ }},
		{name: "ownership", mutate: func(policy *Policy) { policy.Ownership.DetectDoubleRelease = true }},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			candidate := pool.Snapshot().Policy
			tt.mutate(&candidate)
			_, err := pool.PublishPolicy(candidate)
			requirePoolPolicyErrorIs(t, err, ErrInvalidPolicy)
		})
	}
}

func TestPartitionPolicyUpdateRejectsOwnedPoolShapeMutation(t *testing.T) {
	partition := newPartitionPolicyUpdatePartition(t, "alpha")
	partitionPolicyUpdateCorruptPoolPolicy(t, partition, "alpha", func(policy *Policy) {
		policy.Shards.BucketSegmentSlotsPerShard++
	})

	result, err := partition.PublishPolicy(partitionPolicyUpdatePolicy(KiB, false))
	requirePartitionErrorIs(t, err, ErrInvalidPolicy)
	if result.RuntimePublished || !result.Diff.ShapeChanged {
		t.Fatalf("PublishPolicy(shape) = %+v, want rejected owned Pool shape mutation", result)
	}
}

func TestGroupPolicyUpdateRejectsTopologyMutationIfApplicable(t *testing.T) {
	typ := reflect.TypeOf(PoolGroupPolicy{})
	for _, field := range []string{"Pools", "Partitions", "Partitioning"} {
		if _, ok := typ.FieldByName(field); ok {
			t.Fatalf("PoolGroupPolicy exposes topology field %q; live topology mutation must be rejected before publication", field)
		}
	}
}

func TestPoolPolicyUpdatePreservesPressure(t *testing.T) {
	policy := poolTestSmallSingleShardPolicy()
	policy.Pressure = poolTestPressurePolicy()
	pool := MustNew(PoolConfig{Policy: policy})
	defer closePoolForTest(t, pool)
	signal := PressureSignal{Level: PressureLevelHigh, Source: PressureSourceManual, Generation: Generation(101)}
	requirePoolPolicyNoError(t, pool.applyPressure(signal))

	candidate := policy
	candidate.Pressure.High.DisableRetention = true
	result, err := pool.PublishPolicy(candidate)
	requirePoolPolicyNoError(t, err)
	if !result.PressurePreserved || pool.currentRuntimeSnapshot().Pressure != signal {
		t.Fatalf("PublishPolicy() = %+v pressure = %+v, want preserved %+v", result, pool.currentRuntimeSnapshot().Pressure, signal)
	}
	before := pool.Metrics().Drops
	requirePoolPolicyNoError(t, pool.Put(make([]byte, 0, 512)))
	if after := pool.Metrics().Drops; after <= before {
		t.Fatalf("drops after preserved high pressure = %d before = %d, want new policy pressure admission", after, before)
	}
}

func TestPartitionPolicyUpdatePreservesPressure(t *testing.T) {
	partition := newPartitionPolicyUpdatePartition(t, "alpha")
	signal := PressureSignal{Level: PressureLevelHigh, Source: PressureSourceManual, Generation: Generation(102)}
	requirePartitionNoError(t, partition.applyPressure(signal))

	policy := partitionPolicyUpdatePolicy(KiB, false)
	policy.Pressure = PartitionPressurePolicy{Enabled: true, HighOwnedBytes: KiB}
	result, err := partition.PublishPolicy(policy)
	requirePartitionNoError(t, err)
	if !result.PressurePreserved || partition.currentRuntimeSnapshot().Pressure != signal {
		t.Fatalf("PublishPolicy() = %+v pressure = %+v, want preserved %+v", result, partition.currentRuntimeSnapshot().Pressure, signal)
	}
}

func TestGroupPolicyUpdatePreservesPressure(t *testing.T) {
	group := newGroupPolicyUpdateGroup(t, "alpha")
	publication, err := group.PublishPressure(PressureLevelHigh)
	requireGroupNoError(t, err)
	if !publication.FullyApplied() {
		t.Fatalf("PublishPressure() = %+v, want fully applied", publication)
	}
	signal := group.currentRuntimeSnapshot().Pressure

	policy := groupPolicyUpdatePolicy(KiB)
	policy.Pressure = PartitionPressurePolicy{Enabled: true, HighOwnedBytes: KiB}
	result, err := group.PublishPolicy(policy)
	requireGroupNoError(t, err)
	if !result.PressurePreserved || group.currentRuntimeSnapshot().Pressure != signal {
		t.Fatalf("PublishPolicy() = %+v pressure = %+v, want preserved %+v", result, group.currentRuntimeSnapshot().Pressure, signal)
	}
}

func TestPoolPublishPolicyConcurrentWithGetPut(t *testing.T) {
	pool := MustNew(PoolConfig{Policy: poolTestSmallSingleShardPolicy()})
	defer closePoolForTest(t, pool)
	var wg sync.WaitGroup
	errs := make(chan error, 64)
	for index := 0; index < 4; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for iteration := 0; iteration < 16; iteration++ {
				buffer, err := pool.Get(300)
				if err != nil {
					errs <- err
					return
				}
				if err := pool.Put(buffer); err != nil {
					errs <- err
					return
				}
			}
		}()
	}
	for iteration := 0; iteration < 16; iteration++ {
		policy := poolPolicyUpdateExpandedPolicy()
		if iteration%2 == 1 {
			policy = poolTestSmallSingleShardPolicy()
		}
		if _, err := pool.PublishPolicy(policy); err != nil {
			t.Fatalf("PublishPolicy() error = %v", err)
		}
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("Get/Put concurrent error = %v", err)
	}
}

func TestPartitionPublishPolicyConcurrentWithClose(t *testing.T) {
	partition := MustNewPoolPartition(partitionPolicyUpdateConfig("alpha"))
	start := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		<-start
		_, err := partition.PublishPolicy(partitionPolicyUpdatePolicy(KiB, false))
		done <- err
	}()
	close(start)
	closeErr := partition.Close()
	publishErr := <-done
	if closeErr != nil {
		t.Fatalf("Close() error = %v", closeErr)
	}
	if publishErr != nil && !errors.Is(publishErr, ErrClosed) {
		t.Fatalf("PublishPolicy concurrent error = %v, want nil or %v", publishErr, ErrClosed)
	}
}

func TestPartitionPublishPolicyConcurrentWithAcquireRelease(t *testing.T) {
	partition := newPartitionPolicyUpdatePartition(t, "alpha")
	var wg sync.WaitGroup
	errs := make(chan error, 32)
	for index := 0; index < 4; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for iteration := 0; iteration < 16; iteration++ {
				lease, err := partition.Acquire("alpha", 300)
				if err != nil {
					errs <- err
					return
				}
				if err := partition.Release(lease, lease.Buffer()); err != nil {
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
		t.Fatalf("Acquire/Release concurrent error = %v", err)
	}
}

func TestPartitionPublishPolicyConcurrentWithTickInto(t *testing.T) {
	partition := newPartitionPolicyUpdatePartition(t, "alpha")
	var wg sync.WaitGroup
	errs := make(chan error, 16)
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

func TestGroupPublishPolicyConcurrentWithClose(t *testing.T) {
	group := MustNewPoolGroup(groupPolicyUpdateConfig("alpha"))
	start := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		<-start
		_, err := group.PublishPolicy(groupPolicyUpdatePolicy(KiB))
		done <- err
	}()
	close(start)
	closeErr := group.Close()
	publishErr := <-done
	if closeErr != nil {
		t.Fatalf("Close() error = %v", closeErr)
	}
	if publishErr != nil && !errors.Is(publishErr, ErrClosed) {
		t.Fatalf("PublishPolicy concurrent error = %v, want nil or %v", publishErr, ErrClosed)
	}
}

func TestGroupPublishPolicyConcurrentWithAcquireRelease(t *testing.T) {
	group := newGroupPolicyUpdateGroup(t, "alpha")
	var wg sync.WaitGroup
	errs := make(chan error, 32)
	for index := 0; index < 4; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for iteration := 0; iteration < 16; iteration++ {
				lease, err := group.Acquire("alpha-pool", 300)
				if err != nil {
					errs <- err
					return
				}
				if err := group.Release(lease, lease.Buffer()); err != nil {
					errs <- err
					return
				}
			}
		}()
	}
	for iteration := 0; iteration < 16; iteration++ {
		if _, err := group.PublishPolicy(groupPolicyUpdatePolicy(KiB)); err != nil {
			t.Fatalf("PublishPolicy() error = %v", err)
		}
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("Acquire/Release concurrent error = %v", err)
	}
}

func TestGroupPublishPolicyConcurrentWithTickInto(t *testing.T) {
	group := newGroupPolicyUpdateGroup(t, "alpha")
	var wg sync.WaitGroup
	errs := make(chan error, 16)
	wg.Add(1)
	go func() {
		defer wg.Done()
		var report PoolGroupCoordinatorReport
		for iteration := 0; iteration < 16; iteration++ {
			if err := group.TickInto(&report); err != nil {
				errs <- err
				return
			}
		}
	}()
	for iteration := 0; iteration < 16; iteration++ {
		if _, err := group.PublishPolicy(groupPolicyUpdatePolicy(KiB)); err != nil {
			t.Fatalf("PublishPolicy() error = %v", err)
		}
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("TickInto concurrent error = %v", err)
	}
}

func TestPolicyUpdateDoesNotEnterPoolHotPath(t *testing.T) {
	for _, file := range []string{"pool_get.go", "pool_put.go"} {
		source := readPolicyFlowSource(t, file)
		for _, forbidden := range []string{
			"PublishPolicy",
			"UpdatePolicy",
			"policyUpdate",
			"partitionController",
			"groupCoordinator",
			"ExecuteTrim",
			"PlanTrim",
			"activeRegistry",
		} {
			if strings.Contains(source, forbidden) {
				t.Fatalf("%s contains %q; Pool hot path must not enter policy-update/control-plane code", file, forbidden)
			}
		}
	}
}

func TestGroupPolicyUpdateDoesNotScanPoolShards(t *testing.T) {
	source := readPolicyFlowSource(t, "group_policy_update.go")
	for _, forbidden := range []string{".shards", "mustClassStateFor", "PoolShard", "ClassState"} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("group_policy_update.go contains %q; group policy update must not scan Pool shard/class internals", forbidden)
		}
	}
}

func TestGroupPolicyUpdateDoesNotExecutePoolTrimDirectly(t *testing.T) {
	source := readPolicyFlowSource(t, "group_policy_update.go")
	for _, forbidden := range []string{".Trim(", ".TrimClass(", ".TrimShard("} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("group_policy_update.go contains %q; group policy update must not execute Pool trim directly", forbidden)
		}
	}
}

func TestPartitionPolicyUpdateOwnsManagedPoolControlCalls(t *testing.T) {
	source := policyFlowSourceWithoutLineComments(readPolicyFlowSource(t, "partition_policy_update.go"))
	for _, required := range []string{"planPartitionPolicyBudgetBatchLocked", "applyPlannedPoolBudgetBatchLocked", "executeTrimPlan"} {
		if !strings.Contains(source, required) {
			t.Fatalf("partition_policy_update.go missing %q; partition policy update must own managed Pool control calls", required)
		}
	}
	if strings.Contains(source, "PoolGroup") {
		t.Fatalf("partition_policy_update.go references PoolGroup; partition policy update must stay partition-local")
	}
}

func retainGroupBuffers(t *testing.T, group *PoolGroup, poolName string, count int, size int) {
	t.Helper()
	leases := make([]Lease, count)
	for index := range leases {
		lease, err := group.Acquire(poolName, size)
		requireGroupNoError(t, err)
		leases[index] = lease
	}
	for _, lease := range leases {
		requireGroupNoError(t, group.Release(lease, lease.Buffer()))
	}
}

func tickAllGroupPartitions(t *testing.T, group *PoolGroup) {
	t.Helper()
	for _, partitionName := range group.PartitionNames() {
		partition, ok := group.partition(partitionName)
		if !ok {
			t.Fatalf("partition %q missing", partitionName)
		}
		var report PartitionControllerReport
		requirePartitionNoError(t, partition.TickInto(&report))
		if !report.BudgetPublication.Published {
			t.Fatalf("partition %q budget publication = %+v, want published", partitionName, report.BudgetPublication)
		}
	}
}

func readPolicyFlowSource(t *testing.T, file string) string {
	t.Helper()
	source, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", file, err)
	}
	return string(source)
}

func policyFlowSourceWithoutLineComments(source string) string {
	lines := strings.Split(source, "\n")
	var builder strings.Builder
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "//") {
			continue
		}
		builder.WriteString(line)
		builder.WriteByte('\n')
	}
	return builder.String()
}

func requirePoolPolicyNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func requirePoolPolicyErrorIs(t *testing.T, err error, target error) {
	t.Helper()
	if !errors.Is(err, target) {
		t.Fatalf("error = %v, want errors.Is(..., %v)", err, target)
	}
}
