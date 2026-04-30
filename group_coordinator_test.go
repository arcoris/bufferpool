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
	"reflect"
	"testing"
	"time"

	"arcoris.dev/bufferpool/internal/clock"
)

func TestPoolGroupTickInto(t *testing.T) {
	group := testNewPoolGroup(t, "alpha", "beta")
	before := group.Sample().Generation
	var report PoolGroupCoordinatorReport
	requireGroupNoError(t, group.TickInto(&report))
	if report.Generation != before.Next() {
		t.Fatalf("Generation = %s, want %s", report.Generation, before.Next())
	}
	if report.Sample.PartitionCount != 2 {
		t.Fatalf("Sample.PartitionCount = %d, want 2", report.Sample.PartitionCount)
	}
	if report.Metrics.PartitionCount != 2 {
		t.Fatalf("Metrics.PartitionCount = %d, want 2", report.Metrics.PartitionCount)
	}
}

func TestPoolGroupTickIntoNilDstNoop(t *testing.T) {
	group := testNewPoolGroup(t, "alpha")
	before := group.Sample().Generation
	requireGroupNoError(t, group.TickInto(nil))
	if after := group.Sample().Generation; after != before {
		t.Fatalf("TickInto(nil) advanced generation from %s to %s", before, after)
	}
}

func TestPoolGroupTickReturnsReport(t *testing.T) {
	group := testNewPoolGroup(t, "alpha")
	report, err := group.Tick()
	requireGroupNoError(t, err)
	if report.Sample.PartitionCount != 1 {
		t.Fatalf("Sample.PartitionCount = %d, want 1", report.Sample.PartitionCount)
	}
}

func TestPoolGroupTickDoesNotMutateGroupPolicy(t *testing.T) {
	group := testNewPoolGroup(t, "alpha")
	before := group.Policy()
	_, err := group.Tick()
	requireGroupNoError(t, err)
	after := group.Policy()
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("Tick mutated policy: before=%#v after=%#v", before, after)
	}
}

// TestPoolGroupTickIntoFirstCycleHasNoWindowActivity verifies the first applied
// cycle uses its current sample as the window baseline.
func TestPoolGroupTickIntoFirstCycleHasNoWindowActivity(t *testing.T) {
	group := testNewPoolGroup(t, "alpha")
	lease, err := group.Acquire("alpha-pool", 300)
	requireGroupNoError(t, err)
	requireGroupNoError(t, group.Release(lease, lease.Buffer()))

	var report PoolGroupCoordinatorReport
	requireGroupNoError(t, group.TickInto(&report))
	if report.Scores.Activity != 0 {
		t.Fatalf("TickInto activity score = %v, want zero without a previous sample", report.Scores.Activity)
	}
	if report.Sample.Aggregate.LeaseCounters.Acquisitions != 1 || report.Sample.Aggregate.LeaseCounters.Releases != 1 {
		t.Fatalf("TickInto sample counters = %+v, want real observation", report.Sample.Aggregate.LeaseCounters)
	}
}

// TestPoolGroupTickIntoDoesNotPublishWithoutRetainedBudget verifies unbounded
// groups do not publish meaningless zero targets.
func TestPoolGroupTickIntoDoesNotPublishWithoutRetainedBudget(t *testing.T) {
	group := testNewPoolGroup(t, "alpha", "beta")
	before := make(map[string]PartitionPolicy)
	for _, name := range group.PartitionNames() {
		partition, ok := group.partition(name)
		if !ok {
			t.Fatalf("missing partition %q", name)
		}
		before[name] = partition.Policy()
	}

	_, err := group.Tick()
	requireGroupNoError(t, err)

	for _, name := range group.PartitionNames() {
		partition, ok := group.partition(name)
		if !ok {
			t.Fatalf("missing partition %q", name)
		}
		after := partition.Policy()
		if !reflect.DeepEqual(before[name], after) {
			t.Fatalf("Tick mutated partition %q policy: before=%#v after=%#v", name, before[name], after)
		}
	}
}

