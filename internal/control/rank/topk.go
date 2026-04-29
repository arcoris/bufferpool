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
	if k > len(candidates) {
		k = len(candidates)
	}
	return topKInto(dst, candidates, k, descendingBefore)
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
	if k > len(candidates) {
		k = len(candidates)
	}
	return topKInto(dst, candidates, k, ascendingBefore)
}

// topKInto keeps only the sorted top k candidates in caller-owned storage.
//
// This avoids full candidate sorting for future group-level controller loops
// that need only a small actionable set. It preserves stable semantics: when
// score and tie-break compare equal, earlier input candidates stay ahead of
// later candidates and later equal candidates outside the current top k do not
// displace earlier ones.
func topKInto(dst, candidates []Candidate, k int, before func(left, right Candidate) bool) []Candidate {
	if cap(dst) < k {
		dst = make([]Candidate, 0, k)
	} else {
		dst = dst[:0]
	}
	for _, candidate := range candidates {
		if len(dst) < k {
			dst = append(dst, candidate)
			bubbleCandidateLeft(dst, len(dst)-1, before)
			continue
		}
		insertAt := candidateInsertPosition(dst, candidate, before)
		if insertAt == len(dst) {
			continue
		}
		copy(dst[insertAt+1:], dst[insertAt:len(dst)-1])
		dst[insertAt] = candidate
	}
	return dst
}

// bubbleCandidateLeft restores sorted order after appending one candidate.
func bubbleCandidateLeft(candidates []Candidate, index int, before func(left, right Candidate) bool) {
	for index > 0 && before(candidates[index], candidates[index-1]) {
		candidates[index], candidates[index-1] = candidates[index-1], candidates[index]
		index--
	}
}

// candidateInsertPosition returns len(candidates) when candidate does not enter
// the current top-k set.
func candidateInsertPosition(candidates []Candidate, candidate Candidate, before func(left, right Candidate) bool) int {
	for index, existing := range candidates {
		if before(candidate, existing) {
			return index
		}
	}
	return len(candidates)
}
