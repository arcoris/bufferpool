package stability

import "math"

// WithinDeadband reports whether current stays within band of previous.
func WithinDeadband(previous, current, band float64) bool {
	if band < 0 {
		band = 0
	}
	return math.Abs(current-previous) <= band
}
