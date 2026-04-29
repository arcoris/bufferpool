package series

import "testing"

func TestGaugeDelta(t *testing.T) {
	tests := []struct {
		name     string
		previous uint64
		current  uint64
		increase uint64
		decrease uint64
		changed  bool
	}{
		{name: "increase", previous: 1, current: 3, increase: 2, changed: true},
		{name: "decrease", previous: 5, current: 2, decrease: 3, changed: true},
		{name: "unchanged", previous: 3, current: 3},
		{name: "max decrease", previous: ^uint64(0), current: 0, decrease: ^uint64(0), changed: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewGaugeDelta(tt.previous, tt.current)
			if got.Increased != tt.increase || got.Decreased != tt.decrease || got.Changed != tt.changed {
				t.Fatalf("NewGaugeDelta() = %+v", got)
			}
		})
	}
}

func BenchmarkControlSeriesGaugeDelta(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = NewGaugeDelta(uint64(i), uint64(i+1))
	}
}
