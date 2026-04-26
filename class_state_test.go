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
	"sync/atomic"
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

	if snapshot.CurrentRetainedBuffers != 0 {
		t.Fatalf("snapshot.CurrentRetainedBuffers = %d, want 0", snapshot.CurrentRetainedBuffers)
	}

	if snapshot.CurrentRetainedBytes != 0 {
		t.Fatalf("snapshot.CurrentRetainedBytes = %d, want 0", snapshot.CurrentRetainedBytes)
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

	testutil.MustPanicWithMessage(t, errBucketInvalidSlotLimit, func() {
		_ = newClassState(class, 1, 0)
	})
}

// TestClassStateMustShardAt verifies low-level shard lookup by index.
func TestClassStateMustShardAt(t *testing.T) {
	t.Parallel()

	class := NewSizeClass(ClassID(0), ClassSizeFromSize(KiB))
	state := newClassState(class, 2, 1)

	if state.mustShardAt(0) == nil {
		t.Fatal("mustShardAt(0) = nil, want shard pointer")
	}

	if state.mustShardAt(1) == nil {
		t.Fatal("mustShardAt(1) = nil, want shard pointer")
	}
}

// TestClassStateMustShardAtPanicsForInvalidIndex verifies shard index validation.
func TestClassStateMustShardAtPanicsForInvalidIndex(t *testing.T) {
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
				_ = state.mustShardAt(tt.shardIndex)
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

	assertClassStateRetainedConsistency(t, after)
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

	assertClassStateRetainedConsistency(t, snapshot)
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
	if beforeDisable.CurrentRetainedBuffers != 1 {
		t.Fatalf("CurrentRetainedBuffers before disable = %d, want 1", beforeDisable.CurrentRetainedBuffers)
	}

	assertClassStateRetainedConsistency(t, beforeDisable)

	generation := state.disableBudget()
	afterDisable := state.state()

	if afterDisable.Budget.Generation != generation {
		t.Fatalf("Budget.Generation = %v, want %v", afterDisable.Budget.Generation, generation)
	}

	if !afterDisable.Budget.IsZero() {
		t.Fatalf("Budget after disable = %+v, want zero", afterDisable.Budget)
	}

	if afterDisable.CurrentRetainedBuffers != 1 {
		t.Fatalf("CurrentRetainedBuffers after disable = %d, want 1", afterDisable.CurrentRetainedBuffers)
	}

	if afterDisable.CurrentRetainedBytes != 1024 {
		t.Fatalf("CurrentRetainedBytes after disable = %d, want 1024", afterDisable.CurrentRetainedBytes)
	}

	for index, shard := range afterDisable.Shards {
		if !shard.Credit.IsZero() {
			t.Fatalf("shard %d Credit = %+v, want disabled zero credit", index, shard.Credit)
		}
	}

	if !afterDisable.IsOverBudget() {
		t.Fatal("IsOverBudget() after disable with retained usage = false, want true")
	}

	assertClassStateRetainedConsistency(t, afterDisable)
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

	assertClassStateRetainedConsistency(t, snapshot)
}

