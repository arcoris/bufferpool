package budget

// Headroom returns remaining capacity below limit.
func Headroom(current, limit uint64) uint64 {
	if limit == 0 || current >= limit {
		return 0
	}
	return limit - current
}
