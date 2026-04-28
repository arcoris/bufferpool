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
	"testing"
)

// This file benchmarks public Get and mixed Get+Put data-plane flows.
//
// These benchmarks keep policy, lifecycle, counters, class routing, shard
// selection, shard credit, and bucket storage active. They are intended to show
// the cost of Pool as a bounded observable runtime component, not as a raw
// allocator or sync.Pool replacement.

// BenchmarkPoolGetHit measures Get when retained storage is guaranteed to hit.
//
// Setup work is outside the timer. Each measured batch performs only Get calls;
// retained buffers are re-seeded between batches to keep the benchmark on the
// hit path. The benchmark validates miss counters outside the timed section so
// a future selector/setup change cannot silently turn this into a mixed path.
func BenchmarkPoolGetHit(b *testing.B) {
	for _, tc := range poolBenchmarkHitCases() {
		tc := tc

		b.Run(tc.name, func(b *testing.B) {
			pool := poolBenchmarkNewPool(b, poolBenchmarkPolicyForCase(tc))
			batchSize := poolBenchmarkBatchSize(tc.capacity)
			var previousMisses uint64

			b.ReportAllocs()
			b.ResetTimer()

			for completed := 0; completed < b.N; {
				batch := batchSize
				if remaining := b.N - completed; remaining < batch {
					batch = remaining
				}

				b.StopTimer()
				poolBenchmarkClearRetained(pool)
				poolBenchmarkSeedRetainedInternal(b, pool, tc.capacity, batch)
				b.StartTimer()

				for index := 0; index < batch; index++ {
					buffer, err := pool.Get(tc.size)
					if err != nil {
						b.Fatalf("Get() returned error: %v", err)
					}
					if len(buffer) != tc.size || cap(buffer) < tc.capacity {
						b.Fatalf("Get() len/cap = %d/%d, want %d/>=%d", len(buffer), cap(buffer), tc.size, tc.capacity)
					}

					poolBenchmarkBufferSink = buffer
				}

				completed += batch

				b.StopTimer()
				var sample poolCounterSample
				pool.sampleCounters(&sample)
				if sample.Counters.Misses != previousMisses {
					b.Fatalf("GetHit observed %d new misses", sample.Counters.Misses-previousMisses)
				}
				previousMisses = sample.Counters.Misses
				b.StartTimer()
			}
		})
	}
}

// BenchmarkPoolGetSeededMixed measures Get with seeded retained storage and
// non-single selectors.
//
// These cases are intentionally not named as guaranteed hits. Selector state,
// shard distribution, and the absence of measured Put calls can produce a real
// mix of hits and allocation misses. Reported metrics show the actual path mix.
func BenchmarkPoolGetSeededMixed(b *testing.B) {
	for _, tc := range poolBenchmarkSeededMixedCases() {
		tc := tc

		b.Run(tc.name, func(b *testing.B) {
			pool := poolBenchmarkNewPool(b, poolBenchmarkPolicyForCase(tc))
			batchSize := poolBenchmarkBatchSize(tc.capacity)

			b.ReportAllocs()
			b.ResetTimer()

			for completed := 0; completed < b.N; {
				batch := batchSize
				if remaining := b.N - completed; remaining < batch {
					batch = remaining
				}

				b.StopTimer()
				poolBenchmarkClearRetained(pool)
				poolBenchmarkSeedRetainedPublic(b, pool, tc.capacity, batch*tc.shards)
				b.StartTimer()

				for index := 0; index < batch; index++ {
					buffer, err := pool.Get(tc.size)
					if err != nil {
						b.Fatalf("Get() returned error: %v", err)
					}
					if len(buffer) != tc.size || cap(buffer) < tc.capacity {
						b.Fatalf("Get() len/cap = %d/%d, want %d/>=%d", len(buffer), cap(buffer), tc.size, tc.capacity)
					}

					poolBenchmarkBufferSink = buffer
				}

				completed += batch
			}

			poolBenchmarkReportOutcomeRatios(b, pool)
		})
	}
}

