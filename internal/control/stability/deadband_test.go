package stability

import (
	"math"
	"testing"
)

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
	if !WithinDeadband(math.Inf(1), 0, 0) {
		t.Fatalf("non-finite values should be sanitized to zero")
	}
}
