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

func TestAllocatePartitionBudgetTargetsUsesScores(t *testing.T) {
	t.Parallel()

	targets := allocatePartitionBudgetTargets(Generation(7), SizeFromBytes(1200), []partitionBudgetAllocationInput{
		{PartitionName: "alpha", BaseRetainedBytes: SizeFromBytes(100), MinRetainedBytes: SizeFromBytes(50), MaxRetainedBytes: SizeFromBytes(1000), Score: 1},
		{PartitionName: "beta", BaseRetainedBytes: SizeFromBytes(100), MinRetainedBytes: SizeFromBytes(50), MaxRetainedBytes: SizeFromBytes(1000), Score: 3},
	})

	assertPartitionBudgetTarget(t, targets[0], "alpha", Generation(7), 350)
	assertPartitionBudgetTarget(t, targets[1], "beta", Generation(7), 850)
	assertBudgetTargetSumAtMost(t, partitionBudgetTargetBytes(targets), 1200)
}

func TestAllocatePartitionBudgetTargetsUsesEqualRemainderWhenScoresAreZero(t *testing.T) {
	t.Parallel()

	targets := allocatePartitionBudgetTargets(Generation(8), SizeFromBytes(900), []partitionBudgetAllocationInput{
		{PartitionName: "alpha", MaxRetainedBytes: SizeFromBytes(900)},
		{PartitionName: "beta", MaxRetainedBytes: SizeFromBytes(900)},
		{PartitionName: "gamma", MaxRetainedBytes: SizeFromBytes(900)},
	})

	assertPartitionBudgetTarget(t, targets[0], "alpha", Generation(8), 300)
	assertPartitionBudgetTarget(t, targets[1], "beta", Generation(8), 300)
	assertPartitionBudgetTarget(t, targets[2], "gamma", Generation(8), 300)
	assertBudgetTargetSumAtMost(t, partitionBudgetTargetBytes(targets), 900)
}

func TestAllocatePoolBudgetTargetsClampsMinAndMax(t *testing.T) {
	t.Parallel()

	targets := allocatePoolBudgetTargets(Generation(9), SizeFromBytes(1000), []poolBudgetAllocationInput{
		{PoolName: "small", BaseRetainedBytes: SizeFromBytes(700), MinRetainedBytes: SizeFromBytes(256), MaxRetainedBytes: SizeFromBytes(512), Score: 10},
		{PoolName: "large", BaseRetainedBytes: SizeFromBytes(100), MinRetainedBytes: SizeFromBytes(128), MaxRetainedBytes: SizeFromBytes(1024), Score: 0},
	})

	assertPoolBudgetTarget(t, targets[0], "small", Generation(9), 512)
	assertPoolBudgetTarget(t, targets[1], "large", Generation(9), 488)
	assertBudgetTargetSumAtMost(t, poolBudgetTargetBytes(targets), 1000)
}

func TestAllocateClassBudgetTargetsPreservesExactParentBound(t *testing.T) {
	t.Parallel()

	targets := allocateClassBudgetTargets(Generation(10), SizeFromBytes(1536), []classBudgetAllocationInput{
		{ClassID: ClassID(0), BaseTargetBytes: SizeFromBytes(512), MaxTargetBytes: SizeFromBytes(2048), Score: 1},
		{ClassID: ClassID(1), BaseTargetBytes: SizeFromBytes(512), MaxTargetBytes: SizeFromBytes(2048), Score: 1},
	})

	if targets[0].TargetBytes.Bytes()+targets[1].TargetBytes.Bytes() != 1536 {
		t.Fatalf("class target sum = %d, want 1536", targets[0].TargetBytes.Bytes()+targets[1].TargetBytes.Bytes())
	}
	if targets[0].Generation != Generation(10) || targets[1].Generation != Generation(10) {
		t.Fatalf("class target generations = %s/%s, want 10/10", targets[0].Generation, targets[1].Generation)
	}
}

func TestBudgetWeightedShareHandlesLargeProducts(t *testing.T) {
	t.Parallel()

	share := budgetWeightedShare(^uint64(0), ^uint64(0)-1, ^uint64(0))
	if share != ^uint64(0)-1 {
		t.Fatalf("budgetWeightedShare(max, max-1, max) = %d, want %d", share, ^uint64(0)-1)
	}
}

func TestAllocateShardCreditsForClassBudget(t *testing.T) {
	t.Parallel()

	credits := allocateShardCreditsForClassBudget(
		ClassSizeFromSize(4*KiB),
		4,
		ClassBudgetTarget{Generation: Generation(11), ClassID: ClassID(0), TargetBytes: 40 * KiB},
	)

	wantBuffers := []uint64{3, 3, 2, 2}
	wantBytes := []uint64{uint64(12 * KiB), uint64(12 * KiB), uint64(8 * KiB), uint64(8 * KiB)}
	for index, credit := range credits {
		if credit.TargetBuffers != wantBuffers[index] {
			t.Fatalf("credit[%d].TargetBuffers = %d, want %d", index, credit.TargetBuffers, wantBuffers[index])
		}
		if credit.TargetBytes != wantBytes[index] {
			t.Fatalf("credit[%d].TargetBytes = %d, want %d", index, credit.TargetBytes, wantBytes[index])
		}
	}
}

func assertPartitionBudgetTarget(t *testing.T, got PartitionBudgetTarget, name string, generation Generation, bytes uint64) {
	t.Helper()

	if got.PartitionName != name {
		t.Fatalf("PartitionName = %q, want %q", got.PartitionName, name)
	}
	if got.Generation != generation {
		t.Fatalf("Generation = %s, want %s", got.Generation, generation)
	}
	if got.RetainedBytes.Bytes() != bytes {
		t.Fatalf("RetainedBytes = %d, want %d", got.RetainedBytes.Bytes(), bytes)
	}
}

func assertPoolBudgetTarget(t *testing.T, got PoolBudgetTarget, name string, generation Generation, bytes uint64) {
	t.Helper()

	if got.PoolName != name {
		t.Fatalf("PoolName = %q, want %q", got.PoolName, name)
	}
	if got.Generation != generation {
		t.Fatalf("Generation = %s, want %s", got.Generation, generation)
	}
	if got.RetainedBytes.Bytes() != bytes {
		t.Fatalf("RetainedBytes = %d, want %d", got.RetainedBytes.Bytes(), bytes)
	}
}

func assertBudgetTargetSumAtMost(t *testing.T, got uint64, max uint64) {
	t.Helper()

	if got > max {
		t.Fatalf("target sum = %d, want <= %d", got, max)
	}
}

func partitionBudgetTargetBytes(targets []PartitionBudgetTarget) uint64 {
	var total uint64
	for _, target := range targets {
		total = poolSaturatingAdd(total, target.RetainedBytes.Bytes())
	}
	return total
}

func poolBudgetTargetBytes(targets []PoolBudgetTarget) uint64 {
	var total uint64
	for _, target := range targets {
		total = poolSaturatingAdd(total, target.RetainedBytes.Bytes())
	}
	return total
}
