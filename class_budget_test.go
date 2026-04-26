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

// TestNewClassBudget verifies construction of a class-local budget owner.
//
// A new class budget has immutable ClassSize and no assigned/effective target.
// The zero effective target is valid: credit is published later by the owning
// class/controller layer.
func TestNewClassBudget(t *testing.T) {
	t.Parallel()

	classSize := ClassSizeFromSize(4 * KiB)

	budget := newClassBudget(classSize)
	snapshot := budget.snapshot()

	if budget.classSize != classSize {
		t.Fatalf("budget.classSize = %s, want %s", budget.classSize, classSize)
	}

	if snapshot.ClassSize != classSize {
		t.Fatalf("snapshot.ClassSize = %s, want %s", snapshot.ClassSize, classSize)
	}

	if !snapshot.IsZero() {
		t.Fatalf("new budget snapshot = %+v, want zero target", snapshot)
	}

	if snapshot.IsEffective() {
		t.Fatal("new budget IsEffective() = true, want false")
	}

	if snapshot.AssignedBytes != 0 {
		t.Fatalf("AssignedBytes = %d, want 0", snapshot.AssignedBytes)
	}

	if snapshot.TargetBuffers != 0 {
		t.Fatalf("TargetBuffers = %d, want 0", snapshot.TargetBuffers)
	}

	if snapshot.TargetBytes != 0 {
		t.Fatalf("TargetBytes = %d, want 0", snapshot.TargetBytes)
	}

	if snapshot.RemainderBytes != 0 {
		t.Fatalf("RemainderBytes = %d, want 0", snapshot.RemainderBytes)
	}
}

// TestNewClassBudgetPanicsForZeroClassSize verifies that budget construction
// requires a concrete normalized class size.
func TestNewClassBudgetPanicsForZeroClassSize(t *testing.T) {
	t.Parallel()

	testutil.MustPanicWithMessage(t, errClassBudgetZeroClassSize, func() {
		_ = newClassBudget(ClassSize(0))
	})
}

// TestNewClassBudgetLimit verifies whole-buffer budget normalization.
//
// Assigned bytes are converted into whole buffers of the class size. Any bytes
// that cannot fund a complete class-sized buffer are kept as remainder bytes.
func TestNewClassBudgetLimit(t *testing.T) {
	t.Parallel()

	classSize := ClassSizeFromSize(4 * KiB)

	tests := []struct {
		name string

		assignedBytes Size

		wantAssigned  uint64
		wantBuffers   uint64
		wantTarget    uint64
		wantRemainder uint64
		wantZero      bool
		wantEffective bool
	}{
		{
			name:          "zero assignment",
			assignedBytes: 0,
			wantAssigned:  0,
			wantBuffers:   0,
			wantTarget:    0,
			wantRemainder: 0,
			wantZero:      true,
			wantEffective: false,
		},
		{
			name:          "smaller than one class buffer",
			assignedBytes: 2 * KiB,
			wantAssigned:  uint64(2 * KiB),
			wantBuffers:   0,
			wantTarget:    0,
			wantRemainder: uint64(2 * KiB),
			wantZero:      false,
			wantEffective: false,
		},
		{
			name:          "exactly one class buffer",
			assignedBytes: 4 * KiB,
			wantAssigned:  uint64(4 * KiB),
			wantBuffers:   1,
			wantTarget:    uint64(4 * KiB),
			wantRemainder: 0,
			wantZero:      false,
			wantEffective: true,
		},
		{
			name:          "unaligned multiple",
			assignedBytes: 10 * KiB,
			wantAssigned:  uint64(10 * KiB),
			wantBuffers:   2,
			wantTarget:    uint64(8 * KiB),
			wantRemainder: uint64(2 * KiB),
			wantZero:      false,
			wantEffective: true,
		},
		{
			name:          "exact larger multiple",
			assignedBytes: 16 * KiB,
			wantAssigned:  uint64(16 * KiB),
			wantBuffers:   4,
			wantTarget:    uint64(16 * KiB),
			wantRemainder: 0,
			wantZero:      false,
			wantEffective: true,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			limit := newClassBudgetLimit(classSize, tt.assignedBytes)

			if limit.ClassSize != classSize {
				t.Fatalf("ClassSize = %s, want %s", limit.ClassSize, classSize)
			}

			if limit.AssignedBytes != tt.wantAssigned {
				t.Fatalf("AssignedBytes = %d, want %d", limit.AssignedBytes, tt.wantAssigned)
			}

			if limit.TargetBuffers != tt.wantBuffers {
				t.Fatalf("TargetBuffers = %d, want %d", limit.TargetBuffers, tt.wantBuffers)
			}

			if limit.TargetBytes != tt.wantTarget {
				t.Fatalf("TargetBytes = %d, want %d", limit.TargetBytes, tt.wantTarget)
			}

			if limit.RemainderBytes != tt.wantRemainder {
				t.Fatalf("RemainderBytes = %d, want %d", limit.RemainderBytes, tt.wantRemainder)
			}

			if got := limit.IsZero(); got != tt.wantZero {
				t.Fatalf("IsZero() = %t, want %t", got, tt.wantZero)
			}

			if got := limit.IsEffective(); got != tt.wantEffective {
				t.Fatalf("IsEffective() = %t, want %t", got, tt.wantEffective)
			}

			limit.validate()
		})
	}
}

