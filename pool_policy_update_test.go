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
	"reflect"
	"sync"
	"testing"
	"time"
)

func TestPoolPublishPolicyPublishesRuntimeSnapshot(t *testing.T) {
	t.Parallel()

	pool := MustNew(PoolConfig{Policy: poolTestSmallSingleShardPolicy()})
	defer closePoolForTest(t, pool)

	candidate := poolPolicyUpdateExpandedPolicy()
	before := pool.Snapshot()
	result, err := pool.PublishPolicy(candidate)
	if err != nil {
		t.Fatalf("PublishPolicy() error = %v", err)
	}

	after := pool.Snapshot()
	if !result.Published || !result.RuntimePublished {
		t.Fatalf("PublishPolicy() result = %+v, want published runtime snapshot", result)
	}
	if !after.Generation.After(before.Generation) || after.Generation != result.Generation {
		t.Fatalf("snapshot generation = %s result = %s before = %s, want new published generation",
			after.Generation, result.Generation, before.Generation)
	}
	if after.Policy.Retention.SoftRetainedBytes != candidate.Retention.SoftRetainedBytes {
		t.Fatalf("published soft retention = %s, want %s",
			after.Policy.Retention.SoftRetainedBytes, candidate.Retention.SoftRetainedBytes)
	}
	if !result.ClassBudgetPublication.Published {
		t.Fatalf("class budget report = %+v, want published", result.ClassBudgetPublication)
	}
}

func TestPoolPublishPolicyWrapperReturnsError(t *testing.T) {
	t.Parallel()

	pool := MustNew(PoolConfig{Policy: poolTestSmallSingleShardPolicy()})
	defer closePoolForTest(t, pool)

	candidate := pool.Snapshot().Policy
	candidate.Classes.Sizes = append(candidate.Classes.SizesCopy(), ClassSizeFromBytes(2048))
	if err := pool.UpdatePolicy(candidate); !errors.Is(err, ErrInvalidPolicy) {
		t.Fatalf("UpdatePolicy(shape change) error = %v, want %v", err, ErrInvalidPolicy)
	}
}

func TestPoolPublishPolicyRejectsClosedPool(t *testing.T) {
	t.Parallel()

	pool := MustNew(PoolConfig{Policy: poolTestSmallSingleShardPolicy()})
	before := pool.Snapshot()
	if err := pool.Close(); err != nil {
		t.Fatalf("Pool.Close() error = %v", err)
	}

	result, err := pool.PublishPolicy(poolPolicyUpdateExpandedPolicy())
	if !errors.Is(err, ErrClosed) {
		t.Fatalf("PublishPolicy(closed) error = %v, want %v", err, ErrClosed)
	}
	if result.Published || result.RuntimePublished || result.FailureReason != policyUpdateFailureClosed {
		t.Fatalf("PublishPolicy(closed) result = %+v, want closed rejection", result)
	}
	after := pool.Snapshot()
	if after.Generation != before.Generation {
		t.Fatalf("closed PublishPolicy changed generation: got %s want %s", after.Generation, before.Generation)
	}
}

func TestPoolPublishPolicyRejectsClassShapeChange(t *testing.T) {
	t.Parallel()

	pool := MustNew(PoolConfig{Policy: poolTestSmallSingleShardPolicy()})
	defer closePoolForTest(t, pool)

	candidate := pool.Snapshot().Policy
	candidate.Retention.SoftRetainedBytes = 4 * KiB
	candidate.Retention.HardRetainedBytes = 8 * KiB
	candidate.Retention.MaxRequestSize = 2 * KiB
	candidate.Retention.MaxRetainedBufferCapacity = 2 * KiB
	candidate.Retention.MaxClassRetainedBytes = 4 * KiB
	candidate.Retention.MaxShardRetainedBytes = 4 * KiB
	candidate.Classes.Sizes = []ClassSize{ClassSizeFromBytes(512), ClassSizeFromBytes(2048)}
	result, err := pool.PublishPolicy(candidate)
	if !errors.Is(err, ErrInvalidPolicy) {
		t.Fatalf("PublishPolicy(class shape) error = %v, want %v", err, ErrInvalidPolicy)
	}
	if result.FailureReason != policyUpdateFailureShapeChange || result.Published || result.RuntimePublished {
		t.Fatalf("PublishPolicy(class shape) result = %+v, want rejected shape change", result)
	}
}

