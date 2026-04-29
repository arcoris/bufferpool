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

	controlactivity "arcoris.dev/bufferpool/internal/control/activity"
)

// TestPoolPartitionActivityUsesLeaseOpsRate verifies ownership churn is driven
// by the lease throughput signal rather than inferred from Pool get/put volume.
func TestPoolPartitionActivityUsesLeaseOpsRate(t *testing.T) {
	evaluator := NewPoolPartitionScoreEvaluator(PoolPartitionScoreEvaluatorConfig{})
	score := evaluator.activityScore(partitionScoreSignals{
		leaseOpsPerSecond: defaultPartitionHighLeaseOpsPerSecond,
	})

	if score.Value <= 0 {
		t.Fatalf("activity score = %+v, want lease throughput to contribute", score)
	}
}

// TestPoolPartitionActivityDoesNotUseGetsPlusPutsAsLeaseOps prevents Pool data
// plane volume from being counted as ownership lease churn.
func TestPoolPartitionActivityDoesNotUseGetsPlusPutsAsLeaseOps(t *testing.T) {
	evaluator := NewPoolPartitionScoreEvaluator(PoolPartitionScoreEvaluatorConfig{})
	signals := partitionScoreSignals{
		getsPerSecond: defaultPartitionHighGetsPerSecond,
		putsPerSecond: defaultPartitionHighPutsPerSecond,
	}

	score := evaluator.activityScore(signals)
	oldInferredLeaseOpsScore := controlactivity.Hotness(controlactivity.HotnessInput{
		GetsPerSecond:     signals.getsPerSecond,
		PutsPerSecond:     signals.putsPerSecond,
		LeaseOpsPerSecond: signals.getsPerSecond + signals.putsPerSecond,
	}, PoolPartitionActivityScoreConfig{}.controlConfig())

	if score.Value >= oldInferredLeaseOpsScore {
		t.Fatalf("activity score = %v, old inferred lease score = %v; lease ops should not be gets+puts", score.Value, oldInferredLeaseOpsScore)
	}
}

func TestPoolPartitionActivityDefaultDisablesByteThroughput(t *testing.T) {
	config := PoolPartitionActivityScoreConfig{}.controlConfig()
	if config.HighBytesPerSecond != 0 {
		t.Fatalf("default HighBytesPerSecond = %v, want byte activity disabled", config.HighBytesPerSecond)
	}

	rates := PoolPartitionWindowRates{
		ReturnedBytesPerSecond: 1_000_000,
		DroppedBytesPerSecond:  1_000_000,
	}
	scores := NewPoolPartitionScores(rates, PoolPartitionEWMAState{}, PartitionBudgetSnapshot{}, PartitionPressureSnapshot{})
	if scores.Activity.Value != 0 {
		t.Fatalf("activity from byte diagnostics = %v, want zero", scores.Activity.Value)
	}
}

func TestPoolPartitionActivityByteThroughputPolicyDocumented(t *testing.T) {
	evaluator := NewPoolPartitionScoreEvaluator(PoolPartitionScoreEvaluatorConfig{
		ActivityConfig: PoolPartitionActivityScoreConfig{
			HighBytesPerSecond: 1,
			BytesWeight:        1,
		},
	})
	rates := PoolPartitionWindowRates{
		ReturnedBytesPerSecond: 1,
		DroppedBytesPerSecond:  1,
	}

	scores := evaluator.Scores(rates, PoolPartitionEWMAState{}, PartitionBudgetSnapshot{}, PartitionPressureSnapshot{})
	if scores.Activity.Value != 0 {
		t.Fatalf("custom byte config without byte activity adapter = %v, want zero", scores.Activity.Value)
	}
}
