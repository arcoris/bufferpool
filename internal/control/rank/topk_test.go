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
	dst := make([]Candidate, 0, 8)
	top = TopKDescendingInto(dst, candidates, 2)
	if len(top) != 2 || cap(top) != cap(dst) || top[0].Index != 2 || top[1].Index != 3 {
		t.Fatalf("TopKDescendingInto() = %+v", top)
	}
	all = TopKAscendingInto(dst, candidates, 10)
	if len(all) != 3 || cap(all) != cap(dst) || all[0].Index != 1 || all[2].Index != 2 {
		t.Fatalf("TopKAscendingInto() = %+v", all)
	}
}

func TestTopKIntoReusesDestination(t *testing.T) {
	candidates := []Candidate{{Index: 1, Score: 0.1}, {Index: 2, Score: 0.9}, {Index: 3, Score: 0.5}}
	dst := make([]Candidate, 0, 8)
	top := TopKDescendingInto(dst, candidates, 2)
	if len(top) != 2 || cap(top) != cap(dst) {
		t.Fatalf("TopKDescendingInto() len/cap = %d/%d, want len 2 cap %d", len(top), cap(top), cap(dst))
	}
	top[0].Index = 99
	if candidates[1].Index != 2 {
		t.Fatalf("TopKDescendingInto returned input-owned storage")
	}
}

func TestTopKIntoDoesNotMutateInput(t *testing.T) {
	candidates := []Candidate{{Index: 1, Score: 0.1}, {Index: 2, Score: 0.9}, {Index: 3, Score: 0.5}}
	original := append([]Candidate(nil), candidates...)
	_ = TopKAscendingInto(nil, candidates, 2)
	for index := range candidates {
		if candidates[index] != original[index] {
			t.Fatalf("TopKAscendingInto mutated input: got %+v want %+v", candidates, original)
		}
	}
	if got := TopKDescendingInto(nil, nil, 2); got != nil {
		t.Fatalf("TopKDescendingInto(nil) = %+v, want nil", got)
	}
	if got := TopKAscendingInto(nil, candidates, 0); got != nil {
		t.Fatalf("TopKAscendingInto(k=0) = %+v, want nil", got)
	}
}

var rankBenchmarkCases = []struct {
	name  string
	count int
}{
	{name: "candidates_4", count: 4},
	{name: "candidates_16", count: 16},
	{name: "candidates_64", count: 64},
	{name: "candidates_256", count: 256},
	{name: "candidates_1024", count: 1024},
}

func benchmarkCandidates(count int) []Candidate {
	candidates := make([]Candidate, count)
	for index := range candidates {
		candidates[index] = Candidate{
			Index:    index,
			Score:    float64((index*37)%100) / 100,
			TieBreak: uint64(index),
		}
	}
	return candidates
}

func rankBenchmarkK(count int) int {
	if count < 8 {
		return count
	}
	return 8
}

func BenchmarkControlRankTopK(b *testing.B) {
	for _, tt := range rankBenchmarkCases {
		b.Run(tt.name, func(b *testing.B) {
			candidates := benchmarkCandidates(tt.count)
			k := rankBenchmarkK(tt.count)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = TopKDescending(candidates, k)
			}
		})
	}
}

func BenchmarkControlRankTopKInto(b *testing.B) {
	for _, tt := range rankBenchmarkCases {
		b.Run(tt.name, func(b *testing.B) {
			candidates := benchmarkCandidates(tt.count)
			dst := make([]Candidate, 0, len(candidates))
			k := rankBenchmarkK(tt.count)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				dst = TopKDescendingInto(dst[:0], candidates, k)
			}
			_ = dst
		})
	}
}
