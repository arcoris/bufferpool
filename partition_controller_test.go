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
	"testing"
	"time"

	"arcoris.dev/bufferpool/internal/clock"
)

// TestPoolPartitionTickReportsCurrentStateAndAdvancesGeneration verifies tick report coherence.
func TestPoolPartitionTickReportsCurrentStateAndAdvancesGeneration(t *testing.T) {
	config := testPartitionConfig("primary")
	// Keep the opt-in scheduler disabled here so the test measures one explicit
	// manual Tick call. Scheduler dispatch uses the same TickInto path and has
	// separate integration coverage.
	config.Policy.Trim = PartitionTrimPolicy{Enabled: true, MaxPoolsPerCycle: 1, MaxBytesPerCycle: 4 * KiB}
	partition, err := NewPoolPartition(config)
	requirePartitionNoError(t, err)
	t.Cleanup(func() { requirePartitionNoError(t, partition.Close()) })

	before := partition.Sample().Generation
	report, err := partition.Tick()
	requirePartitionNoError(t, err)

	if !report.Generation.After(before) {
		t.Fatalf("Tick generation = %s, want after %s", report.Generation, before)
	}
	if report.Generation != before.Next() {
		t.Fatalf("Tick generation = %s, want exactly %s", report.Generation, before.Next())
	}
	if report.PolicyGeneration != InitialGeneration {
		t.Fatalf("PolicyGeneration = %s, want %s", report.PolicyGeneration, InitialGeneration)
	}
	if report.Sample.Generation != report.Generation {
		t.Fatalf("sample/report generation = %s/%s", report.Sample.Generation, report.Generation)
	}
	if report.Metrics.Generation != report.Generation {
		t.Fatalf("metrics/report generation = %s/%s", report.Metrics.Generation, report.Generation)
	}
	if report.Sample.PolicyGeneration != report.PolicyGeneration || report.Metrics.PolicyGeneration != report.PolicyGeneration {
		t.Fatalf("policy generation mismatch: report=%s sample=%s metrics=%s", report.PolicyGeneration, report.Sample.PolicyGeneration, report.Metrics.PolicyGeneration)
	}
	if report.Lifecycle != LifecycleActive {
		t.Fatalf("Tick lifecycle = %s, want active", report.Lifecycle)
	}
	if report.Sample.PoolCount != 1 || report.Metrics.PoolCount != 1 {
		t.Fatalf("Tick pool count mismatch: sample=%d metrics=%d", report.Sample.PoolCount, report.Metrics.PoolCount)
	}
	if !report.TrimPlan.Enabled {
		t.Fatalf("Tick trim plan should be enabled")
	}
	if !report.TrimResult.Attempted || report.TrimResult.Executed {
		t.Fatalf("Tick trim result = %+v, want attempted bounded trim without retained removals", report.TrimResult)
	}
}

