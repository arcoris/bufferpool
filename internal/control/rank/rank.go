package rank

import "sort"

// SortDescending stably sorts candidates by score descending and tie-break ascending.
func SortDescending(candidates []Candidate) {
	sort.SliceStable(candidates, func(i, j int) bool {
		left := NormalizedScore(candidates[i])
		right := NormalizedScore(candidates[j])
		if left == right {
			return candidates[i].TieBreak < candidates[j].TieBreak
		}
		return left > right
	})
}

// SortAscending stably sorts candidates by score ascending and tie-break ascending.
func SortAscending(candidates []Candidate) {
	sort.SliceStable(candidates, func(i, j int) bool {
		left := NormalizedScore(candidates[i])
		right := NormalizedScore(candidates[j])
		if left == right {
			return candidates[i].TieBreak < candidates[j].TieBreak
		}
		return left < right
	})
}
