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
	controldecision "arcoris.dev/bufferpool/internal/control/decision"
	controlnumeric "arcoris.dev/bufferpool/internal/control/numeric"
)

const (
	// DefaultRecommendationHighRiskThreshold prioritizes ownership and misuse
	// diagnostics before retention tuning. Risk should be clearly high before it
	// produces an investigative recommendation.
	DefaultRecommendationHighRiskThreshold = 0.70

	// DefaultRecommendationHighWasteThreshold requires a strong waste signal
	// before recommending shrink or trim consideration.
	DefaultRecommendationHighWasteThreshold = 0.65

	// DefaultRecommendationHighPressureThreshold requires meaningful pressure
	// before waste becomes a trim recommendation instead of ordinary shrink
	// consideration.
	DefaultRecommendationHighPressureThreshold = 0.70

	// DefaultRecommendationHighUsefulnessThreshold avoids recommending growth
	// from weak or ambiguous usefulness signals.
	DefaultRecommendationHighUsefulnessThreshold = 0.70

	// DefaultRecommendationLowPressureThreshold keeps growth recommendations out
	// of pressure windows where retained memory has elevated opportunity cost.
	DefaultRecommendationLowPressureThreshold = 0.33

	// DefaultRecommendationMinimumActionConfidence keeps observe/no-op as the
	// default when signals are too weak for actionable advice.
	DefaultRecommendationMinimumActionConfidence = 0.55

	// recommendationPressureWasteConfidenceSignalCount documents that trim
	// confidence uses the arithmetic mean of exactly two independent signals:
	// pressure and waste. The pair average avoids letting one high signal hide a
	// weak companion signal before recommending trim planning.
	recommendationPressureWasteConfidenceSignalCount = 2
)

// PoolPartitionRecommendationKind identifies a partition-local recommendation.
type PoolPartitionRecommendationKind uint8

const (
	// PoolPartitionRecommendationNone means no recommendation was produced.
	PoolPartitionRecommendationNone PoolPartitionRecommendationKind = iota

	// PoolPartitionRecommendationObserve means continue observing without mutation.
	PoolPartitionRecommendationObserve

	// PoolPartitionRecommendationGrowRetention recommends considering larger retention.
	PoolPartitionRecommendationGrowRetention

	// PoolPartitionRecommendationShrinkRetention recommends considering smaller retention.
	PoolPartitionRecommendationShrinkRetention

	// PoolPartitionRecommendationTrim recommends planning trim work.
	PoolPartitionRecommendationTrim

	// PoolPartitionRecommendationInvestigateOwnership recommends investigating ownership risk.
	PoolPartitionRecommendationInvestigateOwnership
)

// PoolPartitionRecommendation is a pure recommendation projection.
//
// Recommendations do not mutate policies, execute trim, publish runtime
// snapshots, or start controller loops. They are explainable outputs for future
// controller decision code.
type PoolPartitionRecommendation struct {
	// Kind is the partition recommendation kind.
	Kind PoolPartitionRecommendationKind

	// Confidence is a normalized confidence value.
	Confidence float64

	// Reason is a stable diagnostic reason.
	Reason string

	// Scores are the inputs used to derive the recommendation.
	Scores PoolPartitionScores
}

// NewPoolPartitionRecommendation derives a pure recommendation from scores.
//
// The priority order is safety first, then memory pressure, then waste, then
// useful growth. That ordering keeps correctness diagnostics from being hidden
// by retention-tuning signals. The result is advice only; it does not mutate
// policies or execute trim.
func NewPoolPartitionRecommendation(scores PoolPartitionScores) PoolPartitionRecommendation {
	if scores.Risk.Value >= DefaultRecommendationHighRiskThreshold &&
		scores.Risk.Value >= DefaultRecommendationMinimumActionConfidence {
		return newPoolPartitionRecommendation(
			PoolPartitionRecommendationInvestigateOwnership,
			controldecision.KindInvestigate,
			scores.Risk.Value,
			"ownership_risk",
			scores,
		)
	}
	if scores.Pressure.Value >= DefaultRecommendationHighPressureThreshold &&
		scores.Waste.Value >= DefaultRecommendationHighWasteThreshold &&
		pressureWasteConfidence(scores) >= DefaultRecommendationMinimumActionConfidence {
		return newPoolPartitionRecommendation(
			PoolPartitionRecommendationTrim,
			controldecision.KindTrim,
			pressureWasteConfidence(scores),
			"pressure_waste",
			scores,
		)
	}
	if scores.Waste.Value >= DefaultRecommendationHighWasteThreshold &&
		scores.Waste.Value >= DefaultRecommendationMinimumActionConfidence {
		return newPoolPartitionRecommendation(
			PoolPartitionRecommendationShrinkRetention,
			controldecision.KindShrink,
			scores.Waste.Value,
			"waste",
			scores,
		)
	}
	if scores.Usefulness.Value >= DefaultRecommendationHighUsefulnessThreshold &&
		scores.Pressure.Value < DefaultRecommendationLowPressureThreshold &&
		scores.Usefulness.Value >= DefaultRecommendationMinimumActionConfidence {
		return newPoolPartitionRecommendation(
			PoolPartitionRecommendationGrowRetention,
			controldecision.KindGrow,
			scores.Usefulness.Value,
			"usefulness",
			scores,
		)
	}
	return newPoolPartitionRecommendation(
		PoolPartitionRecommendationObserve,
		controldecision.KindObserve,
		maxFloat64(scores.Usefulness.Value, scores.Waste.Value, scores.Pressure.Value, scores.Risk.Value),
		"observe",
		scores,
	)
}

// pressureWasteConfidence returns the paired-signal confidence used for trim
// recommendations.
//
// Trim planning should require both memory pressure and waste. Averaging the
// two normalized scores is a structural pair operation, not a tunable scoring
// coefficient; recommendation thresholds still control whether the averaged
// signal becomes actionable.
func pressureWasteConfidence(scores PoolPartitionScores) float64 {
	return (scores.Pressure.Value + scores.Waste.Value) / recommendationPressureWasteConfidenceSignalCount
}

// newPoolPartitionRecommendation maps a generic decision to a root recommendation.
//
// The control decision package owns generic confidence clamping while this
// adapter preserves partition-specific kind names and attaches the source
// scores used for diagnostics.
func newPoolPartitionRecommendation(
	kind PoolPartitionRecommendationKind,
	controlKind controldecision.Kind,
	confidence float64,
	reason string,
	scores PoolPartitionScores,
) PoolPartitionRecommendation {
	recommendation := controldecision.NewRecommendation(controlKind, confidence, reason)
	return PoolPartitionRecommendation{
		Kind:       kind,
		Confidence: controlnumeric.Clamp01(recommendation.Confidence),
		Reason:     recommendation.Reason,
		Scores:     scores,
	}
}
