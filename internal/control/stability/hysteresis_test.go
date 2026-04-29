package stability

import (
	"math"
	"testing"
)

func TestHysteresis(t *testing.T) {
	h := Hysteresis{Enter: 0.8, Exit: 0.2}
	if err := h.Validate(); err != nil {
		t.Fatalf("Validate() = %v", err)
	}
	if !h.ShouldEnter(0.8) || !h.ShouldExit(0.2) {
		t.Fatalf("boundary thresholds failed")
	}
	if err := (Hysteresis{Enter: 0.1, Exit: 0.2}).Validate(); err == nil {
		t.Fatalf("invalid ordering should fail")
	}
	if err := (Hysteresis{Enter: math.NaN(), Exit: 0}).Validate(); err == nil {
		t.Fatalf("non-finite threshold should fail")
	}
}

func BenchmarkControlStabilityHysteresis(b *testing.B) {
	h := Hysteresis{Enter: 0.8, Exit: 0.2}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = h.ShouldEnter(0.9)
		_ = h.ShouldExit(0.1)
	}
}
