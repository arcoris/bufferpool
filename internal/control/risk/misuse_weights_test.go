package risk

import (
	"math"
	"testing"
)

func TestDefaultMisuseWeights(t *testing.T) {
	weights := DefaultMisuseWeights()
	if weights.InvalidRelease <= 0 || weights.DoubleRelease <= 0 {
		t.Fatalf("DefaultMisuseWeights() = %+v, want positive weights", weights)
	}
	if math.IsNaN(weights.InvalidRelease) || math.IsNaN(weights.DoubleRelease) ||
		math.IsInf(weights.InvalidRelease, 0) || math.IsInf(weights.DoubleRelease, 0) {
		t.Fatalf("DefaultMisuseWeights() = %+v, want finite weights", weights)
	}
	if got := MisuseRiskWithWeights(1, 0, weights); got == 0 {
		t.Fatalf("default invalid-release weight should produce nonzero misuse risk")
	}
}

func TestNormalizeMisuseWeights(t *testing.T) {
	if got, want := normalizeMisuseWeights(MisuseWeights{}), DefaultMisuseWeights(); got != want {
		t.Fatalf("normalizeMisuseWeights(zero) = %+v, want %+v", got, want)
	}
	weights := normalizeMisuseWeights(MisuseWeights{InvalidRelease: -1, DoubleRelease: math.NaN()})
	if weights.InvalidRelease != 0 || weights.DoubleRelease != 0 {
		t.Fatalf("normalizeMisuseWeights(invalid) = %+v, want zeroed weights", weights)
	}
	custom := normalizeMisuseWeights(MisuseWeights{InvalidRelease: 2, DoubleRelease: 3})
	if custom.InvalidRelease != 2 || custom.DoubleRelease != 3 {
		t.Fatalf("normalizeMisuseWeights(custom) = %+v, want preserved weights", custom)
	}
}
