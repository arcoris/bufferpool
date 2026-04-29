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
// caller preallocates per-Pool storage so lease sampling and Pool scanning do
// not allocate.
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
// observation/planning pass. Tick returns a detailed report and may allocate
// per-Pool sample/report storage.
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

// BenchmarkPoolPartitionTickInto measures reusable manual controller reports.
// It still performs the current all-Pool detailed scan, but caller-owned sample
// storage removes the per-call report allocation.
func BenchmarkPoolPartitionTickInto(b *testing.B) {
	partition := partitionBenchmarkNew(b, 16)
	report := PartitionControllerReport{
		Sample: PoolPartitionSample{
			Pools: make([]PoolPartitionPoolSample, 0, 16),
		},
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
// active-index copy. The primitive is cheap, but current TickInto still treats
// all Pools as active until future idle expiry narrows the active set.
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

// BenchmarkPoolPartitionSampleSelectedIndexes measures the selected-index
// sampling boundary used by future active-only controllers. This is scaffolding;
// the current active registry still initializes every Pool as active.
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

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				partition.sampleIndexesWithRuntimeAndGeneration(&sample, runtime, generation, tc.indexes, true)
			}
			partitionBenchmarkSampleSink = sample
		})
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