// TestClassStateTryGetGuardsRequestedSize verifies class routing invariants.
func TestClassStateTryGetGuardsRequestedSize(t *testing.T) {
	t.Parallel()

	class := NewSizeClass(ClassID(1), ClassSizeFromSize(KiB))
	state := newClassState(class, 1, 4)

	testutil.MustPanicWithMessage(t, errClassStateRequestExceedsClassSize, func() {
		_ = state.tryGet(0, 2*KiB)
	})

	_ = state.tryGet(0, KiB)
	_ = state.tryGet(0, 512)
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

	if retain.RejectedByClass() {
		t.Fatal("RejectedByClass() = true, want false")
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

	if retain.ClassRetainDecision() != classRetainAccept {
		t.Fatalf("ClassRetainDecision() = %s, want %s", retain.ClassRetainDecision(), classRetainAccept)
	}

	afterRetain := state.state()

	if afterRetain.Counters.Puts != 1 {
		t.Fatalf("class Puts after retain = %d, want 1", afterRetain.Counters.Puts)
	}

	if afterRetain.Counters.Retains != 1 {
		t.Fatalf("class Retains after retain = %d, want 1", afterRetain.Counters.Retains)
	}

	if afterRetain.CurrentRetainedBuffers != 1 {
		t.Fatalf("class CurrentRetainedBuffers after retain = %d, want 1", afterRetain.CurrentRetainedBuffers)
	}

	if afterRetain.Shards[0].Counters.Retains != 1 {
		t.Fatalf("shard Retains after retain = %d, want 1", afterRetain.Shards[0].Counters.Retains)
	}

	assertClassStateRetainedConsistency(t, afterRetain)

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

	if afterGet.CurrentRetainedBuffers != 0 {
		t.Fatalf("class CurrentRetainedBuffers after get = %d, want 0", afterGet.CurrentRetainedBuffers)
	}

	if afterGet.CurrentRetainedBytes != 0 {
		t.Fatalf("class CurrentRetainedBytes after get = %d, want 0", afterGet.CurrentRetainedBytes)
	}

	if afterGet.Shards[0].Counters.Hits != 1 {
		t.Fatalf("shard Hits after get = %d, want 1", afterGet.Shards[0].Counters.Hits)
	}

	assertClassStateRetainedConsistency(t, afterGet)
}

// TestClassStateSelectedRetainAndGet verifies selector-based class operations.
func TestClassStateSelectedRetainAndGet(t *testing.T) {
	t.Parallel()

	class := NewSizeClass(ClassID(1), ClassSizeFromSize(KiB))
	state := newClassState(class, 2, 4)
	state.updateBudget(4 * KiB)

	var selector singleShardSelector

	retain := state.tryRetainSelected(selector, make([]byte, 0, 1024))
	if !retain.Retained() {
		t.Fatalf("tryRetainSelected Retained() = false, decision=%s", retain.CreditDecision())
	}

	if retain.ShardIndex != 0 {
		t.Fatalf("retain ShardIndex = %d, want 0", retain.ShardIndex)
	}

	get := state.tryGetSelected(selector, 512)
	if !get.Hit() {
		t.Fatal("tryGetSelected Hit() = false, want true")
	}

	if get.ShardIndex != 0 {
		t.Fatalf("get ShardIndex = %d, want 0", get.ShardIndex)
	}

	assertClassStateRetainedConsistency(t, state.state())
}

// TestClassStateTryRetainRejectsCapacityBelowClassSize verifies that retained
// buffers must be able to serve the class.
func TestClassStateTryRetainRejectsCapacityBelowClassSize(t *testing.T) {
	t.Parallel()

	class := NewSizeClass(ClassID(1), ClassSizeFromSize(KiB))
	state := newClassState(class, 1, 4)
	state.updateBudget(4 * KiB)

	result := state.tryRetain(0, make([]byte, 0, 512))

	assertClassStateClassRejectedRetain(t, state.state(), result, classRetainRejectCapacityBelowClassSize, 512)
}

// TestClassStateTryRetainRejectsNilBufferBeforeShard verifies class-local nil
// buffer rejection.
func TestClassStateTryRetainRejectsNilBufferBeforeShard(t *testing.T) {
	t.Parallel()

	class := NewSizeClass(ClassID(1), ClassSizeFromSize(KiB))
	state := newClassState(class, 1, 4)
	state.updateBudget(4 * KiB)

	result := state.tryRetain(0, nil)

	assertClassStateClassRejectedRetain(t, state.state(), result, classRetainRejectNilBuffer, 0)
}

// TestClassStateTryRetainRejectsZeroCapacityBeforeShard verifies class-local
// zero-capacity rejection.
func TestClassStateTryRetainRejectsZeroCapacityBeforeShard(t *testing.T) {
	t.Parallel()

	class := NewSizeClass(ClassID(1), ClassSizeFromSize(KiB))
	state := newClassState(class, 1, 4)
	state.updateBudget(4 * KiB)

	result := state.tryRetain(0, make([]byte, 0))

	assertClassStateClassRejectedRetain(t, state.state(), result, classRetainRejectZeroCapacity, 0)
}

// TestClassTableAndClassStateCapacityCompatibility verifies request and capacity
// routing compatibility between the class table and class runtime state.
func TestClassTableAndClassStateCapacityCompatibility(t *testing.T) {
	t.Parallel()

	table := newClassTable([]ClassSize{
		ClassSizeFromBytes(512),
		ClassSizeFromSize(KiB),
		ClassSizeFromSize(2 * KiB),
	})

	class512, ok := table.classForExactSize(ClassSizeFromBytes(512))
	if !ok {
		t.Fatal("classForExactSize(512 B) ok = false, want true")
	}

	class1KiB, ok := table.classForExactSize(ClassSizeFromSize(KiB))
	if !ok {
		t.Fatal("classForExactSize(1 KiB) ok = false, want true")
	}

	state512 := newClassState(class512, 1, 4)
	state512.updateBudget(2 * KiB)

	state1KiB := newClassState(class1KiB, 1, 4)
	state1KiB.updateBudget(4 * KiB)

	capacityClass, ok := table.classForCapacity(SizeFromInt(700))
	if !ok {
		t.Fatal("classForCapacity(700 B) ok = false, want true")
	}

	if !capacityClass.Equal(class512) {
		t.Fatalf("classForCapacity(700 B) = %s, want %s", capacityClass, class512)
	}

	retain512 := state512.tryRetain(0, make([]byte, 0, 700))
	if !retain512.Retained() {
		t.Fatalf("512 B class retain Retained() = false, decision=%s", retain512.CreditDecision())
	}

	assertClassStateRetainedConsistency(t, state512.state())

	beforeReject := state1KiB.state()
	reject1KiB := state1KiB.tryRetain(0, make([]byte, 0, 700))
	if !reject1KiB.RejectedByClass() {
		t.Fatal("1 KiB class RejectedByClass() = false, want true")
	}

	if reject1KiB.ClassRetainDecision() != classRetainRejectCapacityBelowClassSize {
		t.Fatalf("ClassRetainDecision() = %s, want %s", reject1KiB.ClassRetainDecision(), classRetainRejectCapacityBelowClassSize)
	}

	afterReject := state1KiB.state()
	if afterReject.Shards[0].Counters != beforeReject.Shards[0].Counters {
		t.Fatalf("rejected retain changed shard counters: before=%+v after=%+v", beforeReject.Shards[0].Counters, afterReject.Shards[0].Counters)
	}

	if afterReject.Shards[0].Bucket != beforeReject.Shards[0].Bucket {
		t.Fatalf("rejected retain changed bucket state: before=%+v after=%+v", beforeReject.Shards[0].Bucket, afterReject.Shards[0].Bucket)
	}

	requestClass, ok := table.classForRequest(SizeFromInt(700))
	if !ok {
		t.Fatal("classForRequest(700 B) ok = false, want true")
	}

	if !requestClass.Equal(class1KiB) {
		t.Fatalf("classForRequest(700 B) = %s, want %s", requestClass, class1KiB)
	}

	retain1KiB := state1KiB.tryRetain(0, make([]byte, 0, 1024))
	if !retain1KiB.Retained() {
		t.Fatalf("1 KiB class valid retain Retained() = false, decision=%s", retain1KiB.CreditDecision())
	}

	get := state1KiB.tryGet(0, SizeFromInt(700))
	if !get.Hit() {
		t.Fatal("tryGet(700 B) Hit() = false, want true")
	}

	if get.Capacity() < uint64(class1KiB.Bytes()) {
		t.Fatalf("tryGet(700 B) capacity = %d, want at least %d", get.Capacity(), class1KiB.Bytes())
	}

	testutil.MustPanicWithMessage(t, errClassStateRequestExceedsClassSize, func() {
		_ = state1KiB.tryGet(0, 2*KiB)
	})

	assertClassStateRetainedConsistency(t, state1KiB.state())
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

	if result.RejectedByClass() {
		t.Fatal("RejectedByClass() = true, want false")
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

	if result.ClassRetainDecision() != classRetainAccept {
		t.Fatalf("ClassRetainDecision() = %s, want %s", result.ClassRetainDecision(), classRetainAccept)
	}

	snapshot := state.state()

	if snapshot.Counters.Puts != 1 {
		t.Fatalf("class Puts = %d, want 1", snapshot.Counters.Puts)
	}

	if snapshot.Counters.ReturnedBytes != 1024 {
		t.Fatalf("class ReturnedBytes = %d, want 1024", snapshot.Counters.ReturnedBytes)
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

	if snapshot.Shards[0].Counters.Puts != 1 {
		t.Fatalf("shard Puts = %d, want 1", snapshot.Shards[0].Counters.Puts)
	}

	if snapshot.Shards[0].Counters.ReturnedBytes != 1024 {
		t.Fatalf("shard ReturnedBytes = %d, want 1024", snapshot.Shards[0].Counters.ReturnedBytes)
	}

	if snapshot.Shards[0].Counters.DroppedBytes != 1024 {
		t.Fatalf("shard DroppedBytes = %d, want 1024", snapshot.Shards[0].Counters.DroppedBytes)
	}

	if snapshot.Shards[0].Bucket.RetainedBuffers != 0 {
		t.Fatalf("bucket RetainedBuffers = %d, want 0", snapshot.Shards[0].Bucket.RetainedBuffers)
	}

	assertClassStateRetainedConsistency(t, snapshot)
}

// TestClassStateTryRetainRejectedByBucket verifies class-level drop accounting
// when credit accepts but physical bucket storage is full.
func TestClassStateTryRetainRejectedByBucket(t *testing.T) {
	t.Parallel()

	class := NewSizeClass(ClassID(1), ClassSizeFromSize(KiB))
	state := newClassState(class, 1, 1)
	state.updateBudget(2 * KiB)

	first := state.tryRetain(0, make([]byte, 0, 1024))
	if !first.Retained() {
		t.Fatalf("first Retained() = false, decision=%s", first.CreditDecision())
	}

	second := state.tryRetain(0, make([]byte, 0, 1024))

	if second.Retained() {
		t.Fatal("second Retained() = true, want false")
	}

	if second.RejectedByClass() {
		t.Fatal("second RejectedByClass() = true, want false")
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

	if second.ClassRetainDecision() != classRetainAccept {
		t.Fatalf("second ClassRetainDecision() = %s, want %s", second.ClassRetainDecision(), classRetainAccept)
	}

	snapshot := state.state()

	if snapshot.Counters.Puts != 2 {
		t.Fatalf("class Puts = %d, want 2", snapshot.Counters.Puts)
	}

	if snapshot.Counters.ReturnedBytes != 2048 {
		t.Fatalf("class ReturnedBytes = %d, want 2048", snapshot.Counters.ReturnedBytes)
	}

	if snapshot.Counters.Retains != 1 {
		t.Fatalf("class Retains = %d, want 1", snapshot.Counters.Retains)
	}

	if snapshot.Counters.Drops != 1 {
		t.Fatalf("class Drops = %d, want 1", snapshot.Counters.Drops)
	}

	if snapshot.Counters.DroppedBytes != 1024 {
		t.Fatalf("class DroppedBytes = %d, want 1024", snapshot.Counters.DroppedBytes)
	}

	if snapshot.CurrentRetainedBuffers != 1 {
		t.Fatalf("class CurrentRetainedBuffers = %d, want 1", snapshot.CurrentRetainedBuffers)
	}

	if snapshot.Shards[0].Counters.Puts != 2 {
		t.Fatalf("shard Puts = %d, want 2", snapshot.Shards[0].Counters.Puts)
	}

	if snapshot.Shards[0].Counters.ReturnedBytes != 2048 {
		t.Fatalf("shard ReturnedBytes = %d, want 2048", snapshot.Shards[0].Counters.ReturnedBytes)
	}

	if snapshot.CurrentRetainedBytes != 1024 {
		t.Fatalf("class CurrentRetainedBytes = %d, want 1024", snapshot.CurrentRetainedBytes)
	}

	if snapshot.Shards[0].Counters.Retains != 1 {
		t.Fatalf("shard Retains = %d, want 1", snapshot.Shards[0].Counters.Retains)
	}

	if snapshot.Shards[0].Counters.Drops != 1 {
		t.Fatalf("shard Drops = %d, want 1", snapshot.Shards[0].Counters.Drops)
	}

	if snapshot.Shards[0].Counters.DroppedBytes != 1024 {
		t.Fatalf("shard DroppedBytes = %d, want 1024", snapshot.Shards[0].Counters.DroppedBytes)
	}

	if snapshot.Shards[0].Bucket.RetainedBuffers != 1 {
		t.Fatalf("bucket RetainedBuffers = %d, want 1", snapshot.Shards[0].Bucket.RetainedBuffers)
	}

	assertClassStateRetainedConsistency(t, snapshot)
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

	assertClassStateRetainedConsistency(t, snapshot)
}

func TestClassStateRecordAllocationPanicsForZeroCapacity(t *testing.T) {
	t.Parallel()

	class := NewSizeClass(ClassID(1), ClassSizeFromSize(KiB))
	state := newClassState(class, 1, 4)

	testutil.MustPanicWithMessage(t, errClassStateInvalidAllocationCapacity, func() {
		state.recordAllocation(0, 0)
	})

	snapshot := state.state()
	if !snapshot.Counters.IsZero() {
		t.Fatalf("class counters after panic = %+v, want zero", snapshot.Counters)
	}
	if !snapshot.Shards[0].Counters.IsZero() {
		t.Fatalf("shard counters after panic = %+v, want zero", snapshot.Shards[0].Counters)
	}
}

func TestClassStateRecordAllocationPanicsForCapacityBelowClassSize(t *testing.T) {
	t.Parallel()

	class := NewSizeClass(ClassID(1), ClassSizeFromSize(KiB))
	state := newClassState(class, 1, 4)

	testutil.MustPanicWithMessage(t, errClassStateAllocationBelowClassSize, func() {
		state.recordAllocation(0, 512)
	})

	snapshot := state.state()
	if !snapshot.Counters.IsZero() {
		t.Fatalf("class counters after panic = %+v, want zero", snapshot.Counters)
	}
	if !snapshot.Shards[0].Counters.IsZero() {
		t.Fatalf("shard counters after panic = %+v, want zero", snapshot.Shards[0].Counters)
	}
}

func TestClassStateRecordAllocationAcceptsClassSizeCapacity(t *testing.T) {
	t.Parallel()

	class := NewSizeClass(ClassID(1), ClassSizeFromSize(KiB))
	state := newClassState(class, 1, 4)

	state.recordAllocation(0, uint64(KiB))

	snapshot := state.state()
	if snapshot.Counters.Allocations != 1 {
		t.Fatalf("class Allocations = %d, want 1", snapshot.Counters.Allocations)
	}
	if snapshot.Counters.AllocatedBytes != uint64(KiB) {
		t.Fatalf("class AllocatedBytes = %d, want %d", snapshot.Counters.AllocatedBytes, uint64(KiB))
	}
	if snapshot.Shards[0].Counters.Allocations != 1 {
		t.Fatalf("shard Allocations = %d, want 1", snapshot.Shards[0].Counters.Allocations)
	}
	if snapshot.Shards[0].Counters.AllocatedBytes != uint64(KiB) {
		t.Fatalf("shard AllocatedBytes = %d, want %d", snapshot.Shards[0].Counters.AllocatedBytes, uint64(KiB))
	}

	assertClassStateRetainedConsistency(t, snapshot)
}

func TestClassStateRecordAllocationAcceptsCapacityAboveClassSize(t *testing.T) {
	t.Parallel()

	class := NewSizeClass(ClassID(1), ClassSizeFromSize(KiB))
	state := newClassState(class, 1, 4)

	state.recordAllocation(0, uint64(2*KiB))

	snapshot := state.state()
	if snapshot.Counters.Allocations != 1 {
		t.Fatalf("class Allocations = %d, want 1", snapshot.Counters.Allocations)
	}
	if snapshot.Counters.AllocatedBytes != uint64(2*KiB) {
		t.Fatalf("class AllocatedBytes = %d, want %d", snapshot.Counters.AllocatedBytes, uint64(2*KiB))
	}
	if snapshot.Shards[0].Counters.Allocations != 1 {
		t.Fatalf("shard Allocations = %d, want 1", snapshot.Shards[0].Counters.Allocations)
	}
	if snapshot.Shards[0].Counters.AllocatedBytes != uint64(2*KiB) {
		t.Fatalf("shard AllocatedBytes = %d, want %d", snapshot.Shards[0].Counters.AllocatedBytes, uint64(2*KiB))
	}

	assertClassStateRetainedConsistency(t, snapshot)
}

func TestClassStateRecordAllocatedBufferPanicsForInvalidCapacity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		buffer      []byte
		wantMessage string
	}{
		{
			name:        "nil",
			buffer:      nil,
			wantMessage: errClassStateInvalidAllocationCapacity,
		},
		{
			name:        "zero capacity",
			buffer:      make([]byte, 0),
			wantMessage: errClassStateInvalidAllocationCapacity,
		},
		{
			name:        "below class size",
			buffer:      make([]byte, 0, 512),
			wantMessage: errClassStateAllocationBelowClassSize,
		},
	}

	class := NewSizeClass(ClassID(1), ClassSizeFromSize(KiB))

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			state := newClassState(class, 1, 4)

			testutil.MustPanicWithMessage(t, tt.wantMessage, func() {
				state.recordAllocatedBuffer(0, tt.buffer)
			})

			snapshot := state.state()
			if !snapshot.Counters.IsZero() {
				t.Fatalf("class counters after panic = %+v, want zero", snapshot.Counters)
			}
			if !snapshot.Shards[0].Counters.IsZero() {
				t.Fatalf("shard counters after panic = %+v, want zero", snapshot.Shards[0].Counters)
			}

			assertClassStateRetainedConsistency(t, snapshot)
		})
	}
}

