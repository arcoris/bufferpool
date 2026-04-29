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
	"fmt"
	"testing"
)

func benchmarkPoolGroup(b *testing.B, partitions int) *PoolGroup {
	b.Helper()
	partitionNames := make([]string, partitions)
	for index := range partitionNames {
		partitionNames[index] = "partition-" + string(rune('a'+index))
	}
	group, err := NewPoolGroup(testGroupConfig(partitionNames...))
	if err != nil {
		b.Fatalf("NewPoolGroup() error = %v", err)
	}
	b.Cleanup(func() {
		if closeErr := group.Close(); closeErr != nil {
			b.Fatalf("PoolGroup.Close() error = %v", closeErr)
		}
	})
	return group
}

func BenchmarkPoolGroupSample(b *testing.B) {
	for _, partitions := range []int{1, 4, 16} {
		b.Run(fmt.Sprintf("partitions_%d", partitions), func(b *testing.B) {
			group := benchmarkPoolGroup(b, partitions)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = group.Sample()
			}
		})
	}
}

func BenchmarkPoolGroupSampleInto(b *testing.B) {
	for _, partitions := range []int{1, 4, 16} {
		b.Run(fmt.Sprintf("partitions_%d", partitions), func(b *testing.B) {
			group := benchmarkPoolGroup(b, partitions)
			var sample PoolGroupSample
			group.SampleInto(&sample)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				group.SampleInto(&sample)
			}
		})
	}
}

func BenchmarkPoolGroupMetrics(b *testing.B) {
	group := benchmarkPoolGroup(b, 4)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = group.Metrics()
	}
}

func BenchmarkPoolGroupTickInto(b *testing.B) {
	group := benchmarkPoolGroup(b, 4)
	var report PoolGroupCoordinatorReport
	_ = group.TickInto(&report)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := group.TickInto(&report); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkPoolGroupScoreEvaluatorScoreValues(b *testing.B) {
	evaluator := NewPoolGroupScoreEvaluator(PoolGroupScoreEvaluatorConfig{})
	rates := PoolGroupWindowRates{Aggregate: PoolPartitionWindowRates{HitRatio: 1, RetainRatio: 1, LeaseOpsPerSecond: 100}}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = evaluator.ScoreValues(rates, PartitionBudgetSnapshot{}, PartitionPressureSnapshot{})
	}
}