// TestNewClassBudgetLimitPanicsForZeroClassSize verifies the construction guard.
func TestNewClassBudgetLimitPanicsForZeroClassSize(t *testing.T) {
	t.Parallel()

	testutil.MustPanicWithMessage(t, errClassBudgetZeroClassSize, func() {
		_ = newClassBudgetLimit(ClassSize(0), 4*KiB)
	})
}

// TestClassBudgetLimitValidatePanicsForInvalidArithmetic verifies internal
// budget-limit invariants.
//
// These cases are constructed manually because newClassBudgetLimit always
// produces valid arithmetic.
func TestClassBudgetLimitValidatePanicsForInvalidArithmetic(t *testing.T) {
	t.Parallel()

	classSize := ClassSizeFromSize(4 * KiB)

	tests := []struct {
		name string

		limit classBudgetLimit
		want  string
	}{
		{
			name: "zero class size",
			limit: classBudgetLimit{
				ClassSize:     0,
				AssignedBytes: 0,
			},
			want: errClassBudgetZeroClassSize,
		},
		{
			name: "target bytes do not match target buffers",
			limit: classBudgetLimit{
				ClassSize:      classSize,
				AssignedBytes:  uint64(8 * KiB),
				TargetBuffers:  2,
				TargetBytes:    uint64(4 * KiB),
				RemainderBytes: uint64(4 * KiB),
			},
			want: errClassBudgetInvalidLimit,
		},
		{
			name: "assigned bytes do not match target plus remainder",
			limit: classBudgetLimit{
				ClassSize:      classSize,
				AssignedBytes:  uint64(9 * KiB),
				TargetBuffers:  2,
				TargetBytes:    uint64(8 * KiB),
				RemainderBytes: 0,
			},
			want: errClassBudgetInvalidLimit,
		},
		{
			name: "remainder reaches class size",
			limit: classBudgetLimit{
				ClassSize:      classSize,
				AssignedBytes:  uint64(12 * KiB),
				TargetBuffers:  2,
				TargetBytes:    uint64(8 * KiB),
				RemainderBytes: uint64(4 * KiB),
			},
			want: errClassBudgetInvalidLimit,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			testutil.MustPanicWithMessage(t, tt.want, func() {
				tt.limit.validate()
			})
		})
	}
}

