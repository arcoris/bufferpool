package risk

import "arcoris.dev/bufferpool/internal/control/numeric"

// ReturnFailureRisk scores Pool handoff failure ratios.
//
// This component is about return-path reliability, not retained-memory
// usefulness. It separates admission/runtime failures from close-related
// failures so shutdown noise can remain lower severity than failures that may
// indicate pressure or policy admission problems.
func ReturnFailureRisk(failureRatio, admissionRatio, closedRatio float64) float64 {
	return ReturnFailureRiskWithWeights(failureRatio, admissionRatio, closedRatio, DefaultReturnFailureWeights())
}

// ReturnFailureRiskWithWeights scores Pool handoff failure ratios with weights.
//
// Admission/runtime failures are normally more actionable for adaptive control
// than closed-Pool failures, because close-time handoff failures can be expected
// during hard or graceful shutdown.
func ReturnFailureRiskWithWeights(failureRatio, admissionRatio, closedRatio float64, weights ReturnFailureWeights) float64 {
	return numeric.WeightedAverage([]numeric.WeightedValue{
		{Value: failureRatio, Weight: weights.Aggregate},
		{Value: admissionRatio, Weight: weights.Admission},
		{Value: closedRatio, Weight: weights.Closed},
	})
}
