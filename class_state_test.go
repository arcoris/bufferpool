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
	"sync"
	"testing"

	"arcoris.dev/bufferpool/internal/testutil"
)

// TestNewClassState verifies construction of runtime state for one normalized
// size class.
//
// classState owns class-local counters, class-local budget, and the shard set for
// an already selected SizeClass. A newly constructed class starts with disabled
// effective budget and disabled shard credit.
func TestNewClassState(t *testing.T) {
	t.Parallel()

	class := NewSizeClass(ClassID(3), ClassSizeFromSize(4*KiB))

	state := newClassState(class, 2, 4)
	snapshot := state.state()

	if state.descriptor() != class {
		t.Fatalf("descriptor() = %s, want %s", state.descriptor(), class)
	}

	if state.classID() != class.ID() {
		t.Fatalf("classID() = %s, want %s", state.classID(), class.ID())
	}

	if state.classSize() != class.Size() {
		t.Fatalf("classSize() = %s, want %s", state.classSize(), class.Size())
	}

	if state.shardCount() != 2 {
		t.Fatalf("shardCount() = %d, want 2", state.shardCount())
	}

	if snapshot.Class != class {
		t.Fatalf("snapshot.Class = %s, want %s", snapshot.Class, class)
	}

	if snapshot.Budget.ClassSize != class.Size() {
		t.Fatalf("snapshot.Budget.ClassSize = %s, want %s", snapshot.Budget.ClassSize, class.Size())
	}

	if !snapshot.Budget.IsZero() {
		t.Fatalf("snapshot.Budget = %+v, want zero budget", snapshot.Budget)
	}

	if !snapshot.Counters.IsZero() {
		t.Fatalf("snapshot.Counters = %+v, want zero counters", snapshot.Counters)
	}

	if snapshot.ShardCount() != 2 {
		t.Fatalf("snapshot.ShardCount() = %d, want 2", snapshot.ShardCount())
	}

	for index, shard := range snapshot.Shards {
		if shard.Bucket.SlotLimit != 4 {
			t.Fatalf("shard %d Bucket.SlotLimit = %d, want 4", index, shard.Bucket.SlotLimit)
		}

		if !shard.Credit.IsZero() {
			t.Fatalf("shard %d Credit = %+v, want disabled zero credit", index, shard.Credit)
		}

		if !shard.Counters.IsZero() {
			t.Fatalf("shard %d Counters = %+v, want zero counters", index, shard.Counters)
		}
	}
}

// TestNewClassStatePanicsForInvalidClass verifies that class runtime state cannot
// be built for an invalid size-class descriptor.
func TestNewClassStatePanicsForInvalidClass(t *testing.T) {
	t.Parallel()

	testutil.MustPanicWithMessage(t, errClassStateInvalidClass, func() {
		_ = newClassState(SizeClass{}, 1, 1)
	})
}

// TestNewClassStatePanicsForInvalidShardCount verifies shard-count validation.
func TestNewClassStatePanicsForInvalidShardCount(t *testing.T) {
	t.Parallel()

	class := NewSizeClass(ClassID(0), ClassSizeFromSize(KiB))

	tests := []struct {
		name       string
		shardCount int
	}{
		{
			name:       "zero",
			shardCount: 0,
		},
		{
			name:       "negative",
			shardCount: -1,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			testutil.MustPanicWithMessage(t, errClassStateInvalidShardCount, func() {
				_ = newClassState(class, tt.shardCount, 1)
			})
		})
	}
}

// TestNewClassStatePanicsForInvalidBucketSlotLimit verifies that class
// construction preserves shard/bucket slot-limit invariants.
func TestNewClassStatePanicsForInvalidBucketSlotLimit(t *testing.T) {
	t.Parallel()

	class := NewSizeClass(ClassID(0), ClassSizeFromSize(KiB))

	testutil.MustPanicWithMessage(t, errBucketSegmentInvalidSlotLimit, func() {
		_ = newClassState(class, 1, 0)
	})
}

// TestClassStateShardAt verifies shard lookup by index.
func TestClassStateShardAt(t *testing.T) {
	t.Parallel()

	class := NewSizeClass(ClassID(0), ClassSizeFromSize(KiB))
	state := newClassState(class, 2, 1)

	if state.shardAt(0) == nil {
		t.Fatal("shardAt(0) = nil, want shard pointer")
	}

	if state.shardAt(1) == nil {
		t.Fatal("shardAt(1) = nil, want shard pointer")
	}
}

// TestClassStateShardAtPanicsForInvalidIndex verifies shard index validation.
func TestClassStateShardAtPanicsForInvalidIndex(t *testing.T) {
	t.Parallel()

	class := NewSizeClass(ClassID(0), ClassSizeFromSize(KiB))
	state := newClassState(class, 2, 1)

	tests := []struct {
		name       string
		shardIndex int
	}{
		{
			name:       "negative",
			shardIndex: -1,
		},
		{
			name:       "equal to shard count",
			shardIndex: 2,
		},
		{
			name:       "above shard count",
			shardIndex: 3,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			testutil.MustPanicWithMessage(t, errClassStateShardIndexOutOfRange, func() {
				_ = state.shardAt(tt.shardIndex)
			})
		})
	}
}

