package rank

// TopKDescending returns a sorted copy of the top k highest-scoring candidates.
func TopKDescending(candidates []Candidate, k int) []Candidate {
	return TopKDescendingInto(nil, candidates, k)
}

// TopKDescendingInto returns the top k highest-scoring candidates using dst.
//
// The input candidates slice is never mutated. Existing dst capacity is reused
// when possible, but the returned slice is caller-owned and sorted. A non-
// positive k or empty input returns nil.
func TopKDescendingInto(dst, candidates []Candidate, k int) []Candidate {
	if k <= 0 || len(candidates) == 0 {
		return nil
	}
	copied := copyCandidatesInto(dst, candidates)
	SortDescending(copied)
	if k > len(copied) {
		k = len(copied)
	}
	return copied[:k]
}

// TopKAscending returns a sorted copy of the top k lowest-scoring candidates.
func TopKAscending(candidates []Candidate, k int) []Candidate {
	return TopKAscendingInto(nil, candidates, k)
}

// TopKAscendingInto returns the top k lowest-scoring candidates using dst.
//
// The input candidates slice is never mutated. Existing dst capacity is reused
// when possible, but the returned slice is caller-owned and sorted. A non-
// positive k or empty input returns nil.
func TopKAscendingInto(dst, candidates []Candidate, k int) []Candidate {
	if k <= 0 || len(candidates) == 0 {
		return nil
	}
	copied := copyCandidatesInto(dst, candidates)
	SortAscending(copied)
	if k > len(copied) {
		k = len(copied)
	}
	return copied[:k]
}
