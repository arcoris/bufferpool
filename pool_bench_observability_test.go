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

import "testing"

// This file separates public diagnostics from controller-oriented sampling.
//
// Snapshot is allowed to allocate because it returns caller-owned public DTOs.
// Metrics and sampleCounters should avoid public class/shard allocation and are
// the relevant paths for future PoolPartition controller ticks.

// BenchmarkPoolSnapshot measures the public diagnostic snapshot path.
func BenchmarkPoolSnapshot(b *testing.B) {
	for _, tc := range poolBenchmarkObservabilityCases() {
		tc := tc

		b.Run(tc.name, func(b *testing.B) {
			pool := poolBenchmarkObservabilityPool(b, tc)

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				poolBenchmarkSnapshotSink = pool.Snapshot()
			}
		})
	}
}

// BenchmarkPoolMetrics measures the public aggregate metrics path.
func BenchmarkPoolMetrics(b *testing.B) {
	for _, tc := range poolBenchmarkObservabilityCases() {
		tc := tc

		b.Run(tc.name, func(b *testing.B) {
			pool := poolBenchmarkObservabilityPool(b, tc)

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				poolBenchmarkMetricsSink = pool.Metrics()
			}
		})
	}
}

// BenchmarkPoolSampleCounters measures the allocation-conscious internal sample
// path that future controllers should prefer over public Snapshot.
func BenchmarkPoolSampleCounters(b *testing.B) {
	for _, tc := range poolBenchmarkObservabilityCases() {
		tc := tc

		b.Run(tc.name, func(b *testing.B) {
			pool := poolBenchmarkObservabilityPool(b, tc)
			var sample poolCounterSample

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				pool.sampleCounters(&sample)
			}

			poolBenchmarkSampleSink = sample
		})
	}
}

// BenchmarkPoolNewMetricsFromSnapshot measures projection cost after a snapshot
// has already been built.
func BenchmarkPoolNewMetricsFromSnapshot(b *testing.B) {
	for _, tc := range poolBenchmarkObservabilityCases() {
		tc := tc

		b.Run(tc.name, func(b *testing.B) {
			pool := poolBenchmarkObservabilityPool(b, tc)
			snapshot := pool.Snapshot()

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				poolBenchmarkMetricsSink = NewPoolMetrics(snapshot)
			}
		})
	}
}

func poolBenchmarkObservabilityCases() []poolBenchmarkCase {
	return []poolBenchmarkCase{
		{
			name:         "classes_2/shards_1/retained_0",
			classes:      poolBenchmarkClasses(2),
			shards:       1,
			selector:     ShardSelectionModeSingle,
			retainedSeed: 0,
		},
		{
			name:         "classes_2/shards_8/retained_16",
			classes:      poolBenchmarkClasses(2),
			shards:       8,
			selector:     ShardSelectionModeRandom,
			retainedSeed: 16,
		},
		{
			name:         "classes_8/shards_32/retained_256",
			classes:      poolBenchmarkClasses(8),
			shards:       32,
			selector:     ShardSelectionModeRandom,
			retainedSeed: 256,
		},
		{
			name:         "classes_16/shards_32/retained_256",
			classes:      poolBenchmarkClasses(16),
			shards:       32,
			selector:     ShardSelectionModeRandom,
			retainedSeed: 256,
		},
	}
}

func poolBenchmarkObservabilityPool(b *testing.B, tc poolBenchmarkCase) *Pool {
	b.Helper()

	policy := poolBenchmarkPolicyForCase(tc)
	pool := poolBenchmarkNewPool(b, policy)
	if tc.retainedSeed > 0 {
		poolBenchmarkSeedRetained(b, pool, int(tc.classes[0].Bytes()), tc.retainedSeed)
	}

	return pool
}
