package rank

func copyCandidates(candidates []Candidate) []Candidate {
	return copyCandidatesInto(nil, candidates)
}

// copyCandidatesInto copies candidates into caller-owned storage.
//
// It centralizes the defensive-copy rule used by TopK helpers: ranking may sort
// the returned slice, but it must never reorder the caller's input slice.
func copyCandidatesInto(dst, candidates []Candidate) []Candidate {
	if len(candidates) == 0 {
		return nil
	}
	if cap(dst) < len(candidates) {
		dst = make([]Candidate, len(candidates))
	} else {
		dst = dst[:len(candidates)]
	}
	copy(dst, candidates)
	return dst
}
