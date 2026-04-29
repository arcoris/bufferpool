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
	scorer := NewWasteScorer(DefaultWasteWeights())
	input := WasteInput{LowHitScore: 0.8, RetainedPressure: 0.7, LowActivityScore: 0.4, DropScore: 0.2}
	if got, want := scorer.Score(input).Value, Waste(input).Value; got != want {
		t.Fatalf("WasteScorer.Score() = %v, want %v", got, want)
	}
	if got, want := scorer.ScoreValue(input), Waste(input).Value; got != want {
		t.Fatalf("WasteScorer.ScoreValue() = %v, want %v", got, want)
	}
	disabled := NewWasteScorer(WasteWeights{LowHit: -1, Drop: 1})
	if got := disabled.ScoreValue(WasteInput{LowHitScore: 1, DropScore: 1}); got != 1 {
		t.Fatalf("sanitized waste weights produced %v, want 1", got)
	}
}

func TestWasteScorerZeroValue(t *testing.T) {
	var scorer WasteScorer
	input := WasteInput{LowHitScore: 1, RetainedPressure: 1, LowActivityScore: 1, DropScore: 1}
	if got := scorer.Score(input); got.Value != 0 || len(got.Components) != 0 {
		t.Fatalf("zero WasteScorer.Score() = %+v, want zero score", got)
	}
	if got := scorer.ScoreValue(input); got != 0 {
		t.Fatalf("zero WasteScorer.ScoreValue() = %v, want 0", got)
	}
}

func TestWasteScorerScoreValueMatchesScore(t *testing.T) {
	scorer := NewWasteScorer(DefaultWasteWeights())
	input := WasteInput{LowHitScore: 0.8, RetainedPressure: 0.7, LowActivityScore: 0.4, DropScore: 0.2}
	if got, want := scorer.ScoreValue(input), scorer.Score(input).Value; got != want {
		t.Fatalf("WasteScorer.ScoreValue() = %v, want %v", got, want)
	}
}

func BenchmarkControlScoreWaste(b *testing.B) {
	input := WasteInput{LowHitScore: 0.8, RetainedPressure: 0.7, LowActivityScore: 0.4, DropScore: 0.2}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = Waste(input)
	}
}

func BenchmarkControlScoreWasteScorer(b *testing.B) {
	input := WasteInput{LowHitScore: 0.8, RetainedPressure: 0.7, LowActivityScore: 0.4, DropScore: 0.2}
	scorer := NewWasteScorer(DefaultWasteWeights())
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = scorer.ScoreValue(input)
	}
}
