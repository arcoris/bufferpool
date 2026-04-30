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
	"testing"
	"time"
)

// TestPoolPartitionControllerEvaluation verifies pure controller projection.
func TestPoolPartitionControllerEvaluation(t *testing.T) {
	previous := PoolPartitionSample{
		PoolCounters: PoolCountersSnapshot{
			Gets:        10,
			Hits:        8,
			Misses:      2,
			Allocations: 2,
			Puts:        8,
			Retains:     7,
			Drops:       1,
		},
	}
	current := previous
	current.Generation = Generation(2)
	current.PoolCounters.Gets += 10
	current.PoolCounters.Hits += 9
	current.PoolCounters.Misses += 1
	current.PoolCounters.Allocations += 1
	current.PoolCounters.Puts += 10
	current.PoolCounters.Retains += 9
	current.PoolCounters.Drops += 1

	evaluation := NewPoolPartitionControllerEvaluation(
		previous,
		current,
		time.Second,
		PoolPartitionEWMAState{},
		PoolPartitionEWMAConfig{HalfLife: time.Second},
		PartitionBudgetSnapshot{},
		PartitionPressureSnapshot{},
	)

	if evaluation.Window.Delta.Gets != 10 || evaluation.Rates.GetsPerSecond != 10 {
		t.Fatalf("evaluation window/rates = %+v / %+v", evaluation.Window.Delta, evaluation.Rates)
	}
	if !evaluation.EWMA.Initialized || evaluation.Scores.Usefulness.Value == 0 {
		t.Fatalf("evaluation EWMA/scores = %+v / %+v", evaluation.EWMA, evaluation.Scores)
	}
	if evaluation.Recommendation.Kind == PoolPartitionRecommendationNone {
		t.Fatalf("evaluation recommendation = %+v", evaluation.Recommendation)
	}

	zeroElapsed := NewPoolPartitionControllerEvaluation(
		previous,
		current,
		0,
		PoolPartitionEWMAState{},
		PoolPartitionEWMAConfig{},
		PartitionBudgetSnapshot{},
		PartitionPressureSnapshot{},
	)
	if zeroElapsed.Rates.GetsPerSecond != 0 {
		t.Fatalf("zero elapsed throughput = %+v", zeroElapsed.Rates)
	}
}
