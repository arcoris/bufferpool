package score

import "testing"

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

func TestDefaultWasteWeights(t *testing.T) {
	weights := DefaultWasteWeights()
	if weights.LowHit != DefaultWasteLowHitWeight ||
		weights.RetainedPressure != DefaultWasteRetainedPressureWeight ||
		weights.LowActivity != DefaultWasteLowActivityWeight ||
		weights.Drop != DefaultWasteDropWeight {
		t.Fatalf("DefaultWasteWeights() = %+v", weights)
	}
}
