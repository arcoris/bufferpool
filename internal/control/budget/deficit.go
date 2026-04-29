package budget

// Deficit returns consumption above limit.
func Deficit(current, limit uint64) uint64 {
	if limit == 0 || current <= limit {
		return 0
	}
	return current - limit
}
