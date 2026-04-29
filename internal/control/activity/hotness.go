package activity

import "arcoris.dev/bufferpool/internal/control/numeric"

const (
	// DefaultHotnessGetsWeight gives acquisition demand the strongest activity
	// weight because gets represent direct buffer demand.
	DefaultHotnessGetsWeight = 0.35

	// DefaultHotnessPutsWeight captures return flow. It is lower than gets
	// because puts are useful only when paired with future demand.
	DefaultHotnessPutsWeight = 0.25

	// DefaultHotnessBytesWeight keeps byte movement highly visible because large
	// buffers are more relevant to retained-memory pressure.
	DefaultHotnessBytesWeight = 0.30

	// DefaultHotnessLeaseOpsWeight keeps ownership activity visible but small so
	// lease churn does not dominate actual buffer reuse demand.
	DefaultHotnessLeaseOpsWeight = 0.10
)

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

	// GetsWeight weights acquisition demand when HighGetsPerSecond is enabled.
	GetsWeight float64

	// PutsWeight weights return flow when HighPutsPerSecond is enabled.
	PutsWeight float64

	// BytesWeight weights byte throughput when HighBytesPerSecond is enabled.
	BytesWeight float64

	// LeaseOpsWeight weights ownership activity when HighLeaseOpsPerSecond is enabled.
	LeaseOpsWeight float64
}

// HotnessScorer is a prepared activity evaluator with normalized config.
//
// Hotness measures recent throughput intensity. It is distinct from usefulness:
// a workload can be hot but inefficient if it misses frequently, and it can be
// useful but low-volume if retained buffers still avoid allocations.
type HotnessScorer struct {
	config HotnessConfig
}

// NewHotnessScorer returns a scorer with normalized stable thresholds/weights.
func NewHotnessScorer(config HotnessConfig) HotnessScorer {
	return HotnessScorer{config: config.Normalize()}
}

// Score returns the normalized hotness score for input.
func (s HotnessScorer) Score(input HotnessInput) float64 {
	return hotnessWithNormalizedConfig(input, s.config)
}

// Normalize fills default weights when no usable weights are configured.
//
// Thresholds are not defaulted here: a dimension remains disabled when its high
// value is zero or negative. Negative and non-finite weights are converted to
// zero so invalid configuration cannot produce negative activity.
func (c HotnessConfig) Normalize() HotnessConfig {
	c.GetsWeight = usableWeight(c.GetsWeight)
	c.PutsWeight = usableWeight(c.PutsWeight)
	c.BytesWeight = usableWeight(c.BytesWeight)
	c.LeaseOpsWeight = usableWeight(c.LeaseOpsWeight)
	if c.GetsWeight == 0 && c.PutsWeight == 0 && c.BytesWeight == 0 && c.LeaseOpsWeight == 0 {
		c.GetsWeight = DefaultHotnessGetsWeight
		c.PutsWeight = DefaultHotnessPutsWeight
		c.BytesWeight = DefaultHotnessBytesWeight
		c.LeaseOpsWeight = DefaultHotnessLeaseOpsWeight
	}
	return c
}

// Hotness returns a normalized activity score from enabled dimensions.
func Hotness(input HotnessInput, config HotnessConfig) float64 {
	return NewHotnessScorer(config).Score(input)
}

// hotnessWithNormalizedConfig evaluates input after HotnessScorer has
// normalized weights.
func hotnessWithNormalizedConfig(input HotnessInput, config HotnessConfig) float64 {
	var values [4]numeric.WeightedValue
	count := 0
	count = appendNormalized(values[:], count, input.GetsPerSecond, config.HighGetsPerSecond, config.GetsWeight)
	count = appendNormalized(values[:], count, input.PutsPerSecond, config.HighPutsPerSecond, config.PutsWeight)
	count = appendNormalized(values[:], count, input.BytesPerSecond, config.HighBytesPerSecond, config.BytesWeight)
	count = appendNormalized(values[:], count, input.LeaseOpsPerSecond, config.HighLeaseOpsPerSecond, config.LeaseOpsWeight)
	return numeric.WeightedAverage(values[:count])
}

// appendNormalized appends one enabled hotness dimension into values.
//
// Disabled dimensions, zero thresholds, and zero weights are skipped rather
// than contributing zero-weight components to the fixed scratch array.
func appendNormalized(values []numeric.WeightedValue, count int, value, high, weight float64) int {
	if high <= 0 || weight <= 0 {
		return count
	}
	values[count] = numeric.WeightedValue{
		Value:  numeric.NormalizeFloatToLimit(numeric.FiniteOrZero(value), high),
		Weight: weight,
	}
	return count + 1
}

// usableWeight normalizes optional activity weights.
//
// Negative and non-finite weights are disabled so activity scoring cannot be
// inverted or poisoned by invalid configuration.
func usableWeight(weight float64) float64 {
	weight = numeric.FiniteOrZero(weight)
	if weight < 0 {
		return 0
	}
	return weight
}
