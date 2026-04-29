package risk

import "testing"

func TestDefaultRiskWeights(t *testing.T) {
	weights := DefaultWeights()
	if weights.ReturnFailure != DefaultRiskReturnFailureWeight ||
		weights.Ownership != DefaultRiskOwnershipWeight ||
		weights.Misuse != DefaultRiskMisuseWeight {
		t.Fatalf("DefaultWeights() = %+v", weights)
	}
}

func TestDefaultReturnFailureWeights(t *testing.T) {
	weights := DefaultReturnFailureWeights()
	if weights.Aggregate != DefaultReturnFailureAggregateWeight ||
		weights.Admission != DefaultReturnFailureAdmissionWeight ||
		weights.Closed != DefaultReturnFailureClosedWeight {
		t.Fatalf("DefaultReturnFailureWeights() = %+v", weights)
	}
}

func TestDefaultOwnershipWeights(t *testing.T) {
	weights := DefaultOwnershipWeights()
	if weights.OwnershipViolation != DefaultOwnershipViolationWeight ||
		weights.DoubleRelease != DefaultOwnershipDoubleReleaseWeight {
		t.Fatalf("DefaultOwnershipWeights() = %+v", weights)
	}
}
