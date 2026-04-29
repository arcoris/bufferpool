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
	value := controlactivity.Hotness(
		controlactivity.HotnessInput{
			GetsPerSecond:     signals.getsPerSecond,
			PutsPerSecond:     signals.putsPerSecond,
			LeaseOpsPerSecond: signals.getsPerSecond + signals.putsPerSecond,
		},
		controlactivity.HotnessConfig{
			HighGetsPerSecond:     100_000,
			HighPutsPerSecond:     100_000,
			HighLeaseOpsPerSecond: 200_000,
		},
	)
	return PoolPartitionActivityScore{Value: value}
}