// TestClassBudgetLimitLimit verifies conversion to a single-shard credit limit.
//
// This helper is useful when an owner treats the class budget as one shard.
// Most class owners should use shardCreditPlan instead.
func TestClassBudgetLimitLimit(t *testing.T) {
	t.Parallel()

	classSize := ClassSizeFromSize(4 * KiB)

	t.Run("ineffective limit returns disabled shard credit", func(t *testing.T) {
		t.Parallel()

		limit := newClassBudgetLimit(classSize, 2*KiB)
		credit := limit.Limit()

		if !credit.IsZero() {
			t.Fatalf("Limit() = %+v, want disabled credit", credit)
		}
	})

	t.Run("effective limit returns matching shard credit", func(t *testing.T) {
		t.Parallel()

		limit := newClassBudgetLimit(classSize, 10*KiB)
		credit := limit.Limit()

		if credit.TargetBuffers != 2 {
			t.Fatalf("TargetBuffers = %d, want 2", credit.TargetBuffers)
		}

		if credit.TargetBytes != uint64(8*KiB) {
			t.Fatalf("TargetBytes = %d, want %d", credit.TargetBytes, uint64(8*KiB))
		}

		if !credit.IsEnabled() {
			t.Fatal("credit IsEnabled() = false, want true")
		}
	})
}

// TestClassBudgetUpdateAssignedBytes verifies applying a raw assigned byte target
// to a class budget owner.
func TestClassBudgetUpdateAssignedBytes(t *testing.T) {
	t.Parallel()

	classSize := ClassSizeFromSize(4 * KiB)
	budget := newClassBudget(classSize)

	before := budget.snapshot()
	generation := budget.updateAssignedBytes(10 * KiB)
	after := budget.snapshot()

	if after.Generation != generation {
		t.Fatalf("snapshot Generation = %v, want returned generation %v", after.Generation, generation)
	}

	if after.Generation == before.Generation {
		t.Fatalf("Generation did not advance: before=%v after=%v", before.Generation, after.Generation)
	}

	if after.ClassSize != classSize {
		t.Fatalf("ClassSize = %s, want %s", after.ClassSize, classSize)
	}

	if after.AssignedBytes != uint64(10*KiB) {
		t.Fatalf("AssignedBytes = %d, want %d", after.AssignedBytes, uint64(10*KiB))
	}

	if after.TargetBuffers != 2 {
		t.Fatalf("TargetBuffers = %d, want 2", after.TargetBuffers)
	}

	if after.TargetBytes != uint64(8*KiB) {
		t.Fatalf("TargetBytes = %d, want %d", after.TargetBytes, uint64(8*KiB))
	}

	if after.RemainderBytes != uint64(2*KiB) {
		t.Fatalf("RemainderBytes = %d, want %d", after.RemainderBytes, uint64(2*KiB))
	}

	if after.IsZero() {
		t.Fatal("IsZero() = true, want false")
	}

	if !after.IsEffective() {
		t.Fatal("IsEffective() = false, want true")
	}
}

// TestClassBudgetUpdate verifies applying a precomputed limit.
func TestClassBudgetUpdate(t *testing.T) {
	t.Parallel()

	classSize := ClassSizeFromSize(2 * KiB)
	budget := newClassBudget(classSize)

	limit := newClassBudgetLimit(classSize, 9*KiB)

	generation := budget.update(limit)
	snapshot := budget.snapshot()

	if snapshot.Generation != generation {
		t.Fatalf("Generation = %v, want %v", snapshot.Generation, generation)
	}

	if snapshot.Limit() != limit {
		t.Fatalf("snapshot.Limit() = %+v, want %+v", snapshot.Limit(), limit)
	}
}

// TestClassBudgetUpdatePanicsForClassSizeMismatch verifies that a class budget
// cannot accept a limit computed for another class size.
func TestClassBudgetUpdatePanicsForClassSizeMismatch(t *testing.T) {
	t.Parallel()

	budget := newClassBudget(ClassSizeFromSize(4 * KiB))
	limit := newClassBudgetLimit(ClassSizeFromSize(8*KiB), 32*KiB)

	testutil.MustPanicWithMessage(t, errClassBudgetClassSizeMismatch, func() {
		_ = budget.update(limit)
	})
}

// TestClassBudgetUpdatePanicsForZeroOwnerClassSize verifies defensive behavior
// for a manually corrupted budget owner.
func TestClassBudgetUpdatePanicsForZeroOwnerClassSize(t *testing.T) {
	t.Parallel()

	var budget classBudget

	limit := newClassBudgetLimit(ClassSizeFromSize(4*KiB), 8*KiB)

	testutil.MustPanicWithMessage(t, errClassBudgetZeroClassSize, func() {
		_ = budget.update(limit)
	})
}