// TestPoolPartitionTickIntoReusesReportStorage verifies reusable tick reports.
func TestPoolPartitionTickIntoReusesReportStorage(t *testing.T) {
	config := testPartitionConfig("primary", "secondary")
	config.Policy.Trim = PartitionTrimPolicy{Enabled: true, MaxPoolsPerCycle: 1, MaxBytesPerCycle: 4 * KiB}
	partition, err := NewPoolPartition(config)
	requirePartitionNoError(t, err)
	t.Cleanup(func() { requirePartitionNoError(t, partition.Close()) })

	report := PartitionControllerReport{
		Sample: PoolPartitionSample{
			Pools: make([]PoolPartitionPoolSample, 0, 8),
		},
	}
	requirePartitionNoError(t, partition.TickInto(&report))

	if report.Generation.IsZero() || report.Sample.Generation != report.Generation || report.Metrics.Generation != report.Generation {
		t.Fatalf("TickInto report has incoherent generation: %+v", report)
	}
	if report.PolicyGeneration != report.Sample.PolicyGeneration || report.PolicyGeneration != report.Metrics.PolicyGeneration {
		t.Fatalf("TickInto policy generation mismatch: %+v", report)
	}
	if len(report.Sample.Pools) != 2 {
		t.Fatalf("len(report.Sample.Pools) = %d, want 2", len(report.Sample.Pools))
	}
	if cap(report.Sample.Pools) != 8 {
		t.Fatalf("cap(report.Sample.Pools) = %d, want reused capacity 8", cap(report.Sample.Pools))
	}
	if !report.TrimResult.Attempted || report.TrimResult.Executed {
		t.Fatalf("TickInto trim result = %+v, want attempted bounded trim without retained removals", report.TrimResult)
	}

	report.Sample.Pools = append(report.Sample.Pools, PoolPartitionPoolSample{Name: "stale"})
	requirePartitionNoError(t, partition.TickInto(&report))
	if len(report.Sample.Pools) != 2 {
		t.Fatalf("reused len(report.Sample.Pools) = %d, want 2 without stale entries", len(report.Sample.Pools))
	}
	for _, pool := range report.Sample.Pools {
		if pool.Name == "stale" {
			t.Fatalf("TickInto retained stale pool entry")
		}
	}
}

// TestPoolPartitionTickIntoNilDestinationIsNoOp verifies nil destination behavior.
func TestPoolPartitionTickIntoNilDestinationIsNoOp(t *testing.T) {
	partition := testNewPoolPartition(t, "primary")
	before := partition.Sample().Generation

	requirePartitionNoError(t, partition.TickInto(nil))

	after := partition.Sample().Generation
	if after != before {
		t.Fatalf("TickInto(nil) advanced generation from %s to %s", before, after)
	}
}

// TestPoolPartitionTickIntoConsumesDirtyMarkers verifies applied controller semantics.
func TestPoolPartitionTickIntoConsumesDirtyMarkers(t *testing.T) {
	partition := testNewPoolPartition(t, "primary")
	partition.activeRegistry.resetDirty()
	lease, err := partition.Acquire("primary", 300)
	requirePartitionNoError(t, err)
	defer func() { requirePartitionNoError(t, partition.Release(lease, lease.Buffer())) }()

	if dirty := partition.activeRegistry.dirtyIndexes(nil); len(dirty) != 1 {
		t.Fatalf("dirty before TickInto = %v, want one dirty pool", dirty)
	}

	var report PartitionControllerReport
	requirePartitionNoError(t, partition.TickInto(&report))

	if dirty := partition.activeRegistry.dirtyIndexes(nil); len(dirty) != 0 {
		t.Fatalf("dirty after TickInto = %v, want consumed", dirty)
	}
	if len(report.PoolBudgetTargets) != 1 || len(report.PoolBudgetTargets[0].ClassTargets) == 0 {
		t.Fatalf("PoolBudgetTargets = %+v, want applied class targets", report.PoolBudgetTargets)
	}
}

// TestPoolPartitionTickRejectsClosedPartition verifies lifecycle gating for ticks.
func TestPoolPartitionTickRejectsClosedPartition(t *testing.T) {
	partition, err := NewPoolPartition(testPartitionConfig("primary"))
	requirePartitionNoError(t, err)
	requirePartitionNoError(t, partition.Close())

	_, err = partition.Tick()
	requirePartitionErrorIs(t, err, ErrClosed)

	err = partition.TickInto(&PartitionControllerReport{})
	requirePartitionErrorIs(t, err, ErrClosed)

	err = partition.TickInto(nil)
	requirePartitionErrorIs(t, err, ErrClosed)
}

// TestPoolPartitionManualTickAllowedWhenControllerDisabled verifies manual tick semantics.
func TestPoolPartitionManualTickAllowedWhenControllerDisabled(t *testing.T) {
	config := testPartitionConfig("primary")
	config.Policy.Controller = PartitionControllerPolicy{Enabled: false}
	partition, err := NewPoolPartition(config)
	requirePartitionNoError(t, err)
	t.Cleanup(func() { requirePartitionNoError(t, partition.Close()) })

	report, err := partition.Tick()
	requirePartitionNoError(t, err)
	if report.Generation.IsZero() || report.Sample.Generation != report.Generation || report.Metrics.Generation != report.Generation {
		t.Fatalf("manual Tick report has incoherent generation: %+v", report)
	}
}

