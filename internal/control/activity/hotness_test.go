package activity

import "testing"

func TestHotness(t *testing.T) {
	tests := []struct {
		name   string
		input  HotnessInput
		config HotnessConfig
		want   float64
	}{
		{name: "zero config", input: HotnessInput{GetsPerSecond: 10}},
		{name: "one dimension", input: HotnessInput{GetsPerSecond: 5}, config: HotnessConfig{HighGetsPerSecond: 10}, want: 0.5},
		{name: "all dimensions", input: HotnessInput{GetsPerSecond: 5, PutsPerSecond: 10}, config: HotnessConfig{HighGetsPerSecond: 10, HighPutsPerSecond: 10}, want: 0.75},
		{name: "over threshold", input: HotnessInput{GetsPerSecond: 20}, config: HotnessConfig{HighGetsPerSecond: 10}, want: 1},
		{name: "negative input", input: HotnessInput{GetsPerSecond: -1}, config: HotnessConfig{HighGetsPerSecond: 10}, want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Hotness(tt.input, tt.config); got != tt.want {
				t.Fatalf("Hotness() = %v, want %v", got, tt.want)
			}
		})
	}
}
