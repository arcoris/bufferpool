package score

import "testing"

func TestWeightedScore(t *testing.T) {
	components := []Component{NewComponent("a", 1, 3), NewComponent("b", 0, 1)}
	score := NewWeightedScore(components)
	if score.Value != 0.75 {
		t.Fatalf("NewWeightedScore() = %+v, want 0.75", score)
	}
	components[0].Value = 0
	if score.Components[0].Value != 1 {
		t.Fatalf("components were not defensively copied")
	}
	if dominant := score.DominantComponent(); dominant.Name != "a" {
		t.Fatalf("DominantComponent() = %+v", dominant)
	}
	var dst WeightedScore
	dst.Components = make([]Component, 0, 4)
	fresh := []Component{NewComponent("a", 1, 3), NewComponent("b", 0, 1)}
	into := NewWeightedScoreInto(&dst, fresh)
	if into.Value != score.Value || cap(into.Components) != 4 {
		t.Fatalf("NewWeightedScoreInto() = %+v", into)
	}
	if value := WeightedScoreValue(fresh); value != score.Value {
		t.Fatalf("WeightedScoreValue() = %v, want %v", value, score.Value)
	}
	if !NewWeightedScore(nil).IsZero() {
		t.Fatalf("empty score should be zero")
	}
	if !NewWeightedScore([]Component{NewComponent("zero", 1, 0)}).IsZero() {
		t.Fatalf("zero-weight score should be zero")
	}
}

func BenchmarkControlScoreWeightedScore(b *testing.B) {
	components := []Component{NewComponent("a", 1, 2), NewComponent("b", 0.5, 1)}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = NewWeightedScore(components)
	}
}

func BenchmarkControlScoreWeightedScoreValue(b *testing.B) {
	components := []Component{NewComponent("a", 1, 2), NewComponent("b", 0.5, 1)}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = WeightedScoreValue(components)
	}
}
