package risk

import "testing"

func TestOwnershipRisk(t *testing.T) {
	if OwnershipRisk(1, 0) <= OwnershipRisk(0, 1) {
		t.Fatalf("ownership violation should have higher severity than double release")
	}
}
