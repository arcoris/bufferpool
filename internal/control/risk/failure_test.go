package risk

import "testing"

func TestReturnFailureRisk(t *testing.T) {
	if ReturnFailureRisk(0, 1, 0) <= ReturnFailureRisk(0, 0, 1) {
		t.Fatalf("admission failure should have higher severity than closed failure")
	}
	if got := ReturnFailureRiskWithWeights(0, 0, 1, ReturnFailureWeights{Closed: 1}); got != 1 {
		t.Fatalf("custom closed failure risk = %v, want 1", got)
	}
}