// TestClassStateUpdateBudget verifies class budget publication and per-shard
// credit distribution.
//
// The class budget owns conversion from assigned bytes to whole class buffers.
// updateBudget should also publish the derived credit plan to all shards.
func TestClassStateUpdateBudget(t *testing.T) {
	t.Parallel()

	class := NewSizeClass(ClassID(1), ClassSizeFromSize(4*KiB))
	state := newClassState(class, 2, 4)

	before := state.budgetSnapshot()
	generation := state.updateBudget(16 * KiB)
	after := state.state()

	if after.Budget.Generation != generation {
		t.Fatalf("Budget.Generation = %v, want %v", after.Budget.Generation, generation)
	}

	if after.Budget.Generation == before.Generation {
		t.Fatalf("budget generation did not advance: before=%v after=%v", before.Generation, after.Budget.Generation)
	}

	if after.Budget.AssignedBytes != uint64(16*KiB) {
		t.Fatalf("Budget.AssignedBytes = %d, want %d", after.Budget.AssignedBytes, uint64(16*KiB))
	}

	if after.Budget.TargetBuffers != 4 {
		t.Fatalf("Budget.TargetBuffers = %d, want 4", after.Budget.TargetBuffers)
	}

	if after.Budget.TargetBytes != uint64(16*KiB) {
		t.Fatalf("Budget.TargetBytes = %d, want %d", after.Budget.TargetBytes, uint64(16*KiB))
	}

	for index, shard := range after.Shards {
		if shard.Credit.TargetBuffers != 2 {
			t.Fatalf("shard %d Credit.TargetBuffers = %d, want 2", index, shard.Credit.TargetBuffers)
		}

		if shard.Credit.TargetBytes != uint64(8*KiB) {
			t.Fatalf("shard %d Credit.TargetBytes = %d, want %d", index, shard.Credit.TargetBytes, uint64(8*KiB))
		}

		if !shard.Credit.IsEnabled() {
			t.Fatalf("shard %d Credit.IsEnabled() = false, want true", index)
		}
	}
}

// TestClassStateUpdateBudgetLimit verifies applying a precomputed class budget
// limit.
func TestClassStateUpdateBudgetLimit(t *testing.T) {
	t.Parallel()

	class := NewSizeClass(ClassID(1), ClassSizeFromSize(4*KiB))
	state := newClassState(class, 2, 4)

	limit := newClassBudgetLimit(class.Size(), 24*KiB)

	generation := state.updateBudgetLimit(limit)
	snapshot := state.state()

	if snapshot.Budget.Generation != generation {
		t.Fatalf("Budget.Generation = %v, want %v", snapshot.Budget.Generation, generation)
	}

	if snapshot.Budget.Limit() != limit {
		t.Fatalf("Budget.Limit() = %+v, want %+v", snapshot.Budget.Limit(), limit)
	}

	wantBuffers := []uint64{3, 3}
	for index, shard := range snapshot.Shards {
		if shard.Credit.TargetBuffers != wantBuffers[index] {
			t.Fatalf("shard %d Credit.TargetBuffers = %d, want %d", index, shard.Credit.TargetBuffers, wantBuffers[index])
		}

		if shard.Credit.TargetBytes != wantBuffers[index]*class.Size().Bytes() {
			t.Fatalf("shard %d Credit.TargetBytes = %d, want %d", index, shard.Credit.TargetBytes, wantBuffers[index]*class.Size().Bytes())
		}
	}
}

// TestClassStateUpdateBudgetLimitPanicsForClassSizeMismatch verifies that a class
// state cannot apply a budget limit computed for another class size.
func TestClassStateUpdateBudgetLimitPanicsForClassSizeMismatch(t *testing.T) {
	t.Parallel()

	class := NewSizeClass(ClassID(1), ClassSizeFromSize(4*KiB))
	state := newClassState(class, 2, 4)

	limit := newClassBudgetLimit(ClassSizeFromSize(8*KiB), 32*KiB)

	testutil.MustPanicWithMessage(t, errClassBudgetClassSizeMismatch, func() {
		_ = state.updateBudgetLimit(limit)
	})
}