// TestClassBudgetDisable verifies disabling the effective class budget.
//
// Disabling the class budget does not physically remove retained buffers. It only
// publishes a zero target for future shard-credit planning.
func TestClassBudgetDisable(t *testing.T) {
	t.Parallel()

	budget := newClassBudget(ClassSizeFromSize(4 * KiB))

	firstGeneration := budget.updateAssignedBytes(16 * KiB)
	disabledGeneration := budget.disable()

	if disabledGeneration == firstGeneration {
		t.Fatalf("disable generation = %v, want value after %v", disabledGeneration, firstGeneration)
	}

	snapshot := budget.snapshot()

	if snapshot.Generation != disabledGeneration {
		t.Fatalf("Generation = %v, want %v", snapshot.Generation, disabledGeneration)
	}

	if !snapshot.IsZero() {
		t.Fatalf("disabled snapshot = %+v, want zero target", snapshot)
	}

	if snapshot.IsEffective() {
		t.Fatal("disabled snapshot IsEffective() = true, want false")
	}

	if snapshot.ClassSize != ClassSizeFromSize(4*KiB) {
		t.Fatalf("ClassSize after disable = %s, want 4 KiB", snapshot.ClassSize)
	}
}

// TestClassBudgetSnapshotLimit verifies conversion from snapshot to limit.
func TestClassBudgetSnapshotLimit(t *testing.T) {
	t.Parallel()

	snapshot := classBudgetSnapshot{
		Generation:     Generation(7),
		ClassSize:      ClassSizeFromSize(4 * KiB),
		AssignedBytes:  uint64(10 * KiB),
		TargetBuffers:  2,
		TargetBytes:    uint64(8 * KiB),
		RemainderBytes: uint64(2 * KiB),
	}

	limit := snapshot.Limit()

	if limit.ClassSize != snapshot.ClassSize {
		t.Fatalf("Limit().ClassSize = %s, want %s", limit.ClassSize, snapshot.ClassSize)
	}

	if limit.AssignedBytes != snapshot.AssignedBytes {
		t.Fatalf("Limit().AssignedBytes = %d, want %d", limit.AssignedBytes, snapshot.AssignedBytes)
	}

	if limit.TargetBuffers != snapshot.TargetBuffers {
		t.Fatalf("Limit().TargetBuffers = %d, want %d", limit.TargetBuffers, snapshot.TargetBuffers)
	}

	if limit.TargetBytes != snapshot.TargetBytes {
		t.Fatalf("Limit().TargetBytes = %d, want %d", limit.TargetBytes, snapshot.TargetBytes)
	}

	if limit.RemainderBytes != snapshot.RemainderBytes {
		t.Fatalf("Limit().RemainderBytes = %d, want %d", limit.RemainderBytes, snapshot.RemainderBytes)
	}
}

// TestClassBudgetSnapshotPredicates verifies snapshot zero/effective semantics.
func TestClassBudgetSnapshotPredicates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string

		snapshot classBudgetSnapshot

		wantZero      bool
		wantEffective bool
	}{
		{
			name: "zero target",
			snapshot: classBudgetSnapshot{
				ClassSize: ClassSizeFromSize(4 * KiB),
			},
			wantZero:      true,
			wantEffective: false,
		},
		{
			name: "assigned but ineffective target",
			snapshot: classBudgetSnapshot{
				ClassSize:      ClassSizeFromSize(4 * KiB),
				AssignedBytes:  uint64(2 * KiB),
				RemainderBytes: uint64(2 * KiB),
			},
			wantZero:      false,
			wantEffective: false,
		},
		{
			name: "effective target",
			snapshot: classBudgetSnapshot{
				ClassSize:      ClassSizeFromSize(4 * KiB),
				AssignedBytes:  uint64(10 * KiB),
				TargetBuffers:  2,
				TargetBytes:    uint64(8 * KiB),
				RemainderBytes: uint64(2 * KiB),
			},
			wantZero:      false,
			wantEffective: true,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.snapshot.IsZero(); got != tt.wantZero {
				t.Fatalf("IsZero() = %t, want %t", got, tt.wantZero)
			}

			if got := tt.snapshot.IsEffective(); got != tt.wantEffective {
				t.Fatalf("IsEffective() = %t, want %t", got, tt.wantEffective)
			}
		})
	}
}

