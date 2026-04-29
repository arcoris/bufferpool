package numeric

import (
	"math"
	"testing"
)

func TestSafeRatios(t *testing.T) {
	if got := SafeRatio(1, 0); got != 0 {
		t.Fatalf("SafeRatio zero denominator = %v, want 0", got)
	}
	if got := SafeRatio(3, 2); got != 1.5 {
		t.Fatalf("SafeRatio = %v, want 1.5", got)
	}
	if got := SafeIntRatio(-1, 2); got != -0.5 {
		t.Fatalf("SafeIntRatio = %v, want -0.5", got)
	}
	if got := SafeFloatRatio(math.Inf(1), 1); got != 0 {
		t.Fatalf("SafeFloatRatio inf = %v, want 0", got)
	}
	if got := SafeFloatRatio(math.NaN(), 1); got != 0 {
		t.Fatalf("SafeFloatRatio nan = %v, want 0", got)
	}
}

func BenchmarkControlNumericSafeRatio(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = SafeRatio(uint64(i+1), uint64(i+2))
	}
}
