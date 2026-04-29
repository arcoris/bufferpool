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
	scorer := NewUsefulnessScorer(DefaultUsefulnessWeights())
	input := UsefulnessInput{HitRatio: 0.8, RetainRatio: 0.7, AllocationAvoidance: 0.9, ActivityScore: 0.6, DropPenalty: 0.1}
	if got, want := scorer.Score(input).Value, Usefulness(input).Value; got != want {
		t.Fatalf("UsefulnessScorer.Score() = %v, want %v", got, want)
	}
	if got, want := scorer.ScoreValue(input), Usefulness(input).Value; got != want {
		t.Fatalf("UsefulnessScorer.ScoreValue() = %v, want %v", got, want)
	}
	disabled := NewUsefulnessScorer(UsefulnessWeights{HitRatio: -1, AllocationAvoidance: 1})
	if got := disabled.ScoreValue(UsefulnessInput{HitRatio: 1, AllocationAvoidance: 1}); got != 1 {
		t.Fatalf("sanitized usefulness weights produced %v, want 1", got)
	}
}

func TestUsefulnessScorerZeroValue(t *testing.T) {
	var scorer UsefulnessScorer
	input := UsefulnessInput{HitRatio: 1, RetainRatio: 1, AllocationAvoidance: 1, ActivityScore: 1}
	if got := scorer.Score(input); got.Value != 0 || len(got.Components) != 0 {
		t.Fatalf("zero UsefulnessScorer.Score() = %+v, want zero score", got)
	}
	if got := scorer.ScoreValue(input); got != 0 {
		t.Fatalf("zero UsefulnessScorer.ScoreValue() = %v, want 0", got)
	}
}

func TestUsefulnessScorerScoreValueMatchesScore(t *testing.T) {
	scorer := NewUsefulnessScorer(DefaultUsefulnessWeights())
	input := UsefulnessInput{HitRatio: 0.8, RetainRatio: 0.7, AllocationAvoidance: 0.9, ActivityScore: 0.6, DropPenalty: 0.1}
	if got, want := scorer.ScoreValue(input), scorer.Score(input).Value; got != want {
		t.Fatalf("UsefulnessScorer.ScoreValue() = %v, want %v", got, want)
	}
}

func BenchmarkControlScoreUsefulness(b *testing.B) {
	input := UsefulnessInput{HitRatio: 0.8, RetainRatio: 0.7, AllocationAvoidance: 0.9, ActivityScore: 0.6, DropPenalty: 0.1}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = Usefulness(input)
	}
}

func BenchmarkControlScoreUsefulnessScorer(b *testing.B) {
	input := UsefulnessInput{HitRatio: 0.8, RetainRatio: 0.7, AllocationAvoidance: 0.9, ActivityScore: 0.6, DropPenalty: 0.1}
	scorer := NewUsefulnessScorer(DefaultUsefulnessWeights())
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = scorer.ScoreValue(input)
	}
}
