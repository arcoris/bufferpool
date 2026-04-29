package stability

import "testing"

func TestCooldown(t *testing.T) {
	if CoolingDown(1, 2, 0) {
		t.Fatalf("zero cooldown should be false")
	}
	if !CoolingDown(1, 2, 3) {
		t.Fatalf("inside cooldown should be true")
	}
	if CoolingDown(1, 4, 3) {
		t.Fatalf("after cooldown should be false")
	}
	if CoolingDown(4, 1, 3) {
		t.Fatalf("current before last should be false")
	}
}
