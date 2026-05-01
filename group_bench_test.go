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
	"fmt"
	"testing"
)

func benchmarkPoolGroup(b *testing.B, partitions int) *PoolGroup {
	b.Helper()
	partitionNames := make([]string, partitions)
	for index := range partitionNames {
		partitionNames[index] = "partition-" + string(rune('a'+index))
	}
	group, err := NewPoolGroup(testGroupConfig(partitionNames...))
	if err != nil {
		b.Fatalf("NewPoolGroup() error = %v", err)
	}
	b.Cleanup(func() {
		if closeErr := group.Close(); closeErr != nil {
			b.Fatalf("PoolGroup.Close() error = %v", closeErr)
		}
	})
	return group
}

func benchmarkBudgetedPoolGroup(b *testing.B, partitions int, retained Size) *PoolGroup {
	b.Helper()
	partitionNames := make([]string, partitions)
	for index := range partitionNames {
		partitionNames[index] = "partition-" + string(rune('a'+index))
	}
	config := testGroupConfig(partitionNames...)
	config.Policy.Budget.MaxRetainedBytes = retained
	group, err := NewPoolGroup(config)
	if err != nil {
		b.Fatalf("NewPoolGroup() error = %v", err)
	}
	b.Cleanup(func() {
		if closeErr := group.Close(); closeErr != nil {
			b.Fatalf("PoolGroup.Close() error = %v", closeErr)
		}
	})
	return group
}

// seedPoolGroupActivity creates real retained and lease counters before benchmarks.
func seedPoolGroupActivity(b *testing.B, group *PoolGroup, partitions int) {
	b.Helper()
	for index := 0; index < partitions; index++ {
		partitionName := "partition-" + string(rune('a'+index))
		poolName := partitionName + "-pool"
		lease, err := group.Acquire(poolName, 256)
		if err != nil {
			b.Fatalf("PoolGroup.Acquire() error = %v", err)
		}
		if err := group.Release(lease, lease.Buffer()); err != nil {
			b.Fatalf("PoolGroup.Release() error = %v", err)
		}
	}
}

func BenchmarkPoolGroupSample(b *testing.B) {
	for _, partitions := range []int{1, 4, 16} {
		b.Run(fmt.Sprintf("partitions_%d", partitions), func(b *testing.B) {
			group := benchmarkPoolGroup(b, partitions)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = group.Sample()
			}
		})
	}
}

func BenchmarkPoolGroupSampleInto(b *testing.B) {
	for _, partitions := range []int{1, 4, 16} {
		b.Run(fmt.Sprintf("partitions_%d", partitions), func(b *testing.B) {
			group := benchmarkPoolGroup(b, partitions)
			var sample PoolGroupSample
			group.SampleInto(&sample)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				group.SampleInto(&sample)
			}
		})
	}
}

// BenchmarkPoolGroupSampleIntoAfterActivity measures reusable sampling with counters.
func BenchmarkPoolGroupSampleIntoAfterActivity(b *testing.B) {
	group := benchmarkPoolGroup(b, 4)
	seedPoolGroupActivity(b, group, 4)
	var sample PoolGroupSample
	group.SampleInto(&sample)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		group.SampleInto(&sample)
	}
}

func BenchmarkPoolGroupMetrics(b *testing.B) {
	group := benchmarkPoolGroup(b, 4)
	seedPoolGroupActivity(b, group, 4)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = group.Metrics()
	}
}

func BenchmarkPoolGroupTickInto(b *testing.B) {
	group := benchmarkPoolGroup(b, 4)
	var report PoolGroupCoordinatorReport
	_ = group.TickInto(&report)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := group.TickInto(&report); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkPoolGroupTickWithControllerStatus measures one manual foreground
// coordinator tick with lightweight status publication enabled.
func BenchmarkPoolGroupTickWithControllerStatus(b *testing.B) {
	group := benchmarkPoolGroup(b, 4)
	var report PoolGroupCoordinatorReport
	_ = group.TickInto(&report)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := group.TickInto(&report); err != nil {
			b.Fatal(err)
		}
		controllerStatusBenchmarkSink = report.Status
	}
}

