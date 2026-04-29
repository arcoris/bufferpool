package decision

import "arcoris.dev/bufferpool/internal/control/numeric"

// Kind describes a generic recommendation kind.
type Kind uint8

const (
	// KindNone means no recommendation.
	KindNone Kind = iota

	// KindObserve means continue observing without action.
	KindObserve

	// KindGrow recommends increasing capacity or budget.
	KindGrow

	// KindShrink recommends reducing capacity or budget.
	KindShrink

	// KindTrim recommends trimming retained state.
	KindTrim

	// KindInvestigate recommends investigating safety or misuse signals.
	KindInvestigate
)

// String returns a stable diagnostic name for k.
func (k Kind) String() string {
	switch k {
	case KindNone:
		return "none"
	case KindObserve:
		return "observe"
	case KindGrow:
		return "grow"
	case KindShrink:
		return "shrink"
	case KindTrim:
		return "trim"
	case KindInvestigate:
		return "investigate"
	default:
		return "unknown"
	}
}

// IsKnown reports whether k is one of the declared recommendation kinds.
func (k Kind) IsKnown() bool {
	return k == KindNone ||
		k == KindObserve ||
		k == KindGrow ||
		k == KindShrink ||
		k == KindTrim ||
		k == KindInvestigate
}

// Recommendation is a generic controller recommendation.
type Recommendation struct {
	// Kind is the generic recommendation kind.
	Kind Kind

	// Confidence is a normalized confidence value.
	Confidence float64

	// Reason is a stable diagnostic reason.
	Reason string
}

// NewRecommendation returns a recommendation with clamped confidence.
func NewRecommendation(kind Kind, confidence float64, reason string) Recommendation {
	return Recommendation{Kind: kind, Confidence: numeric.Clamp01(confidence), Reason: reason}
}

// IsActionable reports whether the recommendation implies a concrete action.
func (r Recommendation) IsActionable() bool {
	return r.Kind != KindNone && r.Kind != KindObserve
}
