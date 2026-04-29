package rate

import "arcoris.dev/bufferpool/internal/control/numeric"

// HitRatio returns hits divided by hits plus misses.
//
// The denominator uses saturating addition so an extreme or synthetic counter
// pair cannot wrap to a small value and inflate the ratio. Ordinary windows are
// unaffected because hits+misses is expected to fit in uint64.
func HitRatio(hits, misses uint64) float64 {
	return numeric.SafeRatio(hits, OutcomeTotal(hits, misses))
}

// MissRatio returns misses divided by hits plus misses.
//
// Like HitRatio, the denominator is saturated rather than wrapped.
func MissRatio(hits, misses uint64) float64 {
	return numeric.SafeRatio(misses, OutcomeTotal(hits, misses))
}

// OutcomeTotal returns successes plus failures as a saturated denominator.
//
// It is useful for two-outcome windows such as hits/misses and
// successes/failures. Saturation preserves a conservative non-zero denominator
// under overflow instead of allowing uint64 wraparound.
func OutcomeTotal(successes, failures uint64) uint64 {
	return numeric.SaturatingAddUint64(successes, failures)
}

// AllocationRatio returns allocations divided by gets.
func AllocationRatio(allocations, gets uint64) float64 {
	return numeric.SafeRatio(allocations, gets)
}

// RetainRatio returns retains divided by puts.
func RetainRatio(retains, puts uint64) float64 {
	return numeric.SafeRatio(retains, puts)
}

// DropRatio returns drops divided by puts.
func DropRatio(drops, puts uint64) float64 {
	return numeric.SafeRatio(drops, puts)
}

// SuccessRatio returns successes divided by attempts.
func SuccessRatio(successes, attempts uint64) float64 {
	return numeric.SafeRatio(successes, attempts)
}

// FailureRatio returns failures divided by attempts.
func FailureRatio(failures, attempts uint64) float64 {
	return numeric.SafeRatio(failures, attempts)
}

// InvalidRatio returns invalid events divided by attempts.
func InvalidRatio(invalid, attempts uint64) float64 {
	return numeric.SafeRatio(invalid, attempts)
}
