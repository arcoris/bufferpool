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

// This file isolates return-path admission costs.
//
// Owner-side drops stop before class/shard storage. Class/shard drops include
// class routing, selector choice, shard locking, credit checks, bucket checks,
// and lower-level counters.

// BenchmarkPoolPutRetain measures successful retention without pairing every
// measured Put with a measured Get.
//
// Each batch allocates returned buffers outside the timer, measures Put only,
// then clears retained storage outside the timer so the next batch remains a
// retain path rather than a bucket-full drop path.
func BenchmarkPoolPutRetain(b *testing.B) {
	cases := []poolBenchmarkCase{
		{name: "cap_512/shards_1/selector_single", capacity: 512, shards: 1, selector: ShardSelectionModeSingle},
		{name: "cap_512/shards_32/selector_random", capacity: 512, shards: 32, selector: ShardSelectionModeRandom},
		{name: "cap_1024/shards_8/selector_round_robin", capacity: 1024, shards: 8, selector: ShardSelectionModeRoundRobin},
		{name: "cap_4096/shards_32/selector_random", capacity: 4096, shards: 32, selector: ShardSelectionModeRandom},
	}

	for _, tc := range cases {
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
				buffers := make([][]byte, batch)
				for index := range buffers {
					buffers[index] = make([]byte, 0, tc.capacity)
				}
				b.StartTimer()

				for index := range buffers {
					if err := pool.Put(buffers[index]); err != nil {
						b.Fatalf("Put() returned error: %v", err)
					}
				}

				b.StopTimer()
				poolBenchmarkClearRetained(pool)
				b.StartTimer()

				completed += batch
			}
		})
	}
}

// BenchmarkPoolPutOwnerDrop measures drops decided before class/shard storage.
func BenchmarkPoolPutOwnerDrop(b *testing.B) {
	b.Run("returned_buffers_disabled", func(b *testing.B) {
		policy := poolBenchmarkPolicyForCase(poolBenchmarkCase{})
		policy.Admission.ReturnedBuffers = ReturnedBufferPolicyDrop
		pool := poolBenchmarkNewPool(b, policy)
		buffer := make([]byte, 0, 512)

		b.ReportAllocs()
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			poolBenchmarkErrorSink = pool.Put(buffer)
			if poolBenchmarkErrorSink != nil {
				b.Fatalf("Put() returned error: %v", poolBenchmarkErrorSink)
			}
		}
	})

	b.Run("oversized/action_drop", func(b *testing.B) {
		policy := poolBenchmarkPolicyForCase(poolBenchmarkCase{})
		pool := poolBenchmarkNewPool(b, policy)
		buffer := make([]byte, 0, 128*KiB.Int())

		b.ReportAllocs()
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			poolBenchmarkErrorSink = pool.Put(buffer)
			if poolBenchmarkErrorSink != nil {
				b.Fatalf("Put() returned error: %v", poolBenchmarkErrorSink)
			}
		}
	})

	b.Run("oversized/action_error", func(b *testing.B) {
		policy := poolBenchmarkPolicyForCase(poolBenchmarkCase{})
		policy.Admission.OversizedReturn = AdmissionActionError
		pool := poolBenchmarkNewPool(b, policy)
		buffer := make([]byte, 0, 128*KiB.Int())

		b.ReportAllocs()
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			poolBenchmarkErrorSink = pool.Put(buffer)
			if !errors.Is(poolBenchmarkErrorSink, ErrBufferTooLarge) {
				b.Fatalf("Put() error = %v, want ErrBufferTooLarge", poolBenchmarkErrorSink)
			}
		}
	})

	b.Run("unsupported_class/action_drop", func(b *testing.B) {
		policy := poolBenchmarkPolicyForCase(poolBenchmarkCase{})
		pool := poolBenchmarkNewPool(b, policy)
		buffer := make([]byte, 0, 256)

		b.ReportAllocs()
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			poolBenchmarkErrorSink = pool.Put(buffer)
			if poolBenchmarkErrorSink != nil {
				b.Fatalf("Put() returned error: %v", poolBenchmarkErrorSink)
			}
		}
	})

	b.Run("closed_pool_drop_returns", func(b *testing.B) {
		policy := poolBenchmarkPolicyForCase(poolBenchmarkCase{})
		pool := poolBenchmarkNewPoolWithConfig(b, PoolConfig{
			Policy:           policy,
			ClosedOperations: PoolClosedOperationModeDropReturns,
			CloseMode:        PoolCloseModeKeepRetained,
		})
		if err := pool.Close(); err != nil {
			b.Fatalf("Close() returned error: %v", err)
		}
		buffer := make([]byte, 0, 512)

		b.ReportAllocs()
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			poolBenchmarkErrorSink = pool.Put(buffer)
			if poolBenchmarkErrorSink != nil {
				b.Fatalf("Put() returned error: %v", poolBenchmarkErrorSink)
			}
		}
	})
}