// TestClassShardCreditPlanEvenDistribution verifies even distribution of class
// buffer credit across shards.
func TestClassShardCreditPlanEvenDistribution(t *testing.T) {
	t.Parallel()

	limit := newClassBudgetLimit(ClassSizeFromSize(4*KiB), 32*KiB)
	plan := newClassShardCreditPlan(limit, 4)

	if plan.ClassSize != ClassSizeFromSize(4*KiB) {
		t.Fatalf("ClassSize = %s, want 4 KiB", plan.ClassSize)
	}

	if plan.ShardCount != 4 {
		t.Fatalf("ShardCount = %d, want 4", plan.ShardCount)
	}

	if plan.TargetBuffers != 8 {
		t.Fatalf("TargetBuffers = %d, want 8", plan.TargetBuffers)
	}

	if plan.TargetBytes != uint64(32*KiB) {
		t.Fatalf("TargetBytes = %d, want %d", plan.TargetBytes, uint64(32*KiB))
	}

	if plan.BaseBuffersPerShard != 2 {
		t.Fatalf("BaseBuffersPerShard = %d, want 2", plan.BaseBuffersPerShard)
	}

	if plan.ExtraShards != 0 {
		t.Fatalf("ExtraShards = %d, want 0", plan.ExtraShards)
	}

	for index := 0; index < plan.ShardCount; index++ {
		credit := plan.creditForShard(index)

		if credit.TargetBuffers != 2 {
			t.Fatalf("shard %d TargetBuffers = %d, want 2", index, credit.TargetBuffers)
		}

		if credit.TargetBytes != uint64(8*KiB) {
			t.Fatalf("shard %d TargetBytes = %d, want %d", index, credit.TargetBytes, uint64(8*KiB))
		}
	}

	total := plan.totalCredit()

	if total.Buffers != 8 {
		t.Fatalf("total Buffers = %d, want 8", total.Buffers)
	}

	if total.Bytes != uint64(32*KiB) {
		t.Fatalf("total Bytes = %d, want %d", total.Bytes, uint64(32*KiB))
	}
}

// TestClassShardCreditPlanUnevenDistribution verifies deterministic remainder
// distribution.
//
// The first ExtraShards shards receive one additional buffer.
func TestClassShardCreditPlanUnevenDistribution(t *testing.T) {
	t.Parallel()

	limit := newClassBudgetLimit(ClassSizeFromSize(4*KiB), 40*KiB)
	plan := newClassShardCreditPlan(limit, 4)

	if plan.TargetBuffers != 10 {
		t.Fatalf("TargetBuffers = %d, want 10", plan.TargetBuffers)
	}

	if plan.BaseBuffersPerShard != 2 {
		t.Fatalf("BaseBuffersPerShard = %d, want 2", plan.BaseBuffersPerShard)
	}

	if plan.ExtraShards != 2 {
		t.Fatalf("ExtraShards = %d, want 2", plan.ExtraShards)
	}

	wantBuffers := []uint64{3, 3, 2, 2}
	wantBytes := []uint64{
		uint64(12 * KiB),
		uint64(12 * KiB),
		uint64(8 * KiB),
		uint64(8 * KiB),
	}

	for index := 0; index < plan.ShardCount; index++ {
		credit := plan.creditForShard(index)

		if credit.TargetBuffers != wantBuffers[index] {
			t.Fatalf("shard %d TargetBuffers = %d, want %d", index, credit.TargetBuffers, wantBuffers[index])
		}

		if credit.TargetBytes != wantBytes[index] {
			t.Fatalf("shard %d TargetBytes = %d, want %d", index, credit.TargetBytes, wantBytes[index])
		}
	}

	total := plan.totalCredit()

	if total.Buffers != 10 {
		t.Fatalf("total Buffers = %d, want 10", total.Buffers)
	}

	if total.Bytes != uint64(40*KiB) {
		t.Fatalf("total Bytes = %d, want %d", total.Bytes, uint64(40*KiB))
	}
}

