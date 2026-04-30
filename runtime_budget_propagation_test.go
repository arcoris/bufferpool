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

func TestPoolPartitionApplyPoolBudgetTargetsUpdatesOwnedPool(t *testing.T) {
	t.Parallel()

	partition := MustNewPoolPartition(poolBudgetTestPartitionConfig())
	t.Cleanup(func() {
		requirePartitionNoError(t, partition.Close())
	})

	partition.markDirtyProcessed()
	beforeDirtyGeneration := partition.activeRegistry.generationSnapshot()
	err := partition.applyPoolBudgetTargets([]PoolBudgetTarget{
		{
			Generation: Generation(80),
			PoolName:   "primary",
			ClassTargets: []ClassBudgetTarget{
				{Generation: Generation(80), ClassID: ClassID(0), TargetBytes: 2 * KiB},
				{Generation: Generation(80), ClassID: ClassID(1), TargetBytes: 4 * KiB},
			},
		},
	})
	if err != nil {
		t.Fatalf("applyPoolBudgetTargets() returned error: %v", err)
	}

	snapshot, ok := partition.PoolSnapshot("primary")
	if !ok {
		t.Fatal("PoolSnapshot(primary) not found")
	}
	if snapshot.Generation.Before(Generation(80)) {
		t.Fatalf("pool generation = %s, want >= 80", snapshot.Generation)
	}
	assertPoolClassBudgetSnapshot(t, snapshot.Classes[0].Budget, Generation(80), 2*KiB)
	assertPoolClassBudgetSnapshot(t, snapshot.Classes[1].Budget, Generation(80), 4*KiB)

	afterDirtyGeneration := partition.activeRegistry.generationSnapshot()
	if !afterDirtyGeneration.After(beforeDirtyGeneration) {
		t.Fatalf("active registry generation = %s, want after %s", afterDirtyGeneration, beforeDirtyGeneration)
	}
	dirtyIndexes := partition.controllerDirtyIndexes(nil)
	if len(dirtyIndexes) != 1 || dirtyIndexes[0] != 0 {
		t.Fatalf("dirty indexes = %v, want [0]", dirtyIndexes)
	}
}

func TestPoolPartitionApplyPoolBudgetTargetsRejectsMissingPool(t *testing.T) {
	t.Parallel()

	partition := MustNewPoolPartition(poolBudgetTestPartitionConfig())
	t.Cleanup(func() {
		requirePartitionNoError(t, partition.Close())
	})

	err := partition.applyPoolBudgetTargets([]PoolBudgetTarget{{Generation: Generation(81), PoolName: "missing", RetainedBytes: KiB}})
	if err == nil {
		t.Fatal("applyPoolBudgetTargets(missing) returned nil")
	}
	if !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("applyPoolBudgetTargets(missing) error = %v, want ErrInvalidOptions", err)
	}
}

// TestPoolPartitionApplyPoolBudgetTargetsRejectsMissingPoolWithoutPartialMutation
// verifies pool budget batches validate all targets before mutating any owned
// Pool.
func TestPoolPartitionApplyPoolBudgetTargetsRejectsMissingPoolWithoutPartialMutation(t *testing.T) {
	t.Parallel()

	config := poolBudgetTestPartitionConfig()
	config.Pools = append(config.Pools, PartitionPoolConfig{Name: "secondary", Config: PoolConfig{Policy: poolTestSmallSingleShardPolicy()}})
	partition := MustNewPoolPartition(config)
	t.Cleanup(func() {
		requirePartitionNoError(t, partition.Close())
	})

	before, ok := partition.PoolSnapshot("primary")
	if !ok {
		t.Fatal("PoolSnapshot(primary) not found")
	}
	err := partition.applyPoolBudgetTargets([]PoolBudgetTarget{
		{
			Generation: Generation(82),
			PoolName:   "primary",
			ClassTargets: []ClassBudgetTarget{
				{Generation: Generation(82), ClassID: ClassID(0), TargetBytes: 2 * KiB},
			},
		},
		{Generation: Generation(82), PoolName: "missing", RetainedBytes: KiB},
	})
	if !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("applyPoolBudgetTargets(partial invalid) error = %v, want ErrInvalidOptions", err)
	}
	after, ok := partition.PoolSnapshot("primary")
	if !ok {
		t.Fatal("PoolSnapshot(primary) after apply not found")
	}
	if after.Generation != before.Generation {
		t.Fatalf("primary generation changed after rejected batch: got %s want %s", after.Generation, before.Generation)
	}
}

func TestPoolGroupApplyPartitionBudgetTargetsUpdatesPartitionPolicy(t *testing.T) {
	t.Parallel()

	group := MustNewPoolGroup(PoolGroupConfig{
		Partitions: []GroupPartitionConfig{
			{Name: "alpha", Config: poolBudgetTestPartitionConfig()},
		},
	})
	t.Cleanup(func() {
		requireGroupNoError(t, group.Close())
	})

	err := group.applyPartitionBudgetTargets([]PartitionBudgetTarget{
		{Generation: Generation(90), PartitionName: "alpha", RetainedBytes: 12 * KiB},
	})
	if err != nil {
		t.Fatalf("applyPartitionBudgetTargets() returned error: %v", err)
	}

	snapshot, ok := group.PartitionSnapshot("alpha")
	if !ok {
		t.Fatal("PartitionSnapshot(alpha) not found")
	}
	if snapshot.PolicyGeneration.Before(Generation(90)) {
		t.Fatalf("partition policy generation = %s, want >= 90", snapshot.PolicyGeneration)
	}
	if snapshot.Policy.Budget.MaxRetainedBytes != 12*KiB {
		t.Fatalf("MaxRetainedBytes = %s, want 12 KiB", snapshot.Policy.Budget.MaxRetainedBytes)
	}
}

func TestPoolGroupApplyPartitionBudgetTargetsRejectsMissingPartition(t *testing.T) {
	t.Parallel()

	group := MustNewPoolGroup(PoolGroupConfig{
		Partitions: []GroupPartitionConfig{
			{Name: "alpha", Config: poolBudgetTestPartitionConfig()},
		},
	})
	t.Cleanup(func() {
		requireGroupNoError(t, group.Close())
	})

	err := group.applyPartitionBudgetTargets([]PartitionBudgetTarget{{Generation: Generation(91), PartitionName: "missing", RetainedBytes: KiB}})
	if err == nil {
		t.Fatal("applyPartitionBudgetTargets(missing) returned nil")
	}
	if !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("applyPartitionBudgetTargets(missing) error = %v, want ErrInvalidOptions", err)
	}
}

func poolBudgetTestPartitionConfig() PoolPartitionConfig {
	return PoolPartitionConfig{
		Name: "alpha",
		Pools: []PartitionPoolConfig{
			{Name: "primary", Config: PoolConfig{Policy: poolTestSmallSingleShardPolicy()}},
		},
	}
}
