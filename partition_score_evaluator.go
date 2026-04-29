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
	controlactivity "arcoris.dev/bufferpool/internal/control/activity"
	controlnumeric "arcoris.dev/bufferpool/internal/control/numeric"
	controlrisk "arcoris.dev/bufferpool/internal/control/risk"
	controlscore "arcoris.dev/bufferpool/internal/control/score"
)

var (
	// defaultPoolPartitionScoreEvaluator prepares default partition score
	// weights once for the existing NewPoolPartitionScores convenience API. It
	// owns no runtime state and never mutates partition policy.
	defaultPoolPartitionScoreEvaluator = NewPoolPartitionScoreEvaluator(PoolPartitionScoreEvaluatorConfig{})
)

// PoolPartitionUsefulnessScoreWeights configures partition usefulness scoring.
//
// A zero value inside PoolPartitionScoreEvaluatorConfig selects shared
// conservative defaults. A non-zero value is interpreted as an explicit
// partition-domain adapter config and is sanitized by the shared scorer.
type PoolPartitionUsefulnessScoreWeights struct {
	// HitRatio weights direct retained-buffer reuse success.
	HitRatio float64

	// AllocationAvoidance weights avoided allocation and GC pressure.
	AllocationAvoidance float64

	// RetainRatio weights successful returned-buffer retention.
	RetainRatio float64

	// Activity weights recent partition hotness.
	Activity float64

	// DropPenalty weights current-window return drops as a subtractive penalty.
	DropPenalty float64
}

// PoolPartitionWasteScoreWeights configures partition waste scoring.
//
// A zero value inside PoolPartitionScoreEvaluatorConfig selects shared
// conservative defaults. A non-zero value is interpreted as an explicit
// partition-domain adapter config and is sanitized by the shared scorer.
type PoolPartitionWasteScoreWeights struct {
	// LowHit weights retained capacity that is not serving reuse hits.
	LowHit float64

	// RetainedPressure weights retained memory opportunity cost.
	RetainedPressure float64

	// LowActivity weights cold partition workload behavior.
	LowActivity float64

	// Drop weights current-window returned-buffer drop behavior.
	Drop float64
}

// PoolPartitionScoreEvaluatorConfig configures a partition score evaluator.
//
// Zero weight/config groups use shared conservative defaults. This lets callers
// create the default evaluator with PoolPartitionScoreEvaluatorConfig{} while
// still allowing future partition policy to provide explicit domain weights and
// thresholds.
type PoolPartitionScoreEvaluatorConfig struct {
	// UsefulnessWeights configures usefulness score composition.
	UsefulnessWeights PoolPartitionUsefulnessScoreWeights

	// WasteWeights configures waste score composition.
	WasteWeights PoolPartitionWasteScoreWeights

	// ActivityConfig configures partition activity scoring.
	ActivityConfig PoolPartitionActivityScoreConfig

	// RiskConfig configures partition risk scoring.
	RiskConfig PoolPartitionRiskScoreConfig
}

// PoolPartitionScoreEvaluator is the root-domain adapter over shared scorers.
//
// The evaluator is immutable and safe to reuse across controller ticks. It owns
// no runtime state, publishes no runtime policy, executes no trim, and starts no
// background work. The zero value is valid but disabled for evaluator-owned
// usefulness, waste, activity, and risk scoring; construct one with
// NewPoolPartitionScoreEvaluator for default or custom scoring behavior. Budget
// and pressure projections remain pure stateless helpers and are still
// evaluated by a zero-value evaluator.
type PoolPartitionScoreEvaluator struct {
	usefulness controlscore.UsefulnessScorer
	waste      controlscore.WasteScorer
	activity   controlactivity.HotnessScorer
	risk       controlrisk.Scorer
}

// PoolPartitionScoreValues contains scalar partition score values.
//
// It is intended for future tight controller loops that do not need diagnostic
// component copies. The diagnostic Scores method remains the explainable public
// projection.
type PoolPartitionScoreValues struct {
	// Usefulness is the scalar usefulness score.
	Usefulness float64

	// Waste is the scalar waste score.
	Waste float64

	// Budget is the scalar budget pressure score.
	Budget float64

	// Pressure is the scalar pressure severity score.
	Pressure float64

	// Activity is the scalar activity hotness score.
	Activity float64

	// Risk is the scalar ownership and return-path risk score.
	Risk float64
}

