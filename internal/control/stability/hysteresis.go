package stability

import (
	"errors"

	"arcoris.dev/bufferpool/internal/control/numeric"
)

const (
	// errHysteresisNonFiniteThreshold explains NaN or infinity in thresholds.
	errHysteresisNonFiniteThreshold = "control/stability: hysteresis thresholds must be finite"

	// errHysteresisInvertedThresholds explains an unusable high-state band.
	errHysteresisInvertedThresholds = "control/stability: enter threshold must be >= exit threshold"
)

// Hysteresis describes enter and exit thresholds for a high-state signal.
type Hysteresis struct {
	// Enter is the threshold at or above which the state should be entered.
	Enter float64

	// Exit is the threshold at or below which the state should be exited.
	Exit float64
}

// ShouldEnter reports whether value crosses the enter threshold.
func (h Hysteresis) ShouldEnter(value float64) bool {
	return numeric.FiniteOrZero(value) >= h.Enter
}

// ShouldExit reports whether value crosses the exit threshold.
func (h Hysteresis) ShouldExit(value float64) bool {
	return numeric.FiniteOrZero(value) <= h.Exit
}

// Validate checks finite thresholds and enter >= exit ordering.
func (h Hysteresis) Validate() error {
	if !numeric.IsFinite(h.Enter) || !numeric.IsFinite(h.Exit) {
		return errors.New(errHysteresisNonFiniteThreshold)
	}
	if h.Enter < h.Exit {
		return errors.New(errHysteresisInvertedThresholds)
	}
	return nil
}
