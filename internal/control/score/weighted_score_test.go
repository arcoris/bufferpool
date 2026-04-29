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
	if !NewWeightedScore(nil).IsZero() {
		t.Fatalf("empty score should be zero")
	}
	if !NewWeightedScore([]Component{NewComponent("zero", 1, 0)}).IsZero() {
		t.Fatalf("zero-weight score should be zero")
	}
}

func BenchmarkControlScoreWeighted(b *testing.B) {
	components := []Component{NewComponent("a", 1, 2), NewComponent("b", 0.5, 1)}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = NewWeightedScore(components)
	}
}
