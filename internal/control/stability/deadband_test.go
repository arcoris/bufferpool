package stability

import "testing"

func TestDeadband(t *testing.T) {
	if !WithinDeadband(1, 1.05, 0.1) {
		t.Fatalf("value should be within deadband")
	}
	if WithinDeadband(1, 1.2, 0.1) {
		t.Fatalf("value should be outside deadband")
	}
	if !WithinDeadband(1, 1, -1) {
		t.Fatalf("negative band should be treated as zero")
	}
}
