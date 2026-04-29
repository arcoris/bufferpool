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
	// scaffolding, not production auto-tuning: it makes very high get throughput
	// visible without letting ordinary traffic saturate activity too easily.
	defaultPartitionHighGetsPerSecond = 100_000

	// defaultPartitionHighPutsPerSecond is the initial "hot" return-flow
	// threshold. It mirrors get throughput so balanced workloads score
	// predictably while avoiding activity inflation from modest return volume.
	defaultPartitionHighPutsPerSecond = 100_000

	// defaultPartitionHighLeaseOpsPerSecond keeps ownership operation volume
	// visible without letting lease churn dominate buffer demand. It is higher
	// than get/put thresholds because acquisitions and releases can both occur
	// in one logical lease cycle. This is a tunable partition-domain threshold.
	defaultPartitionHighLeaseOpsPerSecond = 200_000
)

// PoolPartitionActivityScore is a normalized recent hotness projection.
type PoolPartitionActivityScore struct {
	// Value is normalized hotness derived from window or smoothed rates.
	Value float64
}

// PoolPartitionActivityScoreConfig configures partition hotness projection.
//
// A zero value inside PoolPartitionScoreEvaluatorConfig selects conservative
// partition defaults. A non-zero value is fully explicit: partial configs do
// not inherit per-field defaults, and omitted thresholds stay disabled before
// the shared hotness config normalizer sanitizes weights. Byte throughput is
// disabled by default because partition rates currently expose returned and
// dropped byte throughput separately rather than one domain-owned byte activity
// signal.
type PoolPartitionActivityScoreConfig struct {
	// HighGetsPerSecond normalizes Pool acquisition throughput.
	HighGetsPerSecond float64

	// HighPutsPerSecond normalizes Pool return throughput.
	HighPutsPerSecond float64

	// HighBytesPerSecond normalizes byte throughput when a future adapter
	// supplies one activity byte signal.
	HighBytesPerSecond float64

	// HighLeaseOpsPerSecond normalizes successful LeaseRegistry operation throughput.
	HighLeaseOpsPerSecond float64

	// GetsWeight weights acquisition demand.
	GetsWeight float64

	// PutsWeight weights return flow.
	PutsWeight float64

	// BytesWeight weights byte throughput when enabled.
	BytesWeight float64

	// LeaseOpsWeight weights successful ownership operation churn.
	LeaseOpsWeight float64
}

// activityScore projects selected rate signals into hotness through the evaluator.
//
// The current thresholds are conservative scaffolding for controller
// evaluation. They are not adaptive policy, and they do not change active
// registry state; future controller work can replace the adapter inputs without
// changing the shared activity package.
func (e PoolPartitionScoreEvaluator) activityScore(signals partitionScoreSignals) PoolPartitionActivityScore {
	// Lease operation throughput is sampled from LeaseRegistry counters. Pool
	// Get/Put volume is a data-plane signal and must not be inferred as
	// ownership churn.
	//
	// Byte activity is intentionally not supplied here. ReturnedBytesPerSecond
	// and DroppedBytesPerSecond have different control meanings and remain rate
	// diagnostics until a future domain-owned byte activity adapter defines how
	// to combine them.
	value := e.activity.Score(controlactivity.HotnessInput{
		GetsPerSecond:     signals.getsPerSecond,
		PutsPerSecond:     signals.putsPerSecond,
		LeaseOpsPerSecond: signals.leaseOpsPerSecond,
	})
	return PoolPartitionActivityScore{Value: value}
}

// controlConfig maps root-domain activity config to the shared hotness config.
func (c PoolPartitionActivityScoreConfig) controlConfig() controlactivity.HotnessConfig {
	if c == (PoolPartitionActivityScoreConfig{}) {
		return controlactivity.HotnessConfig{
			HighGetsPerSecond:     defaultPartitionHighGetsPerSecond,
			HighPutsPerSecond:     defaultPartitionHighPutsPerSecond,
			HighLeaseOpsPerSecond: defaultPartitionHighLeaseOpsPerSecond,
		}
	}
	return controlactivity.HotnessConfig{
		HighGetsPerSecond:     c.HighGetsPerSecond,
		HighPutsPerSecond:     c.HighPutsPerSecond,
		HighBytesPerSecond:    c.HighBytesPerSecond,
		HighLeaseOpsPerSecond: c.HighLeaseOpsPerSecond,
		GetsWeight:            c.GetsWeight,
		PutsWeight:            c.PutsWeight,
		BytesWeight:           c.BytesWeight,
		LeaseOpsWeight:        c.LeaseOpsWeight,
	}
}
