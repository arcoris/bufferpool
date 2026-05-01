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
	"reflect"
	"testing"
)

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

func TestPoolTrimPrefersOverTargetClass(t *testing.T) {
	t.Parallel()

	pool := MustNew(PoolConfig{Policy: poolTestSmallSingleShardPolicy()})
	t.Cleanup(func() {
		if err := pool.Close(); err != nil {
			t.Fatalf("Pool.Close() error = %v", err)
		}
	})
	seedPoolRetainedBuffers(t, pool, 1, 512)
	seedPoolRetainedBuffers(t, pool, 1, 1024)
	_, err := pool.applyClassBudgets([]ClassBudgetTarget{
		{Generation: Generation(20), ClassID: ClassID(0), TargetBytes: 4 * KiB},
		{Generation: Generation(20), ClassID: ClassID(1), TargetBytes: 0},
	})
	if err != nil {
		t.Fatalf("applyClassBudgets() error = %v", err)
	}

	result := pool.Trim(PoolTrimPlan{MaxBuffers: 1, MaxBytes: KiB, MaxClasses: 1, MaxShardsPerClass: 1})
	if !result.Executed || len(result.CandidateClasses) == 0 || result.CandidateClasses[0].ClassID != ClassID(1) {
		t.Fatalf("Trim() = %+v, want class 1 selected first", result)
	}
	snapshot := pool.Snapshot()
	if snapshot.Classes[1].CurrentRetainedBuffers != 0 || snapshot.Classes[0].CurrentRetainedBuffers != 1 {
		t.Fatalf("retained after target-aware trim = class0 %d class1 %d, want class1 trimmed",
			snapshot.Classes[0].CurrentRetainedBuffers,
			snapshot.Classes[1].CurrentRetainedBuffers,
		)
	}
}

func TestPoolTrimCandidateOrderingDeterministic(t *testing.T) {
	t.Parallel()

	pool := MustNew(PoolConfig{Policy: poolTestSmallSingleShardPolicy()})
	t.Cleanup(func() {
		if err := pool.Close(); err != nil {
			t.Fatalf("Pool.Close() error = %v", err)
		}
	})
	seedPoolRetainedBuffers(t, pool, 1, 512)
	seedPoolRetainedBuffers(t, pool, 1, 1024)
	if _, err := pool.applyClassBudgets([]ClassBudgetTarget{
		{Generation: Generation(21), ClassID: ClassID(0), TargetBytes: 4 * KiB},
		{Generation: Generation(21), ClassID: ClassID(1), TargetBytes: 0},
	}); err != nil {
		t.Fatalf("applyClassBudgets() error = %v", err)
	}

	first := poolTrimCandidateReports(pool.poolTrimCandidates(PressureLevelNormal))
	second := poolTrimCandidateReports(pool.poolTrimCandidates(PressureLevelNormal))
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("candidate order is not deterministic: first=%+v second=%+v", first, second)
	}
	if len(first) == 0 || first[0].ClassID != ClassID(1) {
		t.Fatalf("candidate order = %+v, want over-target class first", first)
	}
}

func TestTrimVictimScorePrefersOverTargetClass(t *testing.T) {
	t.Parallel()

	overTarget := NewTrimVictimScore(TrimVictimScoreInput{
		OverTargetBytes: 512,
		RetainedBytes:   1024,
		RetainedBuffers: 1,
		ClassBytes:      512,
	})
	underTarget := NewTrimVictimScore(TrimVictimScoreInput{
		RetainedBytes:   1024,
		RetainedBuffers: 1,
		ClassBytes:      512,
	})
	if overTarget.Value <= underTarget.Value {
		t.Fatalf("over-target score = %.6f, under-target score = %.6f, want over-target higher", overTarget.Value, underTarget.Value)
	}
}

