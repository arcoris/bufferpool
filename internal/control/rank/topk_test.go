package rank

import (
	"container/heap"
	"strconv"
	"testing"
)

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

func TestTopKDescendingIntoMatchesReference(t *testing.T) {
	for _, count := range []int{0, 1, 4, 16, 64, 128, 256, 1024} {
		for _, k := range rankBenchmarkKCases(count) {
			candidates := benchmarkCandidates(count)
			dst := make([]Candidate, 0, count)
			got := TopKDescendingInto(dst, candidates, k)
			want := topKDescendingReference(candidates, k)

			requireCandidatesEqual(t, got, want)
		}
	}
}

func TestTopKAscendingIntoMatchesReference(t *testing.T) {
	for _, count := range []int{0, 1, 4, 16, 64, 128, 256, 1024} {
		for _, k := range rankBenchmarkKCases(count) {
			candidates := benchmarkCandidates(count)
			dst := make([]Candidate, 0, count)
			got := TopKAscendingInto(dst, candidates, k)
			want := topKAscendingReference(candidates, k)

			requireCandidatesEqual(t, got, want)
		}
	}
}

var rankBenchmarkCases = []struct {
	name  string
	count int
}{
	{name: "candidates_4", count: 4},
	{name: "candidates_16", count: 16},
	{name: "candidates_64", count: 64},
	{name: "candidates_128", count: 128},
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

func rankBenchmarkKCases(count int) []int {
	if count <= 0 {
		return nil
	}
	values := []int{1}
	if count >= 8 {
		values = append(values, 8)
	}
	if count >= 32 {
		values = append(values, 32)
	}
	return values
}

func BenchmarkControlRankTopK(b *testing.B) {
	for _, tt := range rankBenchmarkCases {
		for _, k := range rankBenchmarkKCases(tt.count) {
			b.Run(tt.name+"/k_"+strconv.Itoa(k), func(b *testing.B) {
				candidates := benchmarkCandidates(tt.count)
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					_ = TopKDescending(candidates, k)
				}
			})
		}
	}
}

func BenchmarkControlRankTopKInto(b *testing.B) {
	for _, tt := range rankBenchmarkCases {
		for _, k := range rankBenchmarkKCases(tt.count) {
			b.Run(tt.name+"/k_"+strconv.Itoa(k), func(b *testing.B) {
				candidates := benchmarkCandidates(tt.count)
				dst := make([]Candidate, 0, len(candidates))
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					dst = TopKDescendingInto(dst[:0], candidates, k)
				}
				_ = dst
			})
		}
	}
}

func BenchmarkControlRankTopKHeapReference(b *testing.B) {
	for _, tt := range rankBenchmarkCases {
		for _, k := range rankBenchmarkKCases(tt.count) {
			b.Run(tt.name+"/k_"+strconv.Itoa(k), func(b *testing.B) {
				candidates := benchmarkCandidates(tt.count)
				dst := make([]Candidate, 0, k)
				items := make(candidateHeapItems, 0, k)
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					dst, items = topKDescendingHeapReference(dst[:0], items[:0], candidates, k)
				}
				_ = dst
			})
		}
	}
}

func topKDescendingReference(candidates []Candidate, k int) []Candidate {
	copied := copyCandidates(candidates)
	sortDescendingReference(copied)
	if k <= 0 || len(copied) == 0 {
		return nil
	}
	if k > len(copied) {
		k = len(copied)
	}
	return copied[:k]
}

func topKAscendingReference(candidates []Candidate, k int) []Candidate {
	copied := copyCandidates(candidates)
	sortAscendingReference(copied)
	if k <= 0 || len(copied) == 0 {
		return nil
	}
	if k > len(copied) {
		k = len(copied)
	}
	return copied[:k]
}

func topKDescendingHeapReference(dst []Candidate, items candidateHeapItems, candidates []Candidate, k int) ([]Candidate, candidateHeapItems) {
	if k <= 0 || len(candidates) == 0 {
		return nil, items[:0]
	}
	if k > len(candidates) {
		k = len(candidates)
	}
	items = items[:0]
	heap.Init(&items)
	for index, candidate := range candidates {
		item := candidateHeapItem{candidate: candidate, order: index}
		if items.Len() < k {
			heap.Push(&items, item)
			continue
		}
		if candidateHeapBetterDescending(item, items[0]) {
			items[0] = item
			heap.Fix(&items, 0)
		}
	}
	if cap(dst) < len(items) {
		dst = make([]Candidate, len(items))
	} else {
		dst = dst[:len(items)]
	}
	for index, item := range items {
		dst[index] = item.candidate
	}
	SortDescending(dst)
	if k > len(dst) {
		k = len(dst)
	}
	return dst[:k], items
}

type candidateHeapItem struct {
	candidate Candidate
	order     int
}

type candidateHeapItems []candidateHeapItem

func (h candidateHeapItems) Len() int { return len(h) }

func (h candidateHeapItems) Less(left, right int) bool {
	return candidateHeapWorseDescending(h[left], h[right])
}

func (h candidateHeapItems) Swap(left, right int) {
	h[left], h[right] = h[right], h[left]
}

func (h *candidateHeapItems) Push(value any) {
	*h = append(*h, value.(candidateHeapItem))
}

func (h *candidateHeapItems) Pop() any {
	old := *h
	last := len(old) - 1
	value := old[last]
	*h = old[:last]
	return value
}

func candidateHeapBetterDescending(left, right candidateHeapItem) bool {
	if descendingBefore(left.candidate, right.candidate) {
		return true
	}
	if descendingBefore(right.candidate, left.candidate) {
		return false
	}
	return left.order < right.order
}

func candidateHeapWorseDescending(left, right candidateHeapItem) bool {
	if candidateHeapBetterDescending(right, left) {
		return true
	}
	if candidateHeapBetterDescending(left, right) {
		return false
	}
	return left.order > right.order
}
