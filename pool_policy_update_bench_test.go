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

// poolPolicyPublicationBenchmarkSink keeps publication reports observable to
// the compiler while benchmarks focus on the Pool-local control operation cost.
var poolPolicyPublicationBenchmarkSink PoolPolicyPublicationResult

func BenchmarkPoolPublishPolicyNoChange(b *testing.B) {
	policy := poolPolicyPublicationBenchmarkPolicy(false)
	pool := poolBenchmarkNewPool(b, policy)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result, err := pool.PublishPolicy(policy)
		if err != nil {
			b.Fatalf("PublishPolicy(no change) error = %v", err)
		}
		poolPolicyPublicationBenchmarkSink = result
	}
}

func BenchmarkPoolPublishPolicyContract(b *testing.B) {
	base := poolPolicyPublicationBenchmarkPolicy(false)
	contracted := poolPolicyPublicationContractedPolicy(base, false)
	pool := poolBenchmarkNewPool(b, base)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		policy := contracted
		if i%2 == 1 {
			policy = base
		}
		result, err := pool.PublishPolicy(policy)
		if err != nil {
			b.Fatalf("PublishPolicy(contract) error = %v", err)
		}
		poolPolicyPublicationBenchmarkSink = result
	}
}

func BenchmarkPoolPublishPolicyContractWithTrim(b *testing.B) {
	base := poolPolicyPublicationBenchmarkPolicy(false)
	contracted := poolPolicyPublicationContractedPolicy(base, true)
	pool := poolBenchmarkNewPool(b, base)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		poolBenchmarkClearRetained(pool)
		poolBenchmarkSeedRetainedInternal(b, pool, 512, 8)
		if _, err := pool.PublishPolicy(base); err != nil {
			b.Fatalf("PublishPolicy(reset) error = %v", err)
		}
		b.StartTimer()

		result, err := pool.PublishPolicy(contracted)
		if err != nil {
			b.Fatalf("PublishPolicy(contract+trim) error = %v", err)
		}
		if !result.TrimAttempted {
			b.Fatalf("PublishPolicy(contract+trim) = %+v, want bounded trim attempt", result)
		}
		poolPolicyPublicationBenchmarkSink = result
	}
}

func poolPolicyPublicationBenchmarkPolicy(trimOnShrink bool) Policy {
	policy := poolBenchmarkPolicyForCase(poolBenchmarkCase{
		classes: []ClassSize{
			ClassSizeFromBytes(512),
			ClassSizeFromSize(KiB),
			ClassSizeFromSize(4 * KiB),
		},
		shards:      2,
		selector:    ShardSelectionModeProcessorInspired,
		bucketSlots: 64,
	})
	policy.Trim = poolPolicyUpdateTrimPolicy(8, SizeFromBytes(8*512), trimOnShrink)
	return policy
}

func poolPolicyPublicationContractedPolicy(base Policy, trimOnShrink bool) Policy {
	contracted := base
	contracted.Retention.SoftRetainedBytes = SizeFromBytes(base.Retention.SoftRetainedBytes.Bytes() / 2)
	contracted.Trim = poolPolicyUpdateTrimPolicy(8, SizeFromBytes(8*512), trimOnShrink)
	return contracted
}