func TestPoolPartitionTickIntoPublishesClassBudgets(t *testing.T) {
	partition := testNewPoolPartition(t, "primary")

	var report PartitionControllerReport
	requirePartitionNoError(t, partition.TickInto(&report))
	if len(report.PoolBudgetTargets) != 1 {
		t.Fatalf("len(PoolBudgetTargets) = %d, want 1", len(report.PoolBudgetTargets))
	}
	if len(report.PoolBudgetTargets[0].ClassTargets) == 0 {
		t.Fatalf("PoolBudgetTargets[0].ClassTargets is empty")
	}
	snapshot, ok := partition.PoolSnapshot("primary")
	if !ok {
		t.Fatal("PoolSnapshot(primary) not found")
	}
	for _, classTarget := range report.PoolBudgetTargets[0].ClassTargets {
		class := snapshot.Classes[classTarget.ClassID.Index()]
		if class.Budget.Generation.Before(report.Generation) {
			t.Fatalf("class %s budget generation = %s, want >= tick generation %s", class.Class, class.Budget.Generation, report.Generation)
		}
		if class.Budget.AssignedBytes != classTarget.TargetBytes.Bytes() {
			t.Fatalf("class %s assigned bytes = %d, want target %d", class.Class, class.Budget.AssignedBytes, classTarget.TargetBytes.Bytes())
		}
	}
}

func TestPoolPartitionTickIntoUpdatesPreviousSampleAndEWMAWithElapsed(t *testing.T) {
	partition := testNewPoolPartition(t, "primary")
	manual := clock.NewManual(time.Unix(100, 0))
	partition.controller.clock = manual

	var first PartitionControllerReport
	requirePartitionNoError(t, partition.TickInto(&first))

	lease, err := partition.Acquire("primary", 300)
	requirePartitionNoError(t, err)
	requirePartitionNoError(t, partition.Release(lease, lease.Buffer()))
	manual.Advance(time.Second)

	var second PartitionControllerReport
	requirePartitionNoError(t, partition.TickInto(&second))
	if second.Window.Delta.Gets == 0 || second.Rates.GetsPerSecond == 0 {
		t.Fatalf("second tick window/rates = %+v / %+v", second.Window.Delta, second.Rates)
	}
	if !second.EWMA.Initialized || second.EWMA.GetsPerSecond == 0 {
		t.Fatalf("second tick EWMA = %+v", second.EWMA)
	}
	if !partition.controller.hasPreviousSample || !partition.controller.previousSampleTime.Equal(manual.Now()) {
		t.Fatalf("controller previous state not updated")
	}
}

func TestPoolPartitionTickIntoDeactivatesIdlePoolsAndReactivatesDirtyPool(t *testing.T) {
	partition := testNewPoolPartition(t, "alpha", "beta")

	requirePartitionNoError(t, partition.TickInto(&PartitionControllerReport{}))
	requirePartitionNoError(t, partition.TickInto(&PartitionControllerReport{}))
	requirePartitionNoError(t, partition.TickInto(&PartitionControllerReport{}))
	if active := partition.activeRegistry.activeIndexes(nil); len(active) != 0 {
		t.Fatalf("active indexes after idle ticks = %v, want none", active)
	}

	lease, err := partition.Acquire("beta", 300)
	requirePartitionNoError(t, err)
	defer func() { requirePartitionNoError(t, partition.Release(lease, lease.Buffer())) }()
	if active := partition.activeRegistry.activeIndexes(nil); len(active) != 1 || active[0] != 1 {
		t.Fatalf("active indexes after beta acquire = %v, want [1]", active)
	}
}