// BenchmarkPoolGetMiss measures acquisition when retained storage is empty and
// Pool must allocate a class-sized buffer.
func BenchmarkPoolGetMiss(b *testing.B) {
	cases := []poolBenchmarkCase{
		{name: "size_300/class_512", size: 300, capacity: 512, shards: 1, selector: ShardSelectionModeSingle},
		{name: "size_900/class_1024", size: 900, capacity: 1024, shards: 1, selector: ShardSelectionModeSingle},
		{name: "size_4096/class_4096", size: 4096, capacity: 4096, shards: 1, selector: ShardSelectionModeSingle},
	}

	for _, tc := range cases {
		tc := tc

		b.Run(tc.name, func(b *testing.B) {
			pool := poolBenchmarkNewPool(b, poolBenchmarkPolicyForCase(tc))

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				buffer, err := pool.Get(tc.size)
				if err != nil {
					b.Fatalf("Get() returned error: %v", err)
				}
				if len(buffer) != tc.size || cap(buffer) != tc.capacity {
					b.Fatalf("Get() len/cap = %d/%d, want %d/%d", len(buffer), cap(buffer), tc.size, tc.capacity)
				}

				poolBenchmarkBufferSink = buffer
			}
		})
	}
}

// BenchmarkPoolGetReject measures request validation and rejection paths.
//
// The unsupported-class case uses disabled policy validation to construct a
// deliberately inconsistent max-request/class-table shape. That isolates the
// class lookup failure path without changing production validation rules.
func BenchmarkPoolGetReject(b *testing.B) {
	b.Run("negative", func(b *testing.B) {
		pool := poolBenchmarkNewPool(b, poolBenchmarkPolicyForCase(poolBenchmarkCase{}))

		b.ReportAllocs()
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			_, poolBenchmarkErrorSink = pool.Get(-1)
			if !errors.Is(poolBenchmarkErrorSink, ErrInvalidSize) {
				b.Fatalf("Get(-1) error = %v, want ErrInvalidSize", poolBenchmarkErrorSink)
			}
		}
	})

	b.Run("zero_reject", func(b *testing.B) {
		policy := poolBenchmarkPolicyForCase(poolBenchmarkCase{})
		policy.Admission.ZeroSizeRequests = ZeroSizeRequestReject
		pool := poolBenchmarkNewPool(b, policy)

		b.ReportAllocs()
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			_, poolBenchmarkErrorSink = pool.Get(0)
			if !errors.Is(poolBenchmarkErrorSink, ErrInvalidSize) {
				b.Fatalf("Get(0) error = %v, want ErrInvalidSize", poolBenchmarkErrorSink)
			}
		}
	})

	b.Run("request_too_large", func(b *testing.B) {
		pool := poolBenchmarkNewPool(b, poolBenchmarkPolicyForCase(poolBenchmarkCase{}))

		b.ReportAllocs()
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			_, poolBenchmarkErrorSink = pool.Get(128 * KiB.Int())
			if !errors.Is(poolBenchmarkErrorSink, ErrRequestTooLarge) {
				b.Fatalf("Get(128 KiB) error = %v, want ErrRequestTooLarge", poolBenchmarkErrorSink)
			}
		}
	})

	b.Run("unsupported_class", func(b *testing.B) {
		policy := poolBenchmarkPolicyForCase(poolBenchmarkCase{
			classes: []ClassSize{ClassSizeFromBytes(512)},
		})
		policy.Retention.MaxRequestSize = KiB
		pool := poolBenchmarkNewPoolWithConfig(b, PoolConfig{
			Policy:           policy,
			PolicyValidation: PoolPolicyValidationModeDisabled,
		})

		b.ReportAllocs()
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			_, poolBenchmarkErrorSink = pool.Get(900)
			if !errors.Is(poolBenchmarkErrorSink, ErrUnsupportedClass) {
				b.Fatalf("Get(900) error = %v, want ErrUnsupportedClass", poolBenchmarkErrorSink)
			}
		}
	})
}

