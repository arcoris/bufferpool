package rate

// OperationRates groups generic operation outcome ratios.
type OperationRates struct {
	// HitRatio is reuse hits divided by hits plus misses.
	HitRatio float64

	// MissRatio is reuse misses divided by hits plus misses.
	MissRatio float64

	// AllocationRatio is allocations divided by acquisition attempts.
	AllocationRatio float64

	// RetainRatio is retained returns divided by return attempts.
	RetainRatio float64

	// DropRatio is dropped returns divided by return attempts.
	DropRatio float64
}

// NewOperationRates computes generic operation outcome ratios from deltas.
func NewOperationRates(hits, misses, allocations, gets, retains, drops, puts uint64) OperationRates {
	return OperationRates{
		HitRatio:        HitRatio(hits, misses),
		MissRatio:       MissRatio(hits, misses),
		AllocationRatio: AllocationRatio(allocations, gets),
		RetainRatio:     RetainRatio(retains, puts),
		DropRatio:       DropRatio(drops, puts),
	}
}
