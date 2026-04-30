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
	"os"
	"strings"
	"testing"

	"arcoris.dev/bufferpool/internal/testutil"
)

func TestPoolApplyClassBudgetsUpdatesClassTargetsAndShardCredits(t *testing.T) {
	t.Parallel()

	pool := MustNew(PoolConfig{Policy: poolTestSmallSingleShardPolicy()})
	defer closePoolForTest(t, pool)

	generation := pool.applyClassBudgets([]ClassBudgetTarget{
		{Generation: Generation(50), ClassID: ClassID(0), TargetBytes: 2 * KiB},
		{Generation: Generation(50), ClassID: ClassID(1), TargetBytes: 4 * KiB},
	})

	if generation.Before(Generation(50)) {
		t.Fatalf("generation = %s, want >= 50", generation)
	}

	snapshot := pool.Snapshot()
	if snapshot.Generation != generation {
		t.Fatalf("Pool snapshot Generation = %s, want %s", snapshot.Generation, generation)
	}
	assertPoolClassBudgetSnapshot(t, snapshot.Classes[0].Budget, Generation(50), 2*KiB)
	assertPoolClassBudgetSnapshot(t, snapshot.Classes[1].Budget, Generation(50), 4*KiB)

	for _, class := range snapshot.Classes {
		for _, shard := range class.Shards {
			if !shard.Credit.IsEnabled() {
				t.Fatalf("class %s shard %d credit disabled after apply", class.Class, shard.Index)
			}
			if shard.Credit.Generation.Before(Generation(50)) {
				t.Fatalf("class %s shard %d credit generation = %s, want >= 50", class.Class, shard.Index, shard.Credit.Generation)
			}
		}
	}
}

func TestPoolApplyClassBudgetsRejectsUnknownClass(t *testing.T) {
	t.Parallel()

	pool := MustNew(PoolConfig{Policy: poolTestSmallSingleShardPolicy()})
	defer closePoolForTest(t, pool)

	before := pool.Snapshot()
	testutil.MustPanicWithMessage(t, errPoolBudgetTargetClassMissing, func() {
		_ = pool.applyClassBudgets([]ClassBudgetTarget{
			{Generation: Generation(60), ClassID: ClassID(99), TargetBytes: KiB},
		})
	})

	after := pool.Snapshot()
	if after.Generation != before.Generation {
		t.Fatalf("Pool generation changed after rejected class target: got %s want %s", after.Generation, before.Generation)
	}
}

func TestPoolApplyClassBudgetsRejectsDuplicateClass(t *testing.T) {
	t.Parallel()

	pool := MustNew(PoolConfig{Policy: poolTestSmallSingleShardPolicy()})
	defer closePoolForTest(t, pool)

	testutil.MustPanicWithMessage(t, errPoolBudgetTargetDuplicateClass, func() {
		_ = pool.applyClassBudgets([]ClassBudgetTarget{
			{Generation: Generation(61), ClassID: ClassID(0), TargetBytes: KiB},
			{Generation: Generation(61), ClassID: ClassID(0), TargetBytes: 2 * KiB},
		})
	})
}

func TestPoolApplyPoolBudgetComputesDefaultClassTargets(t *testing.T) {
	t.Parallel()

	pool := MustNew(PoolConfig{Policy: poolTestSmallSingleShardPolicy()})
	defer closePoolForTest(t, pool)

	generation := pool.applyPoolBudget(PoolBudgetTarget{
		Generation:    Generation(70),
		PoolName:      "primary",
		RetainedBytes: 3 * KiB,
	})

	snapshot := pool.Snapshot()
	if snapshot.Generation != generation || snapshot.Generation.Before(Generation(70)) {
		t.Fatalf("Pool generation = %s returned %s, want >= 70", snapshot.Generation, generation)
	}
	if sumPoolClassBudgetTargets(snapshot.Classes) > (3 * KiB).Bytes() {
		t.Fatalf("class budget sum = %d, want <= %d", sumPoolClassBudgetTargets(snapshot.Classes), (3 * KiB).Bytes())
	}
}

func TestPoolApplyClassBudgetsKeepsPoolGenerationMonotonic(t *testing.T) {
	t.Parallel()

	pool := MustNew(PoolConfig{Policy: poolTestSmallSingleShardPolicy()})
	defer closePoolForTest(t, pool)

	first := pool.applyClassBudgets([]ClassBudgetTarget{
		{Generation: Generation(100), ClassID: ClassID(0), TargetBytes: 2 * KiB},
	})
	second := pool.applyClassBudgets([]ClassBudgetTarget{
		{Generation: Generation(90), ClassID: ClassID(0), TargetBytes: KiB},
	})
	if !second.After(first) {
		t.Fatalf("second pool generation = %s, want after %s", second, first)
	}
	if pool.Snapshot().Generation != second {
		t.Fatalf("Pool snapshot generation = %s, want %s", pool.Snapshot().Generation, second)
	}
}

func TestPoolGetPutDoNotCallBudgetAllocators(t *testing.T) {
	t.Parallel()

	for _, file := range []string{"pool_get.go", "pool_put.go"} {
		file := file

		t.Run(file, func(t *testing.T) {
			t.Parallel()

			content, err := os.ReadFile(file)
			if err != nil {
				t.Fatalf("read %s: %v", file, err)
			}
			source := string(content)
			for _, forbidden := range []string{
				"allocateBudgetTargets",
				"allocateClassBudgetTargets",
				"allocatePoolBudgetTargets",
				"allocatePartitionBudgetTargets",
				"applyClassBudgets",
				"applyPoolBudget",
			} {
				if strings.Contains(source, forbidden) {
					t.Fatalf("%s contains hot-path budget control call %q", file, forbidden)
				}
			}
		})
	}
}

func assertPoolClassBudgetSnapshot(t *testing.T, got PoolClassBudgetSnapshot, generation Generation, assigned Size) {
	t.Helper()

	if got.Generation.Before(generation) {
		t.Fatalf("budget generation = %s, want >= %s", got.Generation, generation)
	}
	if got.AssignedBytes != assigned.Bytes() {
		t.Fatalf("AssignedBytes = %d, want %d", got.AssignedBytes, assigned.Bytes())
	}
}

func sumPoolClassBudgetTargets(classes []PoolClassSnapshot) uint64 {
	var total uint64
	for _, class := range classes {
		total = poolSaturatingAdd(total, class.Budget.TargetBytes)
	}
	return total
}