// BenchmarkPoolGetPutSequential measures single-goroutine data-plane loops
// after retained storage has been seeded.
//
// Single-selector cases are stable hit loops. Round-robin and random cases are
// named mixed because Get and Put intentionally share selector state, so return
// traffic may replenish a different shard than the next acquisition probes.
func BenchmarkPoolGetPutSequential(b *testing.B) {
	for _, tc := range poolBenchmarkFlowCases() {
		tc := tc

		flow := "hit"
		if tc.selector != ShardSelectionModeSingle {
			flow = "mixed"
		}

		b.Run(poolBenchmarkName(flow, tc.name), func(b *testing.B) {
			pool := poolBenchmarkNewPool(b, poolBenchmarkPolicyForCase(tc))
			poolBenchmarkSeedRetainedPublic(b, pool, tc.capacity, tc.shards*8)

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				buffer, err := pool.Get(tc.size)
				if err != nil {
					b.Fatalf("Get() returned error: %v", err)
				}

				if err := pool.Put(buffer); err != nil {
					b.Fatalf("Put() returned error: %v", err)
				}

				poolBenchmarkBufferSink = buffer
			}
		})
	}
}

// BenchmarkPoolGetPutParallel measures concurrent mixed Get+Put behavior.
//
// Use Go's external -cpu flag to measure GOMAXPROCS scaling, for example:
//
//	go test -run '^$' -bench 'BenchmarkPoolGetPutParallel' -benchmem -cpu 1,2,4,8,16 ./...
func BenchmarkPoolGetPutParallel(b *testing.B) {
	for _, tc := range poolBenchmarkFlowCases() {
		tc := tc

		b.Run(poolBenchmarkName("mixed", tc.name), func(b *testing.B) {
			pool := poolBenchmarkNewPool(b, poolBenchmarkPolicyForCase(tc))
			poolBenchmarkSeedRetainedPublic(b, pool, tc.capacity, tc.shards*8)

			b.ReportAllocs()
			b.ResetTimer()

			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					buffer, err := pool.Get(tc.size)
					if err != nil {
						b.Fatalf("Get() returned error: %v", err)
					}

					if err := pool.Put(buffer); err != nil {
						b.Fatalf("Put() returned error: %v", err)
					}

					poolBenchmarkBufferSink = buffer
				}
			})
		})
	}
}

// BenchmarkPoolGetPutReturnRate controls how often acquired buffers are
// returned to Pool.
//
// Return rate is not the same as hit ratio. Actual hit, miss, retain, and drop
// ratios depend on shard selection, retained supply, allocation, credit, and
// bucket state, so the benchmark reports observed ratios after the loop.
func BenchmarkPoolGetPutReturnRate(b *testing.B) {
	cases := []struct {
		name      string
		putEvery  int
		seedCount int
	}{
		{name: "return_100", putEvery: 1, seedCount: 64},
		{name: "return_90", putEvery: 10, seedCount: 64},
		{name: "return_50", putEvery: 2, seedCount: 32},
		{name: "return_0", putEvery: 0, seedCount: 0},
	}

	for _, tc := range cases {
		tc := tc

		b.Run(tc.name, func(b *testing.B) {
			benchCase := poolBenchmarkCase{size: 300, capacity: 512, shards: 8, selector: ShardSelectionModeRandom}
			pool := poolBenchmarkNewPool(b, poolBenchmarkPolicyForCase(benchCase))
			poolBenchmarkSeedRetainedPublic(b, pool, benchCase.capacity, tc.seedCount)

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				buffer, err := pool.Get(benchCase.size)
				if err != nil {
					b.Fatalf("Get() returned error: %v", err)
				}

				if tc.putEvery == 1 || (tc.putEvery > 1 && i%tc.putEvery != 0) {
					if err := pool.Put(buffer); err != nil {
						b.Fatalf("Put() returned error: %v", err)
					}
				}

				poolBenchmarkBufferSink = buffer
			}

			poolBenchmarkReportOutcomeRatios(b, pool)
		})
	}
}
