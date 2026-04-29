package stability

import (
	"math"

	"arcoris.dev/bufferpool/internal/control/numeric"
)

// WithinDeadband reports whether current stays within band of previous.
//
// The helper is defensive because deadband checks often sit between noisy
// analytical signals and policy-change guards. Non-finite inputs are treated as
// zero and a negative band is treated as zero, so invalid values cannot create a
// permanently open deadband.
func WithinDeadband(previous, current, band float64) bool {
	previous = numeric.FiniteOrZero(previous)
	current = numeric.FiniteOrZero(current)
	band = numeric.FiniteOrZero(band)
	if band < 0 {
		band = 0
	}
	return math.Abs(current-previous) <= band
}
