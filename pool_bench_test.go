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

// BenchmarkPoolGetPutSequential measures the ordinary single-goroutine
// data-plane loop after one buffer has been seeded into retained storage.
func BenchmarkPoolGetPutSequential(b *testing.B) {
	pool := MustNew(PoolConfig{Policy: poolBenchmarkPolicy(1, ShardSelectionModeSingle)})
	b.Cleanup(func() {
		_ = pool.Close()
	})

	if err := pool.Put(make([]byte, 0, 512)); err != nil {
		b.Fatalf("seed Put() returned error: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		buffer, err := pool.Get(300)
		if err != nil {
			b.Fatalf("Get() returned error: %v", err)
		}

		if err := pool.Put(buffer); err != nil {
			b.Fatalf("Put() returned error: %v", err)
		}
	}
}

// BenchmarkPoolGetPutParallel measures concurrent Get/Put with class-local
// shard selection and shard-owned synchronization.
func BenchmarkPoolGetPutParallel(b *testing.B) {
	pool := MustNew(PoolConfig{Policy: poolBenchmarkPolicy(32, ShardSelectionModeRandom)})
	b.Cleanup(func() {
		_ = pool.Close()
	})

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			buffer, err := pool.Get(300)
			if err != nil {
				b.Fatalf("Get() returned error: %v", err)
			}

			if err := pool.Put(buffer); err != nil {
				b.Fatalf("Put() returned error: %v", err)
			}
		}
	})
}

// BenchmarkPoolSnapshot measures the public diagnostic snapshot path. Snapshot
// allocates class/shard slices and clones Policy by design.
func BenchmarkPoolSnapshot(b *testing.B) {
	pool := MustNew(PoolConfig{Policy: poolBenchmarkPolicy(8, ShardSelectionModeRandom)})
	b.Cleanup(func() {
		_ = pool.Close()
	})
	poolBenchmarkSeedRetained(b, pool, 16)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = pool.Snapshot()
	}
}

// BenchmarkPoolMetrics measures the public aggregate metrics path. Metrics uses
// an internal aggregate sample and avoids public class/shard slice allocation.
func BenchmarkPoolMetrics(b *testing.B) {
	pool := MustNew(PoolConfig{Policy: poolBenchmarkPolicy(8, ShardSelectionModeRandom)})
	b.Cleanup(func() {
		_ = pool.Close()
	})
	poolBenchmarkSeedRetained(b, pool, 16)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = pool.Metrics()
	}
}

// BenchmarkPoolSampleCounters measures the internal controller-facing aggregate
// sample path. It reuses caller-owned storage and avoids public DTO allocation.
func BenchmarkPoolSampleCounters(b *testing.B) {
	pool := MustNew(PoolConfig{Policy: poolBenchmarkPolicy(8, ShardSelectionModeRandom)})
	b.Cleanup(func() {
		_ = pool.Close()
	})
	poolBenchmarkSeedRetained(b, pool, 16)

	var sample poolCounterSample

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		pool.sampleCounters(&sample)
	}
}

// BenchmarkPoolShardSelectorParallel measures selector contention for the
// selector modes Pool can wire today without private runtime P-local APIs.
func BenchmarkPoolShardSelectorParallel(b *testing.B) {
	modes := []struct {
		name string
		mode ShardSelectionMode
	}{
		{name: "single", mode: ShardSelectionModeSingle},
		{name: "round_robin", mode: ShardSelectionModeRoundRobin},
		{name: "random", mode: ShardSelectionModeRandom},
	}

	for _, mode := range modes {
		mode := mode

		b.Run(mode.name, func(b *testing.B) {
			selector, err := newPoolShardSelector(mode.mode)
			if err != nil {
				b.Fatalf("newPoolShardSelector() returned error: %v", err)
			}

			b.ReportAllocs()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					_ = selector.SelectShard(32)
				}
			})
		})
	}
}

func poolBenchmarkPolicy(shardsPerClass int, selection ShardSelectionMode) Policy {
	policy := poolTestSmallSingleShardPolicy()
	policy.Retention.SoftRetainedBytes = 16 * KiB
	policy.Retention.HardRetainedBytes = 32 * KiB
	policy.Retention.MaxRetainedBuffers = 128
	policy.Retention.MaxClassRetainedBytes = 16 * KiB
	policy.Retention.MaxClassRetainedBuffers = 64
	policy.Retention.MaxShardRetainedBytes = 16 * KiB
	policy.Retention.MaxShardRetainedBuffers = 64
	policy.Shards.Selection = selection
	policy.Shards.ShardsPerClass = shardsPerClass
	policy.Shards.BucketSlotsPerShard = 64

	return policy
}

func poolBenchmarkSeedRetained(b *testing.B, pool *Pool, count int) {
	b.Helper()

	for index := 0; index < count; index++ {
		if err := pool.Put(make([]byte, 0, 512)); err != nil {
			b.Fatalf("seed Put(%d) returned error: %v", index, err)
		}
	}
}
