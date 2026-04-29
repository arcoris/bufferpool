package rate

import "testing"

func TestRatios(t *testing.T) {
	tests := []struct {
		name string
		got  float64
		want float64
	}{
		{name: "hit", got: HitRatio(3, 1), want: 0.75},
		{name: "miss", got: MissRatio(3, 1), want: 0.25},
		{name: "allocation", got: AllocationRatio(2, 4), want: 0.5},
		{name: "retain", got: RetainRatio(3, 6), want: 0.5},
		{name: "drop", got: DropRatio(2, 4), want: 0.5},
		{name: "success", got: SuccessRatio(2, 5), want: 0.4},
		{name: "failure", got: FailureRatio(3, 5), want: 0.6},
		{name: "invalid", got: InvalidRatio(1, 4), want: 0.25},
		{name: "zero denominator", got: AllocationRatio(1, 0), want: 0},
		{name: "numerator greater", got: FailureRatio(2, 1), want: 2},
		{name: "all zero", got: HitRatio(0, 0), want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Fatalf("ratio = %v, want %v", tt.got, tt.want)
			}
		})
	}
}

func BenchmarkControlRateRatios(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = HitRatio(uint64(i+1), uint64(i+2))
	}
}
