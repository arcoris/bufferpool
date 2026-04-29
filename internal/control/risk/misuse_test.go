package risk

import (
	"math"
	"testing"
)

func TestMisuseRiskWithWeightsDefault(t *testing.T) {
	if got := MisuseRiskWithWeights(1, 1, DefaultMisuseWeights()); got != 1 {
		t.Fatalf("MisuseRiskWithWeights(default) = %v, want 1", got)
	}
	if got, want := MisuseRisk(1, 0), DefaultMisuseInvalidReleaseWeight; got != want {
		t.Fatalf("MisuseRisk(default invalid only) = %v, want %v", got, want)
	}
}

func TestMisuseRiskWithWeightsInvalidReleaseOnly(t *testing.T) {
	got := MisuseRiskWithWeights(1, 0, DefaultMisuseWeights())
	if got != DefaultMisuseInvalidReleaseWeight {
		t.Fatalf("invalid-release misuse risk = %v, want %v", got, DefaultMisuseInvalidReleaseWeight)
	}
}

func TestMisuseRiskWithWeightsDoubleReleaseOnly(t *testing.T) {
	got := MisuseRiskWithWeights(0, 1, DefaultMisuseWeights())
	if got != DefaultMisuseDoubleReleaseWeight {
		t.Fatalf("double-release misuse risk = %v, want %v", got, DefaultMisuseDoubleReleaseWeight)
	}
}

func TestMisuseRiskWithWeightsZeroWeights(t *testing.T) {
	if got := MisuseRiskWithWeights(1, 1, MisuseWeights{}); got != 0 {
		t.Fatalf("zero misuse weights = %v, want 0", got)
	}
}

func TestMisuseRiskWithWeightsCustomWeights(t *testing.T) {
	weights := MisuseWeights{InvalidRelease: 3, DoubleRelease: 1}
	if got := MisuseRiskWithWeights(1, 0, weights); got != 0.75 {
		t.Fatalf("custom invalid-release misuse risk = %v, want 0.75", got)
	}
	if got := MisuseRiskWithWeights(0, 1, weights); got != 0.25 {
		t.Fatalf("custom double-release misuse risk = %v, want 0.25", got)
	}
}

func TestMisuseRiskWithWeightsNonFiniteWeights(t *testing.T) {
	weights := MisuseWeights{InvalidRelease: math.NaN(), DoubleRelease: math.Inf(1)}
	if got := MisuseRiskWithWeights(1, 1, weights); got != 0 {
		t.Fatalf("non-finite misuse weights = %v, want 0", got)
	}
	weights = MisuseWeights{InvalidRelease: math.NaN(), DoubleRelease: 1}
	if got := MisuseRiskWithWeights(1, 0.4, weights); got != 0.4 {
		t.Fatalf("mixed finite misuse weights = %v, want 0.4", got)
	}
}

func BenchmarkControlRiskMisuseRiskWithWeights(b *testing.B) {
	weights := DefaultMisuseWeights()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = MisuseRiskWithWeights(0.01, 0.02, weights)
	}
}
