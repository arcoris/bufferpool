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
	"strconv"
	"testing"
	"time"
)

var (
	// partitionBenchmarkLeaseSink prevents lease benchmark results being optimized away.
	partitionBenchmarkLeaseSink Lease

	// partitionBenchmarkSampleSink prevents sample benchmark results being optimized away.
	partitionBenchmarkSampleSink PoolPartitionSample

	// partitionBenchmarkMetricsSink prevents metrics benchmark results being optimized away.
	partitionBenchmarkMetricsSink PoolPartitionMetrics

	// partitionBenchmarkReportSink prevents tick benchmark results being optimized away.
	partitionBenchmarkReportSink PartitionControllerReport

	// partitionBenchmarkWindowSink prevents window benchmark results being optimized away.
	partitionBenchmarkWindowSink PoolPartitionWindow

	// partitionBenchmarkRatesSink prevents rate benchmark results being optimized away.
	partitionBenchmarkRatesSink PoolPartitionWindowRates

	// partitionBenchmarkActivityScoreSink prevents activity scores being optimized away.
	partitionBenchmarkActivityScoreSink PoolPartitionActivityScore

	// partitionBenchmarkEWMASink prevents EWMA benchmark results being optimized away.
	partitionBenchmarkEWMASink PoolPartitionEWMAState

	// partitionBenchmarkEvaluationSink prevents evaluation benchmark results being optimized away.
	partitionBenchmarkEvaluationSink PoolPartitionControllerEvaluation

	// partitionBenchmarkClassScoreMapSink prevents class score maps being optimized away.
	partitionBenchmarkClassScoreMapSink map[poolClassKey]PoolClassScore

	// partitionBenchmarkPoolClassScoreSink prevents class score results being optimized away.
	partitionBenchmarkPoolClassScoreSink PoolClassScore

	// partitionBenchmarkPoolBudgetScoreSink prevents Pool score aggregation results being optimized away.
	partitionBenchmarkPoolBudgetScoreSink PoolBudgetScore

	// partitionBenchmarkBudgetPublicationSink prevents budget reports being optimized away.
	partitionBenchmarkBudgetPublicationSink PoolPartitionBudgetPublicationReport

	// partitionBenchmarkIndexSink prevents active-registry indexes being optimized away.
	partitionBenchmarkIndexSink []int

	// partitionBenchmarkDrainSink prevents drain benchmark results being optimized away.
	partitionBenchmarkDrainSink PoolPartitionDrainResult
)

// Partition benchmarks measure the owner above Pool: named Pool lookup,
// LeaseRegistry ownership accounting, partition sampling, and explicit
// controller planning. They are not equivalent to bare Pool.Get/Put benchmarks,
// which measure retained-storage data-plane cost without lease records.

// BenchmarkPoolPartitionAcquireReleaseStrict measures strict ownership
// acquire/release through a partition with different pool-registry sizes.
func BenchmarkPoolPartitionAcquireReleaseStrict(b *testing.B) {
	for _, poolCount := range []int{1, 16} {
		poolCount := poolCount
		b.Run("pools_"+strconv.Itoa(poolCount), func(b *testing.B) {
			partition := partitionBenchmarkNew(b, poolCount)
			poolNames := partition.PoolNames()

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				poolName := poolNames[i%len(poolNames)]
				lease, err := partition.Acquire(poolName, 300)
				if err != nil {
					b.Fatalf("Acquire(%q) returned error: %v", poolName, err)
				}
				if err := partition.Release(lease, lease.Buffer()); err != nil {
					b.Fatalf("Release() returned error: %v", err)
				}
				partitionBenchmarkLeaseSink = lease
			}
		})
	}
}

// BenchmarkPoolPartitionManagedAcquireRelease measures managed partition
// routing through PoolPartition and LeaseRegistry.
func BenchmarkPoolPartitionManagedAcquireRelease(b *testing.B) {
	partition := partitionBenchmarkNew(b, 1)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lease, err := partition.Acquire("pool_0", 300)
		if err != nil {
			b.Fatalf("Acquire() returned error: %v", err)
		}
		if err := partition.Release(lease, lease.Buffer()); err != nil {
			b.Fatalf("Release() returned error: %v", err)
		}
		partitionBenchmarkLeaseSink = lease
	}
}

