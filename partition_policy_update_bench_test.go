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

// partitionPolicyPublicationBenchmarkSink keeps publication reports observable
// to the compiler while the benchmark loop focuses on the partition-local
// control operation cost.
var partitionPolicyPublicationBenchmarkSink PoolPartitionPolicyPublicationResult

// BenchmarkPoolPartitionPublishPolicyNoChange measures the steady foreground
// publication path when policy compatibility checks, owned-Pool drift checks,
// and retained-budget planning all succeed but the effective policy value is
// unchanged.
func BenchmarkPoolPartitionPublishPolicyNoChange(b *testing.B) {
	partition, policy := partitionPolicyPublicationBenchmarkNew(b, 4, 2*KiB, false)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result, err := partition.PublishPolicy(policy)
		if err != nil {
			b.Fatalf("PublishPolicy(no change) error = %v", err)
		}
		partitionPolicyPublicationBenchmarkSink = result
	}
}

// BenchmarkPoolPartitionPublishPolicyContract measures alternating retained
// target contraction and expansion without physical trim. The loop keeps the
// same partition and owned Pools so it captures runtime-snapshot publication,
// two-phase budget planning, and prevalidated Pool/class budget application.
func BenchmarkPoolPartitionPublishPolicyContract(b *testing.B) {
	partition, base := partitionPolicyPublicationBenchmarkNew(b, 4, 2*KiB, false)
	contracted := partitionPolicyPublicationBenchmarkPolicy(4, KiB, false)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		policy := contracted
		if i%2 == 1 {
			policy = base
		}
		result, err := partition.PublishPolicy(policy)
		if err != nil {
			b.Fatalf("PublishPolicy(contract) error = %v", err)
		}
		partitionPolicyPublicationBenchmarkSink = result
	}
}

// BenchmarkPoolPartitionPublishPolicyContractWithTrim measures contraction plus
// one bounded retained trim cycle. Retained-buffer setup and reset happen
// outside the timed region so the measured work is policy publication,
// target-aware trim planning, and bounded physical trim dispatch.
func BenchmarkPoolPartitionPublishPolicyContractWithTrim(b *testing.B) {
	partition, base := partitionPolicyPublicationBenchmarkNew(b, 1, 2*KiB, true)
	contracted := partitionPolicyPublicationBenchmarkPolicy(1, SizeFromBytes(512), true)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		if _, err := partition.PublishPolicy(base); err != nil {
			b.Fatalf("PublishPolicy(reset) error = %v", err)
		}
		partitionPolicyPublicationBenchmarkResetRetained(b, partition, 512, 2)
		b.StartTimer()

		result, err := partition.PublishPolicy(contracted)
		if err != nil {
			b.Fatalf("PublishPolicy(contract+trim) error = %v", err)
		}
		if !result.TrimAttempted {
			b.Fatalf("PublishPolicy(contract+trim) = %+v, want bounded trim attempt", result)
		}
		partitionPolicyPublicationBenchmarkSink = result
	}
}

// BenchmarkPoolPartitionPublishPolicyManyPools measures policy publication fan
// out across a larger owned-Pool directory. It keeps setup outside the timed
// loop and uses a feasible retained target so allocation diagnostics remain on
// the successful publication path.
func BenchmarkPoolPartitionPublishPolicyManyPools(b *testing.B) {
	partition, base := partitionPolicyPublicationBenchmarkNew(b, 32, 2*KiB, false)
	contracted := partitionPolicyPublicationBenchmarkPolicy(32, KiB, false)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		policy := contracted
		if i%2 == 1 {
			policy = base
		}
		result, err := partition.PublishPolicy(policy)
		if err != nil {
			b.Fatalf("PublishPolicy(many pools) error = %v", err)
		}
		partitionPolicyPublicationBenchmarkSink = result
	}
}

// partitionPolicyPublicationBenchmarkNew constructs a partition whose initial
// runtime policy already matches the returned policy. That keeps no-change
// benchmarks honest and avoids measuring initial construction or cleanup.
func partitionPolicyPublicationBenchmarkNew(
	b *testing.B,
	poolCount int,
	retainedPerPool Size,
	trimOnShrink bool,
) (*PoolPartition, PartitionPolicy) {
	b.Helper()

	names := make([]string, poolCount)
	for index := range names {
		names[index] = "pool_" + strconv.Itoa(index)
	}
	policy := partitionPolicyPublicationBenchmarkPolicy(poolCount, retainedPerPool, trimOnShrink)
	config := partitionPolicyUpdateConfig(names...)
	config.Policy = policy

	partition := MustNewPoolPartition(config)
	b.Cleanup(func() {
		_ = partition.Close()
	})

	return partition, policy
}

// partitionPolicyPublicationBenchmarkPolicy returns a valid partition policy
// whose retained target is sized per owned Pool. The helper keeps trim bounds
// small and explicit so contraction-with-trim benchmarks remain bounded.
func partitionPolicyPublicationBenchmarkPolicy(
	poolCount int,
	retainedPerPool Size,
	trimOnShrink bool,
) PartitionPolicy {
	retainedBytes := retainedPerPool.Bytes() * uint64(poolCount)
	return PartitionPolicy{
		Budget: PartitionBudgetPolicy{
			MaxRetainedBytes: SizeFromBytes(retainedBytes),
		},
		Trim: PartitionTrimPolicy{
			Enabled:                   true,
			MaxPoolsPerCycle:          poolCount,
			MaxBuffersPerCycle:        8,
			MaxBytesPerCycle:          SizeFromBytes(8 * 512),
			MaxClassesPerPoolPerCycle: 1,
			MaxShardsPerClassPerCycle: 1,
			TrimOnPolicyShrink:        trimOnShrink,
		},
	}
}

// partitionPolicyPublicationBenchmarkResetRetained clears and seeds owned Pool
// retained storage outside timed regions. It does not call Pool.Get or
// Pool.Put, so publication benchmarks keep data-plane operations out of the
// measured contraction path.
func partitionPolicyPublicationBenchmarkResetRetained(
	b *testing.B,
	partition *PoolPartition,
	capacity int,
	countPerPool int,
) {
	b.Helper()

	for _, entry := range partition.registry.entries {
		poolBenchmarkClearRetained(entry.pool)
		poolBenchmarkSeedRetainedInternal(b, entry.pool, capacity, countPerPool)
	}
}