func TestClassStateRecordAllocatedBufferAcceptsCapacityAboveClassSize(t *testing.T) {
	t.Parallel()

	class := NewSizeClass(ClassID(1), ClassSizeFromSize(KiB))
	state := newClassState(class, 1, 4)

	state.recordAllocatedBuffer(0, make([]byte, 7, 2*KiB))

	snapshot := state.state()
	if snapshot.Counters.Allocations != 1 {
		t.Fatalf("class Allocations = %d, want 1", snapshot.Counters.Allocations)
	}
	if snapshot.Counters.AllocatedBytes != uint64(2*KiB) {
		t.Fatalf("class AllocatedBytes = %d, want %d", snapshot.Counters.AllocatedBytes, uint64(2*KiB))
	}
	if snapshot.Shards[0].Counters.Allocations != 1 {
		t.Fatalf("shard Allocations = %d, want 1", snapshot.Shards[0].Counters.Allocations)
	}
	if snapshot.Shards[0].Counters.AllocatedBytes != uint64(2*KiB) {
		t.Fatalf("shard AllocatedBytes = %d, want %d", snapshot.Shards[0].Counters.AllocatedBytes, uint64(2*KiB))
	}

	assertClassStateRetainedConsistency(t, snapshot)
}

// TestClassStateTrimShardRecordsOneClassTrimOperation verifies class-level and
// shard-level trim accounting.
func TestClassStateTrimShardRecordsOneClassTrimOperation(t *testing.T) {
	t.Parallel()

	class := NewSizeClass(ClassID(1), ClassSizeFromSize(KiB))
	state := newClassState(class, 2, 4)
	state.updateBudget(16 * KiB)

	for _, capacity := range []int{1024, 2048, 4096} {
		result := state.tryRetain(0, make([]byte, 0, capacity))
		if !result.Retained() {
			t.Fatalf("tryRetain(%d) Retained() = false, decision=%s", capacity, result.CreditDecision())
		}
	}

	trim := state.trimShard(0, 2)

	if trim.RemovedBuffers != 2 {
		t.Fatalf("RemovedBuffers = %d, want 2", trim.RemovedBuffers)
	}

	if trim.RemovedBytes != 6144 {
		t.Fatalf("RemovedBytes = %d, want 6144", trim.RemovedBytes)
	}

	snapshot := state.state()

	if snapshot.Counters.TrimOperations != 1 {
		t.Fatalf("class TrimOperations = %d, want 1", snapshot.Counters.TrimOperations)
	}

	if snapshot.Counters.TrimmedBuffers != 2 {
		t.Fatalf("class TrimmedBuffers = %d, want 2", snapshot.Counters.TrimmedBuffers)
	}

	if snapshot.Counters.TrimmedBytes != 6144 {
		t.Fatalf("class TrimmedBytes = %d, want 6144", snapshot.Counters.TrimmedBytes)
	}

	if snapshot.CurrentRetainedBuffers != 1 {
		t.Fatalf("class CurrentRetainedBuffers = %d, want 1", snapshot.CurrentRetainedBuffers)
	}

	if snapshot.CurrentRetainedBytes != 1024 {
		t.Fatalf("class CurrentRetainedBytes = %d, want 1024", snapshot.CurrentRetainedBytes)
	}

	if snapshot.Shards[0].Counters.TrimOperations != 1 {
		t.Fatalf("shard TrimOperations = %d, want 1", snapshot.Shards[0].Counters.TrimOperations)
	}

	assertClassStateRetainedConsistency(t, snapshot)
}

