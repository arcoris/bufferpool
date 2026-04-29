package series

import "testing"

func TestCounterDelta(t *testing.T) {
	tests := []struct {
		name     string
		previous uint64
		current  uint64
		delta    uint64
		reset    bool
	}{
		{name: "increase", previous: 1, current: 3, delta: 2},
		{name: "unchanged", previous: 3, current: 3, delta: 0},
		{name: "reset", previous: 5, current: 2, delta: 2, reset: true},
		{name: "max edge", previous: ^uint64(0) - 1, current: ^uint64(0), delta: 1},
		{name: "zero to nonzero", previous: 0, current: 4, delta: 4},
		{name: "nonzero to zero", previous: 4, current: 0, delta: 0, reset: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewCounterDelta(tt.previous, tt.current)
			if got.Delta != tt.delta || got.Reset != tt.reset || DeltaValue(tt.previous, tt.current) != tt.delta {
				t.Fatalf("NewCounterDelta() = %+v, want delta %d reset %v", got, tt.delta, tt.reset)
			}
		})
	}
}

func BenchmarkControlSeriesCounterDelta(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = DeltaValue(uint64(i), uint64(i+1))
	}
}
