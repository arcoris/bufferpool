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
	for name, value := range map[string]float64{
		"return_failure": weights.ReturnFailure,
		"ownership":      weights.Ownership,
		"misuse":         weights.Misuse,
	} {
		if value <= 0 || math.IsNaN(value) || math.IsInf(value, 0) {
			t.Fatalf("DefaultWeights()[%s] = %v, want positive finite weight", name, value)
		}
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
