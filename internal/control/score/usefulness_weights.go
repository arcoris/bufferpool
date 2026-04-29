package score

const (
	// DefaultUsefulnessHitRatioWeight makes reuse success the strongest
	// usefulness signal because hits directly prove retained-buffer reuse. It is
	// stronger than retention or activity signals, encourages keeping capacity
	// that serves gets, and avoids rewarding retained buffers that are never hit.
	// This is a tunable conservative heuristic, not a hard invariant.
	DefaultUsefulnessHitRatioWeight = 0.45

	// DefaultUsefulnessAllocationAvoidanceWeight rewards windows that avoid new
	// allocations, reflecting the library's allocation-pressure goal. It is
	// second to hit ratio because avoided allocations matter most when reuse is
	// actually effective, encourages lower GC pressure, and avoids growing
	// retention only because traffic volume is high. This is a tunable heuristic.
	DefaultUsefulnessAllocationAvoidanceWeight = 0.25

	// DefaultUsefulnessRetainRatioWeight is secondary because retaining returned
	// buffers is only valuable if future gets reuse them. It encourages
	// successful admission without letting admission alone dominate reuse
	// quality, avoiding growth for retained capacity that is not later hit. This
	// is a tunable heuristic.
	DefaultUsefulnessRetainRatioWeight = 0.15

	// DefaultUsefulnessActivityWeight keeps recent hotness visible without
	// allowing raw traffic volume to dominate reuse quality. It encourages
	// attention to active partitions while avoiding a pure throughput score. This
	// is a tunable heuristic.
	DefaultUsefulnessActivityWeight = 0.15

	// DefaultUsefulnessDropPenaltyWeight suppresses usefulness when return drops
	// indicate admission pressure or ineffective retention. It is subtractive and
	// intentionally strong enough to counter otherwise useful signals during
	// pressure spikes, avoiding growth recommendations when returned buffers are
	// not being admitted. This is a tunable heuristic.
	DefaultUsefulnessDropPenaltyWeight = 0.35
)

// UsefulnessWeights configures usefulness score composition.
type UsefulnessWeights struct {
	// HitRatio weights direct reuse success.
	HitRatio float64

	// AllocationAvoidance weights avoided allocation and GC pressure.
	AllocationAvoidance float64

	// RetainRatio weights successful returned-buffer retention.
	RetainRatio float64

	// Activity weights recent workload hotness.
	Activity float64

	// DropPenalty weights the subtractive penalty for returned buffers that were
	// not admitted to retained storage.
	DropPenalty float64
}

// DefaultUsefulnessWeights returns conservative initial usefulness weights.
func DefaultUsefulnessWeights() UsefulnessWeights {
	return UsefulnessWeights{
		HitRatio:            DefaultUsefulnessHitRatioWeight,
		AllocationAvoidance: DefaultUsefulnessAllocationAvoidanceWeight,
		RetainRatio:         DefaultUsefulnessRetainRatioWeight,
		Activity:            DefaultUsefulnessActivityWeight,
		DropPenalty:         DefaultUsefulnessDropPenaltyWeight,
	}
}

// normalizeUsefulnessWeights converts invalid weights to zero and applies the
// documented defaults when the caller leaves the entire config unset.
func normalizeUsefulnessWeights(weights UsefulnessWeights) UsefulnessWeights {
	defaults := DefaultUsefulnessWeights()
	if weights == (UsefulnessWeights{}) {
		return defaults
	}
	weights.HitRatio = usableScoreWeight(weights.HitRatio)
	weights.AllocationAvoidance = usableScoreWeight(weights.AllocationAvoidance)
	weights.RetainRatio = usableScoreWeight(weights.RetainRatio)
	weights.Activity = usableScoreWeight(weights.Activity)
	weights.DropPenalty = usableScoreWeight(weights.DropPenalty)
	return weights
}
