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
	"testing"
)

// TestPoolClassCountAndSizes verifies class-table metadata projection.
func TestPoolClassCountAndSizes(t *testing.T) {
	t.Parallel()

	policy := poolTestSmallSingleShardPolicy()
	pool := MustNew(PoolConfig{Policy: policy})
	defer closePoolForTest(t, pool)

	if got := pool.ClassCount(); got != len(policy.Classes.Sizes) {
		t.Fatalf("ClassCount() = %d, want %d", got, len(policy.Classes.Sizes))
	}

	sizes := pool.ClassSizes()
	if len(sizes) != len(policy.Classes.Sizes) {
		t.Fatalf("len(ClassSizes()) = %d, want %d", len(sizes), len(policy.Classes.Sizes))
	}

	for index, want := range policy.Classes.Sizes {
		if sizes[index] != want {
			t.Fatalf("ClassSizes()[%d] = %s, want %s", index, sizes[index], want)
		}
	}

	sizes[0] = ClassSizeFromSize(8 * MiB)
	if pool.ClassSizes()[0] != policy.Classes.Sizes[0] {
		t.Fatal("ClassSizes() returned mutable Pool-owned slice")
	}
}

// TestNewPoolRejectsUnsupportedShardSelectionMode verifies Pool-level protection
// when policy validation is explicitly disabled.
func TestNewPoolRejectsUnsupportedShardSelectionMode(t *testing.T) {
	t.Parallel()

	policy := poolTestSmallSingleShardPolicy()
	policy.Shards.Selection = ShardSelectionMode(255)

	pool, err := New(PoolConfig{
		Policy:           policy,
		PolicyValidation: PoolPolicyValidationModeDisabled,
	})
	if err == nil {
		t.Fatal("New() returned nil error for unsupported shard selection mode")
	}

	if pool != nil {
		t.Fatalf("New() returned pool for unsupported shard selection mode: %#v", pool)
	}

	if !errors.Is(err, ErrInvalidPolicy) {
		t.Fatalf("New() error does not match ErrInvalidPolicy: %v", err)
	}
}

// TestPoolInitialBudgets verifies standalone static budget publication.
func TestPoolInitialBudgets(t *testing.T) {
	t.Parallel()

	policy := poolTestSmallSingleShardPolicy()
	pool := MustNew(PoolConfig{Policy: policy})
	defer closePoolForTest(t, pool)

	if len(pool.classes) != len(policy.Classes.Sizes) {
		t.Fatalf("len(pool.classes) = %d, want %d", len(pool.classes), len(policy.Classes.Sizes))
	}

	for index := range pool.classes {
		state := pool.classes[index].state()

		if state.Budget.IsZero() {
			t.Fatalf("class %d budget is zero after Pool construction", index)
		}

		if !state.Budget.IsEffective() {
			t.Fatalf("class %d budget is not effective after Pool construction: %#v", index, state.Budget)
		}

		if state.Budget.TargetBytes > policy.Retention.MaxClassRetainedBytes.Bytes() {
			t.Fatalf("class %d target bytes = %d, want <= %d",
				index,
				state.Budget.TargetBytes,
				policy.Retention.MaxClassRetainedBytes.Bytes(),
			)
		}

		for shardIndex, shard := range state.Shards {
			if !shard.Credit.IsEnabled() {
				t.Fatalf("class %d shard %d credit is disabled after Pool construction", index, shardIndex)
			}

			if !shard.Credit.IsConsistent() {
				t.Fatalf("class %d shard %d credit is inconsistent: %#v", index, shardIndex, shard.Credit)
			}
		}
	}
}

// TestPoolInitialBudgetAssignmentsRedistributeCappedShares verifies static
// publication does not lose bytes merely because smaller classes hit their
// configured caps before larger classes.
func TestPoolInitialBudgetAssignmentsRedistributeCappedShares(t *testing.T) {
	t.Parallel()

	policy := poolTestSmallSingleShardPolicy()
	policy.Retention.SoftRetainedBytes = SizeFromBytes(3584)
	policy.Retention.HardRetainedBytes = 4 * KiB
	policy.Retention.MaxRetainedBuffers = 8
	policy.Retention.MaxClassRetainedBytes = 4 * KiB
	policy.Retention.MaxClassRetainedBuffers = 1
	policy.Retention.MaxShardRetainedBytes = 4 * KiB
	policy.Retention.MaxShardRetainedBuffers = 8
	policy.Classes.Sizes = []ClassSize{
		ClassSizeFromBytes(512),
		ClassSizeFromSize(KiB),
		ClassSizeFromSize(2 * KiB),
	}

	table := newClassTable(policy.Classes.Sizes)
	classes := newPoolClassStates(table, policy)
	assignments := poolInitialBudgetAssignments(policy, classes)

	want := []uint64{512, 1024, 2048}
	for index, got := range assignments {
		if got != want[index] {
			t.Fatalf("assignment[%d] = %d, want %d", index, got, want[index])
		}
	}
}