// TestClassStateDisableBudget verifies budget disabling and shard credit
// disabling.
//
// Disabling budget should not physically clear already retained buffers.
func TestClassStateDisableBudget(t *testing.T) {
	t.Parallel()

	class := NewSizeClass(ClassID(1), ClassSizeFromSize(KiB))
	state := newClassState(class, 2, 4)

	state.updateBudget(4 * KiB)

	retain := state.tryRetain(0, make([]byte, 0, 1024))
	if !retain.Retained() {
		t.Fatalf("tryRetain Retained() = false, decision=%s", retain.CreditDecision())
	}

	beforeDisable := state.state()
	if beforeDisable.Counters.CurrentRetainedBuffers != 1 {
		t.Fatalf("CurrentRetainedBuffers before disable = %d, want 1", beforeDisable.Counters.CurrentRetainedBuffers)
	}

	generation := state.disableBudget()
	afterDisable := state.state()

	if afterDisable.Budget.Generation != generation {
		t.Fatalf("Budget.Generation = %v, want %v", afterDisable.Budget.Generation, generation)
	}

	if !afterDisable.Budget.IsZero() {
		t.Fatalf("Budget after disable = %+v, want zero", afterDisable.Budget)
	}

	if afterDisable.Counters.CurrentRetainedBuffers != 1 {
		t.Fatalf("CurrentRetainedBuffers after disable = %d, want 1", afterDisable.Counters.CurrentRetainedBuffers)
	}

	if afterDisable.Counters.CurrentRetainedBytes != 1024 {
		t.Fatalf("CurrentRetainedBytes after disable = %d, want 1024", afterDisable.Counters.CurrentRetainedBytes)
	}

	for index, shard := range afterDisable.Shards {
		if !shard.Credit.IsZero() {
			t.Fatalf("shard %d Credit = %+v, want disabled zero credit", index, shard.Credit)
		}
	}

	if !afterDisable.IsOverBudget() {
		t.Fatal("IsOverBudget() after disable with retained usage = false, want true")
	}
}

// TestClassStateApplyShardCreditPlanPanicsForShardCountMismatch verifies that a
// class state rejects credit plans built for another shard count.
func TestClassStateApplyShardCreditPlanPanicsForShardCountMismatch(t *testing.T) {
	t.Parallel()

	class := NewSizeClass(ClassID(1), ClassSizeFromSize(KiB))
	state := newClassState(class, 2, 4)

	limit := newClassBudgetLimit(class.Size(), 4*KiB)
	plan := newClassShardCreditPlan(limit, 1)

	testutil.MustPanicWithMessage(t, errClassStateInvalidShardCount, func() {
		state.applyShardCreditPlan(plan)
	})
}

// TestClassStateTryGetMiss verifies class-level miss accounting.
//
// classState.tryGet should route to the selected shard and then record class
// counters from the observed shard-local result.
func TestClassStateTryGetMiss(t *testing.T) {
	t.Parallel()

	class := NewSizeClass(ClassID(1), ClassSizeFromSize(KiB))
	state := newClassState(class, 2, 4)

	result := state.tryGet(0, 512)

	if result.Class != class {
		t.Fatalf("result.Class = %s, want %s", result.Class, class)
	}

	if result.ShardIndex != 0 {
		t.Fatalf("result.ShardIndex = %d, want 0", result.ShardIndex)
	}

	if result.Hit() {
		t.Fatal("Hit() = true, want false")
	}

	if !result.Miss() {
		t.Fatal("Miss() = false, want true")
	}

	if result.Buffer() != nil {
		t.Fatalf("Buffer() = %v, want nil", result.Buffer())
	}

	if result.Capacity() != 0 {
		t.Fatalf("Capacity() = %d, want 0", result.Capacity())
	}

	snapshot := state.state()

	if snapshot.Counters.Gets != 1 {
		t.Fatalf("class Gets = %d, want 1", snapshot.Counters.Gets)
	}

	if snapshot.Counters.RequestedBytes != 512 {
		t.Fatalf("class RequestedBytes = %d, want 512", snapshot.Counters.RequestedBytes)
	}

	if snapshot.Counters.Misses != 1 {
		t.Fatalf("class Misses = %d, want 1", snapshot.Counters.Misses)
	}

	if snapshot.Shards[0].Counters.Gets != 1 {
		t.Fatalf("shard Gets = %d, want 1", snapshot.Shards[0].Counters.Gets)
	}

	if snapshot.Shards[0].Counters.Misses != 1 {
		t.Fatalf("shard Misses = %d, want 1", snapshot.Shards[0].Counters.Misses)
	}

	if snapshot.Shards[1].Counters.Gets != 0 {
		t.Fatalf("other shard Gets = %d, want 0", snapshot.Shards[1].Counters.Gets)
	}
}

