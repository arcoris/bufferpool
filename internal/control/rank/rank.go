package rank

// SortDescending stably sorts candidates by score descending and tie-break ascending.
//
// The implementation is a typed stable insertion sort. Candidate lists used by
// controller ranking are expected to be small, and avoiding sort.SliceStable
// keeps this helper allocation-free for repeated control-loop use.
func SortDescending(candidates []Candidate) {
	sortCandidates(candidates, descendingBefore)
}

// SortAscending stably sorts candidates by score ascending and tie-break ascending.
//
// Like SortDescending, this helper favors allocation-free deterministic
// behavior over a general-purpose sort implementation.
func SortAscending(candidates []Candidate) {
	sortCandidates(candidates, ascendingBefore)
}

// sortCandidates performs stable insertion sort using before as the ordering
// predicate. Equal items are not moved ahead of each other, preserving the
// caller's order after score and TieBreak comparisons are equal.
func sortCandidates(candidates []Candidate, before func(left, right Candidate) bool) {
	for index := 1; index < len(candidates); index++ {
		current := candidates[index]
		position := index
		for position > 0 && before(current, candidates[position-1]) {
			candidates[position] = candidates[position-1]
			position--
		}
		candidates[position] = current
	}
}

// descendingBefore reports whether left should sort before right in descending
// score order.
func descendingBefore(left, right Candidate) bool {
	leftScore := NormalizedScore(left)
	rightScore := NormalizedScore(right)
	if leftScore == rightScore {
		return left.TieBreak < right.TieBreak
	}
	return leftScore > rightScore
}

// ascendingBefore reports whether left should sort before right in ascending
// score order.
func ascendingBefore(left, right Candidate) bool {
	leftScore := NormalizedScore(left)
	rightScore := NormalizedScore(right)
	if leftScore == rightScore {
		return left.TieBreak < right.TieBreak
	}
	return leftScore < rightScore
}
