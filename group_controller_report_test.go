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

func TestNewPoolGroupControllerEvaluation(t *testing.T) {
	previous := testGroupSampleWithCounters(Generation(1), PoolCountersSnapshot{Gets: 10, Hits: 5, Misses: 5}, LeaseCountersSnapshot{Acquisitions: 1, Releases: 1})
	current := testGroupSampleWithCounters(Generation(2), PoolCountersSnapshot{Gets: 20, Hits: 15, Misses: 5}, LeaseCountersSnapshot{Acquisitions: 5, Releases: 3})
	evaluator := NewPoolGroupScoreEvaluator(PoolGroupScoreEvaluatorConfig{})
	evaluation := NewPoolGroupControllerEvaluation(previous, current, time.Second, PartitionBudgetSnapshot{}, PartitionPressureSnapshot{}, evaluator)
	if evaluation.Window.Generation != Generation(2) {
		t.Fatalf("Window.Generation = %v, want 2", evaluation.Window.Generation)
	}
	if evaluation.Rates.Aggregate.HitRatio == 0 {
		t.Fatalf("HitRatio = 0, want non-zero")
	}
	if evaluation.Scores.Usefulness == 0 {
		t.Fatalf("Scores.Usefulness = 0, want non-zero")
	}
}

func TestPoolGroupCoordinatorReportZeroValue(t *testing.T) {
	var report PoolGroupCoordinatorReport
	if report.Scores.IsZero() != true {
		t.Fatalf("zero report scores should be zero")
	}
}
