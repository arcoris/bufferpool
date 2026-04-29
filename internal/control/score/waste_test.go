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
	custom := WasteWithWeights(WasteInput{DropScore: 1}, WasteWeights{Drop: 1})
	if custom.Value != 1 {
		t.Fatalf("custom waste = %+v, want 1", custom)
	}
}

func BenchmarkControlScoreWaste(b *testing.B) {
	input := WasteInput{LowHitScore: 0.8, RetainedPressure: 0.7, LowActivityScore: 0.4, DropScore: 0.2}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = Waste(input)
	}
}
