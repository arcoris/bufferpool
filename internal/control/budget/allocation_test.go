package budget

import "testing"

func TestAllocation(t *testing.T) {
	if ProportionalShare(100, 0, 1) != 0 || ProportionalShare(100, 1, 0) != 0 {
		t.Fatalf("zero score share failed")
	}
	if ProportionalShare(100, 1, 4) != 25 {
		t.Fatalf("normal share failed")
	}
	if ClampTarget(5, 10, 20) != 10 || ClampTarget(25, 10, 20) != 20 || ClampTarget(15, 10, 20) != 15 {
		t.Fatalf("ClampTarget failed")
	}
	if ClampTarget(5, 20, 10) != 10 {
		t.Fatalf("ClampTarget inverted bounds failed")
	}
}
