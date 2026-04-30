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

// TestPoolRuntimePolicyCompatible verifies the topology compatibility predicate
// used by runtime snapshot publication.
func TestPoolRuntimePolicyCompatible(t *testing.T) {
	t.Parallel()

	base := poolTestSingleShardPolicy()

	tests := []struct {
		name   string
		mutate func(*Policy)
		want   bool
	}{
		{
			name: "same policy",
			want: true,
		},
		{
			name: "dynamic admission change",
			mutate: func(policy *Policy) {
				policy.Admission.ZeroDroppedBuffers = !policy.Admission.ZeroDroppedBuffers
			},
			want: true,
		},
		{
			name: "dynamic retention limit change",
			mutate: func(policy *Policy) {
				policy.Retention.MaxRequestSize = policy.Retention.MaxRequestSize / 2
			},
			want: true,
		},
		{
			name: "dynamic acquisition fallback change",
			mutate: func(policy *Policy) {
				policy.Shards.AcquisitionFallbackShards++
			},
			want: true,
		},
		{
			name: "class sizes changed",
			mutate: func(policy *Policy) {
				policy.Classes.Sizes[0] = ClassSizeFromBytes(384)
			},
			want: false,
		},
		{
			name: "shards per class changed",
			mutate: func(policy *Policy) {
				policy.Shards.ShardsPerClass++
			},
			want: false,
		},
		{
			name: "bucket slots changed",
			mutate: func(policy *Policy) {
				policy.Shards.BucketSlotsPerShard++
			},
			want: false,
		},
		{
			name: "bucket segment slots changed",
			mutate: func(policy *Policy) {
				policy.Shards.BucketSegmentSlotsPerShard++
			},
			want: false,
		},
		{
			name: "selection changed",
			mutate: func(policy *Policy) {
				policy.Shards.Selection = ShardSelectionModeRandom
			},
			want: false,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			runtime := clonePoolPolicy(base)
			if tt.mutate != nil {
				tt.mutate(&runtime)
			}

			if got := poolRuntimePolicyCompatible(base, runtime); got != tt.want {
				t.Fatalf("poolRuntimePolicyCompatible() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestPoolRuntimeSnapshotPublicationRejectsTopologyChanges verifies that the
// publication hook cannot make Pool.Policy disagree with constructed storage.
func TestPoolRuntimeSnapshotPublicationRejectsTopologyChanges(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*Policy)
	}{
		{
			name: "class sizes",
			mutate: func(policy *Policy) {
				policy.Classes.Sizes[0] = ClassSizeFromBytes(384)
			},
		},
		{
			name: "shards per class",
			mutate: func(policy *Policy) {
				policy.Shards.ShardsPerClass++
			},
		},
		{
			name: "bucket slots per shard",
			mutate: func(policy *Policy) {
				policy.Shards.BucketSlotsPerShard++
			},
		},
		{
			name: "bucket segment slots per shard",
			mutate: func(policy *Policy) {
				policy.Shards.BucketSegmentSlotsPerShard++
			},
		},
		{
			name: "selection mode",
			mutate: func(policy *Policy) {
				policy.Shards.Selection = ShardSelectionModeRandom
			},
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			pool := MustNew(PoolConfig{Policy: poolTestSingleShardPolicy()})
			defer closePoolForTest(t, pool)

			policy := pool.Policy()
			tt.mutate(&policy)

			assertPoolPanic(t, errPoolRuntimeSnapshotIncompatible, func() {
				pool.publishRuntimeSnapshot(newPoolRuntimeSnapshot(InitialGeneration.Next(), policy))
			})
		})
	}
}

// TestPoolRuntimeSnapshotPublicationAcceptsDynamicPolicyChanges verifies
// compatible runtime fields can be published without rebuilding topology.
func TestPoolRuntimeSnapshotPublicationAcceptsDynamicPolicyChanges(t *testing.T) {
	t.Parallel()

	pool := MustNew(PoolConfig{Policy: poolTestSingleShardPolicy()})
	defer closePoolForTest(t, pool)

	policy := pool.Policy()
	policy.Admission.ZeroDroppedBuffers = !policy.Admission.ZeroDroppedBuffers
	policy.Retention.MaxRequestSize = policy.Retention.MaxRequestSize / 2
	policy.Shards.AcquisitionFallbackShards++

	pool.publishRuntimeSnapshot(newPoolRuntimeSnapshot(InitialGeneration.Next(), policy))

	snapshot := pool.Snapshot()
	if snapshot.Generation != InitialGeneration.Next() {
		t.Fatalf("snapshot generation = %s, want %s", snapshot.Generation, InitialGeneration.Next())
	}
	if snapshot.Policy.Admission.ZeroDroppedBuffers != policy.Admission.ZeroDroppedBuffers {
		t.Fatal("dynamic admission change was not published")
	}
	if snapshot.Policy.Retention.MaxRequestSize != policy.Retention.MaxRequestSize {
		t.Fatal("dynamic retention limit change was not published")
	}
	if snapshot.Policy.Shards.AcquisitionFallbackShards != policy.Shards.AcquisitionFallbackShards {
		t.Fatal("dynamic acquisition fallback change was not published")
	}
}
