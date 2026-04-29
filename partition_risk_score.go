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

// PoolPartitionRiskScoreConfig configures partition ownership and handoff risk.
//
// A zero PoolPartitionRiskScoreConfig selects shared conservative defaults. A
// non-zero config is treated as fully explicit: partial configs do not inherit
// per-field defaults, and fields left zero are intentionally passed as zero
// before shared risk scorers sanitize them. This avoids hidden default merging
// and keeps evaluator construction deterministic. The root-domain config keeps
// internal/control types out of the public partition API while preserving one
// explicit evaluator boundary.
type PoolPartitionRiskScoreConfig struct {
	// Weights configures top-level risk component aggregation.
	Weights PoolPartitionRiskScoreWeights

	// ReturnWeights configures Pool return handoff failure risk.
	ReturnWeights PoolPartitionReturnFailureRiskWeights

	// OwnershipWeights configures strict ownership boundary risk.
	OwnershipWeights PoolPartitionOwnershipRiskWeights

	// MisuseWeights configures caller API misuse risk.
	MisuseWeights PoolPartitionMisuseRiskWeights
}

// PoolPartitionRiskScoreWeights configures top-level partition risk aggregation.
type PoolPartitionRiskScoreWeights struct {
	// ReturnFailure weights Pool return handoff failures.
	ReturnFailure float64

	// Ownership weights strict ownership safety failures.
	Ownership float64

	// Misuse weights invalid release and double-release misuse signals.
	Misuse float64
}

// PoolPartitionReturnFailureRiskWeights configures Pool handoff risk.
type PoolPartitionReturnFailureRiskWeights struct {
	// Aggregate weights all Pool return failures.
	Aggregate float64

	// Admission weights non-closed admission/runtime handoff failures.
	Admission float64

	// Closed weights close-related handoff failures.
	Closed float64
}

// PoolPartitionOwnershipRiskWeights configures ownership safety risk.
type PoolPartitionOwnershipRiskWeights struct {
	// OwnershipViolation weights strict ownership violations.
	OwnershipViolation float64

	// DoubleRelease weights attempts to release the same lease more than once.
	DoubleRelease float64
}

// PoolPartitionMisuseRiskWeights configures caller misuse risk.
type PoolPartitionMisuseRiskWeights struct {
	// InvalidRelease weights malformed release attempts.
	InvalidRelease float64

	// DoubleRelease weights double-release caller misuse.
	DoubleRelease float64
}

// riskScore adapts window failure ratios into ownership risk through the evaluator.
//
// Closed-Pool handoff failures and admission/runtime handoff failures are kept
// separate because hard close diagnostics should not carry the same severity as
// unexpected ownership or admission failures.
func (e PoolPartitionScoreEvaluator) riskScore(rates PoolPartitionWindowRates) PoolPartitionRiskScore {
	score := e.risk.Score(controlrisk.Input{
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

// controlScorer maps root-domain risk config to the shared prepared scorer.
func (c PoolPartitionRiskScoreConfig) controlScorer() controlrisk.Scorer {
	if c == (PoolPartitionRiskScoreConfig{}) {
		return controlrisk.DefaultScorer()
	}
	return controlrisk.NewExplicitScorer(
		c.Weights.controlWeights(),
		c.ReturnWeights.controlWeights(),
		c.OwnershipWeights.controlWeights(),
		c.MisuseWeights.controlWeights(),
	)
}

// controlWeights maps root-domain top-level risk weights to shared weights.
func (w PoolPartitionRiskScoreWeights) controlWeights() controlrisk.Weights {
	return controlrisk.Weights{
		ReturnFailure: w.ReturnFailure,
		Ownership:     w.Ownership,
		Misuse:        w.Misuse,
	}
}

// controlWeights maps root-domain return-failure weights to shared weights.
func (w PoolPartitionReturnFailureRiskWeights) controlWeights() controlrisk.ReturnFailureWeights {
	return controlrisk.ReturnFailureWeights{
		Aggregate: w.Aggregate,
		Admission: w.Admission,
		Closed:    w.Closed,
	}
}

// controlWeights maps root-domain ownership weights to shared weights.
func (w PoolPartitionOwnershipRiskWeights) controlWeights() controlrisk.OwnershipWeights {
	return controlrisk.OwnershipWeights{
		OwnershipViolation: w.OwnershipViolation,
		DoubleRelease:      w.DoubleRelease,
	}
}

// controlWeights maps root-domain misuse weights to shared weights.
func (w PoolPartitionMisuseRiskWeights) controlWeights() controlrisk.MisuseWeights {
	return controlrisk.MisuseWeights{
		InvalidRelease: w.InvalidRelease,
		DoubleRelease:  w.DoubleRelease,
	}
}
