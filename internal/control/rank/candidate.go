package rank

import "arcoris.dev/bufferpool/internal/control/numeric"

// Candidate is a generic ranked item.
type Candidate struct {
	// Index identifies caller-owned data.
	Index int

	// Score is the caller-provided ranking score.
	Score float64

	// TieBreak is a deterministic secondary key; lower values sort first.
	TieBreak uint64
}

// NormalizedScore returns candidate.Score as a finite comparison value.
//
// Ranking helpers treat NaN and infinities as zero instead of allowing sort
// comparisons to become non-deterministic. Callers that need stricter behavior
// should validate scores before constructing candidates.
func NormalizedScore(candidate Candidate) float64 {
	return numeric.FiniteOrZero(candidate.Score)
}
