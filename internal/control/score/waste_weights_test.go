package score

import (
	"math"
	"testing"
)

func TestDefaultWasteWeights(t *testing.T) {
	weights := DefaultWasteWeights()
	if weights.LowHit != DefaultWasteLowHitWeight ||
		weights.RetainedPressure != DefaultWasteRetainedPressureWeight ||
		weights.LowActivity != DefaultWasteLowActivityWeight ||
		weights.Drop != DefaultWasteDropWeight {
		t.Fatalf("DefaultWasteWeights() = %+v", weights)
	}
	for name, value := range map[string]float64{
		"low_hit":           weights.LowHit,
		"retained_pressure": weights.RetainedPressure,
		"low_activity":      weights.LowActivity,
		"drop":              weights.Drop,
	} {
		if value <= 0 || math.IsNaN(value) || math.IsInf(value, 0) {
			t.Fatalf("DefaultWasteWeights()[%s] = %v, want positive finite weight", name, value)
		}
	}
}

func TestNormalizeWasteWeights(t *testing.T) {
	defaults := normalizeWasteWeights(WasteWeights{})
	if defaults != DefaultWasteWeights() {
		t.Fatalf("normalizeWasteWeights(zero) = %+v", defaults)
	}
	weights := normalizeWasteWeights(WasteWeights{
		LowHit:           -1,
		RetainedPressure: math.NaN(),
		LowActivity:      math.Inf(1),
		Drop:             2,
	})
	if weights.LowHit != 0 ||
		weights.RetainedPressure != 0 ||
		weights.LowActivity != 0 ||
		weights.Drop != 2 {
		t.Fatalf("normalizeWasteWeights() = %+v", weights)
	}
}
