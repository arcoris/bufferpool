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

// BenchmarkPoolPartitionSample measures the reusable internal sample path used
// by explicit controller ticks and future PoolPartition control loops.
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
			sample := PoolPartitionSample{Pools: make([]PoolPartitionPoolSample, 0, tc.pools)}

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				partition.sample(&sample)
			}
			b.StopTimer()

			partitionBenchmarkSampleSink = sample
			partitionBenchmarkReleaseAll(b, partition, leases)
		})
	}
}

// BenchmarkPoolPartitionTick measures one explicit non-background controller
// observation/planning pass.
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

// BenchmarkPoolPartitionMetrics measures public lifetime-derived metrics.
func BenchmarkPoolPartitionMetrics(b *testing.B) {
	partition := partitionBenchmarkNew(b, 16)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		partitionBenchmarkMetricsSink = partition.Metrics()
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
