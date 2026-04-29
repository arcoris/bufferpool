package stability

// ConsecutiveState tracks consecutive true condition windows.
type ConsecutiveState struct {
	// Count is the current consecutive true count.
	Count uint64
}

// Update returns the next consecutive state for condition.
func (s ConsecutiveState) Update(condition bool) ConsecutiveState {
	if condition {
		s.Count++
		return s
	}
	s.Count = 0
	return s
}

// Reached reports whether Count has reached required. A zero requirement never triggers.
func (s ConsecutiveState) Reached(required uint64) bool {
	return required != 0 && s.Count >= required
}