func TestTrimVictimScorePrefersColdWastefulClass(t *testing.T) {
	t.Parallel()

	coldWasteful := NewTrimVictimScore(TrimVictimScoreInput{
		RetainedBytes:      2048,
		RetainedBuffers:    1,
		ClassBytes:         512,
		CapacityWasteBytes: 1536,
		Coldness:           1,
		ColdnessKnown:      true,
		PressureLevel:      PressureLevelNormal,
	})
	hotTight := NewTrimVictimScore(TrimVictimScoreInput{
		RetainedBytes:   2048,
		RetainedBuffers: 4,
		ClassBytes:      512,
		Coldness:        0,
		ColdnessKnown:   true,
		PressureLevel:   PressureLevelNormal,
	})
	if coldWasteful.Value <= hotTight.Value {
		t.Fatalf("cold wasteful score = %.6f, hot tight score = %.6f, want cold wasteful higher", coldWasteful.Value, hotTight.Value)
	}
}

func TestTrimVictimScorePenalizesRecentActivity(t *testing.T) {
	t.Parallel()

	cold := NewTrimVictimScore(TrimVictimScoreInput{
		RetainedBytes:   1024,
		RetainedBuffers: 1,
		ClassBytes:      512,
		RecentActivity:  0,
		ActivityKnown:   true,
	})
	hot := NewTrimVictimScore(TrimVictimScoreInput{
		RetainedBytes:   1024,
		RetainedBuffers: 1,
		ClassBytes:      512,
		RecentActivity:  trimVictimHotActivityScale,
		ActivityKnown:   true,
	})
	if cold.Value <= hot.Value {
		t.Fatalf("cold score = %.6f, hot score = %.6f, want recent activity to lower score", cold.Value, hot.Value)
	}
}

func TestTrimVictimScoreIncludesCapacityWaste(t *testing.T) {
	t.Parallel()

	score := NewTrimVictimScore(TrimVictimScoreInput{
		RetainedBytes:      2048,
		RetainedBuffers:    1,
		ClassBytes:         512,
		CapacityWasteBytes: 1536,
	})
	if got := trimVictimComponentValue(score, trimVictimScoreComponentCapacityWaste); got <= 0 {
		t.Fatalf("capacity waste component = %.6f, want positive", got)
	}
	if got := trimVictimCapacityWasteBytes(2048, 1, 512); got != 1536 {
		t.Fatalf("trimVictimCapacityWasteBytes() = %d, want 1536", got)
	}
}

func TestTrimVictimScoreUsesStableTieBreakers(t *testing.T) {
	t.Parallel()

	config := testPartitionConfig("beta", "alpha")
	config.Policy.Trim = PartitionTrimPolicy{
		Enabled:                   true,
		MaxPoolsPerCycle:          2,
		MaxBuffersPerCycle:        2,
		MaxBytesPerCycle:          2 * KiB,
		MaxClassesPerPoolPerCycle: 1,
		MaxShardsPerClassPerCycle: 1,
	}
	partition, err := NewPoolPartition(config)
	requirePartitionNoError(t, err)
	t.Cleanup(func() { requirePartitionNoError(t, partition.Close()) })

	alpha, _ := partition.pool("alpha")
	beta, _ := partition.pool("beta")
	seedPoolRetainedBuffers(t, alpha, 1, 512)
	seedPoolRetainedBuffers(t, beta, 1, 512)

	candidates := partitionTrimCandidateReports(partition.partitionTrimCandidates(PressureLevelNormal, partitionTrimScoringContext{}))
	if len(candidates) < 2 || candidates[0].PoolName != "alpha" || candidates[1].PoolName != "beta" {
		t.Fatalf("partition candidates = %+v, want stable name tie-breaker alpha then beta", candidates)
	}
}

