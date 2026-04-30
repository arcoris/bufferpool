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

// TestPoolTrimClassRemovesRetainedBuffers verifies class-scoped physical trim.
func TestPoolTrimClassRemovesRetainedBuffers(t *testing.T) {
	pool := MustNew(PoolConfig{Policy: poolTestSmallSingleShardPolicy()})
	t.Cleanup(func() {
		if err := pool.Close(); err != nil {
			t.Fatalf("Pool.Close() error = %v", err)
		}
	})
	seedPoolRetainedBuffers(t, pool, 2, 512)

	before := pool.Metrics()
	if before.CurrentRetainedBuffers != 2 || before.CurrentRetainedBytes != 1024 {
		t.Fatalf("setup metrics = %+v, want two retained 512-byte buffers", before)
	}

	result := pool.TrimClass(ClassID(0), 1, SizeFromBytes(512))
	if !result.Attempted || !result.Executed || result.TrimmedBuffers != 1 || result.TrimmedBytes != 512 {
		t.Fatalf("TrimClass() = %+v, want one removed buffer", result)
	}
	after := pool.Metrics()
	if after.CurrentRetainedBuffers != 1 || after.CurrentRetainedBytes != 512 {
		t.Fatalf("metrics after TrimClass = %+v, want one retained 512-byte buffer", after)
	}
	if after.TrimOperations != 1 || after.TrimmedBuffers != 1 || after.TrimmedBytes != 512 {
		t.Fatalf("trim counters after TrimClass = %+v, want one trim operation", after)
	}
}

// TestPoolTrimShardBoundsBytes verifies shard-scoped byte bounds.
func TestPoolTrimShardBoundsBytes(t *testing.T) {
	pool := MustNew(PoolConfig{Policy: poolTestSmallSingleShardPolicy()})
	t.Cleanup(func() {
		if err := pool.Close(); err != nil {
			t.Fatalf("Pool.Close() error = %v", err)
		}
	})
	seedPoolRetainedBuffers(t, pool, 2, 512)

	result := pool.TrimShard(ClassID(0), 0, 2, SizeFromBytes(512))
	if !result.Executed || result.TrimmedBuffers != 1 || result.TrimmedBytes != 512 {
		t.Fatalf("TrimShard() = %+v, want byte-bound single removal", result)
	}
	if got := pool.Metrics().CurrentRetainedBytes; got != 512 {
		t.Fatalf("retained bytes after TrimShard = %d, want 512", got)
	}
}

// TestPoolTrimClearsBucketReferences verifies trim clears removed slots.
func TestPoolTrimClearsBucketReferences(t *testing.T) {
	pool := MustNew(PoolConfig{Policy: poolTestSmallSingleShardPolicy()})
	t.Cleanup(func() {
		if err := pool.Close(); err != nil {
			t.Fatalf("Pool.Close() error = %v", err)
		}
	})
	seedPoolRetainedBuffers(t, pool, 1, 512)

	result := pool.TrimClass(ClassID(0), 1, SizeFromBytes(512))
	if !result.Executed {
		t.Fatalf("TrimClass() = %+v, want execution", result)
	}
	assertPoolClassShardSlotsCleared(t, pool, ClassID(0), 0)
}

// TestPoolPressureNormalPreservesRetention verifies normal pressure is a no-op
// for return admission.
func TestPoolPressureNormalPreservesRetention(t *testing.T) {
	policy := poolTestSmallSingleShardPolicy()
	policy.Pressure = poolTestPressurePolicy()
	pool := MustNew(PoolConfig{Policy: policy})
	t.Cleanup(func() {
		if err := pool.Close(); err != nil {
			t.Fatalf("Pool.Close() error = %v", err)
		}
	})

	if err := pool.applyPressure(PressureSignal{Level: PressureLevelNormal, Source: PressureSourceManual, Generation: Generation(10)}); err != nil {
		t.Fatalf("applyPressure(normal) error = %v", err)
	}
	if err := pool.Put(make([]byte, 0, 512)); err != nil {
		t.Fatalf("Put() under normal pressure error = %v", err)
	}
	metrics := pool.Metrics()
	if metrics.Retains != 1 || metrics.CurrentRetainedBytes != 512 || metrics.Drops != 0 {
		t.Fatalf("normal pressure metrics = %+v, want retained buffer", metrics)
	}
}

