package rank

import (
	"math"
	"sort"
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

func TestRankNaNAndInfScores(t *testing.T) {
	candidates := []Candidate{
		{Index: 1, Score: math.NaN(), TieBreak: 1},
		{Index: 2, Score: math.Inf(1), TieBreak: 2},
		{Index: 3, Score: 0.2, TieBreak: 3},
	}
	SortDescending(candidates)
	if candidates[0].Index != 3 || candidates[1].Index != 1 || candidates[2].Index != 2 {
		t.Fatalf("SortDescending(non-finite) = %+v", candidates)
	}
}

func TestSortDescendingMatchesStableSortReference(t *testing.T) {
	for _, count := range []int{0, 1, 4, 16, 64, 128, 256, 1024} {
		candidates := benchmarkCandidates(count)
		got := append([]Candidate(nil), candidates...)
		want := append([]Candidate(nil), candidates...)

		SortDescending(got)
		sortDescendingReference(want)

		requireCandidatesEqual(t, got, want)
	}
}

func TestSortAscendingMatchesStableSortReference(t *testing.T) {
	for _, count := range []int{0, 1, 4, 16, 64, 128, 256, 1024} {
		candidates := benchmarkCandidates(count)
		got := append([]Candidate(nil), candidates...)
		want := append([]Candidate(nil), candidates...)

		SortAscending(got)
		sortAscendingReference(want)

		requireCandidatesEqual(t, got, want)
	}
}

func BenchmarkControlRankSortDescending(b *testing.B) {
	for _, tt := range rankBenchmarkCases {
		b.Run(tt.name, func(b *testing.B) {
			candidates := benchmarkCandidates(tt.count)
			work := make([]Candidate, len(candidates))
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				copy(work, candidates)
				SortDescending(work)
			}
		})
	}
}

func BenchmarkControlRankSortDescendingReference(b *testing.B) {
	for _, tt := range rankBenchmarkCases {
		b.Run(tt.name, func(b *testing.B) {
			candidates := benchmarkCandidates(tt.count)
			work := make([]Candidate, len(candidates))
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				copy(work, candidates)
				sortDescendingReference(work)
			}
		})
	}
}

func sortDescendingReference(candidates []Candidate) {
	sort.SliceStable(candidates, func(left, right int) bool {
		return descendingBefore(candidates[left], candidates[right])
	})
}

func sortAscendingReference(candidates []Candidate) {
	sort.SliceStable(candidates, func(left, right int) bool {
		return ascendingBefore(candidates[left], candidates[right])
	})
}

func requireCandidatesEqual(t *testing.T, got, want []Candidate) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("candidate length = %d, want %d", len(got), len(want))
	}
	for index := range got {
		if got[index] != want[index] {
			t.Fatalf("candidate[%d] = %+v, want %+v\nall got:  %+v\nall want: %+v", index, got[index], want[index], got, want)
		}
	}
}
