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

// PoolGroupScores contains scalar group score values and optional per-partition
// score values.
//
// The first PoolGroup layer intentionally exposes scalar score values only.
// Diagnostic score components remain partition-level concerns for now.
type PoolGroupScores struct {
	// Values contains aggregate group score values.
	Values PoolGroupScoreValues

	// PartitionScores contains optional per-partition scalar score values.
	PartitionScores []PoolGroupPartitionScore
}

// PoolGroupScoreValues contains scalar group score values.
//
// These values are advisory projections. They are not decisions and must not be
// interpreted as permission to mutate partition policies, execute trim, or apply
// group-level budget redistribution.
type PoolGroupScoreValues struct {
	// Usefulness is the aggregate usefulness score.
	Usefulness float64

	// Waste is the aggregate waste score.
	Waste float64

	// Budget is the aggregate budget pressure score.
	Budget float64

	// Pressure is the aggregate pressure severity score.
	Pressure float64

	// Activity is the aggregate activity score.
	Activity float64

	// Risk is the aggregate ownership and return-path risk score.
	Risk float64
}

// PoolGroupPartitionScore ties a group-local partition name to scalar scores.
type PoolGroupPartitionScore struct {
	// Name is the group-local partition name.
	Name string

	// Scores contains scalar partition score values.
	Scores PoolPartitionScoreValues
}

// IsZero reports whether all group score values are zero.
func (s PoolGroupScoreValues) IsZero() bool {
	return s.Usefulness == 0 &&
		s.Waste == 0 &&
		s.Budget == 0 &&
		s.Pressure == 0 &&
		s.Activity == 0 &&
		s.Risk == 0
}
