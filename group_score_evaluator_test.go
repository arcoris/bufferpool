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

func TestPoolGroupScoreEvaluatorZeroValue(t *testing.T) {
	var evaluator PoolGroupScoreEvaluator
	values := evaluator.ScoreValues(PoolGroupWindowRates{}, PoolGroupBudgetSnapshot{}, PoolGroupPressureSnapshot{})
	if !values.IsZero() {
		t.Fatalf("zero evaluator ScoreValues = %#v, want zero", values)
	}
}

func TestPoolGroupScoreEvaluatorUsesAggregateRates(t *testing.T) {
	evaluator := NewPoolGroupScoreEvaluator(PoolGroupScoreEvaluatorConfig{})
	values := evaluator.ScoreValues(PoolGroupWindowRates{Aggregate: PoolPartitionWindowRates{HitRatio: 1, RetainRatio: 1, LeaseOpsPerSecond: 100}}, PoolGroupBudgetSnapshot{}, PoolGroupPressureSnapshot{})
	if values.Usefulness == 0 {
		t.Fatalf("Usefulness = 0, want non-zero for useful aggregate rates")
	}
	if values.Activity == 0 {
		t.Fatalf("Activity = 0, want non-zero for active aggregate rates")
	}
}

func TestPoolGroupScoreEvaluatorPartitionScoreValues(t *testing.T) {
	evaluator := NewPoolGroupScoreEvaluator(PoolGroupScoreEvaluatorConfig{})
	values := evaluator.PartitionScoreValues(PoolPartitionWindowRates{HitRatio: 1, RetainRatio: 1, LeaseOpsPerSecond: 100}, PoolPartitionEWMAState{}, PartitionBudgetSnapshot{}, PartitionPressureSnapshot{})
	if values.Usefulness == 0 || values.Activity == 0 {
		t.Fatalf("PartitionScoreValues = %#v, want useful and active", values)
	}
}

func TestPoolGroupScoreValuesIsZero(t *testing.T) {
	if !(PoolGroupScoreValues{}).IsZero() {
		t.Fatalf("zero score values should be zero")
	}
	if (PoolGroupScoreValues{Risk: 1}).IsZero() {
		t.Fatalf("non-zero risk should not be zero")
	}
}
