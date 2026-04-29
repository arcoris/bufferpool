package budget

import "arcoris.dev/bufferpool/internal/control/numeric"

// RedistributionCandidate is one generic target candidate.
type RedistributionCandidate struct {
	// Index identifies the caller-owned candidate.
	Index int

	// Score is the candidate's relative share signal.
	Score float64

	// Current is the current budget for the candidate.
	Current uint64

	// Min is the optional lower target bound.
	Min uint64

	// Max is the optional upper target bound.
	Max uint64
}

// RedistributionTarget is one generic target output.
type RedistributionTarget struct {
	// Index identifies the caller-owned candidate.
	Index int

	// Target is the computed budget target.
	Target uint64
}

// DistributeByScore returns deterministic per-candidate targets.
//
// The helper is simple proportional scaffolding, not full water-filling. It
// does not mutate the input slice and keeps output order aligned to input order.
func DistributeByScore(total uint64, candidates []RedistributionCandidate) []RedistributionTarget {
	if len(candidates) == 0 {
		return nil
	}
	var scoreSum float64
	for _, candidate := range candidates {
		if candidate.Score > 0 {
			scoreSum += numeric.FiniteOrZero(candidate.Score)
		}
	}
	targets := make([]RedistributionTarget, len(candidates))
	for i, candidate := range candidates {
		target := candidate.Current
		if scoreSum > 0 {
			target = ProportionalShare(total, candidate.Score, scoreSum)
		}
		targets[i] = RedistributionTarget{
			Index:  candidate.Index,
			Target: ClampTarget(target, candidate.Min, candidate.Max),
		}
	}
	return targets
}
