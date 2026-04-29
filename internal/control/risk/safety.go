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
	returnComponent := ReturnFailureRisk(
		input.PoolReturnFailureRatio,
		input.PoolReturnAdmissionRatio,
		input.PoolReturnClosedRatio,
	)
	ownershipComponent := OwnershipRisk(input.OwnershipViolationRatio, input.DoubleReleaseRatio)
	misuseComponent := numeric.Clamp01(input.InvalidReleaseRatio*0.5 + input.DoubleReleaseRatio*0.5)
	value := numeric.WeightedAverage([]numeric.WeightedValue{
		{Value: returnComponent, Weight: 0.30},
		{Value: ownershipComponent, Weight: 0.45},
		{Value: misuseComponent, Weight: 0.25},
	})
	return Score{
		Value:              value,
		ReturnComponent:    returnComponent,
		OwnershipComponent: ownershipComponent,
		MisuseComponent:    misuseComponent,
	}
}
