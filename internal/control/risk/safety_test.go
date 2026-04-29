package risk

import "testing"

func TestRiskScore(t *testing.T) {
	zero := NewScore(Input{})
	if zero.Value != 0 {
		t.Fatalf("zero risk = %+v", zero)
	}
	ownership := NewScore(Input{OwnershipViolationRatio: 1})
	if ownership.Value < 0.3 || ownership.OwnershipComponent < 0.7 {
		t.Fatalf("ownership risk too low: %+v", ownership)
	}
	doubleRelease := NewScore(Input{DoubleReleaseRatio: 1})
	if doubleRelease.Value == 0 {
		t.Fatalf("double release should produce risk: %+v", doubleRelease)
	}
	closed := NewScore(Input{PoolReturnClosedRatio: 1})
	admission := NewScore(Input{PoolReturnAdmissionRatio: 1})
	if closed.ReturnComponent >= admission.ReturnComponent {
		t.Fatalf("closed failure should be lower severity than admission: closed=%+v admission=%+v", closed, admission)
	}
	clamped := NewScore(Input{PoolReturnFailureRatio: 10, OwnershipViolationRatio: 10, InvalidReleaseRatio: 10})
	if clamped.Value > 1 || clamped.ReturnComponent > 1 || clamped.OwnershipComponent > 1 || clamped.MisuseComponent > 1 {
		t.Fatalf("risk should be clamped: %+v", clamped)
	}
	custom := NewScoreWithWeights(
		Input{PoolReturnClosedRatio: 1},
		Weights{ReturnFailure: 1},
		ReturnFailureWeights{Closed: 1},
		OwnershipWeights{},
	)
	if custom.Value != 1 {
		t.Fatalf("custom risk = %+v, want 1", custom)
	}
	scorer := DefaultScorer()
	input := Input{PoolReturnAdmissionRatio: 0.2, OwnershipViolationRatio: 0.3, InvalidReleaseRatio: 0.1}
	if got, want := scorer.Score(input), NewScore(input); got != want {
		t.Fatalf("Scorer.Score() = %+v, want %+v", got, want)
	}
	customScorer := NewScorer(Weights{ReturnFailure: 1}, ReturnFailureWeights{Closed: 1}, OwnershipWeights{})
	if got := customScorer.Score(Input{PoolReturnClosedRatio: 1}); got.Value != 1 {
		t.Fatalf("custom scorer risk = %+v, want value 1", got)
	}
}

func TestRiskScorerDefaultMisuseWeights(t *testing.T) {
	scorer := DefaultScorer()
	got := scorer.Score(Input{InvalidReleaseRatio: 1})
	if got.MisuseComponent != DefaultMisuseInvalidReleaseWeight {
		t.Fatalf("default misuse component = %v, want %v", got.MisuseComponent, DefaultMisuseInvalidReleaseWeight)
	}
	if got.Value == 0 {
		t.Fatalf("default misuse weights should contribute to aggregate risk: %+v", got)
	}
}

func TestRiskScorerCustomMisuseWeights(t *testing.T) {
	scorer := NewScorerWithMisuseWeights(
		Weights{Misuse: 1},
		ReturnFailureWeights{},
		OwnershipWeights{},
		MisuseWeights{InvalidRelease: 1},
	)
	if got := scorer.Score(Input{InvalidReleaseRatio: 1, DoubleReleaseRatio: 1}); got.Value != 1 || got.MisuseComponent != 1 {
		t.Fatalf("custom invalid-release misuse risk = %+v, want value and component 1", got)
	}
	if got := scorer.Score(Input{DoubleReleaseRatio: 1}); got.Value != 0 || got.MisuseComponent != 0 {
		t.Fatalf("custom misuse weights should ignore double release: %+v", got)
	}
}

func TestRiskScorerDoubleReleaseContributesToOwnershipAndMisuse(t *testing.T) {
	score := NewScore(Input{DoubleReleaseRatio: 1})
	if score.OwnershipComponent == 0 {
		t.Fatalf("double release should contribute to ownership risk: %+v", score)
	}
	if score.MisuseComponent == 0 {
		t.Fatalf("double release should contribute to misuse risk: %+v", score)
	}
	if score.Value == 0 {
		t.Fatalf("double release should contribute to aggregate risk: %+v", score)
	}
}

func TestRiskScorerInvalidReleaseContributesOnlyToMisuse(t *testing.T) {
	score := NewScore(Input{InvalidReleaseRatio: 1})
	if score.MisuseComponent == 0 {
		t.Fatalf("invalid release should contribute to misuse risk: %+v", score)
	}
	if score.OwnershipComponent != 0 {
		t.Fatalf("invalid release should not contribute to ownership risk: %+v", score)
	}
}

func BenchmarkControlRiskScore(b *testing.B) {
	input := Input{
		PoolReturnFailureRatio:   0.1,
		PoolReturnAdmissionRatio: 0.2,
		PoolReturnClosedRatio:    0.05,
		InvalidReleaseRatio:      0.01,
		DoubleReleaseRatio:       0.02,
		OwnershipViolationRatio:  0.03,
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = NewScore(input)
	}
}

func BenchmarkControlRiskScorer(b *testing.B) {
	input := Input{
		PoolReturnFailureRatio:   0.1,
		PoolReturnAdmissionRatio: 0.2,
		PoolReturnClosedRatio:    0.05,
		InvalidReleaseRatio:      0.01,
		DoubleReleaseRatio:       0.02,
		OwnershipViolationRatio:  0.03,
	}
	scorer := DefaultScorer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = scorer.Score(input)
	}
}

func BenchmarkControlRiskScorerCustomMisuseWeights(b *testing.B) {
	input := Input{
		PoolReturnFailureRatio:   0.1,
		PoolReturnAdmissionRatio: 0.2,
		PoolReturnClosedRatio:    0.05,
		InvalidReleaseRatio:      0.01,
		DoubleReleaseRatio:       0.02,
		OwnershipViolationRatio:  0.03,
	}
	scorer := NewScorerWithMisuseWeights(
		DefaultWeights(),
		DefaultReturnFailureWeights(),
		DefaultOwnershipWeights(),
		MisuseWeights{InvalidRelease: 3, DoubleRelease: 1},
	)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = scorer.Score(input)
	}
}
