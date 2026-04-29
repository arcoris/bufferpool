package budget

import "arcoris.dev/bufferpool/internal/control/numeric"

// Usage describes current consumption relative to a limit.
type Usage struct {
	// Current is the observed consumption.
	Current uint64

	// Limit is the configured limit; zero means unlimited or unset.
	Limit uint64

	// Utilization is Current divided by Limit when Limit is non-zero.
	Utilization float64

	// OverLimit reports Current greater than Limit when Limit is non-zero.
	OverLimit bool
}

// NewUsage returns consumption and utilization against limit.
func NewUsage(current, limit uint64) Usage {
	usage := Usage{Current: current, Limit: limit}
	if limit == 0 {
		return usage
	}
	usage.Utilization = numeric.SafeRatio(current, limit)
	usage.OverLimit = current > limit
	return usage
}