// NewPoolPartitionScoreEvaluator returns an immutable score evaluator.
//
// A zero config selects shared conservative defaults. Non-zero weight groups are
// treated as explicit partition scoring configuration and are sanitized by the
// internal control scorers.
func NewPoolPartitionScoreEvaluator(config PoolPartitionScoreEvaluatorConfig) PoolPartitionScoreEvaluator {
	return PoolPartitionScoreEvaluator{
		usefulness: controlscore.NewUsefulnessScorer(config.UsefulnessWeights.controlWeights()),
		waste:      controlscore.NewWasteScorer(config.WasteWeights.controlWeights()),
		activity:   controlactivity.NewHotnessScorer(config.ActivityConfig.controlConfig()),
		risk:       config.RiskConfig.controlScorer(),
	}
}

// Scores returns explainable score projections for partition controller inputs.
//
// The method is a pure projection. It does not mutate policies, publish runtime
// snapshots, execute trim, or retain controller state between calls.
func (e PoolPartitionScoreEvaluator) Scores(
	rates PoolPartitionWindowRates,
	ewma PoolPartitionEWMAState,
	budget PartitionBudgetSnapshot,
	pressure PartitionPressureSnapshot,
) PoolPartitionScores {
	signals := newPartitionScoreSignals(rates, ewma)
	budgetScore := newPoolPartitionBudgetScore(budget)
	pressureScore := newPoolPartitionPressureScore(pressure)
	activityScore := e.activityScore(signals)
	riskScore := e.riskScore(rates)
	return PoolPartitionScores{
		Usefulness: e.usefulnessScore(signals, activityScore, rates),
		Waste:      e.wasteScore(signals, budgetScore, pressureScore, activityScore, rates),
		Budget:     budgetScore,
		Pressure:   pressureScore,
		Activity:   activityScore,
		Risk:       riskScore,
	}
}

// ScoreValues returns scalar score projections without diagnostic component copies.
//
// Use this method for allocation-conscious controller loops that only need
// score values. Use Scores when diagnostic component explanations are needed.
func (e PoolPartitionScoreEvaluator) ScoreValues(
	rates PoolPartitionWindowRates,
	ewma PoolPartitionEWMAState,
	budget PartitionBudgetSnapshot,
	pressure PartitionPressureSnapshot,
) PoolPartitionScoreValues {
	signals := newPartitionScoreSignals(rates, ewma)
	budgetScore := newPoolPartitionBudgetScore(budget)
	pressureScore := newPoolPartitionPressureScore(pressure)
	activityScore := e.activityScore(signals)
	riskScore := e.riskScore(rates)
	return PoolPartitionScoreValues{
		Usefulness: e.usefulnessValue(signals, activityScore, rates),
		Waste:      e.wasteValue(signals, budgetScore, pressureScore, activityScore, rates),
		Budget:     budgetScore.Value,
		Pressure:   pressureScore.Value,
		Activity:   activityScore.Value,
		Risk:       riskScore.Value,
	}
}

// usefulnessScore adapts shared usefulness scoring to partitions.
//
// Allocation avoidance is represented as the inverse of allocation ratio. Drop
// penalty intentionally uses the current window rather than the smoothed signal.
// Drops are immediate admission/pressure feedback and should suppress
// usefulness quickly. This remains a score projection only; it does not mutate
// policy without future hysteresis/cooldown guards.
func (e PoolPartitionScoreEvaluator) usefulnessScore(
	signals partitionScoreSignals,
	activity PoolPartitionActivityScore,
	rates PoolPartitionWindowRates,
) PoolPartitionUsefulnessScore {
	score := e.usefulness.Score(e.usefulnessInput(signals, activity, rates))
	return PoolPartitionUsefulnessScore{
		Value:      score.Value,
		Components: poolPartitionScoreComponents(score.Components),
	}
}

