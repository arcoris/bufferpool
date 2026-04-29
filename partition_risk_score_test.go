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

func TestPoolPartitionRiskScoreUsesEvaluatorDefault(t *testing.T) {
	evaluator := NewPoolPartitionScoreEvaluator(PoolPartitionScoreEvaluatorConfig{})
	score := evaluator.riskScore(PoolPartitionWindowRates{
		LeaseOwnershipViolationRatio: 1,
	})

	if score.Value == 0 || score.OwnershipComponent == 0 {
		t.Fatalf("risk score = %+v, want ownership risk", score)
	}
}

func TestPoolPartitionRiskScoreUsesEvaluatorConfig(t *testing.T) {
	rates := PoolPartitionWindowRates{LeaseInvalidReleaseRatio: 1}
	defaultScore := NewPoolPartitionScoreEvaluator(PoolPartitionScoreEvaluatorConfig{}).riskScore(rates)
	customScore := NewPoolPartitionScoreEvaluator(PoolPartitionScoreEvaluatorConfig{
		RiskConfig: PoolPartitionRiskScoreConfig{
			Weights:       PoolPartitionRiskScoreWeights{Misuse: 1},
			MisuseWeights: PoolPartitionMisuseRiskWeights{InvalidRelease: 1},
		},
	}).riskScore(rates)

	if customScore.Value <= defaultScore.Value {
		t.Fatalf("custom risk score = %+v, want greater than default %+v", customScore, defaultScore)
	}
}

func TestPoolPartitionRiskScoreZeroEvaluatorDisabled(t *testing.T) {
	var evaluator PoolPartitionScoreEvaluator
	score := evaluator.riskScore(PoolPartitionWindowRates{
		PoolReturnFailureRatio:       1,
		LeaseInvalidReleaseRatio:     1,
		LeaseOwnershipViolationRatio: 1,
	})

	if score != (PoolPartitionRiskScore{}) {
		t.Fatalf("zero evaluator risk score = %+v, want zero", score)
	}
}