func TestPoolGroupTickIntoPublishesPartitionBudgetTargets(t *testing.T) {
	group := testNewBudgetedPoolGroup(t, 1*MiB, "alpha", "beta")

	before, ok := group.PartitionSnapshot("alpha")
	if !ok {
		t.Fatalf("PartitionSnapshot(alpha) not found")
	}

	var report PoolGroupCoordinatorReport
	requireGroupNoError(t, group.TickInto(&report))
	if report.PublishedGeneration != report.Generation {
		t.Fatalf("PublishedGeneration = %s, want %s", report.PublishedGeneration, report.Generation)
	}
	if !report.BudgetPublication.Published {
		t.Fatalf("BudgetPublication = %+v, want published", report.BudgetPublication)
	}
	if report.BudgetPublication.Generation != report.Generation {
		t.Fatalf("BudgetPublication.Generation = %s, want %s", report.BudgetPublication.Generation, report.Generation)
	}
	if !report.BudgetPublication.Allocation.Feasible {
		t.Fatalf("BudgetPublication.Allocation = %+v, want feasible", report.BudgetPublication.Allocation)
	}
	if report.CoordinatorGeneration.IsZero() {
		t.Fatalf("CoordinatorGeneration is zero")
	}
	if len(report.PartitionScores) != 2 {
		t.Fatalf("len(PartitionScores) = %d, want 2", len(report.PartitionScores))
	}
	if len(report.PartitionBudgetTargets) != 2 {
		t.Fatalf("len(PartitionBudgetTargets) = %d, want 2", len(report.PartitionBudgetTargets))
	}
	if len(report.SkippedPartitions) != 0 {
		t.Fatalf("SkippedPartitions = %+v, want empty", report.SkippedPartitions)
	}
	if total := sumPartitionBudgetTargets(report.PartitionBudgetTargets); total > 1*MiB {
		t.Fatalf("target total = %s, want <= 1 MiB", total)
	}

	alphaTarget, ok := partitionBudgetTargetByName(report.PartitionBudgetTargets, "alpha")
	if !ok {
		t.Fatalf("alpha target not found")
	}
	alphaSnapshot, ok := group.PartitionSnapshot("alpha")
	if !ok {
		t.Fatalf("PartitionSnapshot(alpha) not found after tick")
	}
	if !alphaSnapshot.PolicyGeneration.After(before.PolicyGeneration) {
		t.Fatalf("alpha policy generation = %s, want after %s", alphaSnapshot.PolicyGeneration, before.PolicyGeneration)
	}
	if alphaSnapshot.Policy.Budget.MaxRetainedBytes != alphaTarget.RetainedBytes {
		t.Fatalf("alpha MaxRetainedBytes = %s, want target %s", alphaSnapshot.Policy.Budget.MaxRetainedBytes, alphaTarget.RetainedBytes)
	}
}

func TestPoolGroupTickUnpublishedBudgetReportFieldsAreConsistent(t *testing.T) {
	group := testNewBudgetedPoolGroup(t, 1*MiB, "alpha", "beta")
	alphaBefore, ok := group.PartitionSnapshot("alpha")
	if !ok {
		t.Fatal("PartitionSnapshot(alpha) not found")
	}
	beta, ok := group.partition("beta")
	if !ok {
		t.Fatal("partition beta not found")
	}
	requirePartitionNoError(t, beta.Close())

	var report PoolGroupCoordinatorReport
	requireGroupNoError(t, group.TickInto(&report))
	if report.Generation.IsZero() {
		t.Fatal("Generation is zero, want observed tick attempt")
	}
	if report.CoordinatorGeneration.IsZero() {
		t.Fatal("CoordinatorGeneration is zero, want committed observation")
	}
	if report.PublishedGeneration != NoGeneration {
		t.Fatalf("PublishedGeneration = %s, want NoGeneration", report.PublishedGeneration)
	}
	if report.BudgetPublication.Published {
		t.Fatalf("BudgetPublication.Published = true, want false: %+v", report.BudgetPublication)
	}
	if !report.BudgetPublication.Allocation.Feasible {
		t.Fatalf("BudgetPublication.Allocation = %+v, want feasible allocation with skipped child", report.BudgetPublication.Allocation)
	}
	if report.BudgetPublication.FailureReason == "" {
		t.Fatalf("BudgetPublication.FailureReason empty: %+v", report.BudgetPublication)
	}
	if len(report.SkippedPartitions) != 1 || report.SkippedPartitions[0].PartitionName != "beta" {
		t.Fatalf("SkippedPartitions = %+v, want beta", report.SkippedPartitions)
	}
	alphaAfter, ok := group.PartitionSnapshot("alpha")
	if !ok {
		t.Fatal("PartitionSnapshot(alpha) after tick not found")
	}
	if alphaAfter.PolicyGeneration != alphaBefore.PolicyGeneration {
		t.Fatalf("alpha policy generation changed on skipped publication: got %s want %s", alphaAfter.PolicyGeneration, alphaBefore.PolicyGeneration)
	}
}

