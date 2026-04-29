package rank

import "testing"

func TestCopyCandidates(t *testing.T) {
	candidates := []Candidate{{Index: 1, Score: 0.1}}
	copied := copyCandidates(candidates)
	if len(copied) != 1 || copied[0] != candidates[0] {
		t.Fatalf("copyCandidates() = %+v", copied)
	}
	candidates[0].Index = 2
	if copied[0].Index != 1 {
		t.Fatalf("copyCandidates returned aliased storage")
	}
	if copyCandidates(nil) != nil {
		t.Fatalf("copyCandidates(nil) should return nil")
	}
}
