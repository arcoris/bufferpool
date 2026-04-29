package smooth

import "arcoris.dev/bufferpool/internal/control/numeric"

// EWMA stores one exponentially weighted moving average value.
type EWMA struct {
	// Initialized reports whether Value contains at least one observation.
	Initialized bool

	// Value is the current smoothed finite value.
	Value float64
}

// Update returns a new EWMA after observing value with alpha.
func (e EWMA) Update(alpha float64, value float64) EWMA {
	value = numeric.FiniteOrZero(value)
	if !e.Initialized {
		return EWMA{Initialized: true, Value: value}
	}
	if alpha <= 0 {
		return e
	}
	if alpha > 1 {
		alpha = 1
	}
	next := alpha*value + (1-alpha)*numeric.FiniteOrZero(e.Value)
	return EWMA{Initialized: true, Value: numeric.FiniteOrZero(next)}
}

// UpdateInPlace updates e after observing value with alpha.
func (e *EWMA) UpdateInPlace(alpha float64, value float64) {
	if e == nil {
		return
	}
	*e = e.Update(alpha, value)
}
