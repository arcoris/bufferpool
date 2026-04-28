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
	// leaseBenchmarkErrorSink keeps release validation errors observable to the
	// compiler in benchmarks that intentionally ignore the exact error value.
	leaseBenchmarkErrorSink error

	// leaseBenchmarkSnapshotSink keeps registry snapshots observable to the
	// compiler when measuring Snapshot.
	leaseBenchmarkSnapshotSink LeaseRegistrySnapshot
)

// Lease benchmark command guide:
//
//	go test -run '^$' -bench 'BenchmarkLease' -benchmem ./...
//
// Lease benchmarks measure ownership-aware path overhead: lease record
// allocation, registry bookkeeping, strict/accounting validation, active gauges,
// and best-effort Pool.Put handoff. Bare Pool benchmarks measure the retained
// storage data plane without ownership records. The contracts are different and
// should not be compared as interchangeable APIs.

// BenchmarkLeaseRegistryAcquireReleaseStrict measures the ordinary strict
// ownership path for representative request sizes.
func BenchmarkLeaseRegistryAcquireReleaseStrict(b *testing.B) {
	for _, tc := range []struct {
		name string
		size int
	}{
		{name: "size_128", size: 128},
		{name: "size_300", size: 300},
		{name: "size_900", size: 900},
	} {
		tc := tc
		b.Run(tc.name, func(b *testing.B) {
			pool := poolBenchmarkNewPool(b, poolTestSmallSingleShardPolicy())
			registry := leaseBenchmarkNewRegistry(b, DefaultLeaseConfig())

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				lease, err := registry.Acquire(pool, tc.size)
				if err != nil {
					b.Fatalf("Acquire() returned error: %v", err)
				}
				if err := lease.ReleaseUnchanged(); err != nil {
					b.Fatalf("ReleaseUnchanged() returned error: %v", err)
				}
			}
		})
	}
}

// BenchmarkLeaseRegistryAcquireReleaseAccounting measures the same ownership
// lifecycle without strict backing-array identity validation.
func BenchmarkLeaseRegistryAcquireReleaseAccounting(b *testing.B) {
	pool := poolBenchmarkNewPool(b, poolTestSmallSingleShardPolicy())
	registry := leaseBenchmarkNewRegistry(b, LeaseConfigFromOwnershipPolicy(AccountingOwnershipPolicy()))

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lease, err := registry.Acquire(pool, 300)
		if err != nil {
			b.Fatalf("Acquire() returned error: %v", err)
		}
		if err := lease.ReleaseUnchanged(); err != nil {
			b.Fatalf("ReleaseUnchanged() returned error: %v", err)
		}
	}
}

// BenchmarkLeaseRegistrySnapshot measures diagnostic snapshot cost as the
// active lease set grows.
func BenchmarkLeaseRegistrySnapshot(b *testing.B) {
	for _, active := range []int{0, 16, 256} {
		active := active
		b.Run("active_"+strconv.Itoa(active), func(b *testing.B) {
			pool := poolBenchmarkNewPool(b, poolBenchmarkPolicyForCase(poolBenchmarkCase{
				size:        128,
				capacity:    512,
				shards:      1,
				selector:    ShardSelectionModeSingle,
				bucketSlots: 512,
			}))
			registry := leaseBenchmarkNewRegistry(b, DefaultLeaseConfig())
			leases := leaseBenchmarkAcquireActive(b, registry, pool, active)

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				leaseBenchmarkSnapshotSink = registry.Snapshot()
			}
			b.StopTimer()

			leaseBenchmarkReleaseAll(b, leases)
		})
	}
}

// BenchmarkLeaseReleaseValidation measures rejected strict-release validation
// paths without consuming the active lease.
func BenchmarkLeaseReleaseValidation(b *testing.B) {
	b.Run("strict_foreign_buffer", func(b *testing.B) {
		pool := poolBenchmarkNewPool(b, poolTestSmallSingleShardPolicy())
		registry := leaseBenchmarkNewRegistry(b, DefaultLeaseConfig())
		lease := leaseBenchmarkAcquireOne(b, registry, pool, 128)
		foreign := make([]byte, 128, 512)

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			leaseBenchmarkErrorSink = lease.Release(foreign)
		}
		b.StopTimer()

		if err := lease.ReleaseUnchanged(); err != nil {
			b.Fatalf("cleanup ReleaseUnchanged() returned error: %v", err)
		}
	})

	b.Run("strict_shifted_subslice", func(b *testing.B) {
		pool := poolBenchmarkNewPool(b, poolTestSmallSingleShardPolicy())
		registry := leaseBenchmarkNewRegistry(b, DefaultLeaseConfig())
		lease := leaseBenchmarkAcquireOne(b, registry, pool, 128)
		shifted := lease.Buffer()[1:]

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			leaseBenchmarkErrorSink = lease.Release(shifted)
		}
		b.StopTimer()

		if err := lease.ReleaseUnchanged(); err != nil {
			b.Fatalf("cleanup ReleaseUnchanged() returned error: %v", err)
		}
	})
}

// BenchmarkLeaseReleaseAfterPoolClose measures ownership completion when the
// retained-storage handoff is rejected by Pool lifecycle.
func BenchmarkLeaseReleaseAfterPoolClose(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		pool := MustNew(PoolConfig{Policy: poolTestSmallSingleShardPolicy()})
		registry := MustNewLeaseRegistry(DefaultLeaseConfig())
		lease, err := registry.Acquire(pool, 128)
		if err != nil {
			b.Fatalf("Acquire() returned error: %v", err)
		}
		buffer := lease.Buffer()
		if err := pool.Close(); err != nil {
			b.Fatalf("Pool.Close() returned error: %v", err)
		}

		b.StartTimer()
		leaseBenchmarkErrorSink = lease.Release(buffer)
		b.StopTimer()

		if err := registry.Close(); err != nil {
			b.Fatalf("LeaseRegistry.Close() returned error: %v", err)
		}
	}
}

// leaseBenchmarkNewRegistry constructs a LeaseRegistry and registers cleanup for
// benchmark subtests.
func leaseBenchmarkNewRegistry(b *testing.B, config LeaseConfig) *LeaseRegistry {
	b.Helper()

	registry := MustNewLeaseRegistry(config)
	b.Cleanup(func() {
		_ = registry.Close()
	})

	return registry
}

// leaseBenchmarkAcquireOne acquires one lease during benchmark setup or measured
// loops and fails the benchmark on unexpected errors.
func leaseBenchmarkAcquireOne(b *testing.B, registry *LeaseRegistry, pool *Pool, size int) Lease {
	b.Helper()

	lease, err := registry.Acquire(pool, size)
	if err != nil {
		b.Fatalf("Acquire() returned error: %v", err)
	}

	return lease
}

// leaseBenchmarkAcquireActive creates a stable active lease set for Snapshot
// benchmarks.
func leaseBenchmarkAcquireActive(b *testing.B, registry *LeaseRegistry, pool *Pool, count int) []Lease {
	b.Helper()

	leases := make([]Lease, count)
	for index := range leases {
		leases[index] = leaseBenchmarkAcquireOne(b, registry, pool, 128)
	}

	return leases
}

// leaseBenchmarkReleaseAll releases setup leases after benchmark timing stops.
func leaseBenchmarkReleaseAll(b *testing.B, leases []Lease) {
	b.Helper()

	for _, lease := range leases {
		if err := lease.ReleaseUnchanged(); err != nil {
			b.Fatalf("ReleaseUnchanged() returned error: %v", err)
		}
	}
}
