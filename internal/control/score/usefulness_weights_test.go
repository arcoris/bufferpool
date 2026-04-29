package score

import (
	"math"
	"testing"
)

func TestDefaultUsefulnessWeights(t *testing.T) {
	weights := DefaultUsefulnessWeights()
	if weights.HitRatio != DefaultUsefulnessHitRatioWeight ||
		weights.AllocationAvoidance != DefaultUsefulnessAllocationAvoidanceWeight ||
		weights.RetainRatio != DefaultUsefulnessRetainRatioWeight ||
		weights.Activity != DefaultUsefulnessActivityWeight ||
		weights.DropPenalty != DefaultUsefulnessDropPenaltyWeight {
		t.Fatalf("DefaultUsefulnessWeights() = %+v", weights)
	}
	for name, value := range map[string]float64{
		"hit_ratio":            weights.HitRatio,
		"allocation_avoidance": weights.AllocationAvoidance,
		"retain_ratio":         weights.RetainRatio,
		"activity":             weights.Activity,
		"drop_penalty":         weights.DropPenalty,
	} {
		if value <= 0 || math.IsNaN(value) || math.IsInf(value, 0) {
			t.Fatalf("DefaultUsefulnessWeights()[%s] = %v, want positive finite weight", name, value)
		}
	}
}

func TestNormalizeUsefulnessWeights(t *testing.T) {
	defaults := normalizeUsefulnessWeights(UsefulnessWeights{})
	if defaults != DefaultUsefulnessWeights() {
		t.Fatalf("normalizeUsefulnessWeights(zero) = %+v", defaults)
	}
	weights := normalizeUsefulnessWeights(UsefulnessWeights{
		HitRatio:            -1,
		AllocationAvoidance: math.NaN(),
		RetainRatio:         math.Inf(1),
		Activity:            2,
		DropPenalty:         3,
	})
	if weights.HitRatio != 0 ||
		weights.AllocationAvoidance != 0 ||
		weights.RetainRatio != 0 ||
		weights.Activity != 2 ||
		weights.DropPenalty != 3 {
		t.Fatalf("normalizeUsefulnessWeights() = %+v", weights)
	}
}