// TestClassShardCreditPlanFewerBuffersThanShards verifies that shards without
// funded buffers receive disabled credit.
func TestClassShardCreditPlanFewerBuffersThanShards(t *testing.T) {
	t.Parallel()

	limit := newClassBudgetLimit(ClassSizeFromSize(4*KiB), 8*KiB)
	plan := newClassShardCreditPlan(limit, 4)

	if plan.TargetBuffers != 2 {
		t.Fatalf("TargetBuffers = %d, want 2", plan.TargetBuffers)
	}

	if plan.BaseBuffersPerShard != 0 {
		t.Fatalf("BaseBuffersPerShard = %d, want 0", plan.BaseBuffersPerShard)
	}

	if plan.ExtraShards != 2 {
		t.Fatalf("ExtraShards = %d, want 2", plan.ExtraShards)
	}

	for index := 0; index < plan.ShardCount; index++ {
		credit := plan.creditForShard(index)

		if index < 2 {
			if credit.TargetBuffers != 1 {
				t.Fatalf("shard %d TargetBuffers = %d, want 1", index, credit.TargetBuffers)
			}

			if credit.TargetBytes != uint64(4*KiB) {
				t.Fatalf("shard %d TargetBytes = %d, want %d", index, credit.TargetBytes, uint64(4*KiB))
			}

			continue
		}

		if !credit.IsZero() {
			t.Fatalf("shard %d credit = %+v, want disabled zero credit", index, credit)
		}
	}

	total := plan.totalCredit()

	if total.Buffers != 2 {
		t.Fatalf("total Buffers = %d, want 2", total.Buffers)
	}

	if total.Bytes != uint64(8*KiB) {
		t.Fatalf("total Bytes = %d, want %d", total.Bytes, uint64(8*KiB))
	}
}

// TestClassShardCreditPlanZeroEffectiveTarget verifies that ineffective class
// budget produces disabled shard credits.
func TestClassShardCreditPlanZeroEffectiveTarget(t *testing.T) {
	t.Parallel()

	limit := newClassBudgetLimit(ClassSizeFromSize(4*KiB), 2*KiB)
	plan := newClassShardCreditPlan(limit, 4)

	if !plan.IsZero() {
		t.Fatalf("plan IsZero() = false, plan=%+v", plan)
	}

	for index := 0; index < plan.ShardCount; index++ {
		credit := plan.creditForShard(index)
		if !credit.IsZero() {
			t.Fatalf("shard %d credit = %+v, want disabled zero credit", index, credit)
		}
	}

	total := plan.totalCredit()
	if !total.IsZero() {
		t.Fatalf("total = %+v, want zero", total)
	}
}

// TestClassShardCreditPlanPanicsForInvalidShardCount verifies shard-count guard.
func TestClassShardCreditPlanPanicsForInvalidShardCount(t *testing.T) {
	t.Parallel()

	limit := newClassBudgetLimit(ClassSizeFromSize(4*KiB), 16*KiB)

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

			testutil.MustPanicWithMessage(t, errClassBudgetShardCountInvalid, func() {
				_ = newClassShardCreditPlan(limit, tt.shardCount)
			})
		})
	}
}

// TestClassShardCreditPlanCreditForShardPanicsForInvalidIndex verifies shard
// index validation.
func TestClassShardCreditPlanCreditForShardPanicsForInvalidIndex(t *testing.T) {
	t.Parallel()

	limit := newClassBudgetLimit(ClassSizeFromSize(4*KiB), 16*KiB)
	plan := newClassShardCreditPlan(limit, 4)

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
			shardIndex: 4,
		},
		{
			name:       "above shard count",
			shardIndex: 5,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			testutil.MustPanicWithMessage(t, errClassBudgetShardIndexOutOfRange, func() {
				_ = plan.creditForShard(tt.shardIndex)
			})
		})
	}
}

