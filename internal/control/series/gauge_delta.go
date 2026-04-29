package series

// GaugeDelta describes directional movement between two gauge observations.
type GaugeDelta struct {
	// Previous is the older gauge value.
	Previous uint64

	// Current is the newer gauge value.
	Current uint64

	// Increased is Current-Previous when Current is larger.
	Increased uint64

	// Decreased is Previous-Current when Current is smaller.
	Decreased uint64

	// Changed reports whether Current differs from Previous.
	Changed bool
}

// NewGaugeDelta returns directional gauge movement without underflow.
func NewGaugeDelta(previous, current uint64) GaugeDelta {
	delta := GaugeDelta{Previous: previous, Current: current}
	if current > previous {
		delta.Increased = current - previous
		delta.Changed = true
		return delta
	}
	if previous > current {
		delta.Decreased = previous - current
		delta.Changed = true
	}
	return delta
}