// BenchmarkPoolPartitionSample measures public value-returning samples. Public
// Sample may allocate per-Pool sample storage; SampleInto is the reusable path
// for callers that need allocation control.
func BenchmarkPoolPartitionSample(b *testing.B) {
	for _, tc := range []struct {
		name   string
		pools  int
		active int
	}{
		{name: "pools_1_active_0", pools: 1, active: 0},
		{name: "pools_16_active_0", pools: 16, active: 0},
		{name: "pools_16_active_256", pools: 16, active: 256},
	} {
		tc := tc
		b.Run(tc.name, func(b *testing.B) {
			partition := partitionBenchmarkNew(b, tc.pools)
			leases := partitionBenchmarkAcquireActive(b, partition, tc.active)

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				partitionBenchmarkSampleSink = partition.Sample()
			}
			b.StopTimer()

			partitionBenchmarkReleaseAll(b, partition, leases)
		})
	}
}

// BenchmarkPoolPartitionSampleInto measures reusable partition sampling. The
// warm call grows nested class storage so the timed loop measures steady-state
// caller reuse rather than first-sample allocation.
func BenchmarkPoolPartitionSampleInto(b *testing.B) {
	for _, tc := range []struct {
		name   string
		pools  int
		active int
	}{
		{name: "pools_1_active_0", pools: 1, active: 0},
		{name: "pools_16_active_0", pools: 16, active: 0},
		{name: "pools_16_active_256", pools: 16, active: 256},
	} {
		tc := tc
		b.Run(tc.name, func(b *testing.B) {
			partition := partitionBenchmarkNew(b, tc.pools)
			leases := partitionBenchmarkAcquireActive(b, partition, tc.active)
			sample := PoolPartitionSample{Pools: make([]PoolPartitionPoolSample, 0, tc.pools)}
			partition.SampleInto(&sample)

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				partition.SampleInto(&sample)
			}
			b.StopTimer()

			partitionBenchmarkSampleSink = sample
			partitionBenchmarkReleaseAll(b, partition, leases)
		})
	}
}

// BenchmarkPoolPartitionTick measures one explicit non-background controller
// cycle. Tick returns a detailed report and may allocate per-Pool sample/report
// storage.
func BenchmarkPoolPartitionTick(b *testing.B) {
	partition := partitionBenchmarkNew(b, 16)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		report, err := partition.Tick()
		if err != nil {
			b.Fatalf("Tick() returned error: %v", err)
		}
		partitionBenchmarkReportSink = report
	}
}

// BenchmarkPoolPartitionTickInto measures reusable applied controller reports.
// The caller-owned report lets samples, windows, and class detail reuse storage
// across foreground controller cycles.
func BenchmarkPoolPartitionTickInto(b *testing.B) {
	partition := partitionBenchmarkNew(b, 16)
	report := PartitionControllerReport{
		Sample: PoolPartitionSample{
			Pools: make([]PoolPartitionPoolSample, 0, 16),
		},
	}
	if err := partition.TickInto(&report); err != nil {
		b.Fatalf("warm TickInto() returned error: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := partition.TickInto(&report); err != nil {
			b.Fatalf("TickInto() returned error: %v", err)
		}
		partitionBenchmarkReportSink = report
	}
}

// BenchmarkPoolPartitionTickClassSamples measures class-aware partition sampling
// used by applied controller ticks.
func BenchmarkPoolPartitionTickClassSamples(b *testing.B) {
	partition := partitionBenchmarkNew(b, 16)
	sample := PoolPartitionSample{Pools: make([]PoolPartitionPoolSample, 0, 16)}

	partition.SampleInto(&sample)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		partition.SampleInto(&sample)
	}
	partitionBenchmarkSampleSink = sample
}