func TestPoolGroupTickIntoZeroScorePartitionsGetEqualTargets(t *testing.T) {
	group := testNewBudgetedPoolGroup(t, 1*MiB, "alpha", "beta")

	var report PoolGroupCoordinatorReport
	requireGroupNoError(t, group.TickInto(&report))

	alpha, ok := partitionBudgetTargetByName(report.PartitionBudgetTargets, "alpha")
	if !ok {
		t.Fatalf("alpha target not found")
	}
	beta, ok := partitionBudgetTargetByName(report.PartitionBudgetTargets, "beta")
	if !ok {
		t.Fatalf("beta target not found")
	}
	if alpha.RetainedBytes != beta.RetainedBytes {
		t.Fatalf("targets = alpha %s beta %s, want equal", alpha.RetainedBytes, beta.RetainedBytes)
	}
}

func TestPoolGroupTickIntoUsesPreviousWindowForPartitionScores(t *testing.T) {
	group := testNewBudgetedPoolGroup(t, 1*MiB, "alpha", "beta")
	manual := clock.NewManual(time.Date(2026, time.April, 30, 12, 0, 0, 0, time.UTC))
	group.coordinator.clock = manual

	var first PoolGroupCoordinatorReport
	requireGroupNoError(t, group.TickInto(&first))

	manual.Advance(time.Second)
	lease, err := group.Acquire("alpha-pool", 300)
	requireGroupNoError(t, err)
	requireGroupNoError(t, group.Release(lease, lease.Buffer()))

	var second PoolGroupCoordinatorReport
	requireGroupNoError(t, group.TickInto(&second))

	alphaScore, ok := partitionScoreByName(second.PartitionScores, "alpha")
	if !ok {
		t.Fatalf("alpha score not found")
	}
	betaScore, ok := partitionScoreByName(second.PartitionScores, "beta")
	if !ok {
		t.Fatalf("beta score not found")
	}
	if alphaScore.Score <= betaScore.Score {
		t.Fatalf("scores = alpha %.6f beta %.6f, want alpha greater", alphaScore.Score, betaScore.Score)
	}
	alphaTarget, ok := partitionBudgetTargetByName(second.PartitionBudgetTargets, "alpha")
	if !ok {
		t.Fatalf("alpha target not found")
	}
	betaTarget, ok := partitionBudgetTargetByName(second.PartitionBudgetTargets, "beta")
	if !ok {
		t.Fatalf("beta target not found")
	}
	if alphaTarget.RetainedBytes <= betaTarget.RetainedBytes {
		t.Fatalf("targets = alpha %s beta %s, want alpha greater", alphaTarget.RetainedBytes, betaTarget.RetainedBytes)
	}
	if second.Window.Delta.Aggregate.LeaseAcquisitions == 0 {
		t.Fatalf("window lease acquisitions = 0, want previous/current movement")
	}
	if second.Rates.Aggregate.LeaseOpsPerSecond == 0 {
		t.Fatalf("LeaseOpsPerSecond = 0, want elapsed-time rate")
	}
}

func TestPoolGroupTickIntoPublishedTargetsDrivePartitionTick(t *testing.T) {
	group := testNewBudgetedPoolGroup(t, 512*KiB, "alpha")

	var groupReport PoolGroupCoordinatorReport
	requireGroupNoError(t, group.TickInto(&groupReport))
	if len(groupReport.PartitionBudgetTargets) != 1 {
		t.Fatalf("len(PartitionBudgetTargets) = %d, want 1", len(groupReport.PartitionBudgetTargets))
	}

	partition, ok := group.partition("alpha")
	if !ok {
		t.Fatalf("partition alpha not found")
	}
	var partitionReport PartitionControllerReport
	requirePartitionNoError(t, partition.TickInto(&partitionReport))
	if len(partitionReport.PoolBudgetTargets) == 0 {
		t.Fatalf("partition PoolBudgetTargets empty")
	}
	if partition.Policy().Budget.MaxRetainedBytes != groupReport.PartitionBudgetTargets[0].RetainedBytes {
		t.Fatalf("partition retained budget = %s, want group target %s", partition.Policy().Budget.MaxRetainedBytes, groupReport.PartitionBudgetTargets[0].RetainedBytes)
	}
	if total := sumPoolBudgetTargets(partitionReport.PoolBudgetTargets); total > groupReport.PartitionBudgetTargets[0].RetainedBytes {
		t.Fatalf("pool target total = %s, want <= partition target %s", total, groupReport.PartitionBudgetTargets[0].RetainedBytes)
	}
}

