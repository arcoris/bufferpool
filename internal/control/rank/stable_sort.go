package rank

func copyCandidates(candidates []Candidate) []Candidate {
	if len(candidates) == 0 {
		return nil
	}
	copied := make([]Candidate, len(candidates))
	copy(copied, candidates)
	return copied
}
