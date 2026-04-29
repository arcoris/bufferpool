package risk

import "arcoris.dev/bufferpool/internal/control/numeric"

// Input contains normalized risk ratios.
type Input struct {
	// PoolReturnFailureRatio covers all failed post-release Pool handoffs.
	PoolReturnFailureRatio float64

	// PoolReturnAdmissionRatio covers non-closed admission/runtime handoff failures.
	PoolReturnAdmissionRatio float64

	// PoolReturnClosedRatio covers closed-Pool handoff failures.
	PoolReturnClosedRatio float64

	// InvalidReleaseRatio covers malformed release attempts.
	InvalidReleaseRatio float64

	// DoubleReleaseRatio covers double-release attempts.
	DoubleReleaseRatio float64

	// OwnershipViolationRatio covers strict ownership violations.
	OwnershipViolationRatio float64
}

// Score is a normalized risk score with component explanation.
type Score struct {
	// Value is the weighted normalized risk.
	Value float64

	// ReturnComponent is the Pool handoff failure contribution.
	ReturnComponent float64

	// OwnershipComponent is the ownership safety contribution.
	OwnershipComponent float64

	// MisuseComponent is the caller misuse contribution.
	MisuseComponent float64
}

// Scorer is a prepared risk evaluator with stable weights.
//
// Risk is a safety and misuse projection, not a retention-efficiency score.
// Root controllers should use it to suppress risky recommendations or surface
// diagnostics, not to mutate ownership state.
type Scorer struct {
	weights          Weights
	returnWeights    ReturnFailureWeights
	ownershipWeights OwnershipWeights
	misuseWeights    MisuseWeights
}

// DefaultScorer returns a scorer with conservative default risk weights.
func DefaultScorer() Scorer {
	return NewScorer(DefaultWeights(), DefaultReturnFailureWeights(), DefaultOwnershipWeights())
}

// NewScorer returns a prepared scorer with sanitized stable weights.
//
// Misuse scoring uses DefaultMisuseWeights so existing callers can keep the
// compact constructor. Use NewScorerWithMisuseWeights when caller-misuse
// weighting is part of a stable controller configuration.
func NewScorer(weights Weights, returnWeights ReturnFailureWeights, ownershipWeights OwnershipWeights) Scorer {
	return NewScorerWithMisuseWeights(weights, returnWeights, ownershipWeights, DefaultMisuseWeights())
}

// NewScorerWithMisuseWeights returns a prepared scorer with explicit misuse
// weights.
//
// Keeping misuse weights explicit is useful for controller experiments where
// invalid-release noise and double-release severity should be tuned separately
// without changing return-failure or ownership weighting.
func NewScorerWithMisuseWeights(
	weights Weights,
	returnWeights ReturnFailureWeights,
	ownershipWeights OwnershipWeights,
	misuseWeights MisuseWeights,
) Scorer {
	return Scorer{
		weights:          normalizeWeights(weights),
		returnWeights:    normalizeReturnFailureWeights(returnWeights),
		ownershipWeights: normalizeOwnershipWeights(ownershipWeights),
		misuseWeights:    normalizeMisuseWeights(misuseWeights),
	}
}

// Score returns the normalized risk score for input.
func (s Scorer) Score(input Input) Score {
	return newScoreWithNormalizedWeights(input, s.weights, s.returnWeights, s.ownershipWeights, s.misuseWeights)
}

// NewScore returns a generic safety risk score.
func NewScore(input Input) Score {
	return DefaultScorer().Score(input)
}

// NewScoreWithWeights returns a risk score with caller-provided weights.
//
// Closed return failures are intentionally separated from admission/runtime
// failures so shutdown diagnostics do not poison normal adaptive scoring.
// Ownership and misuse signals stay pure projections; they do not mutate
// policy or attempt to repair caller behavior.
func NewScoreWithWeights(input Input, weights Weights, returnWeights ReturnFailureWeights, ownershipWeights OwnershipWeights) Score {
	return NewScorer(weights, returnWeights, ownershipWeights).Score(input)
}

// NewScoreWithMisuseWeights returns a risk score with explicit caller-misuse
// weights while keeping the other risk weight groups caller-provided.
func NewScoreWithMisuseWeights(
	input Input,
	weights Weights,
	returnWeights ReturnFailureWeights,
	ownershipWeights OwnershipWeights,
	misuseWeights MisuseWeights,
) Score {
	return NewScorerWithMisuseWeights(weights, returnWeights, ownershipWeights, misuseWeights).Score(input)
}

// newScoreWithNormalizedWeights evaluates risk after Scorer has sanitized all
// configured weights.
//
// Double release intentionally contributes to both ownership and misuse
// components. It is an ownership-boundary signal because the same lease is
// released more than once, and it is also caller API misuse because the caller
// violated the release contract. Keeping both contributions visible makes the
// aggregate risk score explainable while still allowing weights to tune the
// relative severity.
func newScoreWithNormalizedWeights(
	input Input,
	weights Weights,
	returnWeights ReturnFailureWeights,
	ownershipWeights OwnershipWeights,
	misuseWeights MisuseWeights,
) Score {
	returnComponent := ReturnFailureRiskWithWeights(
		input.PoolReturnFailureRatio,
		input.PoolReturnAdmissionRatio,
		input.PoolReturnClosedRatio,
		returnWeights,
	)
	ownershipComponent := OwnershipRiskWithWeights(input.OwnershipViolationRatio, input.DoubleReleaseRatio, ownershipWeights)
	misuseComponent := MisuseRiskWithWeights(input.InvalidReleaseRatio, input.DoubleReleaseRatio, misuseWeights)
	value := numeric.WeightedAverage([]numeric.WeightedValue{
		{Value: returnComponent, Weight: weights.ReturnFailure},
		{Value: ownershipComponent, Weight: weights.Ownership},
		{Value: misuseComponent, Weight: weights.Misuse},
	})
	return Score{
		Value:              value,
		ReturnComponent:    returnComponent,
		OwnershipComponent: ownershipComponent,
		MisuseComponent:    misuseComponent,
	}
}