// BenchmarkPoolPartitionTickAppliedController measures the applied foreground
// controller cycle that publishes Pool/class budgets but does not execute trim.
func BenchmarkPoolPartitionTickAppliedController(b *testing.B) {
	partition := partitionBenchmarkNew(b, 16)
	report := PartitionControllerReport{Sample: PoolPartitionSample{Pools: make([]PoolPartitionPoolSample, 0, 16)}}
	if err := partition.TickInto(&report); err != nil {
		b.Fatalf("warm TickInto() returned error: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := partition.TickInto(&report); err != nil {
			b.Fatalf("TickInto() returned error: %v", err)
		}
	}
	partitionBenchmarkReportSink = report
}

// BenchmarkPoolPartitionMetrics measures public lifetime-derived metrics.
func BenchmarkPoolPartitionMetrics(b *testing.B) {
	partition := partitionBenchmarkNew(b, 16)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		partitionBenchmarkMetricsSink = partition.Metrics()
	}
}

// BenchmarkPoolPartitionActiveRegistryActiveIndexes measures deterministic
// active-index copy after the registry has been initialized in partition order.
func BenchmarkPoolPartitionActiveRegistryActiveIndexes(b *testing.B) {
	names := make([]string, 16)
	for index := range names {
		names[index] = "pool_" + strconv.Itoa(index)
	}
	registry := newPartitionActiveRegistry(names)
	indexes := make([]int, 0, len(names))

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		indexes = registry.activeIndexes(indexes)
	}
	partitionBenchmarkIndexSink = indexes
}

// BenchmarkPoolPartitionActiveRegistryDirtyIndexes measures dirty-index copy.
// Dirty markers are state flags, not an activity event log.
func BenchmarkPoolPartitionActiveRegistryDirtyIndexes(b *testing.B) {
	names := make([]string, 16)
	for index := range names {
		names[index] = "pool_" + strconv.Itoa(index)
	}
	registry := newPartitionActiveRegistry(names)
	for index := 0; index < len(names); index += 2 {
		if err := registry.markDirtyIndex(index); err != nil {
			b.Fatalf("markDirtyIndex(%d) returned error: %v", index, err)
		}
	}
	indexes := make([]int, 0, len(names))

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		indexes = registry.dirtyIndexes(indexes)
	}
	partitionBenchmarkIndexSink = indexes
}

// BenchmarkPoolPartitionActiveRegistry measures controller activity observation
// over the active registry.
func BenchmarkPoolPartitionActiveRegistry(b *testing.B) {
	names := make([]string, 64)
	for index := range names {
		names[index] = "pool_" + strconv.Itoa(index)
	}
	registry := newPartitionActiveRegistry(names)
	indexes := make([]int, len(names))
	activeDeltas := make([]bool, len(names))
	for index := range indexes {
		indexes[index] = index
		activeDeltas[index] = index%4 == 0
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := registry.observeControllerActivity(indexes, activeDeltas); err != nil {
			b.Fatalf("observeControllerActivity() returned error: %v", err)
		}
	}
	partitionBenchmarkIndexSink = registry.activeIndexes(partitionBenchmarkIndexSink)
}

// BenchmarkPoolPartitionSampleSelectedIndexes measures the selected-index
// sampling boundary used by active/dirty controller cycles.
func BenchmarkPoolPartitionSampleSelectedIndexes(b *testing.B) {
	for _, tc := range []struct {
		name    string
		pools   int
		indexes []int
	}{
		{name: "pools_16_active_4", pools: 16, indexes: []int{0, 3, 7, 15}},
		{name: "pools_1024_active_16", pools: 1024, indexes: []int{0, 17, 64, 129, 255, 300, 384, 511, 512, 640, 700, 768, 900, 990, 1000, 1023}},
	} {
		tc := tc
		b.Run(tc.name, func(b *testing.B) {
			partition := partitionBenchmarkNew(b, tc.pools)
			sample := PoolPartitionSample{Pools: make([]PoolPartitionPoolSample, 0, len(tc.indexes))}
			runtime := partition.currentRuntimeSnapshot()
			generation := partition.generation.Load()
			partition.sampleIndexesWithRuntimeAndGeneration(&sample, runtime, generation, tc.indexes, true)

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				partition.sampleIndexesWithRuntimeAndGeneration(&sample, runtime, generation, tc.indexes, true)
			}
			partitionBenchmarkSampleSink = sample
		})
	}
}

