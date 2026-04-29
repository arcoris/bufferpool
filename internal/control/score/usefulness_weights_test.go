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
