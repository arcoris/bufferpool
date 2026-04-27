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

// TestPoolMetricsInitialState verifies aggregate metrics projection after
// construction.
func TestPoolMetricsInitialState(t *testing.T) {
	t.Parallel()

	policy := poolTestSmallSingleShardPolicy()
	pool := MustNew(PoolConfig{
		Name:   "metrics",
		Policy: policy,
	})
	defer closePoolForTest(t, pool)

	metrics := pool.Metrics()

	if metrics.Name != "metrics" {
		t.Fatalf("Metrics().Name = %q, want %q", metrics.Name, "metrics")
	}

	if metrics.Lifecycle != LifecycleActive {
		t.Fatalf("Metrics().Lifecycle = %s, want %s", metrics.Lifecycle, LifecycleActive)
	}
	if metrics.Generation != InitialGeneration {
		t.Fatalf("Metrics().Generation = %s, want %s", metrics.Generation, InitialGeneration)
	}

	if metrics.ClassCount != len(policy.Classes.Sizes) {
		t.Fatalf("Metrics().ClassCount = %d, want %d", metrics.ClassCount, len(policy.Classes.Sizes))
	}

	if metrics.ShardCount != len(policy.Classes.Sizes)*policy.Shards.ShardsPerClass {
		t.Fatalf("Metrics().ShardCount = %d, want %d",
			metrics.ShardCount,
			len(policy.Classes.Sizes)*policy.Shards.ShardsPerClass,
		)
	}

	if !metrics.IsZero() {
		t.Fatalf("initial metrics should be zero: %#v", metrics)
	}
}

// TestPoolMetricsIncludesOwnerSideDrops verifies Metrics uses owner-side return
// counters even when a returned buffer never reaches class/shard storage.
func TestPoolMetricsIncludesOwnerSideDrops(t *testing.T) {
	t.Parallel()

	policy := poolTestSmallSingleShardPolicy()
	policy.Admission.ReturnedBuffers = ReturnedBufferPolicyDrop

	pool := MustNew(PoolConfig{Policy: policy})
	defer closePoolForTest(t, pool)

	if err := pool.Put(make([]byte, 0, 512)); err != nil {
		t.Fatalf("Put() returned error: %v", err)
	}

	metrics := pool.Metrics()
	if metrics.Puts != 1 || metrics.ReturnedBytes != 512 {
		t.Fatalf("put counters = %d/%d, want 1/512", metrics.Puts, metrics.ReturnedBytes)
	}
	if metrics.Drops != 1 || metrics.DroppedBytes != 512 {
		t.Fatalf("drop counters = %d/%d, want 1/512", metrics.Drops, metrics.DroppedBytes)
	}
	if metrics.DropReasons.ReturnedBuffersDisabled != 1 {
		t.Fatalf("returned-buffer-disabled drops = %d, want 1", metrics.DropReasons.ReturnedBuffersDisabled)
	}
	if metrics.DropRatio != PolicyRatioOne {
		t.Fatalf("DropRatio = %d, want %d", metrics.DropRatio, PolicyRatioOne)
	}
}

// TestPoolMetricsAfterActivity verifies metrics projection over Get/Put/reuse.
func TestPoolMetricsAfterActivity(t *testing.T) {
	t.Parallel()

	pool := MustNew(PoolConfig{Policy: poolTestSingleShardPolicy()})
	defer closePoolForTest(t, pool)

	buffer, err := pool.Get(512)
	if err != nil {
		t.Fatalf("Get() returned error: %v", err)
	}

	if err := pool.Put(buffer); err != nil {
		t.Fatalf("Put() returned error: %v", err)
	}

	if _, err := pool.Get(512); err != nil {
		t.Fatalf("second Get() returned error: %v", err)
	}

	metrics := pool.Metrics()

	if metrics.IsZero() {
		t.Fatal("metrics should not be zero after activity")
	}

	if metrics.Gets != 2 {
		t.Fatalf("Gets = %d, want 2", metrics.Gets)
	}

	if metrics.Misses != 1 {
		t.Fatalf("Misses = %d, want 1", metrics.Misses)
	}

	if metrics.Hits != 1 {
		t.Fatalf("Hits = %d, want 1", metrics.Hits)
	}

	if metrics.Allocations != 1 {
		t.Fatalf("Allocations = %d, want 1", metrics.Allocations)
	}

	if metrics.Puts != 1 {
		t.Fatalf("Puts = %d, want 1", metrics.Puts)
	}

	if metrics.Retains != 1 {
		t.Fatalf("Retains = %d, want 1", metrics.Retains)
	}

	if metrics.CurrentRetainedBuffers != 0 || metrics.CurrentRetainedBytes != 0 {
		t.Fatalf("current retained usage = %d/%d, want zero after reuse",
			metrics.CurrentRetainedBuffers,
			metrics.CurrentRetainedBytes,
		)
	}

	if metrics.ReuseAttempts() != 2 {
		t.Fatalf("ReuseAttempts() = %d, want 2", metrics.ReuseAttempts())
	}

	if metrics.PutOutcomes() != 1 {
		t.Fatalf("PutOutcomes() = %d, want 1", metrics.PutOutcomes())
	}

	if metrics.HitRatio != PolicyRatioOne/2 {
		t.Fatalf("HitRatio = %d, want %d", metrics.HitRatio, PolicyRatioOne/2)
	}

	if metrics.MissRatio != PolicyRatioOne/2 {
		t.Fatalf("MissRatio = %d, want %d", metrics.MissRatio, PolicyRatioOne/2)
	}

	if metrics.RetainRatio != PolicyRatioOne {
		t.Fatalf("RetainRatio = %d, want %d", metrics.RetainRatio, PolicyRatioOne)
	}

	if metrics.DropRatio != 0 {
		t.Fatalf("DropRatio = %d, want 0", metrics.DropRatio)
	}
}

