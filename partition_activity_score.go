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

import controlactivity "arcoris.dev/bufferpool/internal/control/activity"

const (
	// defaultPartitionHighGetsPerSecond is the initial "hot" acquisition
	// threshold for partition activity projection. It is conservative
	// scaffolding, not production auto-tuning.
	defaultPartitionHighGetsPerSecond = 100_000

	// defaultPartitionHighPutsPerSecond is the initial "hot" return-flow
	// threshold. It mirrors get throughput so balanced workloads score
	// predictably.
	defaultPartitionHighPutsPerSecond = 100_000

	// defaultPartitionHighLeaseOpsPerSecond keeps ownership operation volume
	// visible without letting lease churn dominate buffer demand.
	defaultPartitionHighLeaseOpsPerSecond = 200_000
)

var (
	// partitionActivityScorer prepares the default hotness thresholds once for
	// repeated partition controller evaluations. It remains an immutable value
	// adapter; the shared activity package still has no PoolPartition knowledge.
	partitionActivityScorer = controlactivity.NewHotnessScorer(controlactivity.HotnessConfig{
		HighGetsPerSecond:     defaultPartitionHighGetsPerSecond,
		HighPutsPerSecond:     defaultPartitionHighPutsPerSecond,
		HighLeaseOpsPerSecond: defaultPartitionHighLeaseOpsPerSecond,
	})
)

// PoolPartitionActivityScore is a normalized recent hotness projection.
type PoolPartitionActivityScore struct {
	// Value is normalized hotness derived from window or smoothed rates.
	Value float64
}

// newPoolPartitionActivityScore projects selected rate signals into hotness.
//
// The current thresholds are conservative scaffolding for controller
// evaluation. They are not adaptive policy, and they do not change active
// registry state; future controller work can replace the adapter inputs without
// changing the shared activity package.
func newPoolPartitionActivityScore(signals partitionScoreSignals) PoolPartitionActivityScore {
	// Lease operation throughput is sampled from LeaseRegistry counters. Pool
	// Get/Put volume is a data-plane signal and must not be inferred as
	// ownership churn.
	value := partitionActivityScorer.Score(controlactivity.HotnessInput{
		GetsPerSecond:     signals.getsPerSecond,
		PutsPerSecond:     signals.putsPerSecond,
		LeaseOpsPerSecond: signals.leaseOpsPerSecond,
	})
	return PoolPartitionActivityScore{Value: value}
}