// TestClassStateClearShardRecordsOneClassOperation verifies class-level and
// shard-level clear accounting for one selected shard.
func TestClassStateClearShardRecordsOneClassOperation(t *testing.T) {
	t.Parallel()

	class := NewSizeClass(ClassID(1), ClassSizeFromSize(KiB))
	state := newClassState(class, 2, 4)
	state.updateBudget(8 * KiB)

	for _, capacity := range []int{1024, 2048} {
		result := state.tryRetain(0, make([]byte, 0, capacity))
		if !result.Retained() {
			t.Fatalf("tryRetain(%d) Retained() = false, decision=%s", capacity, result.CreditDecision())
		}
	}

	result := state.clearShard(0)

	if result.RemovedBuffers != 2 {
		t.Fatalf("RemovedBuffers = %d, want 2", result.RemovedBuffers)
	}

	if result.RemovedBytes != 3072 {
		t.Fatalf("RemovedBytes = %d, want 3072", result.RemovedBytes)
	}

	snapshot := state.state()

	if snapshot.Counters.ClearOperations != 1 {
		t.Fatalf("class ClearOperations = %d, want 1", snapshot.Counters.ClearOperations)
	}

	if snapshot.Counters.ClearedBuffers != 2 {
		t.Fatalf("class ClearedBuffers = %d, want 2", snapshot.Counters.ClearedBuffers)
	}

	if snapshot.Counters.ClearedBytes != 3072 {
		t.Fatalf("class ClearedBytes = %d, want 3072", snapshot.Counters.ClearedBytes)
	}

	if snapshot.CurrentRetainedBuffers != 0 {
		t.Fatalf("class CurrentRetainedBuffers = %d, want 0", snapshot.CurrentRetainedBuffers)
	}

	if snapshot.CurrentRetainedBytes != 0 {
		t.Fatalf("class CurrentRetainedBytes = %d, want 0", snapshot.CurrentRetainedBytes)
	}

	if snapshot.Shards[0].Counters.ClearOperations != 1 {
		t.Fatalf("shard ClearOperations = %d, want 1", snapshot.Shards[0].Counters.ClearOperations)
	}

	if snapshot.Shards[1].Counters.ClearOperations != 0 {
		t.Fatalf("other shard ClearOperations = %d, want 0", snapshot.Shards[1].Counters.ClearOperations)
	}

	assertClassStateRetainedConsistency(t, snapshot)
}

// TestClassStateClearRecordsOneClassOperation verifies that one class-wide clear
// call records exactly one class-level clear operation.
func TestClassStateClearRecordsOneClassOperation(t *testing.T) {
	t.Parallel()

	class := NewSizeClass(ClassID(1), ClassSizeFromSize(KiB))
	state := newClassState(class, 4, 4)
	state.updateBudget(4 * KiB)

	for _, shardIndex := range []int{0, 2} {
		result := state.tryRetain(shardIndex, make([]byte, 0, 1024))
		if !result.Retained() {
			t.Fatalf("tryRetain(shard=%d) Retained() = false, decision=%s", shardIndex, result.CreditDecision())
		}
	}

	result := state.clear()
	if result.RemovedBuffers != 2 {
		t.Fatalf("RemovedBuffers = %d, want 2", result.RemovedBuffers)
	}

	snapshot := state.state()
	if snapshot.Counters.ClearOperations != 1 {
		t.Fatalf("class ClearOperations = %d, want 1", snapshot.Counters.ClearOperations)
	}
	if snapshot.Counters.ClearedBuffers != 2 {
		t.Fatalf("class ClearedBuffers = %d, want 2", snapshot.Counters.ClearedBuffers)
	}
	if snapshot.Counters.ClearedBytes != 2048 {
		t.Fatalf("class ClearedBytes = %d, want 2048", snapshot.Counters.ClearedBytes)
	}

	assertClassStateRetainedConsistency(t, snapshot)
}