// usefulnessValue returns the scalar partition usefulness score.
func (e PoolPartitionScoreEvaluator) usefulnessValue(
	signals partitionScoreSignals,
	activity PoolPartitionActivityScore,
	rates PoolPartitionWindowRates,
) float64 {
	return e.usefulness.ScoreValue(e.usefulnessInput(signals, activity, rates))
}

// usefulnessInput builds the shared usefulness input from partition signals.
func (e PoolPartitionScoreEvaluator) usefulnessInput(
	signals partitionScoreSignals,
	activity PoolPartitionActivityScore,
	rates PoolPartitionWindowRates,
) controlscore.UsefulnessInput {
	return controlscore.UsefulnessInput{
		HitRatio:            signals.hitRatio,
		RetainRatio:         signals.retainRatio,
		AllocationAvoidance: controlnumeric.Invert01(signals.allocationRatio),
		ActivityScore:       activity.Value,
		DropPenalty:         rates.DropRatio,
	}
}

// wasteScore adapts shared waste scoring to partitions.
//
// Waste combines cold/ineffective workload signals with memory pressure. The
// retained-pressure input uses the stronger of budget utilization and pressure
// severity so either policy boundary can explain a high waste score. DropScore
// intentionally uses the current window so waste can reflect immediate
// return-path pressure. Long-term reuse quality still comes from the selected
// raw-or-EWMA hit/activity signals.
func (e PoolPartitionScoreEvaluator) wasteScore(
	signals partitionScoreSignals,
	budget PoolPartitionBudgetScore,
	pressure PoolPartitionPressureScore,
	activity PoolPartitionActivityScore,
	rates PoolPartitionWindowRates,
) PoolPartitionWasteScore {
	score := e.waste.Score(e.wasteInput(signals, budget, pressure, activity, rates))
	return PoolPartitionWasteScore{
		Value:      score.Value,
		Components: poolPartitionScoreComponents(score.Components),
	}
}

// wasteValue returns the scalar partition waste score.
func (e PoolPartitionScoreEvaluator) wasteValue(
	signals partitionScoreSignals,
	budget PoolPartitionBudgetScore,
	pressure PoolPartitionPressureScore,
	activity PoolPartitionActivityScore,
	rates PoolPartitionWindowRates,
) float64 {
	return e.waste.ScoreValue(e.wasteInput(signals, budget, pressure, activity, rates))
}

// wasteInput builds the shared waste input from partition signals.
func (e PoolPartitionScoreEvaluator) wasteInput(
	signals partitionScoreSignals,
	budget PoolPartitionBudgetScore,
	pressure PoolPartitionPressureScore,
	activity PoolPartitionActivityScore,
	rates PoolPartitionWindowRates,
) controlscore.WasteInput {
	return controlscore.WasteInput{
		LowHitScore:      controlnumeric.Invert01(signals.hitRatio),
		RetainedPressure: controlnumeric.Clamp01(maxFloat64(budget.Value, pressure.Value)),
		LowActivityScore: controlnumeric.Invert01(activity.Value),
		DropScore:        rates.DropRatio,
	}
}

// controlWeights maps root-domain usefulness weights to shared control weights.
func (w PoolPartitionUsefulnessScoreWeights) controlWeights() controlscore.UsefulnessWeights {
	if w == (PoolPartitionUsefulnessScoreWeights{}) {
		return controlscore.DefaultUsefulnessWeights()
	}
	return controlscore.UsefulnessWeights{
		HitRatio:            w.HitRatio,
		AllocationAvoidance: w.AllocationAvoidance,
		RetainRatio:         w.RetainRatio,
		Activity:            w.Activity,
		DropPenalty:         w.DropPenalty,
	}
}

// controlWeights maps root-domain waste weights to shared control weights.
func (w PoolPartitionWasteScoreWeights) controlWeights() controlscore.WasteWeights {
	if w == (PoolPartitionWasteScoreWeights{}) {
		return controlscore.DefaultWasteWeights()
	}
	return controlscore.WasteWeights{
		LowHit:           w.LowHit,
		RetainedPressure: w.RetainedPressure,
		LowActivity:      w.LowActivity,
		Drop:             w.Drop,
	}
}
