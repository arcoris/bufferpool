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
	"reflect"
	"testing"
)

func TestPoolPartitionApplyPoolBudgetTargetsUpdatesOwnedPool(t *testing.T) {
	t.Parallel()

	partition := MustNewPoolPartition(poolBudgetTestPartitionConfig())
	t.Cleanup(func() {
		requirePartitionNoError(t, partition.Close())
	})

	partition.markDirtyProcessed()
	beforeDirtyGeneration := partition.activeRegistry.generationSnapshot()
	err := partition.applyPoolBudgetTargets([]PoolBudgetTarget{
		{
			Generation: Generation(80),
			PoolName:   "primary",
			ClassTargets: []ClassBudgetTarget{
				{Generation: Generation(80), ClassID: ClassID(0), TargetBytes: 2 * KiB},
				{Generation: Generation(80), ClassID: ClassID(1), TargetBytes: 4 * KiB},
			},
		},
	})
	if err != nil {
		t.Fatalf("applyPoolBudgetTargets() returned error: %v", err)
	}

	snapshot, ok := partition.PoolSnapshot("primary")
	if !ok {
		t.Fatal("PoolSnapshot(primary) not found")
	}
	if snapshot.Generation.Before(Generation(80)) {
		t.Fatalf("pool generation = %s, want >= 80", snapshot.Generation)
	}
	assertPoolClassBudgetSnapshot(t, snapshot.Classes[0].Budget, Generation(80), 2*KiB)
	assertPoolClassBudgetSnapshot(t, snapshot.Classes[1].Budget, Generation(80), 4*KiB)

	afterDirtyGeneration := partition.activeRegistry.generationSnapshot()
	if !afterDirtyGeneration.After(beforeDirtyGeneration) {
		t.Fatalf("active registry generation = %s, want after %s", afterDirtyGeneration, beforeDirtyGeneration)
	}
	dirtyIndexes := partition.controllerDirtyIndexes(nil)
	if len(dirtyIndexes) != 1 || dirtyIndexes[0] != 0 {
		t.Fatalf("dirty indexes = %v, want [0]", dirtyIndexes)
	}
}

func TestPoolPartitionApplyPoolBudgetTargetsRejectsMissingPool(t *testing.T) {
	t.Parallel()

	partition := MustNewPoolPartition(poolBudgetTestPartitionConfig())
	t.Cleanup(func() {
		requirePartitionNoError(t, partition.Close())
	})

	err := partition.applyPoolBudgetTargets([]PoolBudgetTarget{{Generation: Generation(81), PoolName: "missing", RetainedBytes: KiB}})
	if err == nil {
		t.Fatal("applyPoolBudgetTargets(missing) returned nil")
	}
	if !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("applyPoolBudgetTargets(missing) error = %v, want ErrInvalidOptions", err)
	}
}

// TestPoolPartitionApplyPoolBudgetTargetsRejectsMissingPoolWithoutPartialMutation
// verifies pool budget batches validate all targets before mutating any owned
// Pool.
func TestPoolPartitionApplyPoolBudgetTargetsRejectsMissingPoolWithoutPartialMutation(t *testing.T) {
	t.Parallel()

	config := poolBudgetTestPartitionConfig()
	config.Pools = append(config.Pools, PartitionPoolConfig{Name: "secondary", Config: PoolConfig{Policy: poolTestSmallSingleShardPolicy()}})
	partition := MustNewPoolPartition(config)
	t.Cleanup(func() {
		requirePartitionNoError(t, partition.Close())
	})

	before, ok := partition.PoolSnapshot("primary")
	if !ok {
		t.Fatal("PoolSnapshot(primary) not found")
	}
	err := partition.applyPoolBudgetTargets([]PoolBudgetTarget{
		{
			Generation: Generation(82),
			PoolName:   "primary",
			ClassTargets: []ClassBudgetTarget{
				{Generation: Generation(82), ClassID: ClassID(0), TargetBytes: 2 * KiB},
			},
		},
		{Generation: Generation(82), PoolName: "missing", RetainedBytes: KiB},
	})
	if !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("applyPoolBudgetTargets(partial invalid) error = %v, want ErrInvalidOptions", err)
	}
	after, ok := partition.PoolSnapshot("primary")
	if !ok {
		t.Fatal("PoolSnapshot(primary) after apply not found")
	}
	if after.Generation != before.Generation {
		t.Fatalf("primary generation changed after rejected batch: got %s want %s", after.Generation, before.Generation)
	}
}

