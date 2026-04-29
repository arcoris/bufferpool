package activity

import "arcoris.dev/bufferpool/internal/control/numeric"

// IdleConfig configures quiet-window idle detection.
type IdleConfig struct {
	// Threshold is the maximum hotness considered quiet.
	Threshold float64

	// QuietWindows is the required consecutive quiet window count. Zero disables idle detection.
	QuietWindows uint64
}

// IsIdle reports whether hotness has stayed quiet for enough windows.
func IsIdle(hotness float64, quietWindowCount uint64, config IdleConfig) bool {
	if config.QuietWindows == 0 {
		return false
	}
	return numeric.Clamp01(hotness) <= numeric.Clamp01(config.Threshold) && quietWindowCount >= config.QuietWindows
}