// TestClassStateClearAggregatesRemovedStorage verifies aggregate clear across
// all class-owned shards.
//
// clear is one class-level operation while each shard still records its local
// bucket clear operation.
func TestClassStateClearAggregatesRemovedStorage(t *testing.T) {
	t.Parallel()

	class := NewSizeClass(ClassID(1), ClassSizeFromSize(KiB))
	state := newClassState(class, 4, 4)
	state.updateBudget(16 * KiB)

	retained := []struct {
		shardIndex int
		capacity   int
	}{
		{shardIndex: 0, capacity: 1024},
		{shardIndex: 0, capacity: 2048},
		{shardIndex: 1, capacity: 1024},
		{shardIndex: 1, capacity: 2048},
		{shardIndex: 3, capacity: 4096},
	}

	for _, item := range retained {
		result := state.tryRetain(item.shardIndex, make([]byte, 0, item.capacity))
		if !result.Retained() {
			t.Fatalf("tryRetain(shard=%d, capacity=%d) Retained() = false, decision=%s", item.shardIndex, item.capacity, result.CreditDecision())
		}
	}

	result := state.clear()

	if result.RemovedBuffers != 5 {
		t.Fatalf("RemovedBuffers = %d, want 5", result.RemovedBuffers)
	}

	if result.RemovedBytes != 10240 {
		t.Fatalf("RemovedBytes = %d, want 10240", result.RemovedBytes)
	}

	if result.IsZero() {
		t.Fatal("IsZero() = true, want false")
	}

	snapshot := state.state()

	if snapshot.Counters.ClearOperations != 1 {
		t.Fatalf("class ClearOperations = %d, want 1", snapshot.Counters.ClearOperations)
	}

	if snapshot.Counters.ClearedBuffers != 5 {
		t.Fatalf("class ClearedBuffers = %d, want 5", snapshot.Counters.ClearedBuffers)
	}

	if snapshot.Counters.ClearedBytes != 10240 {
		t.Fatalf("class ClearedBytes = %d, want 10240", snapshot.Counters.ClearedBytes)
	}

	if snapshot.CurrentRetainedBuffers != 0 {
		t.Fatalf("class CurrentRetainedBuffers = %d, want 0", snapshot.CurrentRetainedBuffers)
	}

	if snapshot.CurrentRetainedBytes != 0 {
		t.Fatalf("class CurrentRetainedBytes = %d, want 0", snapshot.CurrentRetainedBytes)
	}

	for index, shard := range snapshot.Shards {
		if shard.Bucket.RetainedBuffers != 0 {
			t.Fatalf("shard %d Bucket.RetainedBuffers = %d, want 0", index, shard.Bucket.RetainedBuffers)
		}

		if shard.Counters.ClearOperations != 1 {
			t.Fatalf("shard %d ClearOperations = %d, want 1", index, shard.Counters.ClearOperations)
		}
	}

	assertClassStateRetainedConsistency(t, snapshot)

	again := state.clear()
	if !again.IsZero() {
		t.Fatalf("second clear() = %+v, want zero removal", again)
	}

	afterSecond := state.state()
	if afterSecond.Counters.ClearOperations != 2 {
		t.Fatalf("class ClearOperations after second clear = %d, want 2", afterSecond.Counters.ClearOperations)
	}

	if afterSecond.Counters.ClearedBuffers != 5 {
		t.Fatalf("class ClearedBuffers after second clear = %d, want 5", afterSecond.Counters.ClearedBuffers)
	}

	if afterSecond.Counters.ClearedBytes != 10240 {
		t.Fatalf("class ClearedBytes after second clear = %d, want 10240", afterSecond.Counters.ClearedBytes)
	}

	for index, shard := range afterSecond.Shards {
		if shard.Counters.ClearOperations != 2 {
			t.Fatalf("shard %d ClearOperations after second clear = %d, want 2", index, shard.Counters.ClearOperations)
		}
	}

	assertClassStateRetainedConsistency(t, afterSecond)
}

// TestClassStateClearIsSafeWhenAlreadyEmpty verifies zero aggregate clear
// behavior.
func TestClassStateClearIsSafeWhenAlreadyEmpty(t *testing.T) {
	t.Parallel()

	class := NewSizeClass(ClassID(1), ClassSizeFromSize(KiB))
	state := newClassState(class, 2, 4)

	result := state.clear()

	if !result.IsZero() {
		t.Fatalf("clear() = %+v, want zero removal", result)
	}

	snapshot := state.state()

	if snapshot.Counters.ClearOperations != 1 {
		t.Fatalf("class ClearOperations = %d, want 1", snapshot.Counters.ClearOperations)
	}

	if snapshot.Counters.ClearedBuffers != 0 {
		t.Fatalf("class ClearedBuffers = %d, want 0", snapshot.Counters.ClearedBuffers)
	}

	if snapshot.Counters.ClearedBytes != 0 {
		t.Fatalf("class ClearedBytes = %d, want 0", snapshot.Counters.ClearedBytes)
	}

	for index, shard := range snapshot.Shards {
		if shard.Counters.ClearOperations != 1 {
			t.Fatalf("shard %d ClearOperations = %d, want 1", index, shard.Counters.ClearOperations)
		}
	}

	assertClassStateRetainedConsistency(t, snapshot)
}

// TestClassStateClearDerivedRetainedUsageIsZeroWithoutConcurrentMutation
// verifies class-level current retained usage after a non-concurrent clear.
func TestClassStateClearDerivedRetainedUsageIsZeroWithoutConcurrentMutation(t *testing.T) {
	t.Parallel()

	class := NewSizeClass(ClassID(1), ClassSizeFromSize(KiB))
	state := newClassState(class, 2, 4)
	state.updateBudget(4 * KiB)

	for shardIndex := 0; shardIndex < 2; shardIndex++ {
		result := state.tryRetain(shardIndex, make([]byte, 0, 1024))
		if !result.Retained() {
			t.Fatalf("tryRetain(shard=%d) Retained() = false, decision=%s", shardIndex, result.CreditDecision())
		}
	}

	_ = state.clear()

	snapshot := state.state()
	if snapshot.CurrentRetainedBuffers != 0 {
		t.Fatalf("CurrentRetainedBuffers = %d, want 0", snapshot.CurrentRetainedBuffers)
	}
	if snapshot.CurrentRetainedBytes != 0 {
		t.Fatalf("CurrentRetainedBytes = %d, want 0", snapshot.CurrentRetainedBytes)
	}
	if !snapshot.RetainedUsage().IsZero() {
		t.Fatalf("RetainedUsage() = %+v, want zero", snapshot.RetainedUsage())
	}

	assertClassStateRetainedConsistency(t, snapshot)
}

