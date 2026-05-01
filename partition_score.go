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
	controlnumeric "arcoris.dev/bufferpool/internal/control/numeric"
	controlscore "arcoris.dev/bufferpool/internal/control/score"
)

// PoolPartitionScoreComponent explains one normalized score contribution.
type PoolPartitionScoreComponent struct {
	// Name identifies the signal in diagnostics and test output.
	Name string

	// Value is the normalized component value after clamping to [0, 1].
	Value float64

	// Weight is the non-negative component weight used in the source score.
	Weight float64
}

// PoolPartitionScores groups pure control-plane score projections.
//
// Scores are initial stable heuristics over window rates, EWMA state, budget,
// pressure, activity, and ownership risk. They do not mutate policies, publish
// runtime snapshots, execute trim, or imply final production tuning.
type PoolPartitionScores struct {
	// Usefulness estimates whether retained storage is helping this partition.
	Usefulness PoolPartitionUsefulnessScore

	// Waste estimates whether retained storage is inefficient or cold.
	Waste PoolPartitionWasteScore

	// Budget estimates budget utilization pressure.
	Budget PoolPartitionBudgetScore

	// Pressure maps partition pressure level to normalized severity.
	Pressure PoolPartitionPressureScore

	// Activity estimates recent partition hotness from rates.
	Activity PoolPartitionActivityScore

	// Risk estimates ownership and return-path safety risk.
	Risk PoolPartitionRiskScore
}

// PoolPartitionUsefulnessScore explains useful retained-storage signals.
type PoolPartitionUsefulnessScore struct {
	// Value is the normalized usefulness score.
	Value float64

	// Components explain the score inputs.
	Components []PoolPartitionScoreComponent
}

// PoolPartitionWasteScore explains wasteful retained-storage signals.
type PoolPartitionWasteScore struct {
	// Value is the normalized waste score.
	Value float64

	// Components explain the score inputs.
	Components []PoolPartitionScoreComponent
}

// NewPoolPartitionScores returns score projections for partition controller inputs.
//
// Window rates are the primary adaptive input. EWMA state is used when it has
// already been initialized, otherwise raw window rates keep the first
// evaluation meaningful. Budget and pressure snapshots are domain adapters over
// current partition policy; this function only projects them into scores and
// never publishes a new policy.
func NewPoolPartitionScores(
	rates PoolPartitionWindowRates,
	ewma PoolPartitionEWMAState,
	budget PartitionBudgetSnapshot,
	pressure PartitionPressureSnapshot,
) PoolPartitionScores {
	return defaultPoolPartitionScoreEvaluator.Scores(rates, ewma, budget, pressure)
}

// partitionScoreSignals holds the rate source selected for score formulas.
//
// The fields stay unexported because they are an adapter detail: public callers
// should work with PoolPartitionWindowRates, PoolPartitionEWMAState, and
// PoolPartitionScores. Each value is finite-protected by the shared score and
// numeric helpers before it contributes to a public score.
type partitionScoreSignals struct {
	// hitRatio is the selected reuse hit ratio signal.
	hitRatio float64

	// missRatio is the selected reuse miss ratio signal.
	missRatio float64

	// allocationRatio is the selected allocation-per-acquisition signal.
	allocationRatio float64

	// retainRatio is the selected retained-return signal.
	retainRatio float64

	// getsPerSecond is the selected acquisition throughput signal.
	getsPerSecond float64

	// putsPerSecond is the selected return throughput signal.
	putsPerSecond float64

	// allocationsPerSecond is the selected allocation throughput signal.
	allocationsPerSecond float64

	// leaseOpsPerSecond is the selected ownership operation throughput signal.
	leaseOpsPerSecond float64
}

// newPartitionScoreSignals chooses raw rates or smoothed rates for reuse and
// activity scoring.
//
// A zero-value EWMA state means no previous controller window has been
// smoothed yet, so the current window rates are used directly. Once EWMA is
// initialized, score formulas consume smoothed values to avoid reacting only to
// one noisy window. Drop ratio is intentionally excluded: partition usefulness
// and waste scoring use the current-window drop ratio as immediate
// admission/pressure feedback.
func newPartitionScoreSignals(rates PoolPartitionWindowRates, ewma PoolPartitionEWMAState) partitionScoreSignals {
	if ewma.Initialized {
		return partitionScoreSignals{
			hitRatio:             ewma.HitRatio,
			missRatio:            ewma.MissRatio,
			allocationRatio:      ewma.AllocationRatio,
			retainRatio:          ewma.RetainRatio,
			getsPerSecond:        ewma.GetsPerSecond,
			putsPerSecond:        ewma.PutsPerSecond,
			allocationsPerSecond: ewma.AllocationsPerSecond,
			leaseOpsPerSecond:    ewma.LeaseOpsPerSecond,
		}
	}
	return partitionScoreSignals{
		hitRatio:             rates.HitRatio,
		missRatio:            rates.MissRatio,
		allocationRatio:      rates.AllocationRatio,
		retainRatio:          rates.RetainRatio,
		getsPerSecond:        rates.GetsPerSecond,
		putsPerSecond:        rates.PutsPerSecond,
		allocationsPerSecond: rates.AllocationsPerSecond,
		leaseOpsPerSecond:    rates.LeaseOpsPerSecond,
	}
}

// poolPartitionScoreComponents copies generic score components into root types.
//
// The defensive copy prevents callers from observing or mutating component
// storage owned by the shared scoring package.
func poolPartitionScoreComponents(components []controlscore.Component) []PoolPartitionScoreComponent {
	if len(components) == 0 {
		return nil
	}
	copied := make([]PoolPartitionScoreComponent, len(components))
	for i, component := range components {
		copied[i] = PoolPartitionScoreComponent{
			Name:   component.Name,
			Value:  component.Value,
			Weight: component.Weight,
		}
	}
	return copied
}

func copyPoolPartitionScores(scores PoolPartitionScores) PoolPartitionScores {
	return PoolPartitionScores{
		Usefulness: PoolPartitionUsefulnessScore{
			Value:      controlnumeric.Clamp01(scores.Usefulness.Value),
			Components: copyPoolPartitionScoreComponents(scores.Usefulness.Components),
		},
		Waste: PoolPartitionWasteScore{
			Value:      controlnumeric.Clamp01(scores.Waste.Value),
			Components: copyPoolPartitionScoreComponents(scores.Waste.Components),
		},
		Budget:   scores.Budget,
		Pressure: scores.Pressure,
		Activity: scores.Activity,
		Risk:     scores.Risk,
	}
}

func copyPoolPartitionScoreComponents(components []PoolPartitionScoreComponent) []PoolPartitionScoreComponent {
	if len(components) == 0 {
		return nil
	}
	copied := make([]PoolPartitionScoreComponent, len(components))
	for i, component := range components {
		weight := controlnumeric.FiniteOrZero(component.Weight)
		if weight < 0 {
			weight = 0
		}
		copied[i] = PoolPartitionScoreComponent{
			Name:   component.Name,
			Value:  controlnumeric.Clamp01(component.Value),
			Weight: weight,
		}
	}
	return copied
}

// maxFloat64 returns the largest finite value in values.
//
// Non-finite values are treated as zero so score projection cannot propagate
// NaN or infinity into public diagnostics.
func maxFloat64(values ...float64) float64 {
	var max float64
	for _, value := range values {
		value = controlnumeric.FiniteOrZero(value)
		if value > max {
			max = value
		}
	}
	return max
}
