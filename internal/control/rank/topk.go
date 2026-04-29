package rank

// TopKDescending returns a sorted copy of the top k highest-scoring candidates.
func TopKDescending(candidates []Candidate, k int) []Candidate {
	if k <= 0 || len(candidates) == 0 {
		return nil
	}
	copied := copyCandidates(candidates)
	SortDescending(copied)
	if k > len(copied) {
		k = len(copied)
	}
	return copied[:k]
}

// TopKAscending returns a sorted copy of the top k lowest-scoring candidates.
func TopKAscending(candidates []Candidate, k int) []Candidate {
	if k <= 0 || len(candidates) == 0 {
		return nil
	}
	copied := copyCandidates(candidates)
	SortAscending(copied)
	if k > len(copied) {
		k = len(copied)
	}
	return copied[:k]
}