// TestClassStateClearDoesNotDisableBudgetOrShardCredit verifies that physical
// cleanup and credit publication remain separate operations.
func TestClassStateClearDoesNotDisableBudgetOrShardCredit(t *testing.T) {
	t.Parallel()

	class := NewSizeClass(ClassID(1), ClassSizeFromSize(KiB))
	state := newClassState(class, 2, 4)
	generation := state.updateBudget(4 * KiB)

	for shardIndex := 0; shardIndex < 2; shardIndex++ {
		result := state.tryRetain(shardIndex, make([]byte, 0, 1024))
		if !result.Retained() {
			t.Fatalf("tryRetain(shard=%d) Retained() = false, decision=%s", shardIndex, result.CreditDecision())
		}
	}

	result := state.clear()
	if result.RemovedBuffers != 2 {
		t.Fatalf("RemovedBuffers = %d, want 2", result.RemovedBuffers)
	}
	if result.RemovedBytes != 2048 {
		t.Fatalf("RemovedBytes = %d, want 2048", result.RemovedBytes)
	}

	snapshot := state.state()
	if snapshot.Budget.Generation != generation {
		t.Fatalf("Budget.Generation = %v, want %v", snapshot.Budget.Generation, generation)
	}
	if !snapshot.Budget.IsEffective() {
		t.Fatalf("Budget = %+v, want effective", snapshot.Budget)
	}

	for index, shard := range snapshot.Shards {
		if !shard.Credit.IsEnabled() {
			t.Fatalf("shard %d Credit.IsEnabled() = false, want true", index)
		}
		if shard.Credit.TargetBuffers != 2 {
			t.Fatalf("shard %d Credit.TargetBuffers = %d, want 2", index, shard.Credit.TargetBuffers)
		}
		if shard.Credit.TargetBytes != uint64(2*KiB) {
			t.Fatalf("shard %d Credit.TargetBytes = %d, want %d", index, shard.Credit.TargetBytes, uint64(2*KiB))
		}
	}

	assertClassStateRetainedConsistency(t, snapshot)
}

