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

var (
	// partitionUsefulnessScorer prepares the default shared usefulness weights
	// for repeated partition controller projections. It is immutable and does
	// not publish runtime policy or retain domain state.
	partitionUsefulnessScorer = controlscore.NewUsefulnessScorer(controlscore.DefaultUsefulnessWeights())

	// partitionWasteScorer prepares the default shared waste weights for
	// repeated partition controller projections. A high score remains a
	// diagnostic signal, not a trim command.
	partitionWasteScorer = controlscore.NewWasteScorer(controlscore.DefaultWasteWeights())
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
	signals := newPartitionScoreSignals(rates, ewma)
	budgetScore := newPoolPartitionBudgetScore(budget)
	pressureScore := newPoolPartitionPressureScore(pressure)
	activityScore := newPoolPartitionActivityScore(signals)
	riskScore := newPoolPartitionRiskScore(rates)
	return PoolPartitionScores{
		Usefulness: newPoolPartitionUsefulnessScore(signals, activityScore, rates),
		Waste:      newPoolPartitionWasteScore(signals, budgetScore, pressureScore, activityScore, rates),
		Budget:     budgetScore,
		Pressure:   pressureScore,
		Activity:   activityScore,
		Risk:       riskScore,
	}
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

	// dropRatio is the selected dropped-return signal.
	dropRatio float64

	// getsPerSecond is the selected acquisition throughput signal.
	getsPerSecond float64

	// putsPerSecond is the selected return throughput signal.
	putsPerSecond float64

	// allocationsPerSecond is the selected allocation throughput signal.
	allocationsPerSecond float64
}

// newPartitionScoreSignals chooses raw rates or smoothed rates for scoring.
//
// A zero-value EWMA state means no previous controller window has been
// smoothed yet, so the current window rates are used directly. Once EWMA is
// initialized, score formulas consume smoothed values to avoid reacting only to
// one noisy window.
func newPartitionScoreSignals(rates PoolPartitionWindowRates, ewma PoolPartitionEWMAState) partitionScoreSignals {
	if ewma.Initialized {
		return partitionScoreSignals{
			hitRatio:             ewma.HitRatio,
			missRatio:            ewma.MissRatio,
			allocationRatio:      ewma.AllocationRatio,
			retainRatio:          ewma.RetainRatio,
			dropRatio:            ewma.DropRatio,
			getsPerSecond:        ewma.GetsPerSecond,
			putsPerSecond:        ewma.PutsPerSecond,
			allocationsPerSecond: ewma.AllocationsPerSecond,
		}
	}
	return partitionScoreSignals{
		hitRatio:             rates.HitRatio,
		missRatio:            rates.MissRatio,
		allocationRatio:      rates.AllocationRatio,
		retainRatio:          rates.RetainRatio,
		dropRatio:            rates.DropRatio,
		getsPerSecond:        rates.GetsPerSecond,
		putsPerSecond:        rates.PutsPerSecond,
		allocationsPerSecond: rates.AllocationsPerSecond,
	}
}

// newPoolPartitionUsefulnessScore adapts shared usefulness scoring to partitions.
//
// Allocation avoidance is represented as the inverse of allocation ratio. Drop
// ratio remains a penalty because frequent drops mean retained storage is less
// useful even if hit and retain ratios are otherwise high.
func newPoolPartitionUsefulnessScore(signals partitionScoreSignals, activity PoolPartitionActivityScore, rates PoolPartitionWindowRates) PoolPartitionUsefulnessScore {
	score := partitionUsefulnessScorer.Score(controlscore.UsefulnessInput{
		HitRatio:            signals.hitRatio,
		RetainRatio:         signals.retainRatio,
		AllocationAvoidance: controlnumeric.Invert01(signals.allocationRatio),
		ActivityScore:       activity.Value,
		DropPenalty:         rates.DropRatio,
	})
	return PoolPartitionUsefulnessScore{
		Value:      score.Value,
		Components: poolPartitionScoreComponents(score.Components),
	}
}

// newPoolPartitionWasteScore adapts shared waste scoring to partitions.
//
// Waste combines cold/ineffective workload signals with memory pressure. The
// retained-pressure input uses the stronger of budget utilization and pressure
// severity so either policy boundary can explain a high waste score.
func newPoolPartitionWasteScore(
	signals partitionScoreSignals,
	budget PoolPartitionBudgetScore,
	pressure PoolPartitionPressureScore,
	activity PoolPartitionActivityScore,
	rates PoolPartitionWindowRates,
) PoolPartitionWasteScore {
	score := partitionWasteScorer.Score(controlscore.WasteInput{
		LowHitScore:      controlnumeric.Invert01(signals.hitRatio),
		RetainedPressure: controlnumeric.Clamp01(maxFloat64(budget.Value, pressure.Value)),
		LowActivityScore: controlnumeric.Invert01(activity.Value),
		DropScore:        rates.DropRatio,
	})
	return PoolPartitionWasteScore{
		Value:      score.Value,
		Components: poolPartitionScoreComponents(score.Components),
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
