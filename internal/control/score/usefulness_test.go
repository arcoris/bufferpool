package score

import "testing"

func TestUsefulness(t *testing.T) {
	high := Usefulness(UsefulnessInput{HitRatio: 1, RetainRatio: 1, AllocationAvoidance: 1, ActivityScore: 1})
	if high.Value != 1 {
		t.Fatalf("high usefulness = %+v", high)
	}
	lowHit := Usefulness(UsefulnessInput{RetainRatio: 1, AllocationAvoidance: 1, ActivityScore: 1})
	if lowHit.Value >= high.Value {
		t.Fatalf("low hit score should reduce usefulness: %+v", lowHit)
	}
	penalized := Usefulness(UsefulnessInput{HitRatio: 1, RetainRatio: 1, AllocationAvoidance: 1, ActivityScore: 1, DropPenalty: 1})
	if penalized.Value >= high.Value {
		t.Fatalf("drop penalty should reduce usefulness: %+v", penalized)
	}
	custom := UsefulnessWithWeights(UsefulnessInput{HitRatio: 1}, UsefulnessWeights{HitRatio: 1})
	if custom.Value != 1 {
		t.Fatalf("custom usefulness = %+v, want 1", custom)
	}
	if !Usefulness(UsefulnessInput{}).IsZero() {
		t.Fatalf("zero usefulness should be zero")
	}
}

func BenchmarkControlScoreUsefulness(b *testing.B) {
	input := UsefulnessInput{HitRatio: 0.8, RetainRatio: 0.7, AllocationAvoidance: 0.9, ActivityScore: 0.6, DropPenalty: 0.1}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = Usefulness(input)
	}
}