// TestClassShardCreditPlanCreditsReturnsMutableCopy verifies that credits()
// returns a new caller-owned slice.
func TestClassShardCreditPlanCreditsReturnsMutableCopy(t *testing.T) {
	t.Parallel()

	limit := newClassBudgetLimit(ClassSizeFromSize(4*KiB), 16*KiB)
	plan := newClassShardCreditPlan(limit, 4)

	first := plan.credits()
	if len(first) != 4 {
		t.Fatalf("len(credits()) = %d, want 4", len(first))
	}

	first[0] = shardCreditLimit{}

	second := plan.credits()

	if second[0].TargetBuffers != 1 {
		t.Fatalf("second credits()[0].TargetBuffers = %d, want 1", second[0].TargetBuffers)
	}

	if second[0].TargetBytes != uint64(4*KiB) {
		t.Fatalf("second credits()[0].TargetBytes = %d, want %d", second[0].TargetBytes, uint64(4*KiB))
	}
}

// TestClassBudgetShardCreditPlanFromSnapshotAndOwner verifies plan helpers on
// both classBudget and classBudgetSnapshot.
func TestClassBudgetShardCreditPlanFromSnapshotAndOwner(t *testing.T) {
	t.Parallel()

	budget := newClassBudget(ClassSizeFromSize(4 * KiB))
	budget.updateAssignedBytes(40 * KiB)

	ownerPlan := budget.shardCreditPlan(4)
	snapshotPlan := budget.snapshot().shardCreditPlan(4)

	if ownerPlan != snapshotPlan {
		t.Fatalf("owner shardCreditPlan = %+v, snapshot shardCreditPlan = %+v", ownerPlan, snapshotPlan)
	}

	if ownerPlan.TargetBuffers != 10 {
		t.Fatalf("TargetBuffers = %d, want 10", ownerPlan.TargetBuffers)
	}

	if ownerPlan.ExtraShards != 2 {
		t.Fatalf("ExtraShards = %d, want 2", ownerPlan.ExtraShards)
	}
}

// TestClassBudgetConcurrentUpdateAndSnapshot verifies safe concurrent budget
// updates and reads.
//
// The snapshot is observational and not globally atomic across all fields. This
// test verifies that concurrent updates and planning do not panic or corrupt
// basic class-size identity.
func TestClassBudgetConcurrentUpdateAndSnapshot(t *testing.T) {
	t.Parallel()

	const goroutines = 8
	const iterations = 512

	classSize := ClassSizeFromSize(4 * KiB)
	budget := newClassBudget(classSize)

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for worker := 0; worker < goroutines; worker++ {
		worker := worker

		go func() {
			defer wg.Done()

			for iteration := 0; iteration < iterations; iteration++ {
				assigned := Size((worker + iteration + 1) * int(KiB))
				budget.updateAssignedBytes(assigned)

				snapshot := budget.snapshot()
				if snapshot.ClassSize != classSize {
					t.Errorf("snapshot.ClassSize = %s, want %s", snapshot.ClassSize, classSize)
				}

				plan := snapshot.shardCreditPlan(4)
				if plan.ClassSize != classSize {
					t.Errorf("plan.ClassSize = %s, want %s", plan.ClassSize, classSize)
				}

				_ = plan.credits()
			}
		}()
	}

	wg.Wait()
}

// TestClassShardCreditTotalIsZero verifies total zero classification.
func TestClassShardCreditTotalIsZero(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string

		total classShardCreditTotal

		want bool
	}{
		{
			name:  "zero",
			total: classShardCreditTotal{},
			want:  true,
		},
		{
			name: "buffers only",
			total: classShardCreditTotal{
				Buffers: 1,
			},
			want: false,
		},
		{
			name: "bytes only",
			total: classShardCreditTotal{
				Bytes: 1,
			},
			want: false,
		},
		{
			name: "buffers and bytes",
			total: classShardCreditTotal{
				Buffers: 1,
				Bytes:   1,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.total.IsZero(); got != tt.want {
				t.Fatalf("IsZero() = %t, want %t", got, tt.want)
			}
		})
	}
}
