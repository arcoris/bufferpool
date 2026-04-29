package rank

import "testing"

func TestTopK(t *testing.T) {
	candidates := []Candidate{{Index: 1, Score: 0.1}, {Index: 2, Score: 0.9}, {Index: 3, Score: 0.5}}
	if got := TopKDescending(candidates, 0); got != nil {
		t.Fatalf("TopKDescending k=0 = %+v", got)
	}
	top := TopKDescending(candidates, 1)
	if len(top) != 1 || top[0].Index != 2 {
		t.Fatalf("TopKDescending() = %+v", top)
	}
	all := TopKAscending(candidates, 10)
	if len(all) != 3 || all[0].Index != 1 || all[2].Index != 2 {
		t.Fatalf("TopKAscending() = %+v", all)
	}
	if candidates[0].Index != 1 {
		t.Fatalf("TopK mutated input")
	}
}

func BenchmarkControlRankTopK(b *testing.B) {
	candidates := []Candidate{
		{Index: 1, Score: 0.1, TieBreak: 1},
		{Index: 2, Score: 0.8, TieBreak: 2},
		{Index: 3, Score: 0.5, TieBreak: 3},
		{Index: 4, Score: 0.8, TieBreak: 0},
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = TopKDescending(candidates, 2)
	}
}