// TestClassStateConcurrentRetainDoesNotExceedShardCredit verifies strict
// shard-local retain under concurrent puts.
func TestClassStateConcurrentRetainDoesNotExceedShardCredit(t *testing.T) {
	t.Parallel()

	const attempts = 64

	class := NewSizeClass(ClassID(1), ClassSizeFromSize(KiB))
	state := newClassState(class, 1, attempts)
	state.updateBudget(KiB)

	start := make(chan struct{})
	var wg sync.WaitGroup
	var retained atomic.Uint64

	wg.Add(attempts)
	for i := 0; i < attempts; i++ {
		go func() {
			defer wg.Done()

			<-start

			result := state.tryRetain(0, make([]byte, 0, 1024))
			if result.Retained() {
				retained.Add(1)
			}
		}()
	}

	close(start)
	wg.Wait()

	snapshot := state.state()
	retainedCount := retained.Load()

	if retainedCount > 1 {
		t.Fatalf("retained attempts = %d, want <= 1", retainedCount)
	}

	if snapshot.CurrentRetainedBuffers > 1 {
		t.Fatalf("class CurrentRetainedBuffers = %d, want <= 1", snapshot.CurrentRetainedBuffers)
	}

	if snapshot.CurrentRetainedBytes > uint64(KiB) {
		t.Fatalf("class CurrentRetainedBytes = %d, want <= %d", snapshot.CurrentRetainedBytes, uint64(KiB))
	}

	if snapshot.Shards[0].Counters.CurrentRetainedBuffers > 1 {
		t.Fatalf("shard CurrentRetainedBuffers = %d, want <= 1", snapshot.Shards[0].Counters.CurrentRetainedBuffers)
	}

	if snapshot.Shards[0].Counters.CurrentRetainedBytes > uint64(KiB) {
		t.Fatalf("shard CurrentRetainedBytes = %d, want <= %d", snapshot.Shards[0].Counters.CurrentRetainedBytes, uint64(KiB))
	}

	if snapshot.Counters.Puts != attempts {
		t.Fatalf("class Puts = %d, want %d", snapshot.Counters.Puts, attempts)
	}

	if snapshot.Counters.Retains != retainedCount {
		t.Fatalf("class Retains = %d, want %d", snapshot.Counters.Retains, retainedCount)
	}

	wantDrops := uint64(attempts) - retainedCount
	if snapshot.Counters.Drops != wantDrops {
		t.Fatalf("class Drops = %d, want %d", snapshot.Counters.Drops, wantDrops)
	}

	if snapshot.Shards[0].Counters.Puts != attempts {
		t.Fatalf("shard Puts = %d, want %d", snapshot.Shards[0].Counters.Puts, attempts)
	}

	if snapshot.Shards[0].Counters.Retains != retainedCount {
		t.Fatalf("shard Retains = %d, want %d", snapshot.Shards[0].Counters.Retains, retainedCount)
	}

	if snapshot.Shards[0].Counters.Drops != wantDrops {
		t.Fatalf("shard Drops = %d, want %d", snapshot.Shards[0].Counters.Drops, wantDrops)
	}

	assertClassStateRetainedConsistency(t, snapshot)
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
// shard current retained state.
func TestClassStateSnapshotRetainedUsage(t *testing.T) {
	t.Parallel()

	snapshot := classStateSnapshot{
		CurrentRetainedBuffers: 3,
		CurrentRetainedBytes:   4096,
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
			},
			want: false,
		},
		{
			name: "zero ineffective budget with retained usage",
			snapshot: classStateSnapshot{
				Budget: classBudgetSnapshot{
					ClassSize: ClassSizeFromSize(KiB),
				},
				CurrentRetainedBuffers: 1,
				CurrentRetainedBytes:   1024,
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
				CurrentRetainedBuffers: 1,
				CurrentRetainedBytes:   1024,
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
				CurrentRetainedBuffers: 2,
				CurrentRetainedBytes:   2048,
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
				CurrentRetainedBuffers: 5,
				CurrentRetainedBytes:   2048,
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
				CurrentRetainedBuffers: 2,
				CurrentRetainedBytes:   uint64(8 * KiB),
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

// TestStaticRuntimeCoreIntegration exercises the current internal static runtime
// path without relying on any public pool layer.
func TestStaticRuntimeCoreIntegration(t *testing.T) {
	t.Parallel()

	table := newClassTable([]ClassSize{
		ClassSizeFromBytes(512),
		ClassSizeFromSize(KiB),
		ClassSizeFromSize(2 * KiB),
	})

	requestClass, ok := table.classForRequest(SizeFromInt(700))
	if !ok {
		t.Fatal("classForRequest(700 B) ok = false, want true")
	}
	if requestClass.Size() != ClassSizeFromSize(KiB) {
		t.Fatalf("classForRequest(700 B) = %s, want 1 KiB class", requestClass)
	}

	capacityClass, ok := table.classForCapacity(SizeFromInt(700))
	if !ok {
		t.Fatal("classForCapacity(700 B) ok = false, want true")
	}
	if capacityClass.Size() != ClassSizeFromBytes(512) {
		t.Fatalf("classForCapacity(700 B) = %s, want 512 B class", capacityClass)
	}

	capacityState := newClassState(capacityClass, 1, 2)
	capacityState.updateBudget(SizeFromBytes(1024))
	capacityRetain := capacityState.tryRetain(0, make([]byte, 0, 700))
	if !capacityRetain.Retained() {
		t.Fatalf("512 B retain Retained() = false, decision=%s", capacityRetain.CreditDecision())
	}
	assertClassStateRetainedConsistency(t, capacityState.state())

	state := newClassState(requestClass, 1, 3)
	state.updateBudget(2 * KiB)

	retain := state.tryRetain(0, make([]byte, 7, 1024))
	if !retain.Retained() {
		t.Fatalf("initial retain Retained() = false, decision=%s", retain.CreditDecision())
	}
	assertClassStateRetainedConsistency(t, state.state())

	get := state.tryGet(0, SizeFromInt(700))
	if !get.Hit() {
		t.Fatal("tryGet(700 B) Hit() = false, want true")
	}
	if len(get.Buffer()) != 0 {
		t.Fatalf("len(Buffer()) = %d, want 0", len(get.Buffer()))
	}
	if cap(get.Buffer()) < requestClass.Int() {
		t.Fatalf("cap(Buffer()) = %d, want at least %d", cap(get.Buffer()), requestClass.Int())
	}
	if get.Capacity() != 1024 {
		t.Fatalf("tryGet(700 B) Capacity() = %d, want 1024", get.Capacity())
	}
	assertClassStateRetainedConsistency(t, state.state())

	miss := state.tryGet(0, SizeFromInt(700))
	if !miss.Miss() {
		t.Fatal("second tryGet(700 B) Miss() = false, want true")
	}

	state.recordAllocation(0, uint64(KiB))
	afterAllocation := state.state()
	if afterAllocation.Counters.Allocations != 1 {
		t.Fatalf("class Allocations after recordAllocation = %d, want 1", afterAllocation.Counters.Allocations)
	}
	if afterAllocation.Shards[0].Counters.Allocations != 1 {
		t.Fatalf("shard Allocations after recordAllocation = %d, want 1", afterAllocation.Shards[0].Counters.Allocations)
	}

	beforeClassReject := state.state()
	classReject := state.tryRetain(0, make([]byte, 0, 700))
	if !classReject.RejectedByClass() {
		t.Fatal("700 B retain into 1 KiB class RejectedByClass() = false, want true")
	}
	afterClassReject := state.state()
	if afterClassReject.Shards[0].Counters != beforeClassReject.Shards[0].Counters {
		t.Fatalf("class reject changed shard counters: before=%+v after=%+v", beforeClassReject.Shards[0].Counters, afterClassReject.Shards[0].Counters)
	}
	if afterClassReject.Shards[0].Bucket != beforeClassReject.Shards[0].Bucket {
		t.Fatalf("class reject changed bucket state: before=%+v after=%+v", beforeClassReject.Shards[0].Bucket, afterClassReject.Shards[0].Bucket)
	}

	firstCreditBuffer := state.tryRetain(0, make([]byte, 0, 1024))
	if !firstCreditBuffer.Retained() {
		t.Fatalf("first credit buffer Retained() = false, decision=%s", firstCreditBuffer.CreditDecision())
	}

	secondCreditBuffer := state.tryRetain(0, make([]byte, 0, 1024))
	if !secondCreditBuffer.Retained() {
		t.Fatalf("second credit buffer Retained() = false, decision=%s", secondCreditBuffer.CreditDecision())
	}

	creditReject := state.tryRetain(0, make([]byte, 0, 1024))
	if !creditReject.RejectedByCredit() {
		t.Fatalf("credit reject RejectedByCredit() = false, decision=%s", creditReject.CreditDecision())
	}
	assertClassStateRetainedConsistency(t, state.state())

	trim := state.trimShard(0, 1)
	if trim.RemovedBuffers != 1 {
		t.Fatalf("trim RemovedBuffers = %d, want 1", trim.RemovedBuffers)
	}
	if trim.RemovedBytes != 1024 {
		t.Fatalf("trim RemovedBytes = %d, want 1024", trim.RemovedBytes)
	}
	assertClassStateRetainedConsistency(t, state.state())

	clear := state.clear()
	if clear.RemovedBuffers != 1 {
		t.Fatalf("clear RemovedBuffers = %d, want 1", clear.RemovedBuffers)
	}
	if clear.RemovedBytes != 1024 {
		t.Fatalf("clear RemovedBytes = %d, want 1024", clear.RemovedBytes)
	}
	assertClassStateRetainedConsistency(t, state.state())

	clearState := newClassState(requestClass, 2, 2)
	clearState.updateBudget(4 * KiB)
	for shardIndex := 0; shardIndex < 2; shardIndex++ {
		result := clearState.tryRetain(shardIndex, make([]byte, 0, 1024))
		if !result.Retained() {
			t.Fatalf("clear setup retain shard=%d Retained() = false, decision=%s", shardIndex, result.CreditDecision())
		}
	}
	clearAcrossShards := clearState.clear()
	if clearAcrossShards.RemovedBuffers != 2 {
		t.Fatalf("multi-shard clear RemovedBuffers = %d, want 2", clearAcrossShards.RemovedBuffers)
	}
	if clearAcrossShards.RemovedBytes != 2048 {
		t.Fatalf("multi-shard clear RemovedBytes = %d, want 2048", clearAcrossShards.RemovedBytes)
	}
	clearSnapshot := clearState.state()
	if clearSnapshot.CurrentRetainedBuffers != 0 {
		t.Fatalf("multi-shard clear CurrentRetainedBuffers = %d, want 0", clearSnapshot.CurrentRetainedBuffers)
	}
	assertClassStateRetainedConsistency(t, clearSnapshot)

	bucketFullState := newClassState(requestClass, 1, 1)
	bucketFullState.updateBudget(2 * KiB)
	if result := bucketFullState.tryRetain(0, make([]byte, 0, 1024)); !result.Retained() {
		t.Fatalf("bucket setup retain Retained() = false, decision=%s", result.CreditDecision())
	}
	bucketReject := bucketFullState.tryRetain(0, make([]byte, 0, 1024))
	if !bucketReject.RejectedByBucket() {
		t.Fatalf("bucket reject RejectedByBucket() = false, decision=%s", bucketReject.CreditDecision())
	}
	assertClassStateRetainedConsistency(t, bucketFullState.state())

	overBudgetState := newClassState(requestClass, 1, 1)
	overBudgetState.updateBudget(KiB)
	if result := overBudgetState.tryRetain(0, make([]byte, 0, 1024)); !result.Retained() {
		t.Fatalf("over-budget setup retain Retained() = false, decision=%s", result.CreditDecision())
	}
	overBudgetState.disableBudget()
	overBudgetSnapshot := overBudgetState.state()
	if overBudgetSnapshot.CurrentRetainedBuffers != 1 {
		t.Fatalf("CurrentRetainedBuffers after disableBudget = %d, want 1", overBudgetSnapshot.CurrentRetainedBuffers)
	}
	if !overBudgetSnapshot.IsOverBudget() {
		t.Fatal("IsOverBudget() after disabling budget with retained usage = false, want true")
	}
	assertClassStateRetainedConsistency(t, overBudgetSnapshot)

	snapshot := state.state()
	if snapshot.Counters.Gets != 2 {
		t.Fatalf("class Gets = %d, want 2", snapshot.Counters.Gets)
	}
	if snapshot.Counters.Hits != 1 {
		t.Fatalf("class Hits = %d, want 1", snapshot.Counters.Hits)
	}
	if snapshot.Counters.Misses != 1 {
		t.Fatalf("class Misses = %d, want 1", snapshot.Counters.Misses)
	}
	if snapshot.Counters.Allocations != 1 {
		t.Fatalf("class Allocations = %d, want 1", snapshot.Counters.Allocations)
	}
	if snapshot.Counters.Puts != 5 {
		t.Fatalf("class Puts = %d, want 5", snapshot.Counters.Puts)
	}
	if snapshot.Counters.Retains != 3 {
		t.Fatalf("class Retains = %d, want 3", snapshot.Counters.Retains)
	}
	if snapshot.Counters.Drops != 2 {
		t.Fatalf("class Drops = %d, want 2", snapshot.Counters.Drops)
	}
	if snapshot.Counters.TrimOperations != 1 {
		t.Fatalf("class TrimOperations = %d, want 1", snapshot.Counters.TrimOperations)
	}
	if snapshot.Counters.ClearOperations != 1 {
		t.Fatalf("class ClearOperations = %d, want 1", snapshot.Counters.ClearOperations)
	}
	if snapshot.CurrentRetainedBuffers != 0 {
		t.Fatalf("CurrentRetainedBuffers = %d, want 0", snapshot.CurrentRetainedBuffers)
	}
	if snapshot.CurrentRetainedBytes != 0 {
		t.Fatalf("CurrentRetainedBytes = %d, want 0", snapshot.CurrentRetainedBytes)
	}

	assertClassStateRetainedConsistency(t, snapshot)
}

// TestClassStateConcurrentRetainAndGet verifies class-state routing under
// concurrent retain/get operations across shards.
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

	if afterRetain.CurrentRetainedBuffers != uint64(total) {
		t.Fatalf("class CurrentRetainedBuffers after retain = %d, want %d", afterRetain.CurrentRetainedBuffers, total)
	}

	assertClassStateRetainedConsistency(t, afterRetain)

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

	if afterGet.CurrentRetainedBuffers != 0 {
		t.Fatalf("class CurrentRetainedBuffers after get = %d, want 0", afterGet.CurrentRetainedBuffers)
	}

	if afterGet.CurrentRetainedBytes != 0 {
		t.Fatalf("class CurrentRetainedBytes after get = %d, want 0", afterGet.CurrentRetainedBytes)
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

	assertClassStateRetainedConsistency(t, afterGet)
}

func assertClassStateClassRejectedRetain(
	t *testing.T,
	snapshot classStateSnapshot,
	result classRetainResult,
	wantDecision classRetainDecision,
	wantCapacity uint64,
) {
	t.Helper()

	if result.Retained() {
		t.Fatal("Retained() = true, want false")
	}

	if !result.Dropped() {
		t.Fatal("Dropped() = false, want true")
	}

	if !result.RejectedByClass() {
		t.Fatal("RejectedByClass() = false, want true")
	}

	if result.RejectedByCredit() {
		t.Fatal("RejectedByCredit() = true, want false")
	}

	if result.RejectedByBucket() {
		t.Fatal("RejectedByBucket() = true, want false")
	}

	if result.ClassRetainDecision() != wantDecision {
		t.Fatalf("ClassRetainDecision() = %s, want %s", result.ClassRetainDecision(), wantDecision)
	}

	if result.CreditDecision() != shardCreditNotEvaluated {
		t.Fatalf("CreditDecision() = %s, want %s", result.CreditDecision(), shardCreditNotEvaluated)
	}

	if result.Capacity() != wantCapacity {
		t.Fatalf("Capacity() = %d, want %d", result.Capacity(), wantCapacity)
	}

	if snapshot.Counters.Puts != 1 {
		t.Fatalf("class Puts = %d, want 1", snapshot.Counters.Puts)
	}

	if snapshot.Counters.ReturnedBytes != wantCapacity {
		t.Fatalf("class ReturnedBytes = %d, want %d", snapshot.Counters.ReturnedBytes, wantCapacity)
	}

	if snapshot.Counters.Retains != 0 {
		t.Fatalf("class Retains = %d, want 0", snapshot.Counters.Retains)
	}

	if snapshot.Counters.Drops != 1 {
		t.Fatalf("class Drops = %d, want 1", snapshot.Counters.Drops)
	}

	if snapshot.Counters.DroppedBytes != wantCapacity {
		t.Fatalf("class DroppedBytes = %d, want %d", snapshot.Counters.DroppedBytes, wantCapacity)
	}

	if snapshot.CurrentRetainedBuffers != 0 {
		t.Fatalf("class CurrentRetainedBuffers = %d, want 0", snapshot.CurrentRetainedBuffers)
	}

	if snapshot.CurrentRetainedBytes != 0 {
		t.Fatalf("class CurrentRetainedBytes = %d, want 0", snapshot.CurrentRetainedBytes)
	}

	if len(snapshot.Shards) != 1 {
		t.Fatalf("ShardCount = %d, want 1", len(snapshot.Shards))
	}

	shard := snapshot.Shards[0]
	if !shard.Counters.IsZero() {
		t.Fatalf("shard counters = %+v, want zero", shard.Counters)
	}

	if shard.Bucket.RetainedBuffers != 0 {
		t.Fatalf("bucket RetainedBuffers = %d, want 0", shard.Bucket.RetainedBuffers)
	}

	if shard.Bucket.RetainedBytes != 0 {
		t.Fatalf("bucket RetainedBytes = %d, want 0", shard.Bucket.RetainedBytes)
	}

	assertClassStateRetainedConsistency(t, snapshot)
}

func assertClassStateRetainedConsistency(t *testing.T, snapshot classStateSnapshot) {
	t.Helper()

	var shardCounterBuffers uint64
	var shardCounterBytes uint64
	var bucketBuffers uint64
	var bucketBytes uint64

	for index, shard := range snapshot.Shards {
		assertShardStateRetainedConsistency(t, shard)

		shardBucketBuffers := uint64(shard.Bucket.RetainedBuffers)
		if shard.Counters.CurrentRetainedBuffers != shardBucketBuffers {
			t.Fatalf("shard %d counter retained buffers = %d, bucket retained buffers = %d",
				index,
				shard.Counters.CurrentRetainedBuffers,
				shardBucketBuffers,
			)
		}

		if shard.Counters.CurrentRetainedBytes != shard.Bucket.RetainedBytes {
			t.Fatalf("shard %d counter retained bytes = %d, bucket retained bytes = %d",
				index,
				shard.Counters.CurrentRetainedBytes,
				shard.Bucket.RetainedBytes,
			)
		}

		shardCounterBuffers += shard.Counters.CurrentRetainedBuffers
		shardCounterBytes += shard.Counters.CurrentRetainedBytes
		bucketBuffers += shardBucketBuffers
		bucketBytes += shard.Bucket.RetainedBytes
	}

	if snapshot.CurrentRetainedBuffers != shardCounterBuffers {
		t.Fatalf("class retained buffers = %d, sum shard counter retained buffers = %d",
			snapshot.CurrentRetainedBuffers,
			shardCounterBuffers,
		)
	}

	if snapshot.CurrentRetainedBytes != shardCounterBytes {
		t.Fatalf("class retained bytes = %d, sum shard counter retained bytes = %d",
			snapshot.CurrentRetainedBytes,
			shardCounterBytes,
		)
	}

	if snapshot.CurrentRetainedBuffers != bucketBuffers {
		t.Fatalf("class retained buffers = %d, sum bucket retained buffers = %d",
			snapshot.CurrentRetainedBuffers,
			bucketBuffers,
		)
	}

	if snapshot.CurrentRetainedBytes != bucketBytes {
		t.Fatalf("class retained bytes = %d, sum bucket retained bytes = %d",
			snapshot.CurrentRetainedBytes,
			bucketBytes,
		)
	}
}
