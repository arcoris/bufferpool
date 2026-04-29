package rate

import "arcoris.dev/bufferpool/internal/control/numeric"

// HitRatio returns hits divided by hits plus misses.
func HitRatio(hits, misses uint64) float64 {
	return numeric.SafeRatio(hits, hits+misses)
}

// MissRatio returns misses divided by hits plus misses.
func MissRatio(hits, misses uint64) float64 {
	return numeric.SafeRatio(misses, hits+misses)
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
