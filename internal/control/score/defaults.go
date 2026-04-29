package score

const (
	// ComponentUsefulnessHitRatio names the reuse-success component. Hit ratio
	// receives the strongest usefulness weight because it directly measures
	// whether retained buffers satisfy future gets.
	ComponentUsefulnessHitRatio = "hit_ratio"

	// ComponentUsefulnessRetainRatio names the return-retention component.
	// Retention is useful only when retained buffers are later reused, so its
	// default weight is lower than hit ratio and allocation avoidance.
	ComponentUsefulnessRetainRatio = "retain_ratio"

	// ComponentUsefulnessAllocationAvoidance names the allocation-avoidance
	// component. It is a strong signal because bufferpool's primary value is
	// reducing allocation and GC pressure.
	ComponentUsefulnessAllocationAvoidance = "allocation_avoidance"

	// ComponentUsefulnessActivity names the recent-activity component. Activity
	// moderates usefulness so cold retained capacity does not look valuable only
	// because of old lifetime behavior.
	ComponentUsefulnessActivity = "activity"

	// ComponentWasteLowHit names the ineffective-reuse component. Low hit ratio
	// is the strongest waste signal because retained memory is not producing
	// reuse.
	ComponentWasteLowHit = "low_hit"

	// ComponentWasteRetainedPressure names the memory-pressure component. It
	// captures the opportunity cost of retained bytes under pressure.
	ComponentWasteRetainedPressure = "retained_pressure"

	// ComponentWasteLowActivity names the cold-capacity component. It detects
	// retained memory attached to quiet workloads without overpowering reuse
	// quality.
	ComponentWasteLowActivity = "low_activity"

	// ComponentWasteDrop names the return-drop component. Drops matter, but they
	// can be intentional under pressure, so the default weight is lower than
	// low-hit and retained-pressure signals.
	ComponentWasteDrop = "drop"
)

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

	// DefaultWasteLowHitWeight emphasizes retained capacity that is not serving
	// reuse hits.
	DefaultWasteLowHitWeight = 0.35

	// DefaultWasteRetainedPressureWeight makes memory pressure a strong waste
	// signal because retained bytes have opportunity cost.
	DefaultWasteRetainedPressureWeight = 0.30

	// DefaultWasteLowActivityWeight marks quiet retained capacity as a shrink or
	// trim candidate without overreacting to one quiet window.
	DefaultWasteLowActivityWeight = 0.20

	// DefaultWasteDropWeight keeps drops visible while avoiding domination by
	// intentional pressure-policy drops.
	DefaultWasteDropWeight = 0.15
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

// WasteWeights configures waste score composition.
type WasteWeights struct {
	// LowHit weights ineffective retained reuse.
	LowHit float64

	// RetainedPressure weights retained memory opportunity cost.
	RetainedPressure float64

	// LowActivity weights cold workload behavior.
	LowActivity float64

	// Drop weights returned-buffer drop behavior.
	Drop float64
}

// DefaultWasteWeights returns conservative initial waste weights.
func DefaultWasteWeights() WasteWeights {
	return WasteWeights{
		LowHit:           DefaultWasteLowHitWeight,
		RetainedPressure: DefaultWasteRetainedPressureWeight,
		LowActivity:      DefaultWasteLowActivityWeight,
		Drop:             DefaultWasteDropWeight,
	}
}