func TestPoolGroupApplyPartitionBudgetTargetsUpdatesPartitionPolicy(t *testing.T) {
	t.Parallel()

	group := MustNewPoolGroup(PoolGroupConfig{
		Partitions: []GroupPartitionConfig{
			{Name: "alpha", Config: poolBudgetTestPartitionConfig()},
		},
	})
	t.Cleanup(func() {
		requireGroupNoError(t, group.Close())
	})

	err := group.applyPartitionBudgetTargets([]PartitionBudgetTarget{
		{Generation: Generation(90), PartitionName: "alpha", RetainedBytes: 12 * KiB},
	})
	if err != nil {
		t.Fatalf("applyPartitionBudgetTargets() returned error: %v", err)
	}

	snapshot, ok := group.PartitionSnapshot("alpha")
	if !ok {
		t.Fatal("PartitionSnapshot(alpha) not found")
	}
	if snapshot.PolicyGeneration.Before(Generation(90)) {
		t.Fatalf("partition policy generation = %s, want >= 90", snapshot.PolicyGeneration)
	}
	if snapshot.Policy.Budget.MaxRetainedBytes != 12*KiB {
		t.Fatalf("MaxRetainedBytes = %s, want 12 KiB", snapshot.Policy.Budget.MaxRetainedBytes)
	}
}

func TestPoolGroupApplyPartitionBudgetTargetsRejectsMissingPartition(t *testing.T) {
	t.Parallel()

	group := MustNewPoolGroup(PoolGroupConfig{
		Partitions: []GroupPartitionConfig{
			{Name: "alpha", Config: poolBudgetTestPartitionConfig()},
		},
	})
	t.Cleanup(func() {
		requireGroupNoError(t, group.Close())
	})

	err := group.applyPartitionBudgetTargets([]PartitionBudgetTarget{{Generation: Generation(91), PartitionName: "missing", RetainedBytes: KiB}})
	if err == nil {
		t.Fatal("applyPartitionBudgetTargets(missing) returned nil")
	}
	if !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("applyPartitionBudgetTargets(missing) error = %v, want ErrInvalidOptions", err)
	}
}

func TestPoolApplyBudgetReportsInfeasibleClassBudget(t *testing.T) {
	t.Parallel()

	pool := MustNew(PoolConfig{Policy: poolTestSmallSingleShardPolicy()})
	t.Cleanup(func() {
		if err := pool.Close(); err != nil {
			t.Fatalf("Pool.Close() error = %v", err)
		}
	})

	before := pool.Snapshot()
	target := PoolBudgetTarget{
		Generation:    Generation(120),
		RetainedBytes: SizeFromBytes(512),
		ClassTargets: []ClassBudgetTarget{
			{Generation: Generation(120), ClassID: ClassID(0), TargetBytes: 2 * KiB},
		},
	}
	report, err := pool.planPoolBudget(target)
	if err != nil {
		t.Fatalf("planPoolBudget() error = %v", err)
	}
	if report.Allocation.Feasible {
		t.Fatalf("class allocation feasible = true, want infeasible")
	}
	if _, err := pool.applyPoolBudget(target); !errors.Is(err, ErrInvalidPolicy) {
		t.Fatalf("applyPoolBudget(infeasible) error = %v, want ErrInvalidPolicy", err)
	}
	after := pool.Snapshot()
	if after.Generation != before.Generation {
		t.Fatalf("pool generation after rejected budget = %s, want %s", after.Generation, before.Generation)
	}
}

func TestApplyPoolBudgetTargetsRejectsInfeasibleWithoutPartialMutation(t *testing.T) {
	t.Parallel()

	config := poolBudgetTestPartitionConfig()
	config.Pools = append(config.Pools, PartitionPoolConfig{Name: "secondary", Config: PoolConfig{Policy: poolTestSmallSingleShardPolicy()}})
	partition := MustNewPoolPartition(config)
	t.Cleanup(func() { requirePartitionNoError(t, partition.Close()) })

	beforePrimary, _ := partition.PoolSnapshot("primary")
	beforeSecondary, _ := partition.PoolSnapshot("secondary")
	err := partition.applyPoolBudgetTargets([]PoolBudgetTarget{
		{
			Generation:    Generation(121),
			PoolName:      "primary",
			RetainedBytes: SizeFromBytes(512),
			ClassTargets: []ClassBudgetTarget{
				{Generation: Generation(121), ClassID: ClassID(0), TargetBytes: 2 * KiB},
			},
		},
		{
			Generation:    Generation(121),
			PoolName:      "secondary",
			RetainedBytes: KiB,
		},
	})
	if !errors.Is(err, ErrInvalidPolicy) {
		t.Fatalf("applyPoolBudgetTargets(infeasible) error = %v, want ErrInvalidPolicy", err)
	}
	afterPrimary, _ := partition.PoolSnapshot("primary")
	afterSecondary, _ := partition.PoolSnapshot("secondary")
	if afterPrimary.Generation != beforePrimary.Generation || afterSecondary.Generation != beforeSecondary.Generation {
		t.Fatalf("pool generations changed after rejected batch: primary %s/%s secondary %s/%s",
			beforePrimary.Generation,
			afterPrimary.Generation,
			beforeSecondary.Generation,
			afterSecondary.Generation,
		)
	}
}

