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

func TestSaturatingAddUint64(t *testing.T) {
	tests := []struct {
		name  string
		left  uint64
		right uint64
		want  uint64
	}{
		{name: "zero", left: 0, right: 0, want: 0},
		{name: "normal", left: 10, right: 20, want: 30},
		{name: "max boundary", left: math.MaxUint64 - 1, right: 1, want: math.MaxUint64},
		{name: "overflow", left: math.MaxUint64, right: 1, want: math.MaxUint64},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SaturatingAddUint64(tt.left, tt.right); got != tt.want {
				t.Fatalf("SaturatingAddUint64() = %v, want %v", got, tt.want)
			}
		})
	}
}

func BenchmarkControlNumericSafeRatio(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = SafeRatio(uint64(i+1), uint64(i+2))
	}
}

func BenchmarkControlNumericSaturatingAddUint64(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = SaturatingAddUint64(uint64(i), uint64(i+1))
	}
}