func TestPoolPublishPolicyRejectsShardShapeChange(t *testing.T) {
	t.Parallel()

	pool := MustNew(PoolConfig{Policy: poolTestSmallSingleShardPolicy()})
	defer closePoolForTest(t, pool)

	candidate := pool.Snapshot().Policy
	candidate.Shards.ShardsPerClass = 2
	result, err := pool.PublishPolicy(candidate)
	if !errors.Is(err, ErrInvalidPolicy) {
		t.Fatalf("PublishPolicy(shard shape) error = %v, want %v", err, ErrInvalidPolicy)
	}
	if result.FailureReason != policyUpdateFailureShapeChange || result.Published || result.RuntimePublished {
		t.Fatalf("PublishPolicy(shard shape) result = %+v, want rejected shape change", result)
	}
}

func TestPoolPublishPolicyRejectsOwnershipChange(t *testing.T) {
	t.Parallel()

	pool := MustNew(PoolConfig{Policy: poolTestSmallSingleShardPolicy()})
	defer closePoolForTest(t, pool)

	candidate := pool.Snapshot().Policy
	candidate.Ownership.MaxReturnedCapacityGrowth = 3 * PolicyRatioOne
	result, err := pool.PublishPolicy(candidate)
	if !errors.Is(err, ErrInvalidPolicy) {
		t.Fatalf("PublishPolicy(ownership change) error = %v, want %v", err, ErrInvalidPolicy)
	}
	if result.FailureReason != policyUpdateFailureOwnershipChange || result.Published || result.RuntimePublished {
		t.Fatalf("PublishPolicy(ownership change) result = %+v, want ownership rejection", result)
	}
}

func TestPoolPublishPolicyAllowsRetentionAdmissionPressureTrimChange(t *testing.T) {
	t.Parallel()

	pool := MustNew(PoolConfig{Policy: poolTestSmallSingleShardPolicy()})
	defer closePoolForTest(t, pool)

	candidate := poolPolicyUpdateExpandedPolicy()
	candidate.Admission.ZeroDroppedBuffers = true
	candidate.Pressure = poolTestPressurePolicy()
	candidate.Trim = poolPolicyUpdateTrimPolicy(2, KiB, false)
	result, err := pool.PublishPolicy(candidate)
	if err != nil {
		t.Fatalf("PublishPolicy(live sections) error = %v", err)
	}
	if !result.Published ||
		!result.Diff.RetentionChanged ||
		!result.Diff.AdmissionChanged ||
		!result.Diff.PressureChanged ||
		!result.Diff.TrimChanged {
		t.Fatalf("PublishPolicy(live sections) result = %+v, want live diff sections published", result)
	}
}

func TestPoolPublishPolicyPreservesPressureSignal(t *testing.T) {
	t.Parallel()

	policy := poolTestSmallSingleShardPolicy()
	policy.Pressure = poolTestPressurePolicy()
	pool := MustNew(PoolConfig{Policy: policy})
	defer closePoolForTest(t, pool)

	signal := PressureSignal{Level: PressureLevelHigh, Source: PressureSourceManual, Generation: Generation(44)}
	if err := pool.applyPressure(signal); err != nil {
		t.Fatalf("applyPressure() error = %v", err)
	}

	candidate := policy
	candidate.Pressure.High.DropReturnedCapacityAbove = SizeFromBytes(512)
	result, err := pool.PublishPolicy(candidate)
	if err != nil {
		t.Fatalf("PublishPolicy() error = %v", err)
	}
	if !result.PressurePreserved {
		t.Fatalf("PublishPolicy() result = %+v, want pressure preserved", result)
	}
	if got := pool.currentRuntimeSnapshot().Pressure; got != signal {
		t.Fatalf("pressure after PublishPolicy = %+v, want %+v", got, signal)
	}
}

func TestPoolPublishPolicyContractsClassBudgets(t *testing.T) {
	t.Parallel()

	pool := MustNew(PoolConfig{Policy: poolTestSmallSingleShardPolicy()})
	defer closePoolForTest(t, pool)

	result, err := pool.PublishPolicy(poolPolicyUpdateContractedPolicy(false))
	if err != nil {
		t.Fatalf("PublishPolicy(contract) error = %v", err)
	}
	if !result.Contracted || !result.ClassBudgetPublication.Published {
		t.Fatalf("PublishPolicy(contract) result = %+v, want contracted class budget publication", result)
	}
	if assigned := sumPoolClassBudgetTargets(pool.Snapshot().Classes); assigned > KiB.Bytes() {
		t.Fatalf("assigned class budget bytes = %d, want <= %d", assigned, KiB.Bytes())
	}
}

