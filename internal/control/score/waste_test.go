package score

import "testing"

func TestWaste(t *testing.T) {
	high := Waste(WasteInput{LowHitScore: 1, RetainedPressure: 1, LowActivityScore: 1, DropScore: 1})
	if high.Value != 1 {
		t.Fatalf("high waste = %+v", high)
	}
	low := Waste(WasteInput{})
	if !low.IsZero() {
		t.Fatalf("low waste = %+v, want zero", low)
	}
	if dominant := high.DominantComponent(); dominant.Name == "" {
		t.Fatalf("waste should retain component explanation")
	}
}
