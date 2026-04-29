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

// NewScore returns a generic safety risk score.
func NewScore(input Input) Score {
	return NewScoreWithWeights(input, DefaultWeights(), DefaultReturnFailureWeights(), DefaultOwnershipWeights())
}

// NewScoreWithWeights returns a risk score with caller-provided weights.
//
// Closed return failures are intentionally separated from admission/runtime
// failures so shutdown diagnostics do not poison normal adaptive scoring.
// Ownership and misuse signals stay pure projections; they do not mutate
// policy or attempt to repair caller behavior.
func NewScoreWithWeights(input Input, weights Weights, returnWeights ReturnFailureWeights, ownershipWeights OwnershipWeights) Score {
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