func TestPoolGroupTickIntoManualAndAutoConfigsPublishTargets(t *testing.T) {
	t.Run("manual", func(t *testing.T) {
		group := testNewBudgetedPoolGroup(t, 1*MiB, "alpha", "beta")
		var report PoolGroupCoordinatorReport
		requireGroupNoError(t, group.TickInto(&report))
		if len(report.PartitionBudgetTargets) != len(group.PartitionNames()) {
			t.Fatalf("len(PartitionBudgetTargets) = %d, want %d", len(report.PartitionBudgetTargets), len(group.PartitionNames()))
		}
	})

	t.Run("auto", func(t *testing.T) {
		config := testManagedGroupConfig("api", "worker", "events", "batch")
		config.Policy.Budget.MaxRetainedBytes = 1 * MiB
		config.Partitioning.MinPartitions = 2
		config.Partitioning.MaxPartitions = 2
		group, err := NewPoolGroup(config)
		requireGroupNoError(t, err)
		t.Cleanup(func() { requireGroupNoError(t, group.Close()) })

		var report PoolGroupCoordinatorReport
		requireGroupNoError(t, group.TickInto(&report))
		if len(report.PartitionBudgetTargets) != len(group.PartitionNames()) {
			t.Fatalf("len(PartitionBudgetTargets) = %d, want %d", len(report.PartitionBudgetTargets), len(group.PartitionNames()))
		}
	})
}

func TestAllocatePartitionBudgetTargetsClampMinMax(t *testing.T) {
	targets := allocatePartitionBudgetTargets(Generation(10), 10*KiB, []partitionBudgetAllocationInput{
		{PartitionName: "alpha", MinRetainedBytes: 4 * KiB, MaxRetainedBytes: 4 * KiB, Score: 1},
		{PartitionName: "beta", MaxRetainedBytes: 2 * KiB, Score: 1},
		{PartitionName: "gamma", Score: 1},
	})
	if len(targets) != 3 {
		t.Fatalf("len(targets) = %d, want 3", len(targets))
	}
	alpha, _ := partitionBudgetTargetByName(targets, "alpha")
	beta, _ := partitionBudgetTargetByName(targets, "beta")
	if alpha.RetainedBytes != 4*KiB {
		t.Fatalf("alpha target = %s, want 4 KiB", alpha.RetainedBytes)
	}
	if beta.RetainedBytes > 2*KiB {
		t.Fatalf("beta target = %s, want <= 2 KiB", beta.RetainedBytes)
	}
	if total := sumPartitionBudgetTargets(targets); total > 10*KiB {
		t.Fatalf("target total = %s, want <= 10 KiB", total)
	}
}

func testNewBudgetedPoolGroup(t *testing.T, retained Size, partitionNames ...string) *PoolGroup {
	t.Helper()
	config := testGroupConfig(partitionNames...)
	config.Policy.Budget.MaxRetainedBytes = retained
	group, err := NewPoolGroup(config)
	requireGroupNoError(t, err)
	t.Cleanup(func() {
		requireGroupNoError(t, group.Close())
	})
	return group
}

func partitionBudgetTargetByName(targets []PartitionBudgetTarget, name string) (PartitionBudgetTarget, bool) {
	for _, target := range targets {
		if target.PartitionName == name {
			return target, true
		}
	}
	return PartitionBudgetTarget{}, false
}

func partitionScoreByName(scores []PoolGroupPartitionScore, name string) (PoolGroupPartitionScore, bool) {
	for _, score := range scores {
		if score.PartitionName == name {
			return score, true
		}
	}
	return PoolGroupPartitionScore{}, false
}

func sumPartitionBudgetTargets(targets []PartitionBudgetTarget) Size {
	var total uint64
	for _, target := range targets {
		total = poolSaturatingAdd(total, target.RetainedBytes.Bytes())
	}
	return SizeFromBytes(total)
}

func sumPoolBudgetTargets(targets []PoolBudgetTarget) Size {
	var total uint64
	for _, target := range targets {
		total = poolSaturatingAdd(total, target.RetainedBytes.Bytes())
	}
	return SizeFromBytes(total)
}
