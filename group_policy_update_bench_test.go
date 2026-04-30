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

// groupPolicyPublicationBenchmarkSink keeps publication reports observable to
// the compiler while benchmarks focus on group-level foreground control cost.
var groupPolicyPublicationBenchmarkSink PoolGroupPolicyPublicationResult

// BenchmarkPoolGroupPublishPolicyNoChange measures group runtime policy
// publication when compatibility, retained-budget planning, and partition target
// publication all succeed without changing the effective value.
func BenchmarkPoolGroupPublishPolicyNoChange(b *testing.B) {
	group, policy := groupPolicyPublicationBenchmarkGroup(b, 4, 8*KiB)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result, err := group.PublishPolicy(policy)
		if err != nil {
			b.Fatalf("PublishPolicy(no change) error = %v", err)
		}
		groupPolicyPublicationBenchmarkSink = result
	}
}

// BenchmarkPoolGroupPublishPolicyContract measures alternating group retained
// budget contraction and expansion over a stable partition registry.
func BenchmarkPoolGroupPublishPolicyContract(b *testing.B) {
	group, base := groupPolicyPublicationBenchmarkGroup(b, 4, 8*KiB)
	contracted := groupPolicyPublicationBenchmarkPolicy(4 * KiB)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		policy := contracted
		if i%2 == 1 {
			policy = base
		}
		result, err := group.PublishPolicy(policy)
		if err != nil {
			b.Fatalf("PublishPolicy(contract) error = %v", err)
		}
		groupPolicyPublicationBenchmarkSink = result
	}
}

// BenchmarkPoolGroupPublishPolicyManyPartitions measures retained-budget
// publication fan-out across multiple already-built PoolPartitions.
func BenchmarkPoolGroupPublishPolicyManyPartitions(b *testing.B) {
	group, base := groupPolicyPublicationBenchmarkGroup(b, 16, 32*KiB)
	contracted := groupPolicyPublicationBenchmarkPolicy(16 * KiB)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		policy := contracted
		if i%2 == 1 {
			policy = base
		}
		result, err := group.PublishPolicy(policy)
		if err != nil {
			b.Fatalf("PublishPolicy(many partitions) error = %v", err)
		}
		groupPolicyPublicationBenchmarkSink = result
	}
}

// BenchmarkPoolGroupPublishPolicyManyPools measures group-level publication when
// many group-level Pools have been assigned into a fixed partition topology.
func BenchmarkPoolGroupPublishPolicyManyPools(b *testing.B) {
	poolNames := make([]string, 32)
	for index := range poolNames {
		poolNames[index] = fmt.Sprintf("pool-%02d", index)
	}
	base := groupPolicyPublicationBenchmarkPolicy(64 * KiB)
	config := testManagedGroupConfig(poolNames...)
	config.Policy = base
	config.Partitioning.MinPartitions = 4
	config.Partitioning.MaxPartitions = 4
	for index := range config.Pools {
		config.Pools[index].Config.Policy = poolTestSmallSingleShardPolicy()
	}
	group := MustNewPoolGroup(config)
	b.Cleanup(func() { _ = group.Close() })
	contracted := groupPolicyPublicationBenchmarkPolicy(32 * KiB)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		policy := contracted
		if i%2 == 1 {
			policy = base
		}
		result, err := group.PublishPolicy(policy)
		if err != nil {
			b.Fatalf("PublishPolicy(many pools) error = %v", err)
		}
		groupPolicyPublicationBenchmarkSink = result
	}
}

func groupPolicyPublicationBenchmarkGroup(
	b *testing.B,
	partitions int,
	retained Size,
) (*PoolGroup, PoolGroupPolicy) {
	b.Helper()

	names := make([]string, partitions)
	for index := range names {
		names[index] = fmt.Sprintf("partition-%02d", index)
	}
	policy := groupPolicyPublicationBenchmarkPolicy(retained)
	config := groupPolicyUpdateConfig(names...)
	config.Policy = policy
	group := MustNewPoolGroup(config)
	b.Cleanup(func() { _ = group.Close() })
	return group, policy
}

func groupPolicyPublicationBenchmarkPolicy(retained Size) PoolGroupPolicy {
	policy := DefaultPoolGroupPolicy()
	policy.Budget.MaxRetainedBytes = retained
	return policy
}
