package rank

import "sort"

// SortDescending stably sorts candidates by score descending and tie-break ascending.
func SortDescending(candidates []Candidate) {
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Score == candidates[j].Score {
			return candidates[i].TieBreak < candidates[j].TieBreak
		}
		return candidates[i].Score > candidates[j].Score
	})
}

// SortAscending stably sorts candidates by score ascending and tie-break ascending.
func SortAscending(candidates []Candidate) {
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Score == candidates[j].Score {
			return candidates[i].TieBreak < candidates[j].TieBreak
		}
		return candidates[i].Score < candidates[j].Score
	})
}
