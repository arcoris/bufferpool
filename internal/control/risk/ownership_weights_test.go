package risk

import (
	"math"
	"testing"
)

func TestDefaultOwnershipWeights(t *testing.T) {
	weights := DefaultOwnershipWeights()
	if weights.OwnershipViolation != DefaultOwnershipViolationWeight ||
		weights.DoubleRelease != DefaultOwnershipDoubleReleaseWeight {
		t.Fatalf("DefaultOwnershipWeights() = %+v", weights)
	}
	for name, value := range map[string]float64{
		"ownership_violation": weights.OwnershipViolation,
		"double_release":      weights.DoubleRelease,
	} {
		if value <= 0 || math.IsNaN(value) || math.IsInf(value, 0) {
			t.Fatalf("DefaultOwnershipWeights()[%s] = %v, want positive finite weight", name, value)
		}
	}
}

func TestNormalizeOwnershipWeights(t *testing.T) {
	defaults := normalizeOwnershipWeights(OwnershipWeights{})
	if defaults != DefaultOwnershipWeights() {
		t.Fatalf("normalizeOwnershipWeights(zero) = %+v", defaults)
	}
	weights := normalizeOwnershipWeights(OwnershipWeights{OwnershipViolation: -1, DoubleRelease: math.Inf(1)})
	if weights.OwnershipViolation != 0 || weights.DoubleRelease != 0 {
		t.Fatalf("normalizeOwnershipWeights() = %+v", weights)
	}
}
