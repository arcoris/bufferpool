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
	if scores.Risk.Value >= 0.60 {
		return newPoolPartitionRecommendation(
			PoolPartitionRecommendationInvestigateOwnership,
			controldecision.KindInvestigate,
			scores.Risk.Value,
			"ownership_risk",
			scores,
		)
	}
	if scores.Pressure.Value >= 0.66 && scores.Waste.Value >= 0.50 {
		return newPoolPartitionRecommendation(
			PoolPartitionRecommendationTrim,
			controldecision.KindTrim,
			(scores.Pressure.Value+scores.Waste.Value)/2,
			"pressure_waste",
			scores,
		)
	}
	if scores.Waste.Value >= 0.75 {
		return newPoolPartitionRecommendation(
			PoolPartitionRecommendationShrinkRetention,
			controldecision.KindShrink,
			scores.Waste.Value,
			"waste",
			scores,
		)
	}
	if scores.Usefulness.Value >= 0.75 && scores.Pressure.Value < 0.33 {
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
