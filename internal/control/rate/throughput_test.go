package rate

import (
	"testing"
	"time"
)

func TestThroughput(t *testing.T) {
	tests := []struct {
		name    string
		delta   uint64
		elapsed time.Duration
		want    float64
	}{
		{name: "one second", delta: 10, elapsed: time.Second, want: 10},
		{name: "half second", delta: 10, elapsed: 500 * time.Millisecond, want: 20},
		{name: "zero duration", delta: 10, elapsed: 0, want: 0},
		{name: "negative duration", delta: 10, elapsed: -time.Second, want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := PerSecond(tt.delta, tt.elapsed); got != tt.want {
				t.Fatalf("PerSecond() = %v, want %v", got, tt.want)
			}
			if got := BytesPerSecond(tt.delta, tt.elapsed); got != tt.want {
				t.Fatalf("BytesPerSecond() = %v, want %v", got, tt.want)
			}
		})
	}
}
