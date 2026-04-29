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
}