// BenchmarkPoolPartitionWindow measures safe owning window construction. It may
// allocate to copy per-Pool sample slices.
func BenchmarkPoolPartitionWindow(b *testing.B) {
	previous, current := partitionBenchmarkWindowSamples(16)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		partitionBenchmarkWindowSink = NewPoolPartitionWindow(previous, current)
	}
}

// BenchmarkPoolPartitionWindowReset measures reusable window construction with
// preallocated stored sample slices.
func BenchmarkPoolPartitionWindowReset(b *testing.B) {
	previous, current := partitionBenchmarkWindowSamples(16)
	window := PoolPartitionWindow{
		Previous: PoolPartitionSample{Pools: make([]PoolPartitionPoolSample, 0, len(previous.Pools))},
		Current:  PoolPartitionSample{Pools: make([]PoolPartitionPoolSample, 0, len(current.Pools))},
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		window.Reset(previous, current)
	}
	partitionBenchmarkWindowSink = window
}

// BenchmarkPoolPartitionWindowRates measures window-derived ratio projection.
func BenchmarkPoolPartitionWindowRates(b *testing.B) {
	previous, current := partitionBenchmarkWindowSamples(16)
	window := NewPoolPartitionWindow(previous, current)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		partitionBenchmarkRatesSink = NewPoolPartitionWindowRates(window)
	}
}

// BenchmarkPoolPartitionActivityScore measures the root activity adapter over
// selected rate signals. It does not call Pool.Get/Put or mutate active state.
func BenchmarkPoolPartitionActivityScore(b *testing.B) {
	evaluator := NewPoolPartitionScoreEvaluator(PoolPartitionScoreEvaluatorConfig{})
	signals := partitionScoreSignals{
		getsPerSecond:     defaultPartitionHighGetsPerSecond,
		putsPerSecond:     defaultPartitionHighPutsPerSecond,
		leaseOpsPerSecond: defaultPartitionHighLeaseOpsPerSecond,
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		partitionBenchmarkActivityScoreSink = evaluator.activityScore(signals)
	}
}

// BenchmarkPoolPartitionEWMAUpdate measures pure EWMA projection over window rates.
func BenchmarkPoolPartitionEWMAUpdate(b *testing.B) {
	previous, current := partitionBenchmarkWindowSamples(16)
	window := NewPoolPartitionWindow(previous, current)
	rates := NewPoolPartitionTimedWindowRates(window, time.Second)
	config := PoolPartitionEWMAConfig{HalfLife: time.Second}
	state := PoolPartitionEWMAState{}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		state = state.WithUpdate(config, time.Second, rates)
	}
	partitionBenchmarkEWMASink = state
}

// BenchmarkPoolClassScore measures one typed class score projection. Inputs are
// prepared outside the timed loop so the measurement stays focused on the pure
// score formula and component construction.
func BenchmarkPoolClassScore(b *testing.B) {
	window := partitionBenchmarkScoreWindow(1)
	classWindow := window.Pools[0].Classes[0]
	activity := newPoolPartitionClassActivity(classWindow, time.Second)
	ewma := PoolClassEWMAState{
		Initialized:   true,
		Activity:      activity.Activity,
		GetsPerSecond: activity.GetsPerSecond,
		PutsPerSecond: activity.PutsPerSecond,
		DropRatio:     activity.DropRatio,
	}
	input := PoolClassScoreInput{
		Current:  classWindow.Current,
		Window:   classWindow,
		Activity: activity,
		EWMA:     ewma,
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		partitionBenchmarkPoolClassScoreSink = NewPoolClassScore(input)
	}
}

// BenchmarkPoolScoreAggregation measures typed Pool score aggregation from
// prepared class scores. It verifies the non-raw-sum aggregation cost separately
// from class score construction.
func BenchmarkPoolScoreAggregation(b *testing.B) {
	window := partitionBenchmarkScoreWindow(16)
	ewma := partitionBenchmarkClassEWMA(window)
	classScores := controllerClassScoreMap(window, time.Second, ewma)
	poolWindow := window.Pools[0]

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		partitionBenchmarkPoolBudgetScoreSink = poolWindowScore(poolWindow, classScores)
	}
}

