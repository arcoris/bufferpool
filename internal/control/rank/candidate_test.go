package rank

import (
	"math"
	"testing"
)

func TestCandidate(t *testing.T) {
	candidate := Candidate{Index: 3, Score: 0.75, TieBreak: 9}
	if candidate.Index != 3 || candidate.Score != 0.75 || candidate.TieBreak != 9 {
		t.Fatalf("Candidate = %+v", candidate)
	}
	if got := NormalizedScore(Candidate{Score: math.Inf(1)}); got != 0 {
		t.Fatalf("NormalizedScore(inf) = %v, want 0", got)
	}
}
