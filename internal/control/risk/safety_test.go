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