// TestClassStateTryRetainAndTryGetHit verifies successful retention and reuse
// through the same selected shard.
//
// Both shard-level and class-level counters should remain consistent.
func TestClassStateTryRetainAndTryGetHit(t *testing.T) {
	t.Parallel()

	class := NewSizeClass(ClassID(1), ClassSizeFromSize(KiB))
	state := newClassState(class, 2, 4)
	state.updateBudget(4 * KiB)

	retain := state.tryRetain(0, make([]byte, 7, 1024))

	if retain.Class != class {
		t.Fatalf("retain.Class = %s, want %s", retain.Class, class)
	}

	if retain.ShardIndex != 0 {
		t.Fatalf("retain.ShardIndex = %d, want 0", retain.ShardIndex)
	}

	if !retain.Retained() {
		t.Fatalf("Retained() = false, decision=%s", retain.CreditDecision())
	}

	if retain.Dropped() {
		t.Fatal("Dropped() = true, want false")
	}

	if retain.RejectedByCredit() {
		t.Fatal("RejectedByCredit() = true, want false")
	}

	if retain.RejectedByBucket() {
		t.Fatal("RejectedByBucket() = true, want false")
	}

	if retain.Capacity() != 1024 {
		t.Fatalf("Capacity() = %d, want 1024", retain.Capacity())
	}

	if retain.CreditDecision() != shardCreditAccept {
		t.Fatalf("CreditDecision() = %s, want %s", retain.CreditDecision(), shardCreditAccept)
	}

	afterRetain := state.state()

	if afterRetain.Counters.Puts != 1 {
		t.Fatalf("class Puts after retain = %d, want 1", afterRetain.Counters.Puts)
	}

	if afterRetain.Counters.Retains != 1 {
		t.Fatalf("class Retains after retain = %d, want 1", afterRetain.Counters.Retains)
	}

	if afterRetain.Counters.CurrentRetainedBuffers != 1 {
		t.Fatalf("class CurrentRetainedBuffers after retain = %d, want 1", afterRetain.Counters.CurrentRetainedBuffers)
	}

	if afterRetain.Shards[0].Counters.Retains != 1 {
		t.Fatalf("shard Retains after retain = %d, want 1", afterRetain.Shards[0].Counters.Retains)
	}

	get := state.tryGet(0, 512)

	if !get.Hit() {
		t.Fatal("Hit() = false, want true")
	}

	if get.Miss() {
		t.Fatal("Miss() = true, want false")
	}

	if get.Buffer() == nil {
		t.Fatal("Buffer() = nil, want reused buffer")
	}

	if len(get.Buffer()) != 0 {
		t.Fatalf("len(Buffer()) = %d, want 0", len(get.Buffer()))
	}

	if cap(get.Buffer()) != 1024 {
		t.Fatalf("cap(Buffer()) = %d, want 1024", cap(get.Buffer()))
	}

	if get.Capacity() != 1024 {
		t.Fatalf("Capacity() = %d, want 1024", get.Capacity())
	}

	afterGet := state.state()

	if afterGet.Counters.Gets != 1 {
		t.Fatalf("class Gets after get = %d, want 1", afterGet.Counters.Gets)
	}

	if afterGet.Counters.Hits != 1 {
		t.Fatalf("class Hits after get = %d, want 1", afterGet.Counters.Hits)
	}

	if afterGet.Counters.HitBytes != 1024 {
		t.Fatalf("class HitBytes after get = %d, want 1024", afterGet.Counters.HitBytes)
	}

	if afterGet.Counters.CurrentRetainedBuffers != 0 {
		t.Fatalf("class CurrentRetainedBuffers after get = %d, want 0", afterGet.Counters.CurrentRetainedBuffers)
	}

	if afterGet.Counters.CurrentRetainedBytes != 0 {
		t.Fatalf("class CurrentRetainedBytes after get = %d, want 0", afterGet.Counters.CurrentRetainedBytes)
	}

	if afterGet.Shards[0].Counters.Hits != 1 {
		t.Fatalf("shard Hits after get = %d, want 1", afterGet.Shards[0].Counters.Hits)
	}
}

// TestClassStateTryRetainRejectedByCredit verifies class-level drop accounting
// when shard credit rejects retention.
func TestClassStateTryRetainRejectedByCredit(t *testing.T) {
	t.Parallel()

	class := NewSizeClass(ClassID(1), ClassSizeFromSize(KiB))
	state := newClassState(class, 2, 4)

	result := state.tryRetain(0, make([]byte, 0, 1024))

	if result.Retained() {
		t.Fatal("Retained() = true, want false")
	}

	if !result.Dropped() {
		t.Fatal("Dropped() = false, want true")
	}

	if !result.RejectedByCredit() {
		t.Fatal("RejectedByCredit() = false, want true")
	}

	if result.RejectedByBucket() {
		t.Fatal("RejectedByBucket() = true, want false")
	}

	if result.CreditDecision() != shardCreditRejectNoCredit {
		t.Fatalf("CreditDecision() = %s, want %s", result.CreditDecision(), shardCreditRejectNoCredit)
	}

	snapshot := state.state()

	if snapshot.Counters.Puts != 1 {
		t.Fatalf("class Puts = %d, want 1", snapshot.Counters.Puts)
	}

	if snapshot.Counters.Drops != 1 {
		t.Fatalf("class Drops = %d, want 1", snapshot.Counters.Drops)
	}

	if snapshot.Counters.DroppedBytes != 1024 {
		t.Fatalf("class DroppedBytes = %d, want 1024", snapshot.Counters.DroppedBytes)
	}

	if snapshot.Shards[0].Counters.Drops != 1 {
		t.Fatalf("shard Drops = %d, want 1", snapshot.Shards[0].Counters.Drops)
	}
}

