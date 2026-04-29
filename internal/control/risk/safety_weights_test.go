package risk

import (
	"math"
	"testing"
)

func TestDefaultRiskWeights(t *testing.T) {
	weights := DefaultWeights()
	if weights.ReturnFailure != DefaultRiskReturnFailureWeight ||
		weights.Ownership != DefaultRiskOwnershipWeight ||
		weights.Misuse != DefaultRiskMisuseWeight {
		t.Fatalf("DefaultWeights() = %+v", weights)
	}
}

func TestNormalizeRiskWeights(t *testing.T) {
	defaults := normalizeWeights(Weights{})
	if defaults != DefaultWeights() {
		t.Fatalf("normalizeWeights(zero) = %+v", defaults)
	}
	weights := normalizeWeights(Weights{ReturnFailure: -1, Ownership: math.NaN(), Misuse: 2})
	if weights.ReturnFailure != 0 || weights.Ownership != 0 || weights.Misuse != 2 {
		t.Fatalf("normalizeWeights() = %+v", weights)
	}
}
