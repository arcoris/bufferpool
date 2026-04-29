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
}

// DefaultScorer returns a scorer with conservative default risk weights.
func DefaultScorer() Scorer {
	return NewScorer(DefaultWeights(), DefaultReturnFailureWeights(), DefaultOwnershipWeights())
}

// NewScorer returns a prepared scorer with sanitized stable weights.
func NewScorer(weights Weights, returnWeights ReturnFailureWeights, ownershipWeights OwnershipWeights) Scorer {
	if weights == (Weights{}) {
		weights = DefaultWeights()
	}
	if returnWeights == (ReturnFailureWeights{}) {
		returnWeights = DefaultReturnFailureWeights()
	}
	if ownershipWeights == (OwnershipWeights{}) {
		ownershipWeights = DefaultOwnershipWeights()
	}
	weights.ReturnFailure = usableRiskWeight(weights.ReturnFailure)
	weights.Ownership = usableRiskWeight(weights.Ownership)
	weights.Misuse = usableRiskWeight(weights.Misuse)
	returnWeights.Aggregate = usableRiskWeight(returnWeights.Aggregate)
	returnWeights.Admission = usableRiskWeight(returnWeights.Admission)
	returnWeights.Closed = usableRiskWeight(returnWeights.Closed)
	ownershipWeights.OwnershipViolation = usableRiskWeight(ownershipWeights.OwnershipViolation)
	ownershipWeights.DoubleRelease = usableRiskWeight(ownershipWeights.DoubleRelease)
	return Scorer{
		weights:          weights,
		returnWeights:    returnWeights,
		ownershipWeights: ownershipWeights,
	}
}

// Score returns the normalized risk score for input.
func (s Scorer) Score(input Input) Score {
	return newScoreWithNormalizedWeights(input, s.weights, s.returnWeights, s.ownershipWeights)
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

// newScoreWithNormalizedWeights evaluates risk after Scorer has sanitized all
// configured weights.
//
// Double release contributes to both ownership and misuse components. That is
// intentional: it is both a checked ownership-boundary signal and caller API
// misuse.
func newScoreWithNormalizedWeights(input Input, weights Weights, returnWeights ReturnFailureWeights, ownershipWeights OwnershipWeights) Score {
	returnComponent := ReturnFailureRiskWithWeights(
		input.PoolReturnFailureRatio,
		input.PoolReturnAdmissionRatio,
		input.PoolReturnClosedRatio,
		returnWeights,
	)
	ownershipComponent := OwnershipRiskWithWeights(input.OwnershipViolationRatio, input.DoubleReleaseRatio, ownershipWeights)
	misuseComponent := numeric.Clamp01(input.InvalidReleaseRatio*0.5 + input.DoubleReleaseRatio*0.5)
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

// usableRiskWeight normalizes optional risk weights.
//
// Negative and non-finite weights are treated as zero to keep risk projections
// finite and non-inverting.
func usableRiskWeight(weight float64) float64 {
	weight = numeric.FiniteOrZero(weight)
	if weight < 0 {
		return 0
	}
	return weight
}
