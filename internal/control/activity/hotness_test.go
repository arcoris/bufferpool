package activity

import (
	"math"
	"testing"
)

func TestHotness(t *testing.T) {
	tests := []struct {
		name   string
		input  HotnessInput
		config HotnessConfig
		want   float64
	}{
		{name: "zero config", input: HotnessInput{GetsPerSecond: 10}},
		{name: "one dimension", input: HotnessInput{GetsPerSecond: 5}, config: HotnessConfig{HighGetsPerSecond: 10}, want: 0.5},
		{name: "all dimensions", input: HotnessInput{GetsPerSecond: 5, PutsPerSecond: 10}, config: HotnessConfig{HighGetsPerSecond: 10, HighPutsPerSecond: 10}, want: 0.7083333333333334},
		{name: "custom weights", input: HotnessInput{GetsPerSecond: 5, PutsPerSecond: 10}, config: HotnessConfig{HighGetsPerSecond: 10, HighPutsPerSecond: 10, GetsWeight: 1}, want: 0.5},
		{name: "over threshold", input: HotnessInput{GetsPerSecond: 20}, config: HotnessConfig{HighGetsPerSecond: 10}, want: 1},
		{name: "negative input", input: HotnessInput{GetsPerSecond: -1}, config: HotnessConfig{HighGetsPerSecond: 10}, want: 0},
		{name: "non-finite input", input: HotnessInput{GetsPerSecond: math.Inf(1)}, config: HotnessConfig{HighGetsPerSecond: 10}, want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Hotness(tt.input, tt.config); math.Abs(got-tt.want) > 1e-12 {
				t.Fatalf("Hotness() = %v, want %v", got, tt.want)
			}
		})
	}
	normalized := HotnessConfig{GetsWeight: math.NaN(), PutsWeight: -1}.Normalize()
	if normalized.GetsWeight != DefaultHotnessGetsWeight || normalized.PutsWeight != DefaultHotnessPutsWeight {
		t.Fatalf("Normalize() = %+v, want defaults", normalized)
	}
}
