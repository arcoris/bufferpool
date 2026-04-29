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
