package activity

import "arcoris.dev/bufferpool/internal/control/numeric"

// ActivityState tracks consecutive quiet windows for idle scoring.
type ActivityState struct {
	// QuietWindows is the current consecutive quiet-window count.
	QuietWindows uint64
}

// Update returns the next activity state for hotness and quiet threshold.
func (s ActivityState) Update(hotness float64, threshold float64) ActivityState {
	if numeric.Clamp01(hotness) <= numeric.Clamp01(threshold) {
		s.QuietWindows++
		return s
	}
	s.QuietWindows = 0
	return s
}