func TestApplyPoolBudgetTargetsCannotPartiallyMutateAfterSuccessfulPlanning(t *testing.T) {
	t.Parallel()

	config := poolBudgetTestPartitionConfig()
	config.Pools = append(config.Pools, PartitionPoolConfig{Name: "secondary", Config: PoolConfig{Policy: poolTestSmallSingleShardPolicy()}})
	partition := MustNewPoolPartition(config)
	t.Cleanup(func() { requirePartitionNoError(t, partition.Close()) })

	secondary, ok := partition.registry.pool("secondary")
	if !ok {
		t.Fatal("secondary pool not found")
	}
	requirePartitionNoError(t, secondary.Close())

	beforePrimary, _ := partition.PoolSnapshot("primary")
	err := partition.applyPoolBudgetTargets([]PoolBudgetTarget{
		{
			Generation: Generation(122),
			PoolName:   "primary",
			ClassTargets: []ClassBudgetTarget{
				{Generation: Generation(122), ClassID: ClassID(0), TargetBytes: 2 * KiB},
			},
		},
		{
			Generation:    Generation(122),
			PoolName:      "secondary",
			RetainedBytes: KiB,
		},
	})
	if !errors.Is(err, ErrClosed) {
		t.Fatalf("applyPoolBudgetTargets(closed child) error = %v, want ErrClosed", err)
	}
	afterPrimary, _ := partition.PoolSnapshot("primary")
	if afterPrimary.Generation != beforePrimary.Generation {
		t.Fatalf("primary generation changed after closed-child batch: got %s want %s", afterPrimary.Generation, beforePrimary.Generation)
	}
}

func TestApplyPoolBudgetTargetsReportClassPublicationStatus(t *testing.T) {
	t.Parallel()

	partition := MustNewPoolPartition(poolBudgetTestPartitionConfig())
	t.Cleanup(func() { requirePartitionNoError(t, partition.Close()) })

	if err := partition.beginForegroundOperation(); err != nil {
		t.Fatalf("beginForegroundOperation() error = %v", err)
	}
	report, err := partition.applyPoolBudgetTargetsLocked([]PoolBudgetTarget{
		{
			Generation:    Generation(123),
			PoolName:      "primary",
			RetainedBytes: 4 * KiB,
		},
	})
	partition.endForegroundOperation()
	if err != nil {
		t.Fatalf("applyPoolBudgetTargetsLocked() error = %v", err)
	}
	if !report.Published || len(report.ClassReports) != 1 || !report.ClassReports[0].Published {
		t.Fatalf("publication report = %+v, want published class report", report)
	}
	if len(report.Targets) != 1 || len(report.Targets[0].ClassTargets) == 0 {
		t.Fatalf("publication targets = %+v, want planned class targets", report.Targets)
	}
}

func TestBudgetFeasibilityVisibleInControllerReports(t *testing.T) {
	t.Parallel()

	partition := MustNewPoolPartition(poolBudgetTestPartitionConfig())
	t.Cleanup(func() { requirePartitionNoError(t, partition.Close()) })

	var report PartitionControllerReport
	requirePartitionNoError(t, partition.TickInto(&report))
	if !report.BudgetPublication.Allocation.Feasible {
		t.Fatalf("controller budget allocation = %+v, want feasible", report.BudgetPublication.Allocation)
	}
	if !report.BudgetPublication.Published {
		t.Fatalf("controller budget publication = %+v, want published", report.BudgetPublication)
	}
	if len(report.BudgetPublication.ClassReports) == 0 {
		t.Fatalf("controller budget publication class reports missing")
	}
}