// BenchmarkPartitionControllerClassScoreMap measures the typed class score map
// projection used by applied partition controller budget allocation.
func BenchmarkPartitionControllerClassScoreMap(b *testing.B) {
	window := partitionBenchmarkScoreWindow(16)
	ewma := partitionBenchmarkClassEWMA(window)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		partitionBenchmarkClassScoreMapSink = controllerClassScoreMap(window, time.Second, ewma)
	}
}

// BenchmarkPartitionControllerPoolScoreAggregation measures typed Pool score
// aggregation from precomputed class scores.
func BenchmarkPartitionControllerPoolScoreAggregation(b *testing.B) {
	window := partitionBenchmarkScoreWindow(16)
	ewma := partitionBenchmarkClassEWMA(window)
	classScores := controllerClassScoreMap(window, time.Second, ewma)
	poolWindow := window.Pools[0]

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		partitionBenchmarkPoolBudgetScoreSink = poolWindowScore(poolWindow, classScores)
	}
}

// BenchmarkPartitionControllerScoreCycle measures the controller score slice
// that builds typed class scores and aggregates every Pool score from the same
// updated EWMA snapshot.
func BenchmarkPartitionControllerScoreCycle(b *testing.B) {
	window := partitionBenchmarkScoreWindow(16)
	ewma := partitionBenchmarkClassEWMA(window)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		classScores := controllerClassScoreMap(window, time.Second, ewma)
		for _, poolWindow := range window.Pools {
			partitionBenchmarkPoolBudgetScoreSink = poolWindowScore(poolWindow, classScores)
		}
		partitionBenchmarkClassScoreMapSink = classScores
	}
}

// BenchmarkPartitionControllerBudgetReportWithScores measures score diagnostic
// construction in the partition-to-Pool budget report path.
func BenchmarkPartitionControllerBudgetReportWithScores(b *testing.B) {
	partition := partitionBenchmarkNew(b, 16)
	window := partitionBenchmarkScoreWindow(16)
	ewma := partitionBenchmarkClassEWMA(window)
	runtime := partition.currentRuntimeSnapshot()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		partitionBenchmarkBudgetPublicationSink = partition.controllerPoolBudgetReport(Generation(i+1), runtime, window, time.Second, ewma)
	}
}

// BenchmarkPartitionControllerReportWithScoreDiagnostics measures the applied
// controller report path with score diagnostics enabled.
func BenchmarkPartitionControllerReportWithScoreDiagnostics(b *testing.B) {
	partition := partitionBenchmarkNew(b, 16)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var report PartitionControllerReport
		if err := partition.TickInto(&report); err != nil {
			b.Fatalf("TickInto() error = %v", err)
		}
		partitionBenchmarkReportSink = report
	}
}

// BenchmarkBudgetPublicationWithScoreDiagnostics measures the budget
// publication report diagnostics without executing Pool.Get or Pool.Put.
func BenchmarkBudgetPublicationWithScoreDiagnostics(b *testing.B) {
	partition := partitionBenchmarkNew(b, 16)
	window := partitionBenchmarkScoreWindow(16)
	ewma := partitionBenchmarkClassEWMA(window)
	runtime := partition.currentRuntimeSnapshot()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		partitionBenchmarkBudgetPublicationSink = partition.controllerPoolBudgetReport(Generation(i+1), runtime, window, time.Second, ewma)
	}
}

// BenchmarkPartitionTickWithScoreDiagnostics measures a full reusable
// foreground partition tick with score diagnostics present in the report.
func BenchmarkPartitionTickWithScoreDiagnostics(b *testing.B) {
	partition := partitionBenchmarkNew(b, 16)
	report := PartitionControllerReport{
		Sample: PoolPartitionSample{
			Pools: make([]PoolPartitionPoolSample, 0, 16),
		},
	}
	if err := partition.TickInto(&report); err != nil {
		b.Fatalf("warm TickInto() error = %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := partition.TickInto(&report); err != nil {
			b.Fatalf("TickInto() error = %v", err)
		}
		partitionBenchmarkReportSink = report
	}
}

