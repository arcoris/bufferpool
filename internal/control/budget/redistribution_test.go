package budget

import "testing"

func TestDistributeByScore(t *testing.T) {
	if got := DistributeByScore(100, nil); got != nil {
		t.Fatalf("empty candidates = %+v", got)
	}
	candidates := []RedistributionCandidate{
		{Index: 1, Score: 1, Current: 10},
		{Index: 2, Score: 1, Current: 10, Min: 60},
		{Index: 3, Score: 0, Current: 7, Max: 5},
	}
	got := DistributeByScore(100, candidates)
	if len(got) != 3 || got[0].Index != 1 || got[0].Target != 50 || got[1].Target != 60 || got[2].Target != 0 {
		t.Fatalf("DistributeByScore() = %+v", got)
	}
	if candidates[0].Current != 10 {
		t.Fatalf("DistributeByScore mutated input")
	}
	zeroScores := DistributeByScore(100, []RedistributionCandidate{{Index: 1, Current: 9}})
	if zeroScores[0].Target != 9 {
		t.Fatalf("zero score distribution = %+v", zeroScores)
	}
}
