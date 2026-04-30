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

var (
	budgetPartitionTargetsSink []PartitionBudgetTarget
	budgetClassTargetsSink     []ClassBudgetTarget
	budgetGenerationSink       Generation
)

func BenchmarkBudgetAllocatePartitions(b *testing.B) {
	inputs := []partitionBudgetAllocationInput{
		{PartitionName: "p0", BaseRetainedBytes: 4 * MiB, MaxRetainedBytes: 64 * MiB, Score: 0.1},
		{PartitionName: "p1", BaseRetainedBytes: 4 * MiB, MaxRetainedBytes: 64 * MiB, Score: 0.9},
		{PartitionName: "p2", BaseRetainedBytes: 4 * MiB, MaxRetainedBytes: 64 * MiB, Score: 0.4},
		{PartitionName: "p3", BaseRetainedBytes: 4 * MiB, MaxRetainedBytes: 64 * MiB, Score: 0.6},
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		budgetPartitionTargetsSink = allocatePartitionBudgetTargets(Generation(i+1), 96*MiB, inputs)
	}
}

func BenchmarkBudgetAllocateClasses(b *testing.B) {
	inputs := []classBudgetAllocationInput{
		{ClassID: ClassID(0), MaxTargetBytes: 16 * MiB, Score: 0.2},
		{ClassID: ClassID(1), MaxTargetBytes: 16 * MiB, Score: 0.4},
		{ClassID: ClassID(2), MaxTargetBytes: 16 * MiB, Score: 0.8},
		{ClassID: ClassID(3), MaxTargetBytes: 16 * MiB, Score: 0.6},
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		budgetClassTargetsSink = allocateClassBudgetTargets(Generation(i+1), 32*MiB, inputs)
	}
}

func BenchmarkPoolApplyClassBudgets(b *testing.B) {
	pool := poolBenchmarkNewPool(b, poolBenchmarkPolicyForCase(poolBenchmarkCase{
		size:        900,
		capacity:    1024,
		shards:      4,
		selector:    ShardSelectionModeProcessorInspired,
		bucketSlots: 64,
	}))
	targets := []ClassBudgetTarget{
		{Generation: Generation(100), ClassID: ClassID(0), TargetBytes: 4 * MiB},
		{Generation: Generation(100), ClassID: ClassID(1), TargetBytes: 8 * MiB},
		{Generation: Generation(100), ClassID: ClassID(2), TargetBytes: 16 * MiB},
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		targets[0].Generation = Generation(100 + i)
		targets[1].Generation = Generation(100 + i)
		targets[2].Generation = Generation(100 + i)
		generation, err := pool.applyClassBudgets(targets)
		if err != nil {
			b.Fatalf("applyClassBudgets failed: %v", err)
		}
		budgetGenerationSink = generation
	}
}
