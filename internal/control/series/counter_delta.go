package series

// CounterDelta describes monotonic counter movement between two observations.
type CounterDelta struct {
	// Previous is the older counter value.
	Previous uint64

	// Current is the newer counter value.
	Current uint64

	// Delta is Current-Previous when monotonic, or Current after a reset.
	Delta uint64

	// Reset reports that Current moved backwards and was treated as a fresh baseline.
	Reset bool
}

// NewCounterDelta returns safe monotonic counter movement.
func NewCounterDelta(previous, current uint64) CounterDelta {
	if current >= previous {
		return CounterDelta{Previous: previous, Current: current, Delta: current - previous}
	}
	return CounterDelta{Previous: previous, Current: current, Delta: current, Reset: true}
}

// DeltaValue returns the delta value from NewCounterDelta.
func DeltaValue(previous, current uint64) uint64 {
	return NewCounterDelta(previous, current).Delta
}
