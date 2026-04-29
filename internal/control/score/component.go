package score

import "arcoris.dev/bufferpool/internal/control/numeric"

// Component is one weighted score input.
type Component struct {
	// Name identifies the signal for diagnostics.
	Name string

	// Value is the normalized component value in [0, 1].
	Value float64

	// Weight is the non-negative component weight.
	Weight float64
}

// NewComponent returns a normalized weighted score component.
func NewComponent(name string, value, weight float64) Component {
	if weight < 0 {
		weight = 0
	}
	return Component{
		Name:   name,
		Value:  numeric.Clamp01(value),
		Weight: numeric.FiniteOrZero(weight),
	}
}