// BenchmarkPoolPutShardDrop measures lower-level retention drops after owner
// admission and class routing have accepted the returned buffer.
func BenchmarkPoolPutShardDrop(b *testing.B) {
	b.Run("credit_exhausted", func(b *testing.B) {
		policy := poolBenchmarkPolicyForCase(poolBenchmarkCase{shards: 1})
		pool := poolBenchmarkNewPool(b, policy)
		class, ok := pool.table.classForCapacity(SizeFromBytes(512))
		if !ok {
			b.Fatal("benchmark class missing")
		}
		pool.mustClassStateFor(class).disableBudget()
		buffer := make([]byte, 0, 512)

		b.ReportAllocs()
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			poolBenchmarkErrorSink = pool.Put(buffer)
			if poolBenchmarkErrorSink != nil {
				b.Fatalf("Put() returned error: %v", poolBenchmarkErrorSink)
			}
		}
	})

	b.Run("bucket_full", func(b *testing.B) {
		policy := poolBenchmarkPolicyForCase(poolBenchmarkCase{
			shards:      1,
			bucketSlots: 1,
		})
		policy.Retention.MaxClassRetainedBuffers = 2
		policy.Retention.MaxShardRetainedBuffers = 2
		pool := poolBenchmarkNewPoolWithConfig(b, PoolConfig{
			Policy:           policy,
			PolicyValidation: PoolPolicyValidationModeDisabled,
		})
		if err := pool.Put(make([]byte, 0, 512)); err != nil {
			b.Fatalf("seed Put() returned error: %v", err)
		}
		buffer := make([]byte, 0, 512)

		b.ReportAllocs()
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			poolBenchmarkErrorSink = pool.Put(buffer)
			if poolBenchmarkErrorSink != nil {
				b.Fatalf("Put() returned error: %v", poolBenchmarkErrorSink)
			}
		}
	})
}

// BenchmarkPoolPutClassDrop measures class-local mismatch cost through the
// internal classState path because correct public capacity routing normally
// prevents this condition.
func BenchmarkPoolPutClassDrop(b *testing.B) {
	b.Run("class_mismatch_internal", func(b *testing.B) {
		pool := poolBenchmarkNewPool(b, poolBenchmarkPolicyForCase(poolBenchmarkCase{shards: 1}))
		class, ok := pool.table.classForExactSize(ClassSizeFromSize(KiB))
		if !ok {
			b.Fatal("benchmark class missing")
		}
		state := pool.mustClassStateFor(class)
		buffer := make([]byte, 0, 512)

		b.ReportAllocs()
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			result := state.tryRetain(0, buffer)
			if !result.RejectedByClass() {
				b.Fatalf("tryRetain() result = %#v, want class rejection", result)
			}
		}
	})
}

// BenchmarkPoolZeroing isolates retained and dropped buffer hygiene costs.
func BenchmarkPoolZeroing(b *testing.B) {
	cases := []struct {
		name         string
		capacity     int
		retainedPath bool
		zeroRetained bool
		zeroDropped  bool
	}{
		{name: "retained_false_dropped_false/cap_512", capacity: 512, retainedPath: true},
		{name: "zero_retained/cap_512", capacity: 512, retainedPath: true, zeroRetained: true},
		{name: "zero_dropped/cap_512", capacity: 512, zeroDropped: true},
		{name: "zero_retained_and_dropped/retained/cap_512", capacity: 512, retainedPath: true, zeroRetained: true, zeroDropped: true},
		{name: "zero_retained_and_dropped/dropped/cap_512", capacity: 512, zeroRetained: true, zeroDropped: true},
		{name: "retained_false_dropped_false/cap_4096", capacity: 4096, retainedPath: true},
		{name: "zero_retained/cap_4096", capacity: 4096, retainedPath: true, zeroRetained: true},
		{name: "zero_dropped/cap_4096", capacity: 4096, zeroDropped: true},
		{name: "zero_retained/cap_65536", capacity: 64 * KiB.Int(), retainedPath: true, zeroRetained: true},
		{name: "zero_dropped/cap_65536", capacity: 64 * KiB.Int(), zeroDropped: true},
	}

	for _, tc := range cases {
		tc := tc

		b.Run(tc.name, func(b *testing.B) {
			benchCase := poolBenchmarkCase{
				capacity:     tc.capacity,
				shards:       1,
				selector:     ShardSelectionModeSingle,
				zeroRetained: tc.zeroRetained,
				zeroDropped:  tc.zeroDropped,
			}
			policy := poolBenchmarkPolicyForCase(benchCase)
			if !tc.retainedPath {
				policy.Admission.ReturnedBuffers = ReturnedBufferPolicyDrop
			}
			pool := poolBenchmarkNewPool(b, policy)
			batchSize := poolBenchmarkBatchSize(tc.capacity)

			b.ReportAllocs()
			b.ResetTimer()

			for completed := 0; completed < b.N; {
				batch := batchSize
				if remaining := b.N - completed; remaining < batch {
					batch = remaining
				}

				b.StopTimer()
				buffers := make([][]byte, batch)
				for index := range buffers {
					buffers[index] = make([]byte, 0, tc.capacity)
				}
				b.StartTimer()

				for index := range buffers {
					if err := pool.Put(buffers[index]); err != nil {
						b.Fatalf("Put() returned error: %v", err)
					}
				}

				if tc.retainedPath {
					b.StopTimer()
					poolBenchmarkClearRetained(pool)
					b.StartTimer()
				}

				completed += batch
			}
		})
	}
}
