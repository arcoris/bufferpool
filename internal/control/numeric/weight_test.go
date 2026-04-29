package numeric

import (
	"math"
	"testing"
)

func TestWeights(t *testing.T) {
	values := []WeightedValue{{Value: 2, Weight: 1}, {Value: 0, Weight: -1}, {Value: 0.5, Weight: 1}}
	if got := WeightedAverage(values); got != 0.75 {
		t.Fatalf("WeightedAverage = %v, want 0.75", got)
	}
	weights := []float64{1, -1, 3}
	normalized := NormalizeWeights(weights)
	if normalized[0] != 0.25 || normalized[1] != 0 || normalized[2] != 0.75 {
		t.Fatalf("NormalizeWeights = %v", normalized)
	}
	if weights[1] != -1 {
		t.Fatalf("NormalizeWeights mutated input")
	}
	if got := WeightedAverage(nil); got != 0 {
		t.Fatalf("WeightedAverage(nil) = %v, want 0", got)
	}
	if got := WeightedAverage([]WeightedValue{{Value: math.Inf(1), Weight: math.Inf(1)}}); got != 0 {
		t.Fatalf("WeightedAverage(non-finite) = %v, want 0", got)
	}
	dst := []float64{9, 9, 9, 9}
	normalized = NormalizeWeightsInto(dst[:0], []float64{math.Inf(1), -1, 2})
	if len(normalized) != 3 || cap(normalized) != cap(dst) {
		t.Fatalf("NormalizeWeightsInto did not reuse dst: len=%d cap=%d", len(normalized), cap(normalized))
	}
	if normalized[0] != 0 || normalized[1] != 0 || normalized[2] != 1 {
		t.Fatalf("NormalizeWeightsInto = %v", normalized)
	}
	zero := NormalizeWeightsInto(normalized, []float64{-1, math.NaN()})
	if zero[0] != 0 || zero[1] != 0 {
		t.Fatalf("NormalizeWeightsInto zero total = %v", zero)
	}
}
