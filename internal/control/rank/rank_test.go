package rank

import (
	"math"
	"testing"
)

func TestSortDescending(t *testing.T) {
	candidates := []Candidate{{Index: 1, Score: 0.2, TieBreak: 2}, {Index: 2, Score: 0.8, TieBreak: 1}, {Index: 3, Score: 0.8, TieBreak: 0}}
	SortDescending(candidates)
	if candidates[0].Index != 3 || candidates[1].Index != 2 || candidates[2].Index != 1 {
		t.Fatalf("SortDescending() = %+v", candidates)
	}
}

func TestSortAscending(t *testing.T) {
	candidates := []Candidate{{Index: 1, Score: 0.2, TieBreak: 2}, {Index: 2, Score: 0.8, TieBreak: 1}, {Index: 3, Score: 0.2, TieBreak: 0}}
	SortAscending(candidates)
	if candidates[0].Index != 3 || candidates[1].Index != 1 || candidates[2].Index != 2 {
		t.Fatalf("SortAscending() = %+v", candidates)
	}
}

func TestSortNormalizesNonFiniteScores(t *testing.T) {
	candidates := []Candidate{
		{Index: 1, Score: math.Inf(1), TieBreak: 2},
		{Index: 2, Score: 0.1, TieBreak: 1},
		{Index: 3, Score: math.NaN(), TieBreak: 0},
	}
	SortDescending(candidates)
	if candidates[0].Index != 2 || candidates[1].Index != 3 || candidates[2].Index != 1 {
		t.Fatalf("SortDescending(non-finite) = %+v", candidates)
	}
}