func TestPoolPublishPolicyContractsShardCredits(t *testing.T) {
	t.Parallel()

	pool := MustNew(PoolConfig{Policy: poolTestSmallSingleShardPolicy()})
	defer closePoolForTest(t, pool)

	before := pool.Snapshot()
	result, err := pool.PublishPolicy(poolPolicyUpdateContractedPolicy(false))
	if err != nil {
		t.Fatalf("PublishPolicy(contract) error = %v", err)
	}
	after := pool.Snapshot()
	if !result.Contracted {
		t.Fatalf("PublishPolicy(contract) result = %+v, want contraction", result)
	}
	for index := range after.Classes {
		if after.Classes[index].Shards[0].Credit.Generation.Before(result.Generation) {
			t.Fatalf("class %d shard credit generation = %s, want >= %s",
				index, after.Classes[index].Shards[0].Credit.Generation, result.Generation)
		}
		if after.Classes[index].Shards[0].Credit.TargetBytes > before.Classes[index].Shards[0].Credit.TargetBytes {
			t.Fatalf("class %d shard credit grew from %d to %d during contraction",
				index,
				before.Classes[index].Shards[0].Credit.TargetBytes,
				after.Classes[index].Shards[0].Credit.TargetBytes,
			)
		}
	}
}

func TestPoolPublishPolicyDoesNotForceCheckedOutBuffer(t *testing.T) {
	t.Parallel()

	pool := MustNew(PoolConfig{Policy: poolTestSmallSingleShardPolicy()})
	defer closePoolForTest(t, pool)

	buffer, err := pool.Get(512)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	buffer = append(buffer[:0], 7)

	if _, err := pool.PublishPolicy(poolPolicyUpdateContractedPolicy(true)); err != nil {
		t.Fatalf("PublishPolicy(contract) error = %v", err)
	}
	if len(buffer) != 1 || buffer[0] != 7 || cap(buffer) < 512 {
		t.Fatalf("checked-out buffer changed after policy contraction: len=%d cap=%d first=%d",
			len(buffer), cap(buffer), buffer[0])
	}
}

func TestPoolPublishPolicyTrimOnShrinkIsBounded(t *testing.T) {
	t.Parallel()

	pool := MustNew(PoolConfig{Policy: poolTestSmallSingleShardPolicy()})
	defer closePoolForTest(t, pool)
	seedPoolRetainedBuffers(t, pool, 4, 512)

	result, err := pool.PublishPolicy(poolPolicyUpdateContractedPolicy(true))
	if err != nil {
		t.Fatalf("PublishPolicy(contract+trim) error = %v", err)
	}
	if !result.TrimAttempted || !result.TrimResult.Attempted {
		t.Fatalf("PublishPolicy(contract+trim) result = %+v, want trim attempt", result)
	}
	if result.TrimResult.TrimmedBuffers > 1 || result.TrimResult.TrimmedBytes > 512 {
		t.Fatalf("trim result = %+v, want bounded to one 512-byte buffer", result.TrimResult)
	}
}

func TestPoolPublishPolicyWithoutTrimLeavesOverTargetRetained(t *testing.T) {
	t.Parallel()

	pool := MustNew(PoolConfig{Policy: poolTestSmallSingleShardPolicy()})
	defer closePoolForTest(t, pool)
	seedPoolRetainedBuffers(t, pool, 4, 512)
	before := pool.Metrics()

	result, err := pool.PublishPolicy(poolPolicyUpdateContractedPolicy(false))
	if err != nil {
		t.Fatalf("PublishPolicy(contract without trim) error = %v", err)
	}
	after := pool.Metrics()
	if result.TrimAttempted {
		t.Fatalf("PublishPolicy(contract without trim) result = %+v, want no trim", result)
	}
	if after.CurrentRetainedBytes != before.CurrentRetainedBytes || after.CurrentRetainedBuffers != before.CurrentRetainedBuffers {
		t.Fatalf("retained changed without trim: before=%+v after=%+v", before, after)
	}
}

func TestPoolPublishPolicyReportsInfeasibleClassBudget(t *testing.T) {
	t.Parallel()

	pool := MustNew(PoolConfig{Policy: poolTestSmallSingleShardPolicy()})
	defer closePoolForTest(t, pool)

	// PublishPolicy normally derives default class targets with zero child
	// minimums, so ordinary policy values cannot fabricate infeasible child
	// minimums. The publication path still uses this class report shape for hard
	// budget diagnostics; this test pins the report conversion before any Pool
	// mutation can occur.
	report, err := pool.planPoolBudgetForPolicy(PoolBudgetTarget{
		Generation:    Generation(91),
		PoolName:      "primary",
		RetainedBytes: KiB,
		ClassTargets: []ClassBudgetTarget{
			{Generation: Generation(91), ClassID: ClassID(0), TargetBytes: KiB},
			{Generation: Generation(91), ClassID: ClassID(1), TargetBytes: KiB},
		},
	}, pool.Snapshot().Policy)
	if err != nil {
		t.Fatalf("planPoolBudgetForPolicy() error = %v", err)
	}
	publication := poolClassBudgetPublicationReportFromAllocation("primary", Generation(91), report)
	if publication.Allocation.Feasible || publication.Allocation.OvercommittedBytes == 0 {
		t.Fatalf("publication = %+v, want infeasible overcommitted class budget", publication)
	}
	if publication.Published {
		t.Fatalf("publication = %+v, want unpublished infeasible report", publication)
	}
}

