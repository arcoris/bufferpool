package activity

import "arcoris.dev/bufferpool/internal/control/numeric"

// HotnessInput contains recent throughput signals.
type HotnessInput struct {
	// GetsPerSecond is acquisition throughput.
	GetsPerSecond float64

	// PutsPerSecond is return throughput.
	PutsPerSecond float64

	// BytesPerSecond is byte movement throughput.
	BytesPerSecond float64

	// LeaseOpsPerSecond is ownership operation throughput.
	LeaseOpsPerSecond float64
}

// HotnessConfig defines high-water marks for activity dimensions.
type HotnessConfig struct {
	// HighGetsPerSecond normalizes acquisition throughput.
	HighGetsPerSecond float64

	// HighPutsPerSecond normalizes return throughput.
	HighPutsPerSecond float64

	// HighBytesPerSecond normalizes byte throughput.
	HighBytesPerSecond float64

	// HighLeaseOpsPerSecond normalizes ownership throughput.
	HighLeaseOpsPerSecond float64
}

// Hotness returns a normalized activity score from enabled dimensions.
func Hotness(input HotnessInput, config HotnessConfig) float64 {
	var values [4]numeric.WeightedValue
	count := 0
	count = appendNormalized(values[:], count, input.GetsPerSecond, config.HighGetsPerSecond)
	count = appendNormalized(values[:], count, input.PutsPerSecond, config.HighPutsPerSecond)
	count = appendNormalized(values[:], count, input.BytesPerSecond, config.HighBytesPerSecond)
	count = appendNormalized(values[:], count, input.LeaseOpsPerSecond, config.HighLeaseOpsPerSecond)
	return numeric.WeightedAverage(values[:count])
}

func appendNormalized(values []numeric.WeightedValue, count int, value, high float64) int {
	if high <= 0 {
		return count
	}
	values[count] = numeric.WeightedValue{
		Value:  numeric.NormalizeFloatToLimit(numeric.FiniteOrZero(value), high),
		Weight: 1,
	}
	return count + 1
}
