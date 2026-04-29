package risk

import "testing"

func TestReturnFailureRisk(t *testing.T) {
	if ReturnFailureRisk(0, 1, 0) <= ReturnFailureRisk(0, 0, 1) {
		t.Fatalf("admission failure should have higher severity than closed failure")
	}
}