func TestPoolPublishPolicyDoesNotMutateRuntimeOnRejectedShapeChange(t *testing.T) {
	t.Parallel()

	pool := MustNew(PoolConfig{Policy: poolTestSmallSingleShardPolicy()})
	defer closePoolForTest(t, pool)

	before := pool.currentRuntimeSnapshot()
	candidate := before.clonePolicy()
	candidate.Shards.BucketSegmentSlotsPerShard = candidate.Shards.BucketSegmentSlotsPerShard + 1
	result, err := pool.PublishPolicy(candidate)
	if !errors.Is(err, ErrInvalidPolicy) {
		t.Fatalf("PublishPolicy(shape) error = %v, want %v", err, ErrInvalidPolicy)
	}
	after := pool.currentRuntimeSnapshot()
	if result.Published || result.RuntimePublished || result.FailureReason != policyUpdateFailureShapeChange {
		t.Fatalf("PublishPolicy(shape) result = %+v, want rejected shape change", result)
	}
	if after.Generation != before.Generation || !reflect.DeepEqual(after.Policy, before.Policy) {
		t.Fatalf("runtime mutated after rejected shape change: before=%+v after=%+v", before, after)
	}
}

func TestPoolPublishPolicyConcurrentWithClose(t *testing.T) {
	pool := MustNew(PoolConfig{Policy: poolTestSmallSingleShardPolicy()})

	start := make(chan struct{})
	var wg sync.WaitGroup
	for index := 0; index < 8; index++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			candidate := poolPolicyUpdateExpandedPolicy()
			candidate.Admission.ZeroDroppedBuffers = index%2 == 0
			_, err := pool.PublishPolicy(candidate)
			if err != nil && !errors.Is(err, ErrClosed) {
				t.Errorf("PublishPolicy concurrent error = %v, want nil or %v", err, ErrClosed)
			}
		}(index)
	}

	close(start)
	if err := pool.Close(); err != nil {
		t.Fatalf("Pool.Close() error = %v", err)
	}
	wg.Wait()
	if pool.Lifecycle() != LifecycleClosed {
		t.Fatalf("Pool lifecycle = %s, want closed", pool.Lifecycle())
	}
}

func poolPolicyUpdateExpandedPolicy() Policy {
	policy := poolTestSmallSingleShardPolicy()
	policy.Retention.SoftRetainedBytes = 3 * KiB
	policy.Retention.HardRetainedBytes = 4 * KiB
	policy.Retention.MaxClassRetainedBytes = 3 * KiB
	policy.Retention.MaxShardRetainedBytes = 3 * KiB
	return policy
}

func poolPolicyUpdateContractedPolicy(trimOnShrink bool) Policy {
	policy := poolTestSmallSingleShardPolicy()
	policy.Retention.SoftRetainedBytes = KiB
	policy.Retention.HardRetainedBytes = 2 * KiB
	policy.Retention.MaxRetainedBuffers = 2
	policy.Retention.MaxClassRetainedBytes = KiB
	policy.Retention.MaxClassRetainedBuffers = 2
	policy.Retention.MaxShardRetainedBytes = KiB
	policy.Retention.MaxShardRetainedBuffers = 2
	policy.Trim = poolPolicyUpdateTrimPolicy(1, SizeFromBytes(512), trimOnShrink)
	return policy
}

func poolPolicyUpdateTrimPolicy(maxBuffers uint64, maxBytes Size, trimOnShrink bool) TrimPolicy {
	return TrimPolicy{
		Enabled:                   true,
		Interval:                  time.Second,
		FullScanInterval:          2 * time.Second,
		MaxBuffersPerCycle:        maxBuffers,
		MaxBytesPerCycle:          maxBytes,
		MaxPoolsPerCycle:          1,
		MaxClassesPerPoolPerCycle: 2,
		MaxShardsPerClassPerCycle: 1,
		TrimOnPolicyShrink:        trimOnShrink,
		TrimOnPressure:            true,
		TrimOnClose:               true,
	}
}
