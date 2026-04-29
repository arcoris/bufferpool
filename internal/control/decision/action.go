package decision

import "arcoris.dev/bufferpool/internal/control/numeric"

// Action is a generic action proposal for a candidate target.
type Action struct {
	// Kind is the proposed action kind.
	Kind Kind

	// TargetIndex identifies caller-owned target data.
	TargetIndex int

	// Confidence is a normalized action confidence.
	Confidence float64

	// Reason is a stable diagnostic reason.
	Reason string
}

// NewAction returns an action proposal with clamped confidence.
func NewAction(kind Kind, targetIndex int, confidence float64, reason string) Action {
	return Action{Kind: kind, TargetIndex: targetIndex, Confidence: numeric.Clamp01(confidence), Reason: reason}
}
