package rank

import "testing"

func TestCandidate(t *testing.T) {
	candidate := Candidate{Index: 3, Score: 0.75, TieBreak: 9}
	if candidate.Index != 3 || candidate.Score != 0.75 || candidate.TieBreak != 9 {
		t.Fatalf("Candidate = %+v", candidate)
	}
}
