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
	score := newPoolPartitionActivityScore(partitionScoreSignals{
		leaseOpsPerSecond: defaultPartitionHighLeaseOpsPerSecond,
	})

	if score.Value <= 0 {
		t.Fatalf("activity score = %+v, want lease throughput to contribute", score)
	}
}

// TestPoolPartitionActivityDoesNotUseGetsPlusPutsAsLeaseOps prevents Pool data
// plane volume from being counted as ownership lease churn.
func TestPoolPartitionActivityDoesNotUseGetsPlusPutsAsLeaseOps(t *testing.T) {
	signals := partitionScoreSignals{
		getsPerSecond: defaultPartitionHighGetsPerSecond,
		putsPerSecond: defaultPartitionHighPutsPerSecond,
	}

	score := newPoolPartitionActivityScore(signals)
	oldInferredLeaseOpsScore := partitionActivityScorer.Score(controlactivity.HotnessInput{
		GetsPerSecond:     signals.getsPerSecond,
		PutsPerSecond:     signals.putsPerSecond,
		LeaseOpsPerSecond: signals.getsPerSecond + signals.putsPerSecond,
	})

	if score.Value >= oldInferredLeaseOpsScore {
		t.Fatalf("activity score = %v, old inferred lease score = %v; lease ops should not be gets+puts", score.Value, oldInferredLeaseOpsScore)
	}
}