func TestPoolPartitionTickBudgetPublicationErrorDoesNotCommitRuntimeBudgetState(t *testing.T) {
	t.Parallel()

	partition := MustNewPoolPartition(poolBudgetTestPartitionConfig())
	t.Cleanup(func() { requirePartitionNoError(t, partition.Close()) })

	if err := partition.activeRegistry.markDirty("primary"); err != nil {
		t.Fatalf("markDirty(primary) error = %v", err)
	}
	pool, ok := partition.registry.pool("primary")
	if !ok {
		t.Fatal("primary pool not found")
	}
	requirePartitionNoError(t, pool.Close())

	beforeGeneration := partition.controller.generation.Load()
	beforeHasPrevious := partition.controller.hasPreviousSample
	beforePreviousTime := partition.controller.previousSampleTime
	beforeEWMA := partition.controller.ewma
	beforeClassEWMA := copyPoolClassEWMAStateMap(partition.controller.ewmaByPoolClass)
	beforeDirty := partition.controllerDirtyIndexes(nil)

	var report PartitionControllerReport
	requirePartitionErrorIs(t, partition.TickInto(&report), ErrClosed)
	if report.Generation.IsZero() {
		t.Fatal("Generation is zero, want observed tick attempt")
	}
	if report.Status.Status != ControllerCycleStatusFailed {
		t.Fatalf("Status = %+v, want failed budget publication", report.Status)
	}
	if report.BudgetPublication.Published {
		t.Fatalf("BudgetPublication.Published = true, want false: %+v", report.BudgetPublication)
	}
	if report.BudgetPublication.FailureReason != controllerCycleReasonClosed {
		t.Fatalf("BudgetPublication.FailureReason = %q, want %q", report.BudgetPublication.FailureReason, controllerCycleReasonClosed)
	}
	if report.TrimResult.Attempted || report.TrimResult.Executed {
		t.Fatalf("TrimResult = %+v, want no trim after failed budget publication", report.TrimResult)
	}
	if after := partition.controller.generation.Load(); after != beforeGeneration {
		t.Fatalf("controller generation = %s, want unchanged %s", after, beforeGeneration)
	}
	if partition.controller.hasPreviousSample != beforeHasPrevious {
		t.Fatalf("hasPreviousSample = %v, want %v", partition.controller.hasPreviousSample, beforeHasPrevious)
	}
	if partition.controller.previousSampleTime != beforePreviousTime {
		t.Fatalf("previousSampleTime changed after unpublished tick")
	}
	if partition.controller.ewma != beforeEWMA {
		t.Fatalf("controller EWMA changed after unpublished tick")
	}
	if !reflect.DeepEqual(partition.controller.ewmaByPoolClass, beforeClassEWMA) {
		t.Fatalf("class EWMA changed after unpublished tick")
	}
	afterDirty := partition.controllerDirtyIndexes(nil)
	if !reflect.DeepEqual(afterDirty, beforeDirty) {
		t.Fatalf("dirty indexes = %v, want unchanged %v", afterDirty, beforeDirty)
	}
}

func TestPoolPartitionTickSuccessfulBudgetCommitsControllerState(t *testing.T) {
	t.Parallel()

	partition := MustNewPoolPartition(poolBudgetTestPartitionConfig())
	t.Cleanup(func() { requirePartitionNoError(t, partition.Close()) })

	if err := partition.activeRegistry.markDirty("primary"); err != nil {
		t.Fatalf("markDirty(primary) error = %v", err)
	}
	beforeGeneration := partition.controller.generation.Load()

	var report PartitionControllerReport
	requirePartitionNoError(t, partition.TickInto(&report))
	if !report.BudgetPublication.Published {
		t.Fatalf("BudgetPublication = %+v, want published", report.BudgetPublication)
	}
	if after := partition.controller.generation.Load(); !after.After(beforeGeneration) {
		t.Fatalf("controller generation = %s, want after %s", after, beforeGeneration)
	}
	if !partition.controller.hasPreviousSample {
		t.Fatal("hasPreviousSample = false, want committed previous sample")
	}
	if !partition.controller.ewma.Initialized {
		t.Fatal("controller EWMA not initialized after successful tick")
	}
	if dirty := partition.controllerDirtyIndexes(nil); len(dirty) != 0 {
		t.Fatalf("dirty indexes = %v, want processed", dirty)
	}
}

func poolBudgetTestPartitionConfig() PoolPartitionConfig {
	return PoolPartitionConfig{
		Name: "alpha",
		Pools: []PartitionPoolConfig{
			{Name: "primary", Config: PoolConfig{Policy: poolTestSmallSingleShardPolicy()}},
		},
	}
}
