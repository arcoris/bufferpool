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

import controlrisk "arcoris.dev/bufferpool/internal/control/risk"

var (
	// partitionRiskScorer prepares the default shared risk weights for repeated
	// controller projections. It only evaluates safety and misuse signals; it
	// never repairs ownership state or mutates partition policy.
	partitionRiskScorer = controlrisk.DefaultScorer()
)

// PoolPartitionRiskScore explains ownership and Pool handoff risk.
type PoolPartitionRiskScore struct {
	// Value is the normalized total risk.
	Value float64

	// ReturnComponent is the Pool handoff failure contribution.
	ReturnComponent float64

	// OwnershipComponent is the ownership safety contribution.
	OwnershipComponent float64

	// MisuseComponent is the invalid/double-release contribution.
	MisuseComponent float64
}

// newPoolPartitionRiskScore adapts window failure ratios into ownership risk.
//
// Closed-Pool handoff failures and admission/runtime handoff failures are kept
// separate because hard close diagnostics should not carry the same severity as
// unexpected ownership or admission failures.
func newPoolPartitionRiskScore(rates PoolPartitionWindowRates) PoolPartitionRiskScore {
	score := partitionRiskScorer.Score(controlrisk.Input{
		PoolReturnFailureRatio:   rates.PoolReturnFailureRatio,
		PoolReturnAdmissionRatio: rates.PoolReturnAdmissionFailureRatio,
		PoolReturnClosedRatio:    rates.PoolReturnClosedFailureRatio,
		InvalidReleaseRatio:      rates.LeaseInvalidReleaseRatio,
		DoubleReleaseRatio:       rates.LeaseDoubleReleaseRatio,
		OwnershipViolationRatio:  rates.LeaseOwnershipViolationRatio,
	})
	return PoolPartitionRiskScore{
		Value:              score.Value,
		ReturnComponent:    score.ReturnComponent,
		OwnershipComponent: score.OwnershipComponent,
		MisuseComponent:    score.MisuseComponent,
	}
}