// TestClassStateTryRetainRejectedByBucket verifies class-level drop accounting
// when credit accepts but physical bucket storage is full.
func TestClassStateTryRetainRejectedByBucket(t *testing.T) {
	t.Parallel()

	class := NewSizeClass(ClassID(1), ClassSizeFromSize(KiB))
	state := newClassState(class, 1, 1)
	state.updateBudget(2 * KiB)

	first := state.tryRetain(0, make([]byte, 0, 512))
	if !first.Retained() {
		t.Fatalf("first Retained() = false, decision=%s", first.CreditDecision())
	}

	second := state.tryRetain(0, make([]byte, 0, 512))

	if second.Retained() {
		t.Fatal("second Retained() = true, want false")
	}

	if second.RejectedByCredit() {
		t.Fatal("second RejectedByCredit() = true, want false")
	}

	if !second.RejectedByBucket() {
		t.Fatal("second RejectedByBucket() = false, want true")
	}

	if second.CreditDecision() != shardCreditAccept {
		t.Fatalf("second CreditDecision() = %s, want %s", second.CreditDecision(), shardCreditAccept)
	}

	snapshot := state.state()

	if snapshot.Counters.Retains != 1 {
		t.Fatalf("class Retains = %d, want 1", snapshot.Counters.Retains)
	}

	if snapshot.Counters.Drops != 1 {
		t.Fatalf("class Drops = %d, want 1", snapshot.Counters.Drops)
	}

	if snapshot.Counters.CurrentRetainedBuffers != 1 {
		t.Fatalf("class CurrentRetainedBuffers = %d, want 1", snapshot.Counters.CurrentRetainedBuffers)
	}

	if snapshot.Shards[0].Counters.Retains != 1 {
		t.Fatalf("shard Retains = %d, want 1", snapshot.Shards[0].Counters.Retains)
	}

	if snapshot.Shards[0].Counters.Drops != 1 {
		t.Fatalf("shard Drops = %d, want 1", snapshot.Shards[0].Counters.Drops)
	}
}

// TestClassStateOperationsPanicForInvalidShardIndex verifies selected-shard
// validation through class-level operations.
func TestClassStateOperationsPanicForInvalidShardIndex(t *testing.T) {
	t.Parallel()

	class := NewSizeClass(ClassID(1), ClassSizeFromSize(KiB))
	state := newClassState(class, 1, 1)

	tests := []struct {
		name string
		fn   func()
	}{
		{
			name: "tryGet",
			fn: func() {
				_ = state.tryGet(1, 1)
			},
		},
		{
			name: "recordAllocation",
			fn: func() {
				state.recordAllocation(1, 1)
			},
		},
		{
			name: "recordAllocatedBuffer",
			fn: func() {
				state.recordAllocatedBuffer(1, make([]byte, 0, 1))
			},
		},
		{
			name: "tryRetain",
			fn: func() {
				_ = state.tryRetain(1, make([]byte, 0, 1))
			},
		},
		{
			name: "trimShard",
			fn: func() {
				_ = state.trimShard(1, 1)
			},
		},
		{
			name: "clearShard",
			fn: func() {
				_ = state.clearShard(1)
			},
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			testutil.MustPanicWithMessage(t, errClassStateShardIndexOutOfRange, tt.fn)
		})
	}
}

// TestClassStateRecordAllocation verifies class-level and shard-level allocation
// accounting.
//
// classState does not allocate buffers. The owner records allocation after a
// miss or bypass path allocates a new backing buffer.
func TestClassStateRecordAllocation(t *testing.T) {
	t.Parallel()

	class := NewSizeClass(ClassID(1), ClassSizeFromSize(KiB))
	state := newClassState(class, 2, 4)

	state.recordAllocation(0, 1024)
	state.recordAllocatedBuffer(1, make([]byte, 7, 2048))

	snapshot := state.state()

	if snapshot.Counters.Allocations != 2 {
		t.Fatalf("class Allocations = %d, want 2", snapshot.Counters.Allocations)
	}

	if snapshot.Counters.AllocatedBytes != 3072 {
		t.Fatalf("class AllocatedBytes = %d, want 3072", snapshot.Counters.AllocatedBytes)
	}

	if snapshot.Shards[0].Counters.Allocations != 1 {
		t.Fatalf("shard 0 Allocations = %d, want 1", snapshot.Shards[0].Counters.Allocations)
	}

	if snapshot.Shards[0].Counters.AllocatedBytes != 1024 {
		t.Fatalf("shard 0 AllocatedBytes = %d, want 1024", snapshot.Shards[0].Counters.AllocatedBytes)
	}

	if snapshot.Shards[1].Counters.Allocations != 1 {
		t.Fatalf("shard 1 Allocations = %d, want 1", snapshot.Shards[1].Counters.Allocations)
	}

	if snapshot.Shards[1].Counters.AllocatedBytes != 2048 {
		t.Fatalf("shard 1 AllocatedBytes = %d, want 2048", snapshot.Shards[1].Counters.AllocatedBytes)
	}
}