func TestPoolTrimCandidateOrderingUsesTrimVictimScore(t *testing.T) {
	t.Parallel()

	pool := MustNew(PoolConfig{Policy: poolTestSmallSingleShardPolicy()})
	t.Cleanup(func() {
		if err := pool.Close(); err != nil {
			t.Fatalf("Pool.Close() error = %v", err)
		}
	})
	seedPoolClassRetainedBuffer(t, pool, ClassID(0), 1024)
	seedPoolRetainedBuffers(t, pool, 1, 1024)
	if _, err := pool.applyClassBudgets([]ClassBudgetTarget{
		{Generation: Generation(32), ClassID: ClassID(0), TargetBytes: 4 * KiB},
		{Generation: Generation(32), ClassID: ClassID(1), TargetBytes: 4 * KiB},
	}); err != nil {
		t.Fatalf("applyClassBudgets() error = %v", err)
	}

	candidates := poolTrimCandidateReports(pool.poolTrimCandidates(PressureLevelNormal))
	if len(candidates) < 2 {
		t.Fatalf("candidates = %+v, want two retained classes", candidates)
	}
	if candidates[0].ClassID != ClassID(0) || candidates[0].CapacityWasteBytes == 0 {
		t.Fatalf("candidates = %+v, want wasteful class 0 selected first", candidates)
	}
	if candidates[0].Score.Value <= candidates[1].Score.Value {
		t.Fatalf("candidate scores = %.6f %.6f, want wasteful class higher", candidates[0].Score.Value, candidates[1].Score.Value)
	}
}

func TestTrimScoringDoesNotBypassMaxBuffers(t *testing.T) {
	t.Parallel()

	pool := MustNew(PoolConfig{Policy: poolTestSmallSingleShardPolicy()})
	t.Cleanup(func() {
		if err := pool.Close(); err != nil {
			t.Fatalf("Pool.Close() error = %v", err)
		}
	})
	seedPoolRetainedBuffers(t, pool, 3, 512)

	result := pool.Trim(PoolTrimPlan{MaxBuffers: 1, MaxBytes: 4 * KiB})
	if result.TrimmedBuffers > 1 {
		t.Fatalf("Trim() = %+v, want max one buffer despite scoring", result)
	}
}

func TestTrimScoringDoesNotBypassMaxBytes(t *testing.T) {
	t.Parallel()

	pool := MustNew(PoolConfig{Policy: poolTestSmallSingleShardPolicy()})
	t.Cleanup(func() {
		if err := pool.Close(); err != nil {
			t.Fatalf("Pool.Close() error = %v", err)
		}
	})
	seedPoolRetainedBuffers(t, pool, 3, 512)

	result := pool.Trim(PoolTrimPlan{MaxBuffers: 3, MaxBytes: SizeFromBytes(512)})
	if result.TrimmedBytes > 512 {
		t.Fatalf("Trim() = %+v, want max 512 bytes despite scoring", result)
	}
}

func TestPoolTrimRejectsClosedPool(t *testing.T) {
	t.Parallel()

	pool := MustNew(PoolConfig{Policy: poolTestSmallSingleShardPolicy()})
	seedPoolRetainedBuffers(t, pool, 1, 512)
	if err := pool.Close(); err != nil {
		t.Fatalf("Pool.Close() error = %v", err)
	}

	result := pool.Trim(PoolTrimPlan{MaxBuffers: 1, MaxBytes: KiB})
	if result.Attempted || result.Executed || result.Reason != errPoolTrimClosed {
		t.Fatalf("Trim(closed) = %+v, want rejected closed pool", result)
	}
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

func seedPoolClassRetainedBuffer(t *testing.T, pool *Pool, classID ClassID, capacity int) {
	t.Helper()
	seedPoolClassShardRetainedBuffer(t, pool, classID, 0, capacity)
}

func seedPoolClassShardRetainedBuffer(t *testing.T, pool *Pool, classID ClassID, shardIndex int, capacity int) {
	t.Helper()
	class, ok := pool.table.classByID(classID)
	if !ok {
		t.Fatalf("class %s is not configured", classID)
	}
	state := pool.mustClassStateFor(class)
	result := state.tryRetain(shardIndex, make([]byte, 0, capacity))
	if !result.Retained() {
		t.Fatalf("tryRetain(class=%s shard=%d capacity=%d) = %#v, want retained", classID, shardIndex, capacity, result)
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
