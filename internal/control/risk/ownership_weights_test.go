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