// TestClassStateTrimShard verifies class-level and shard-level trim accounting.
func TestClassStateTrimShard(t *testing.T) {
	t.Parallel()

	class := NewSizeClass(ClassID(1), ClassSizeFromSize(KiB))
	state := newClassState(class, 2, 4)
	state.updateBudget(8 * KiB)

	for _, capacity := range []int{512, 1024, 2048} {
		result := state.tryRetain(0, make([]byte, 0, capacity))
		if !result.Retained() {
			t.Fatalf("tryRetain(%d) Retained() = false, decision=%s", capacity, result.CreditDecision())
		}
	}

	trim := state.trimShard(0, 2)

	if trim.RemovedBuffers != 2 {
		t.Fatalf("RemovedBuffers = %d, want 2", trim.RemovedBuffers)
	}

	if trim.RemovedBytes != 3072 {
		t.Fatalf("RemovedBytes = %d, want 3072", trim.RemovedBytes)
	}

	snapshot := state.state()

	if snapshot.Counters.TrimOperations != 1 {
		t.Fatalf("class TrimOperations = %d, want 1", snapshot.Counters.TrimOperations)
	}

	if snapshot.Counters.TrimmedBuffers != 2 {
		t.Fatalf("class TrimmedBuffers = %d, want 2", snapshot.Counters.TrimmedBuffers)
	}

	if snapshot.Counters.TrimmedBytes != 3072 {
		t.Fatalf("class TrimmedBytes = %d, want 3072", snapshot.Counters.TrimmedBytes)
	}

	if snapshot.Counters.CurrentRetainedBuffers != 1 {
		t.Fatalf("class CurrentRetainedBuffers = %d, want 1", snapshot.Counters.CurrentRetainedBuffers)
	}

	if snapshot.Counters.CurrentRetainedBytes != 512 {
		t.Fatalf("class CurrentRetainedBytes = %d, want 512", snapshot.Counters.CurrentRetainedBytes)
	}

	if snapshot.Shards[0].Counters.TrimOperations != 1 {
		t.Fatalf("shard TrimOperations = %d, want 1", snapshot.Shards[0].Counters.TrimOperations)
	}
}

// TestClassStateClearShard verifies class-level and shard-level clear accounting
// for one selected shard.
func TestClassStateClearShard(t *testing.T) {
	t.Parallel()

	class := NewSizeClass(ClassID(1), ClassSizeFromSize(KiB))
	state := newClassState(class, 2, 4)
	state.updateBudget(8 * KiB)

	for _, capacity := range []int{512, 1024} {
		result := state.tryRetain(0, make([]byte, 0, capacity))
		if !result.Retained() {
			t.Fatalf("tryRetain(%d) Retained() = false, decision=%s", capacity, result.CreditDecision())
		}
	}

	result := state.clearShard(0)

	if result.RemovedBuffers != 2 {
		t.Fatalf("RemovedBuffers = %d, want 2", result.RemovedBuffers)
	}

	if result.RemovedBytes != 1536 {
		t.Fatalf("RemovedBytes = %d, want 1536", result.RemovedBytes)
	}

	snapshot := state.state()

	if snapshot.Counters.ClearOperations != 1 {
		t.Fatalf("class ClearOperations = %d, want 1", snapshot.Counters.ClearOperations)
	}

	if snapshot.Counters.ClearedBuffers != 2 {
		t.Fatalf("class ClearedBuffers = %d, want 2", snapshot.Counters.ClearedBuffers)
	}

	if snapshot.Counters.ClearedBytes != 1536 {
		t.Fatalf("class ClearedBytes = %d, want 1536", snapshot.Counters.ClearedBytes)
	}

	if snapshot.Counters.CurrentRetainedBuffers != 0 {
		t.Fatalf("class CurrentRetainedBuffers = %d, want 0", snapshot.Counters.CurrentRetainedBuffers)
	}

	if snapshot.Counters.CurrentRetainedBytes != 0 {
		t.Fatalf("class CurrentRetainedBytes = %d, want 0", snapshot.Counters.CurrentRetainedBytes)
	}
}

// TestClassStateClearAll verifies aggregate clear across all class-owned shards.
//
// clear calls clearShard for every shard, so class-level ClearOperations counts
// one operation per shard, including shards that remove nothing.
func TestClassStateClearAll(t *testing.T) {
	t.Parallel()

	class := NewSizeClass(ClassID(1), ClassSizeFromSize(KiB))
	state := newClassState(class, 3, 4)
	state.updateBudget(9 * KiB)

	retained := []struct {
		shardIndex int
		capacity   int
	}{
		{shardIndex: 0, capacity: 512},
		{shardIndex: 0, capacity: 1024},
		{shardIndex: 1, capacity: 2048},
	}

	for _, item := range retained {
		result := state.tryRetain(item.shardIndex, make([]byte, 0, item.capacity))
		if !result.Retained() {
			t.Fatalf("tryRetain(shard=%d, capacity=%d) Retained() = false, decision=%s", item.shardIndex, item.capacity, result.CreditDecision())
		}
	}

	result := state.clear()

	if result.RemovedBuffers != 3 {
		t.Fatalf("RemovedBuffers = %d, want 3", result.RemovedBuffers)
	}

	if result.RemovedBytes != 3584 {
		t.Fatalf("RemovedBytes = %d, want 3584", result.RemovedBytes)
	}

	if result.IsZero() {
		t.Fatal("IsZero() = true, want false")
	}

	snapshot := state.state()

	if snapshot.Counters.ClearOperations != 3 {
		t.Fatalf("class ClearOperations = %d, want 3", snapshot.Counters.ClearOperations)
	}

	if snapshot.Counters.ClearedBuffers != 3 {
		t.Fatalf("class ClearedBuffers = %d, want 3", snapshot.Counters.ClearedBuffers)
	}

	if snapshot.Counters.ClearedBytes != 3584 {
		t.Fatalf("class ClearedBytes = %d, want 3584", snapshot.Counters.ClearedBytes)
	}

	if snapshot.Counters.CurrentRetainedBuffers != 0 {
		t.Fatalf("class CurrentRetainedBuffers = %d, want 0", snapshot.Counters.CurrentRetainedBuffers)
	}

	if snapshot.Counters.CurrentRetainedBytes != 0 {
		t.Fatalf("class CurrentRetainedBytes = %d, want 0", snapshot.Counters.CurrentRetainedBytes)
	}

	for index, shard := range snapshot.Shards {
		if shard.Bucket.RetainedBuffers != 0 {
			t.Fatalf("shard %d Bucket.RetainedBuffers = %d, want 0", index, shard.Bucket.RetainedBuffers)
		}
	}
}