// TestNewPoolMetricsFromSnapshot verifies the pure snapshot-to-metrics
// conversion helper.
func TestNewPoolMetricsFromSnapshot(t *testing.T) {
	t.Parallel()

	snapshot := PoolSnapshot{
		Name:      "projection",
		Lifecycle: LifecycleActive,
		Classes: []PoolClassSnapshot{
			{Shards: []PoolShardSnapshot{{}, {}}},
			{Shards: []PoolShardSnapshot{{}}},
		},
		CurrentRetainedBuffers: 3,
		CurrentRetainedBytes:   1536,
		Counters: PoolCountersSnapshot{
			Hits:                   1,
			Misses:                 3,
			Retains:                2,
			Drops:                  2,
			TrimOperations:         1,
			ClearOperations:        2,
			TrimmedBuffers:         3,
			ClearedBuffers:         4,
			TrimmedBytes:           5,
			ClearedBytes:           6,
			CurrentRetainedBuffers: 3,
			CurrentRetainedBytes:   1536,
		},
	}

	metrics := NewPoolMetrics(snapshot)

	if metrics.Name != "projection" {
		t.Fatalf("Name = %q, want projection", metrics.Name)
	}

	if metrics.ClassCount != 2 {
		t.Fatalf("ClassCount = %d, want 2", metrics.ClassCount)
	}

	if metrics.ShardCount != 3 {
		t.Fatalf("ShardCount = %d, want 3", metrics.ShardCount)
	}

	if metrics.HitRatio != PolicyRatio(2500) {
		t.Fatalf("HitRatio = %d, want 2500", metrics.HitRatio)
	}

	if metrics.MissRatio != PolicyRatio(7500) {
		t.Fatalf("MissRatio = %d, want 7500", metrics.MissRatio)
	}

	if metrics.RetainRatio != PolicyRatio(5000) {
		t.Fatalf("RetainRatio = %d, want 5000", metrics.RetainRatio)
	}

	if metrics.DropRatio != PolicyRatio(5000) {
		t.Fatalf("DropRatio = %d, want 5000", metrics.DropRatio)
	}

	if metrics.RemovalOperations() != 3 {
		t.Fatalf("RemovalOperations() = %d, want 3", metrics.RemovalOperations())
	}

	if metrics.RemovedBuffers() != 7 {
		t.Fatalf("RemovedBuffers() = %d, want 7", metrics.RemovedBuffers())
	}

	if metrics.RemovedBytes() != 11 {
		t.Fatalf("RemovedBytes() = %d, want 11", metrics.RemovedBytes())
	}
}

// TestPoolRatio verifies fixed-point ratio projection.
func TestPoolRatio(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		numerator   uint64
		denominator uint64
		want        PolicyRatio
	}{
		{
			name:        "zero denominator",
			numerator:   1,
			denominator: 0,
			want:        0,
		},
		{
			name:        "zero numerator",
			numerator:   0,
			denominator: 10,
			want:        0,
		},
		{
			name:        "one",
			numerator:   10,
			denominator: 10,
			want:        PolicyRatioOne,
		},
		{
			name:        "greater than denominator clamps",
			numerator:   11,
			denominator: 10,
			want:        PolicyRatioOne,
		},
		{
			name:        "half",
			numerator:   1,
			denominator: 2,
			want:        PolicyRatioOne / 2,
		},
		{
			name:        "large values avoid overflow",
			numerator:   ^uint64(0) - 1,
			denominator: ^uint64(0),
			want:        PolicyRatio(9999),
		},
		{
			name:        "quarter",
			numerator:   1,
			denominator: 4,
			want:        PolicyRatio(2500),
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := poolRatio(tt.numerator, tt.denominator); got != tt.want {
				t.Fatalf("poolRatio(%d, %d) = %d, want %d",
					tt.numerator,
					tt.denominator,
					got,
					tt.want,
				)
			}
		})
	}
}