// TestPoolPressureCriticalDiscardsReturns verifies critical pressure blocks new
// retained storage without scanning existing buckets.
func TestPoolPressureCriticalDiscardsReturns(t *testing.T) {
	policy := poolTestSmallSingleShardPolicy()
	policy.Pressure = poolTestPressurePolicy()
	pool := MustNew(PoolConfig{Policy: policy})
	t.Cleanup(func() {
		if err := pool.Close(); err != nil {
			t.Fatalf("Pool.Close() error = %v", err)
		}
	})

	if err := pool.applyPressure(PressureSignal{Level: PressureLevelCritical, Source: PressureSourceManual, Generation: Generation(10)}); err != nil {
		t.Fatalf("applyPressure(critical) error = %v", err)
	}
	if err := pool.Put(make([]byte, 0, 512)); err != nil {
		t.Fatalf("Put() under critical pressure error = %v", err)
	}
	metrics := pool.Metrics()
	if metrics.Retains != 0 || metrics.Drops != 1 || metrics.CurrentRetainedBytes != 0 {
		t.Fatalf("critical pressure metrics = %+v, want dropped return and no retention", metrics)
	}
	if metrics.DropReasons.PressureRetentionDisabled != 1 {
		t.Fatalf("pressure-retention-disabled drops = %d, want 1", metrics.DropReasons.PressureRetentionDisabled)
	}
}

// TestPoolPressureCapacityThresholdDropReason verifies pressure-driven capacity
// drops stay distinguishable from static oversized-return policy drops.
func TestPoolPressureCapacityThresholdDropReason(t *testing.T) {
	policy := poolTestSmallSingleShardPolicy()
	policy.Pressure = poolTestPressurePolicy()
	policy.Pressure.High.DropReturnedCapacityAbove = SizeFromBytes(256)
	policy.Admission.OversizedReturn = AdmissionActionDrop
	pool := MustNew(PoolConfig{Policy: policy})
	t.Cleanup(func() {
		if err := pool.Close(); err != nil {
			t.Fatalf("Pool.Close() error = %v", err)
		}
	})

	if err := pool.applyPressure(PressureSignal{Level: PressureLevelHigh, Source: PressureSourceManual, Generation: Generation(10)}); err != nil {
		t.Fatalf("applyPressure(high) error = %v", err)
	}
	if err := pool.Put(make([]byte, 0, 512)); err != nil {
		t.Fatalf("Put() under high pressure error = %v", err)
	}
	metrics := pool.Metrics()
	if metrics.DropReasons.PressureCapacityThreshold != 1 {
		t.Fatalf("pressure-capacity-threshold drops = %d, want 1", metrics.DropReasons.PressureCapacityThreshold)
	}
	if metrics.DropReasons.Oversized != 0 {
		t.Fatalf("oversized drops = %d, want 0 for pressure threshold", metrics.DropReasons.Oversized)
	}
}

func seedPoolRetainedBuffers(t *testing.T, pool *Pool, count int, capacity int) {
	t.Helper()
	for index := 0; index < count; index++ {
		if err := pool.Put(make([]byte, 0, capacity)); err != nil {
			t.Fatalf("Put seed %d error = %v", index, err)
		}
	}
}

func assertPoolClassShardSlotsCleared(t *testing.T, pool *Pool, classID ClassID, shardIndex int) {
	t.Helper()
	state := &pool.classes[classID.Index()]
	shard := &state.shards[shardIndex]
	shard.mu.Lock()
	defer shard.mu.Unlock()

	for segment := shard.bucket.head; segment != nil; segment = segment.previous {
		for slotIndex, buffer := range segment.slots {
			if buffer != nil {
				t.Fatalf("class %s shard %d segment slot %d retained stale buffer reference", classID, shardIndex, slotIndex)
			}
		}
	}
}

func poolTestPressurePolicy() PressurePolicy {
	return PressurePolicy{
		Enabled: true,
		Medium: PressureLevelPolicy{
			RetentionScale:          PolicyRatioOne,
			TrimScale:               PolicyRatioOne,
			PreserveSmallHotClasses: true,
		},
		High: PressureLevelPolicy{
			RetentionScale:          PolicyRatioOne / 2,
			TrimScale:               PolicyRatioOne,
			PreserveSmallHotClasses: true,
		},
		Critical: PressureLevelPolicy{
			RetentionScale:   0,
			TrimScale:        PolicyRatioOne,
			DisableRetention: true,
		},
	}
}
