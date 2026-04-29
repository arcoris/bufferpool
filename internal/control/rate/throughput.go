package rate

import (
	"time"

	"arcoris.dev/bufferpool/internal/control/numeric"
)

// PerSecond returns delta events per elapsed second.
func PerSecond(delta uint64, elapsed time.Duration) float64 {
	if elapsed <= 0 {
		return 0
	}
	return numeric.FiniteOrZero(float64(delta) / elapsed.Seconds())
}

// BytesPerSecond returns byte movement per elapsed second.
func BytesPerSecond(deltaBytes uint64, elapsed time.Duration) float64 {
	return PerSecond(deltaBytes, elapsed)
}