// TestPoolInitialBudgetAssignmentsDisableOnZeroSoftBudget verifies a zero soft
// retention target publishes disabled budgets.
func TestPoolInitialBudgetAssignmentsDisableOnZeroSoftBudget(t *testing.T) {
	t.Parallel()

	policy := poolTestSmallSingleShardPolicy()
	policy.Retention.SoftRetainedBytes = 0
	policy.Retention.HardRetainedBytes = 4 * KiB

	table := newClassTable(policy.Classes.Sizes)
	classes := newPoolClassStates(table, policy)
	assignments := poolInitialBudgetAssignments(policy, classes)

	for index, got := range assignments {
		if got != 0 {
			t.Fatalf("assignment[%d] = %d, want zero", index, got)
		}
	}
}

// TestPoolClassStateLookupPanicsForForeignClass verifies ClassID/table ownership
// assumptions.
func TestPoolClassStateLookupPanicsForForeignClass(t *testing.T) {
	t.Parallel()

	pool := MustNew(PoolConfig{Policy: poolTestSmallSingleShardPolicy()})
	defer closePoolForTest(t, pool)

	foreign := NewSizeClass(ClassID(0), ClassSizeFromSize(8*MiB))

	assertPoolPanic(t, errPoolClassDescriptorMismatch, func() {
		_ = pool.mustClassStateFor(foreign)
	})

	outOfRange := NewSizeClass(ClassID(999), ClassSizeFromSize(KiB))

	assertPoolPanic(t, errPoolClassIDOutOfRange, func() {
		_ = pool.mustClassStateFor(outOfRange)
	})
}

// TestPoolRandomShardSelectorProducesValidIndexes verifies the Pool-owned random
// selector stays within shard bounds.
func TestPoolRandomShardSelectorProducesValidIndexes(t *testing.T) {
	t.Parallel()

	selector, err := newPoolShardSelector(ShardSelectionModeRandom, 0)
	if err != nil {
		t.Fatalf("newPoolShardSelector(random) returned error: %v", err)
	}

	const shardCount = 7

	for iteration := 0; iteration < 1024; iteration++ {
		index := selector.SelectShard(shardCount)
		if index < 0 || index >= shardCount {
			t.Fatalf("SelectShard(%d) = %d, want in [0, %d)", shardCount, index, shardCount)
		}
	}
}

// TestPoolProcessorInspiredShardSelectorProducesValidIndexes verifies Pool can
// construct the default selector mode.
func TestPoolProcessorInspiredShardSelectorProducesValidIndexes(t *testing.T) {
	t.Parallel()

	selector, err := newPoolShardSelector(ShardSelectionModeProcessorInspired, 1)
	if err != nil {
		t.Fatalf("newPoolShardSelector(processor-inspired) returned error: %v", err)
	}

	for iteration := 0; iteration < 1024; iteration++ {
		index := selector.SelectShard(11)
		if index < 0 || index >= 11 {
			t.Fatalf("SelectShard(11) = %d, want in [0, 11)", index)
		}
	}
}

// TestPoolStaticBudgetCapSaturates verifies overflow-safe budget cap arithmetic.
func TestPoolStaticBudgetCapSaturates(t *testing.T) {
	t.Parallel()

	policy := poolTestSmallSingleShardPolicy()
	classSize := ClassSizeFromSize(KiB)

	capBytes := poolClassStaticBudgetCap(policy, classSize)
	if capBytes == 0 {
		t.Fatal("poolClassStaticBudgetCap returned zero for enabled policy")
	}

	if capBytes > policy.Retention.MaxClassRetainedBytes.Bytes() {
		t.Fatalf("cap = %d, want <= max class retained bytes %d",
			capBytes,
			policy.Retention.MaxClassRetainedBytes.Bytes(),
		)
	}

	if got := poolSaturatingProduct(^uint64(0), 2); got != ^uint64(0) {
		t.Fatalf("poolSaturatingProduct overflow = %d, want max uint64", got)
	}

	if got := poolMinUint64(1, 2); got != 1 {
		t.Fatalf("poolMinUint64(1, 2) = %d, want 1", got)
	}
}
