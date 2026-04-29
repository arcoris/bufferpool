package rank

// Candidate is a generic ranked item.
type Candidate struct {
	// Index identifies caller-owned data.
	Index int

	// Score is the caller-provided ranking score.
	Score float64

	// TieBreak is a deterministic secondary key; lower values sort first.
	TieBreak uint64
}
