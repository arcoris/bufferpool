package smooth

import "arcoris.dev/bufferpool/internal/control/numeric"

// EWMA stores one exponentially weighted moving average value.
type EWMA struct {
	// Initialized reports whether Value contains at least one observation.
	Initialized bool

	// Value is the current smoothed finite value.
	Value float64
}

// EWMASmoother is a prepared EWMA evaluator with validated stable alpha.
//
// Controller loops usually keep the same smoothing configuration for many
// windows. Preparing the alpha once avoids repeated normalization and
// validation while keeping the update path explicit and allocation-free.
// EWMASmoother does not store moving state; callers pass and receive EWMA
// values so ownership of state remains visible. The zero value is valid but
// disabled: Update returns the input state unchanged until a smoother is
// constructed with NewEWMASmoother or MustNewEWMASmoother.
type EWMASmoother struct {
	alpha float64
}

// NewEWMASmoother returns a prepared smoother for config.
//
// The config is normalized before validation. A zero alpha therefore uses
// DefaultAlpha, while invalid non-zero alpha values return an error.
func NewEWMASmoother(config AlphaConfig) (EWMASmoother, error) {
	config = config.Normalize()
	if err := config.Validate(); err != nil {
		return EWMASmoother{}, err
	}
	return EWMASmoother{alpha: config.Alpha}, nil
}

// MustNewEWMASmoother returns a prepared smoother or panics for invalid config.
//
// It is intended for package-level defaults and tests where invalid
// configuration is a programming error.
func MustNewEWMASmoother(config AlphaConfig) EWMASmoother {
	smoother, err := NewEWMASmoother(config)
	if err != nil {
		panic(err)
	}
	return smoother
}

// Update returns state after observing value with the prepared alpha.
//
// A zero-value EWMASmoother has alpha zero and is intentionally a no-op. This
// avoids hidden default configuration in unconstructed controller state.
func (s EWMASmoother) Update(state EWMA, value float64) EWMA {
	if s.alpha <= 0 {
		return state
	}
	return state.Update(s.alpha, value)
}

// Update returns a new EWMA after observing value with alpha.
//
// This direct one-off helper is useful for tests or simple callers. Repeated
// controller loops with stable alpha should use EWMASmoother so configuration
// validation and normalization happen once.
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
