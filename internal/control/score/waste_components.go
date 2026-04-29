package score

const (
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
