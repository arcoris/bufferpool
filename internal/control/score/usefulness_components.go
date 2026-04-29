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
)
