package risk

import "testing"

func TestOwnershipRisk(t *testing.T) {
	if OwnershipRisk(1, 0) <= OwnershipRisk(0, 1) {
		t.Fatalf("ownership violation should have higher severity than double release")
	}
	if got := OwnershipRiskWithWeights(0, 1, OwnershipWeights{DoubleRelease: 1}); got != 1 {
		t.Fatalf("custom double release risk = %v, want 1", got)
	}
}
