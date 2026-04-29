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
	scorer := NewHotnessScorer(HotnessConfig{HighGetsPerSecond: 10, HighPutsPerSecond: 10})
	input := HotnessInput{GetsPerSecond: 5, PutsPerSecond: 10}
	if got, want := scorer.Score(input), Hotness(input, HotnessConfig{HighGetsPerSecond: 10, HighPutsPerSecond: 10}); got != want {
		t.Fatalf("HotnessScorer.Score() = %v, want %v", got, want)
	}
}

func TestHotnessScorerZeroValue(t *testing.T) {
	var scorer HotnessScorer
	if got := scorer.Score(HotnessInput{GetsPerSecond: 10}); got != 0 {
		t.Fatalf("zero HotnessScorer.Score() = %v, want 0", got)
	}
}

func TestHotnessScorerMatchesHotness(t *testing.T) {
	config := HotnessConfig{HighGetsPerSecond: 10, HighPutsPerSecond: 10}
	scorer := NewHotnessScorer(config)
	input := HotnessInput{GetsPerSecond: 5, PutsPerSecond: 10}
	if got, want := scorer.Score(input), Hotness(input, config); got != want {
		t.Fatalf("HotnessScorer.Score() = %v, want %v", got, want)
	}
}

func TestHotnessOneOffVsPrepared(t *testing.T) {
	config := HotnessConfig{HighGetsPerSecond: 10, GetsWeight: 1}
	input := HotnessInput{GetsPerSecond: 7}
	prepared := NewHotnessScorer(config)
	if got, want := Hotness(input, config), prepared.Score(input); got != want {
		t.Fatalf("Hotness() = %v, want prepared score %v", got, want)
	}
}

func BenchmarkControlActivityHotness(b *testing.B) {
	input := HotnessInput{GetsPerSecond: 5000, PutsPerSecond: 3000, BytesPerSecond: 1 << 20, LeaseOpsPerSecond: 8000}
	config := HotnessConfig{
		HighGetsPerSecond:     10_000,
		HighPutsPerSecond:     10_000,
		HighBytesPerSecond:    2 << 20,
		HighLeaseOpsPerSecond: 20_000,
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = Hotness(input, config)
	}
}

func BenchmarkControlActivityHotnessScorer(b *testing.B) {
	input := HotnessInput{GetsPerSecond: 5000, PutsPerSecond: 3000, BytesPerSecond: 1 << 20, LeaseOpsPerSecond: 8000}
	scorer := NewHotnessScorer(HotnessConfig{
		HighGetsPerSecond:     10_000,
		HighPutsPerSecond:     10_000,
		HighBytesPerSecond:    2 << 20,
		HighLeaseOpsPerSecond: 20_000,
	})
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = scorer.Score(input)
	}
}
