package score

const (
	// DefaultUsefulnessHitRatioWeight makes reuse success the strongest
	// usefulness signal and discourages growth for retained buffers that are not
	// actually hit.
	DefaultUsefulnessHitRatioWeight = 0.45

	// DefaultUsefulnessAllocationAvoidanceWeight rewards windows that avoid new
	// allocations, reflecting the library's allocation-pressure goal.
	DefaultUsefulnessAllocationAvoidanceWeight = 0.25

	// DefaultUsefulnessRetainRatioWeight is secondary because retaining returned
	// buffers is only valuable if future gets reuse them.
	DefaultUsefulnessRetainRatioWeight = 0.15

	// DefaultUsefulnessActivityWeight keeps recent hotness visible without
	// allowing raw traffic volume to dominate reuse quality.
	DefaultUsefulnessActivityWeight = 0.15

	// DefaultUsefulnessDropPenaltyWeight suppresses usefulness when return drops
	// indicate admission pressure or ineffective retention.
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