// TestClassStateClearAllZero verifies zero aggregate clear behavior.
func TestClassStateClearAllZero(t *testing.T) {
	t.Parallel()

	class := NewSizeClass(ClassID(1), ClassSizeFromSize(KiB))
	state := newClassState(class, 2, 4)

	result := state.clear()

	if !result.IsZero() {
		t.Fatalf("clear() = %+v, want zero removal", result)
	}

	snapshot := state.state()

	if snapshot.Counters.ClearOperations != 2 {
		t.Fatalf("class ClearOperations = %d, want 2", snapshot.Counters.ClearOperations)
	}

	if snapshot.Counters.ClearedBuffers != 0 {
		t.Fatalf("class ClearedBuffers = %d, want 0", snapshot.Counters.ClearedBuffers)
	}

	if snapshot.Counters.ClearedBytes != 0 {
		t.Fatalf("class ClearedBytes = %d, want 0", snapshot.Counters.ClearedBytes)
	}
}

// TestClassStateSnapshotReturnsShardCopy verifies snapshot slice isolation.
func TestClassStateSnapshotReturnsShardCopy(t *testing.T) {
	t.Parallel()

	class := NewSizeClass(ClassID(1), ClassSizeFromSize(KiB))
	state := newClassState(class, 2, 4)

	first := state.state()
	first.Shards[0] = shardState{}

	second := state.state()

	if second.Shards[0].Bucket.SlotLimit != 4 {
		t.Fatalf("second.Shards[0].Bucket.SlotLimit = %d, want 4", second.Shards[0].Bucket.SlotLimit)
	}
}

// TestClassStateSnapshotRetainedUsage verifies retained usage derivation from
// class counter snapshot.
func TestClassStateSnapshotRetainedUsage(t *testing.T) {
	t.Parallel()

	snapshot := classStateSnapshot{
		Counters: classCountersSnapshot{
			CurrentRetainedBuffers: 3,
			CurrentRetainedBytes:   4096,
		},
	}

	usage := snapshot.RetainedUsage()

	if usage.RetainedBuffers != 3 {
		t.Fatalf("RetainedUsage().RetainedBuffers = %d, want 3", usage.RetainedBuffers)
	}

	if usage.RetainedBytes != 4096 {
		t.Fatalf("RetainedUsage().RetainedBytes = %d, want 4096", usage.RetainedBytes)
	}
}

// TestClassStateSnapshotIsOverBudget verifies class-level over-budget detection.
func TestClassStateSnapshotIsOverBudget(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string

		snapshot classStateSnapshot

		want bool
	}{
		{
			name: "zero ineffective budget with no retained usage",
			snapshot: classStateSnapshot{
				Budget: classBudgetSnapshot{
					ClassSize: ClassSizeFromSize(KiB),
				},
				Counters: classCountersSnapshot{},
			},
			want: false,
		},
		{
			name: "zero ineffective budget with retained usage",
			snapshot: classStateSnapshot{
				Budget: classBudgetSnapshot{
					ClassSize: ClassSizeFromSize(KiB),
				},
				Counters: classCountersSnapshot{
					CurrentRetainedBuffers: 1,
					CurrentRetainedBytes:   1024,
				},
			},
			want: true,
		},
		{
			name: "assigned but ineffective budget with retained usage",
			snapshot: classStateSnapshot{
				Budget: classBudgetSnapshot{
					ClassSize:      ClassSizeFromSize(4 * KiB),
					AssignedBytes:  uint64(2 * KiB),
					RemainderBytes: uint64(2 * KiB),
				},
				Counters: classCountersSnapshot{
					CurrentRetainedBuffers: 1,
					CurrentRetainedBytes:   1024,
				},
			},
			want: true,
		},
		{
			name: "within effective budget",
			snapshot: classStateSnapshot{
				Budget: classBudgetSnapshot{
					ClassSize:      ClassSizeFromSize(KiB),
					AssignedBytes:  uint64(4 * KiB),
					TargetBuffers:  4,
					TargetBytes:    uint64(4 * KiB),
					RemainderBytes: 0,
				},
				Counters: classCountersSnapshot{
					CurrentRetainedBuffers: 2,
					CurrentRetainedBytes:   2048,
				},
			},
			want: false,
		},
		{
			name: "over buffer target",
			snapshot: classStateSnapshot{
				Budget: classBudgetSnapshot{
					ClassSize:     ClassSizeFromSize(KiB),
					AssignedBytes: uint64(4 * KiB),
					TargetBuffers: 4,
					TargetBytes:   uint64(4 * KiB),
				},
				Counters: classCountersSnapshot{
					CurrentRetainedBuffers: 5,
					CurrentRetainedBytes:   2048,
				},
			},
			want: true,
		},
		{
			name: "over byte target",
			snapshot: classStateSnapshot{
				Budget: classBudgetSnapshot{
					ClassSize:     ClassSizeFromSize(KiB),
					AssignedBytes: uint64(4 * KiB),
					TargetBuffers: 4,
					TargetBytes:   uint64(4 * KiB),
				},
				Counters: classCountersSnapshot{
					CurrentRetainedBuffers: 2,
					CurrentRetainedBytes:   uint64(8 * KiB),
				},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.snapshot.IsOverBudget(); got != tt.want {
				t.Fatalf("IsOverBudget() = %t, want %t", got, tt.want)
			}
		})
	}
}