// BenchmarkPoolPartitionControllerEvaluation measures the pure controller
// projection from two samples. It does not mutate runtime policy or execute
// trim; owning window construction may allocate to copy per-Pool sample slices.
func BenchmarkPoolPartitionControllerEvaluation(b *testing.B) {
	previous, current := partitionBenchmarkWindowSamples(16)
	config := PoolPartitionEWMAConfig{HalfLife: time.Second}
	budget := PartitionBudgetSnapshot{MaxOwnedBytes: 1 << 20, CurrentOwnedBytes: 512 << 10}
	pressure := PartitionPressureSnapshot{Enabled: true, Level: PressureLevelMedium}
	state := PoolPartitionEWMAState{}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		evaluation := NewPoolPartitionControllerEvaluation(previous, current, time.Second, state, config, budget, pressure)
		state = evaluation.EWMA
		partitionBenchmarkEvaluationSink = evaluation
	}
}

// BenchmarkPoolPartitionDrain measures bounded graceful close with no active leases.
//
// Each iteration constructs a fresh partition because successful drain closes the
// partition. The benchmark is for lifecycle cost, not Pool hot-path cost.
func BenchmarkPoolPartitionDrain(b *testing.B) {
	config := testPartitionConfig("primary")

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		partition := MustNewPoolPartition(config)
		result, err := partition.CloseGracefully(PoolPartitionDrainPolicy{})
		if err != nil {
			b.Fatalf("CloseGracefully() returned error: %v", err)
		}
		if !result.Completed {
			b.Fatalf("CloseGracefully() result = %+v, want completed", result)
		}
		partitionBenchmarkDrainSink = result
	}
}

// partitionBenchmarkNew constructs a benchmark partition with poolCount Pools.
func partitionBenchmarkNew(b *testing.B, poolCount int) *PoolPartition {
	b.Helper()

	names := make([]string, poolCount)
	for index := range names {
		names[index] = "pool_" + strconv.Itoa(index)
	}

	partition := MustNewPoolPartition(testPartitionConfig(names...))
	b.Cleanup(func() {
		_ = partition.Close()
	})

	return partition
}

// partitionBenchmarkAcquireActive creates active leases for sample benchmarks.
func partitionBenchmarkAcquireActive(b *testing.B, partition *PoolPartition, count int) []Lease {
	b.Helper()

	if count <= 0 {
		return nil
	}

	poolNames := partition.PoolNames()
	leases := make([]Lease, count)
	for index := range leases {
		poolName := poolNames[index%len(poolNames)]
		lease, err := partition.Acquire(poolName, 300)
		if err != nil {
			b.Fatalf("Acquire(%q) returned error: %v", poolName, err)
		}
		leases[index] = lease
	}

	return leases
}

// partitionBenchmarkReleaseAll releases benchmark leases after timing stops.
func partitionBenchmarkReleaseAll(b *testing.B, partition *PoolPartition, leases []Lease) {
	b.Helper()

	for _, lease := range leases {
		if err := partition.Release(lease, lease.Buffer()); err != nil {
			b.Fatalf("Release() returned error: %v", err)
		}
	}
}

