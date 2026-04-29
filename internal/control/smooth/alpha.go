package smooth

import (
	"errors"
	"math"
	"time"
)

// DefaultAlpha is the default EWMA weight for new observations.
const DefaultAlpha = 0.2

const (
	// errAlphaInvalidRange explains an alpha outside the EWMA update interval.
	//
	// Alpha must be finite and in (0, 1]. A zero alpha would make updates
	// invisible after initialization, while values above one overshoot the new
	// observation instead of smoothing toward it.
	errAlphaInvalidRange = "control/smooth: alpha must be finite and in (0, 1]"
)

// AlphaConfig configures EWMA update weight.
type AlphaConfig struct {
	// Alpha must be greater than zero and less than or equal to one.
	Alpha float64
}

// Normalize fills the default alpha when Alpha is zero.
func (c AlphaConfig) Normalize() AlphaConfig {
	if c.Alpha == 0 {
		c.Alpha = DefaultAlpha
	}
	return c
}

// Validate checks that Alpha is finite and in the accepted range.
func (c AlphaConfig) Validate() error {
	if math.IsNaN(c.Alpha) || math.IsInf(c.Alpha, 0) || c.Alpha <= 0 || c.Alpha > 1 {
		return errors.New(errAlphaInvalidRange)
	}
	return nil
}

// AlphaFromHalfLife returns an EWMA alpha from elapsed time and half-life.
//
// The convention is alpha = 1 - pow(0.5, elapsed/halfLife). Non-positive
// durations return zero so callers can skip the update safely.
func AlphaFromHalfLife(elapsed, halfLife time.Duration) float64 {
	if elapsed <= 0 || halfLife <= 0 {
		return 0
	}
	alpha := 1 - math.Pow(0.5, elapsed.Seconds()/halfLife.Seconds())
	if math.IsNaN(alpha) || math.IsInf(alpha, 0) {
		return 0
	}
	if alpha < 0 {
		return 0
	}
	if alpha > 1 {
		return 1
	}
	return alpha
}
