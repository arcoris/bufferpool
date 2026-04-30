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

// This file contains selector microbenchmarks.
//
// Selector benchmarks intentionally do not represent full Pool performance.
// They isolate shard-index selection cost so contention in round-robin or random
// sequence counters can be compared separately from shard locks, counters,
// credit checks, buckets, lifecycle, and admission.

// BenchmarkPoolShardSelectorSequential measures selector cost without goroutine
// contention.
func BenchmarkPoolShardSelectorSequential(b *testing.B) {
	for _, tc := range poolBenchmarkSelectorCases() {
		tc := tc

		b.Run(tc.name, func(b *testing.B) {
			selector := poolBenchmarkSelector(b, tc.selector)

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				poolBenchmarkIntSink = selector.SelectShard(tc.shards)
			}
		})
	}
}

// BenchmarkShardSelectionProcessorInspired measures the default selector mode.
func BenchmarkShardSelectionProcessorInspired(b *testing.B) {
	benchmarkShardSelectionMode(b, ShardSelectionModeProcessorInspired)
}

// BenchmarkShardSelectionRoundRobin measures the retained legacy selector mode.
func BenchmarkShardSelectionRoundRobin(b *testing.B) {
	benchmarkShardSelectionMode(b, ShardSelectionModeRoundRobin)
}

// benchmarkShardSelectionMode isolates selector cost for one mode and a fixed
// 32-shard topology.
//
// Pool construction, shard locks, counters, lifecycle gates, and retained
// storage are intentionally outside this helper so the benchmark measures only
// the hot shard-index selection primitive.
func benchmarkShardSelectionMode(b *testing.B, mode ShardSelectionMode) {
	selector := poolBenchmarkSelector(b, mode)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		poolBenchmarkIntSink = selector.SelectShard(32)
	}
}

// BenchmarkPoolShardSelectorParallel measures selector cost under concurrent
// calls. It is useful for checking whether selector state, independent of shard
// storage, is a contention point.
func BenchmarkPoolShardSelectorParallel(b *testing.B) {
	for _, tc := range poolBenchmarkSelectorCases() {
		tc := tc

		b.Run(tc.name, func(b *testing.B) {
			selector := poolBenchmarkSelector(b, tc.selector)

			b.ReportAllocs()
			b.ResetTimer()

			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					poolBenchmarkIntSink = selector.SelectShard(tc.shards)
				}
			})
		})
	}
}

func poolBenchmarkSelectorCases() []poolBenchmarkCase {
	var cases []poolBenchmarkCase
	for _, selector := range []ShardSelectionMode{
		ShardSelectionModeSingle,
		ShardSelectionModeRoundRobin,
		ShardSelectionModeRandom,
		ShardSelectionModeProcessorInspired,
	} {
		for _, shards := range []int{1, 2, 8, 32, 128} {
			cases = append(cases, poolBenchmarkCase{
				name:     poolBenchmarkSelectorName(selector) + "/shards_" + strconv.Itoa(shards),
				shards:   shards,
				selector: selector,
			})
		}
	}

	return cases
}

// poolBenchmarkSelector constructs one selector instance for selector
// microbenchmarks.
//
// The class index is fixed at zero because these benchmarks compare selector
// mechanics, not cross-class seed variation.
func poolBenchmarkSelector(b *testing.B, mode ShardSelectionMode) shardSelector {
	b.Helper()

	selector, err := newPoolShardSelector(mode, 0)
	if err != nil {
		b.Fatalf("newPoolShardSelector() returned error: %v", err)
	}

	return selector
}
