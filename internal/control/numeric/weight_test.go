package numeric

import "testing"

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
}
