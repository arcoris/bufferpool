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

import "time"

// PoolGroupWindowRates projects aggregate ratios and optional throughput from a
// PoolGroupWindow.
//
// Group rates are derived from bounded group windows, not lifetime metrics.
// They do not apply EWMA, emit recommendations, mutate policy, or execute trim.
type PoolGroupWindowRates struct {
	// Aggregate contains partition-shaped rates over the group aggregate sample.
	Aggregate PoolPartitionWindowRates
}

// NewPoolGroupWindowRates returns aggregate ratio projections for window.
func NewPoolGroupWindowRates(window PoolGroupWindow) PoolGroupWindowRates {
	return NewPoolGroupTimedWindowRates(window, 0)
}

// NewPoolGroupTimedWindowRates returns aggregate ratio and throughput projections.
//
// If elapsed is zero or negative, throughput fields remain zero. The function
// does not observe wall-clock time and does not mutate group or partition state.
func NewPoolGroupTimedWindowRates(window PoolGroupWindow, elapsed time.Duration) PoolGroupWindowRates {
	partitionWindow := NewPoolPartitionWindow(window.Previous.Aggregate, window.Current.Aggregate)
	return PoolGroupWindowRates{Aggregate: NewPoolPartitionTimedWindowRates(partitionWindow, elapsed)}
}
