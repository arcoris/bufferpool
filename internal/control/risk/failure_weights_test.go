package risk

import (
	"math"
	"testing"
)

func TestDefaultReturnFailureWeights(t *testing.T) {
	weights := DefaultReturnFailureWeights()
	if weights.Aggregate != DefaultReturnFailureAggregateWeight ||
		weights.Admission != DefaultReturnFailureAdmissionWeight ||
		weights.Closed != DefaultReturnFailureClosedWeight {
		t.Fatalf("DefaultReturnFailureWeights() = %+v", weights)
	}
	for name, value := range map[string]float64{
		"aggregate": weights.Aggregate,
		"admission": weights.Admission,
		"closed":    weights.Closed,
	} {
		if value <= 0 || math.IsNaN(value) || math.IsInf(value, 0) {
			t.Fatalf("DefaultReturnFailureWeights()[%s] = %v, want positive finite weight", name, value)
		}
	}
}

func TestNormalizeReturnFailureWeights(t *testing.T) {
	defaults := normalizeReturnFailureWeights(ReturnFailureWeights{})
	if defaults != DefaultReturnFailureWeights() {
		t.Fatalf("normalizeReturnFailureWeights(zero) = %+v", defaults)
	}
	weights := normalizeReturnFailureWeights(ReturnFailureWeights{Aggregate: -1, Admission: math.NaN(), Closed: 2})
	if weights.Aggregate != 0 || weights.Admission != 0 || weights.Closed != 2 {
		t.Fatalf("normalizeReturnFailureWeights() = %+v", weights)
	}
}