// partitionBenchmarkWindowSamples builds synthetic samples with per-Pool entries.
func partitionBenchmarkWindowSamples(poolCount int) (PoolPartitionSample, PoolPartitionSample) {
	previous := PoolPartitionSample{
		Generation:       Generation(1),
		PolicyGeneration: Generation(1),
		Scope:            PoolPartitionSampleScopePartition,
		TotalPoolCount:   poolCount,
		SampledPoolCount: poolCount,
		PoolCount:        poolCount,
		Pools:            make([]PoolPartitionPoolSample, poolCount),
		PoolCounters: PoolCountersSnapshot{
			Gets:          100,
			Hits:          80,
			Misses:        20,
			Allocations:   20,
			Puts:          90,
			ReturnedBytes: 90 * 512,
			Retains:       70,
			RetainedBytes: 70 * 512,
			Drops:         20,
			DroppedBytes:  20 * 512,
			DropReasons: PoolDropReasonCounters{
				Oversized: 3,
			},
		},
		LeaseCounters: LeaseCountersSnapshot{
			Acquisitions:        100,
			Releases:            90,
			PoolReturnAttempts:  90,
			PoolReturnSuccesses: 88,
			PoolReturnFailures:  2,
		},
	}
	current := previous
	current.Generation = Generation(2)
	current.Pools = make([]PoolPartitionPoolSample, poolCount)
	current.PoolCounters.Gets += 40
	current.PoolCounters.Hits += 30
	current.PoolCounters.Misses += 10
	current.PoolCounters.Allocations += 10
	current.PoolCounters.Puts += 36
	current.PoolCounters.ReturnedBytes += 36 * 512
	current.PoolCounters.Retains += 30
	current.PoolCounters.RetainedBytes += 30 * 512
	current.PoolCounters.Drops += 6
	current.PoolCounters.DroppedBytes += 6 * 512
	current.PoolCounters.DropReasons.Oversized += 1
	current.LeaseCounters.Acquisitions += 40
	current.LeaseCounters.Releases += 36
	current.LeaseCounters.PoolReturnAttempts += 36
	current.LeaseCounters.PoolReturnSuccesses += 35
	current.LeaseCounters.PoolReturnFailures += 1
	for index := 0; index < poolCount; index++ {
		name := "pool_" + strconv.Itoa(index)
		previous.Pools[index] = PoolPartitionPoolSample{Name: name, Generation: Generation(1), Lifecycle: LifecycleActive}
		current.Pools[index] = PoolPartitionPoolSample{Name: name, Generation: Generation(1), Lifecycle: LifecycleActive}
	}
	return previous, current
}

func partitionBenchmarkScoreWindow(poolCount int) PoolPartitionWindow {
	window := PoolPartitionWindow{
		Pools: make([]PoolPartitionPoolWindow, poolCount),
	}
	for poolIndex := 0; poolIndex < poolCount; poolIndex++ {
		name := "pool_" + strconv.Itoa(poolIndex)
		window.Pools[poolIndex] = PoolPartitionPoolWindow{
			Name:    name,
			Classes: make([]PoolPartitionClassWindow, 2),
		}
		for classIndex := range window.Pools[poolIndex].Classes {
			classID := ClassID(classIndex)
			classBytes := uint64(512)
			if classIndex > 0 {
				classBytes = KiB.Bytes()
			}
			sample := testPoolClassScoreSample(classID, classBytes, 0, 0)
			base := uint64((poolIndex + 1) * (classIndex + 1))
			window.Pools[poolIndex].Classes[classIndex] = PoolPartitionClassWindow{
				Class:   sample.Class,
				ClassID: classID,
				Current: sample,
				Delta: classCountersDelta{
					Gets:        16 + base,
					Hits:        12 + base,
					Misses:      4,
					Allocations: 2,
					Puts:        14 + base,
					Retains:     12 + base,
					Drops:       2,
				},
			}
		}
	}
	return window
}

func partitionBenchmarkClassEWMA(window PoolPartitionWindow) map[poolClassKey]PoolClassEWMAState {
	ewma := make(map[poolClassKey]PoolClassEWMAState, len(window.Pools)*2)
	for _, poolWindow := range window.Pools {
		for _, classWindow := range poolWindow.Classes {
			activity := newPoolPartitionClassActivity(classWindow, time.Second)
			ewma[poolClassKey{PoolName: poolWindow.Name, ClassID: classWindow.ClassID}] = PoolClassEWMAState{
				Initialized:          true,
				Activity:             activity.Activity,
				GetsPerSecond:        activity.GetsPerSecond,
				PutsPerSecond:        activity.PutsPerSecond,
				AllocationsPerSecond: activity.AllocationsPerSecond,
				DropRatio:            activity.DropRatio,
			}
		}
	}
	return ewma
}
