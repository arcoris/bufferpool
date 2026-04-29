package numeric

import "arcoris.dev/bufferpool/internal/mathx"

// Clamp returns value bounded by minValue and maxValue.
//
// Control algorithms use Clamp as a forgiving adapter around mathx.Clamp:
// inverted bounds are swapped, and non-finite inputs are first converted to
// zero. The final bounded comparison is delegated to mathx.Clamp so the
// project has one strict numeric clamping primitive.
func Clamp(value, minValue, maxValue float64) float64 {
	value = FiniteOrZero(value)
	minValue = FiniteOrZero(minValue)
	maxValue = FiniteOrZero(maxValue)
	if minValue > maxValue {
		minValue, maxValue = maxValue, minValue
	}
	return mathx.Clamp(value, minValue, maxValue)
}

// Clamp01 returns value bounded to the closed interval [0, 1].
func Clamp01(value float64) float64 { return Clamp(value, 0, 1) }

// Invert01 returns one minus value after value is clamped to [0, 1].
func Invert01(value float64) float64 { return 1 - Clamp01(value) }