// BenchmarkPoolGroupControllerStatus measures the lightweight status accessor
// without sampling partitions or publishing budget targets.
func BenchmarkPoolGroupControllerStatus(b *testing.B) {
	group := benchmarkPoolGroup(b, 1)
	var report PoolGroupCoordinatorReport
	_ = group.TickInto(&report)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		controllerStatusBenchmarkSink = group.ControllerStatus()
	}
}

// BenchmarkPoolGroupTickOverlapRejected measures explicit coordinator
// no-overlap rejection without waiting on coordinator.mu.
func BenchmarkPoolGroupTickOverlapRejected(b *testing.B) {
	group := benchmarkPoolGroup(b, 1)
	group.coordinator.cycleGate.running.Store(true)
	var report PoolGroupCoordinatorReport

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := group.TickInto(&report); err != nil {
			b.Fatal(err)
		}
		controllerStatusBenchmarkSink = report.Status
	}
	b.StopTimer()
	group.coordinator.cycleGate.running.Store(false)
}

func BenchmarkPoolGroupTickAppliedCoordinator(b *testing.B) {
	group := benchmarkBudgetedPoolGroup(b, 4, 4*MiB)
	var report PoolGroupCoordinatorReport
	_ = group.TickInto(&report)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := group.TickInto(&report); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkPoolGroupTickManyPartitions(b *testing.B) {
	for _, partitions := range []int{1, 4, 16} {
		b.Run(fmt.Sprintf("partitions_%d", partitions), func(b *testing.B) {
			group := benchmarkBudgetedPoolGroup(b, partitions, Size(partitions)*MiB)
			var report PoolGroupCoordinatorReport
			_ = group.TickInto(&report)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := group.TickInto(&report); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkPoolGroupBudgetRedistribution(b *testing.B) {
	inputs := make([]partitionBudgetAllocationInput, 64)
	for index := range inputs {
		inputs[index] = partitionBudgetAllocationInput{
			PartitionName: fmt.Sprintf("partition-%02d", index),
			Score:         float64(index%8) + 1,
		}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		budgetPartitionTargetsSink = allocatePartitionBudgetTargets(Generation(i+1), 64*MiB, inputs)
	}
}

// BenchmarkPoolGroupAcquireRelease measures group routing over partition leases.
func BenchmarkPoolGroupAcquireRelease(b *testing.B) {
	group := benchmarkPoolGroup(b, 1)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lease, err := group.Acquire("partition-a-pool", 256)
		if err != nil {
			b.Fatal(err)
		}
		if err := group.Release(lease, lease.Buffer()); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkPoolGroupManagedAcquireRelease measures pool-name directory routing
// through PoolGroup, PoolPartition, and LeaseRegistry.
func BenchmarkPoolGroupManagedAcquireRelease(b *testing.B) {
	group, err := NewPoolGroup(testManagedGroupConfig("api"))
	if err != nil {
		b.Fatalf("NewPoolGroup() error = %v", err)
	}
	b.Cleanup(func() {
		if closeErr := group.Close(); closeErr != nil {
			b.Fatalf("PoolGroup.Close() error = %v", closeErr)
		}
	})

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lease, err := group.Acquire("api", 256)
		if err != nil {
			b.Fatal(err)
		}
		if err := group.Release(lease, lease.Buffer()); err != nil {
			b.Fatal(err)
		}
		partitionBenchmarkLeaseSink = lease
	}
}

func BenchmarkPoolGroupAcquireByPoolDirectory(b *testing.B) {
	group, err := NewPoolGroup(testManagedGroupConfig("api", "worker", "events", "batch"))
	if err != nil {
		b.Fatalf("NewPoolGroup() error = %v", err)
	}
	b.Cleanup(func() {
		if closeErr := group.Close(); closeErr != nil {
			b.Fatalf("PoolGroup.Close() error = %v", closeErr)
		}
	})
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lease, err := group.Acquire("events", 256)
		if err != nil {
			b.Fatal(err)
		}
		if err := group.Release(lease, lease.Buffer()); err != nil {
			b.Fatal(err)
		}
		partitionBenchmarkLeaseSink = lease
	}
}

// BenchmarkPoolGroupAcquireReleaseVsPartitionAcquireRelease compares routing cost.
func BenchmarkPoolGroupAcquireReleaseVsPartitionAcquireRelease(b *testing.B) {
	b.Run("partition", func(b *testing.B) {
		partition, err := NewPoolPartition(testPartitionConfig("partition-a-pool"))
		if err != nil {
			b.Fatalf("NewPoolPartition() error = %v", err)
		}
		b.Cleanup(func() {
			if closeErr := partition.Close(); closeErr != nil {
				b.Fatalf("PoolPartition.Close() error = %v", closeErr)
			}
		})
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			lease, err := partition.Acquire("partition-a-pool", 256)
			if err != nil {
				b.Fatal(err)
			}
			if err := partition.Release(lease, lease.Buffer()); err != nil {
				b.Fatal(err)
			}
			partitionBenchmarkLeaseSink = lease
		}
	})

	b.Run("group", func(b *testing.B) {
		group := benchmarkPoolGroup(b, 1)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			lease, err := group.Acquire("partition-a-pool", 256)
			if err != nil {
				b.Fatal(err)
			}
			if err := group.Release(lease, lease.Buffer()); err != nil {
				b.Fatal(err)
			}
			partitionBenchmarkLeaseSink = lease
		}
	})
}

func BenchmarkPoolGroupPartitionAssignment(b *testing.B) {
	pools := make([]string, 64)
	for index := range pools {
		pools[index] = fmt.Sprintf("pool-%02d", index)
	}
	config := testManagedGroupConfig(pools...)
	config.Partitioning.MinPartitions = 8
	config.Partitioning.MaxPartitions = 8
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		partitions, directory, err := newGroupPartitionAssignments(config)
		if err != nil {
			b.Fatal(err)
		}
		if len(partitions) != 8 || len(directory.names) != len(pools) {
			b.Fatalf("assignment size = partitions %d pools %d", len(partitions), len(directory.names))
		}
	}
}

func BenchmarkPoolGroupScoreEvaluatorScoreValues(b *testing.B) {
	evaluator := NewPoolGroupScoreEvaluator(PoolGroupScoreEvaluatorConfig{})
	rates := PoolGroupWindowRates{Aggregate: PoolPartitionWindowRates{HitRatio: 1, RetainRatio: 1, LeaseOpsPerSecond: 100}}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = evaluator.ScoreValues(rates, PoolGroupBudgetSnapshot{}, PoolGroupPressureSnapshot{})
	}
}

// BenchmarkPoolGroupWindowReset measures deep-copy reuse for group samples.
func BenchmarkPoolGroupWindowReset(b *testing.B) {
	previous := testGroupSampleWithCounters(Generation(1), PoolCountersSnapshot{Gets: 10, Hits: 5, Misses: 5}, LeaseCountersSnapshot{Acquisitions: 1, Releases: 1})
	current := testGroupSampleWithCounters(Generation(2), PoolCountersSnapshot{Gets: 20, Hits: 15, Misses: 5}, LeaseCountersSnapshot{Acquisitions: 5, Releases: 3})
	previous.Partitions = []PoolGroupPartitionSample{{
		Name:   "alpha",
		Sample: PoolPartitionSample{Pools: []PoolPartitionPoolSample{{Name: "alpha-pool"}}},
	}}
	current.Partitions = []PoolGroupPartitionSample{{
		Name:   "alpha",
		Sample: PoolPartitionSample{Pools: []PoolPartitionPoolSample{{Name: "alpha-pool"}}},
	}}
	var window PoolGroupWindow
	window.Reset(previous, current)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		window.Reset(previous, current)
	}
}