// TestClassStorageReductionResult verifies aggregate storage-reduction helpers.
func TestClassStorageReductionResult(t *testing.T) {
	t.Parallel()

	var result classStorageReductionResult

	if !result.IsZero() {
		t.Fatalf("zero result IsZero() = false, result=%+v", result)
	}

	result.addBucketTrimResult(bucketTrimResult{
		RemovedBuffers: 2,
		RemovedBytes:   1024,
	})

	result.addBucketTrimResult(bucketTrimResult{
		RemovedBuffers: 3,
		RemovedBytes:   2048,
	})

	if result.IsZero() {
		t.Fatal("non-zero result IsZero() = true, want false")
	}

	if result.RemovedBuffers != 5 {
		t.Fatalf("RemovedBuffers = %d, want 5", result.RemovedBuffers)
	}

	if result.RemovedBytes != 3072 {
		t.Fatalf("RemovedBytes = %d, want 3072", result.RemovedBytes)
	}
}

// TestClassStateConcurrentRetainAndGet verifies class-state synchronization under
// concurrent retain/get operations.
//
// The budget and bucket slot limit are intentionally large enough to avoid
// credit and bucket contention. The test checks that class-level and shard-level
// counters remain consistent.
func TestClassStateConcurrentRetainAndGet(t *testing.T) {
	t.Parallel()

	const shards = 4
	const workers = 8
	const iterations = 128
	const total = workers * iterations

	class := NewSizeClass(ClassID(1), ClassSizeFromSize(64))
	state := newClassState(class, shards, total)
	state.updateBudget(Size(total * 64))

	var retainWG sync.WaitGroup
	retainWG.Add(workers)

	for worker := 0; worker < workers; worker++ {
		worker := worker

		go func() {
			defer retainWG.Done()

			for iteration := 0; iteration < iterations; iteration++ {
				shardIndex := (worker + iteration) % shards
				result := state.tryRetain(shardIndex, make([]byte, 0, 64))
				if !result.Retained() {
					t.Errorf("tryRetain Retained() = false, shard=%d, decision=%s", shardIndex, result.CreditDecision())
				}
			}
		}()
	}

	retainWG.Wait()

	afterRetain := state.state()

	if afterRetain.Counters.Retains != uint64(total) {
		t.Fatalf("class Retains after retain = %d, want %d", afterRetain.Counters.Retains, total)
	}

	if afterRetain.Counters.CurrentRetainedBuffers != uint64(total) {
		t.Fatalf("class CurrentRetainedBuffers after retain = %d, want %d", afterRetain.Counters.CurrentRetainedBuffers, total)
	}

	var getWG sync.WaitGroup
	getWG.Add(workers)

	for worker := 0; worker < workers; worker++ {
		worker := worker

		go func() {
			defer getWG.Done()

			for iteration := 0; iteration < iterations; iteration++ {
				shardIndex := (worker + iteration) % shards
				result := state.tryGet(shardIndex, 32)
				if !result.Hit() {
					t.Errorf("tryGet Hit() = false, shard=%d", shardIndex)
				}
			}
		}()
	}

	getWG.Wait()

	afterGet := state.state()

	if afterGet.Counters.Gets != uint64(total) {
		t.Fatalf("class Gets after get = %d, want %d", afterGet.Counters.Gets, total)
	}

	if afterGet.Counters.Hits != uint64(total) {
		t.Fatalf("class Hits after get = %d, want %d", afterGet.Counters.Hits, total)
	}

	if afterGet.Counters.CurrentRetainedBuffers != 0 {
		t.Fatalf("class CurrentRetainedBuffers after get = %d, want 0", afterGet.Counters.CurrentRetainedBuffers)
	}

	if afterGet.Counters.CurrentRetainedBytes != 0 {
		t.Fatalf("class CurrentRetainedBytes after get = %d, want 0", afterGet.Counters.CurrentRetainedBytes)
	}

	var shardHits uint64
	for _, shard := range afterGet.Shards {
		shardHits += shard.Counters.Hits

		if shard.Bucket.RetainedBuffers != 0 {
			t.Fatalf("shard Bucket.RetainedBuffers = %d, want 0", shard.Bucket.RetainedBuffers)
		}
	}

	if shardHits != uint64(total) {
		t.Fatalf("sum shard Hits = %d, want %d", shardHits, total)
	}
}
