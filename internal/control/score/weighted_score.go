package score

import "arcoris.dev/bufferpool/internal/control/numeric"

// WeightedScore is a finite normalized score with copied components.
type WeightedScore struct {
	// Value is the weighted average of Components.
	Value float64

	// Components are copied diagnostic inputs used to build Value.
	Components []Component
}

// NewWeightedScore returns a normalized score from components.
func NewWeightedScore(components []Component) WeightedScore {
	return NewWeightedScoreInto(nil, components)
}

// NewWeightedScoreInto writes a normalized score into dst.
//
// Component storage is copied into dst.Components and may reuse existing
// capacity. The input slice is never retained or mutated. Passing nil dst is
// valid and returns the same owning result as NewWeightedScore.
func NewWeightedScoreInto(dst *WeightedScore, components []Component) WeightedScore {
	var score WeightedScore
	if dst != nil {
		score = *dst
	}

	if cap(score.Components) < len(components) {
		score.Components = make([]Component, len(components))
	} else {
		score.Components = score.Components[:len(components)]
	}
	for i, component := range components {
		score.Components[i] = NewComponent(component.Name, component.Value, component.Weight)
	}
	score.Value = WeightedScoreValue(score.Components)
	if dst != nil {
		*dst = score
	}
	return score
}

// WeightedScoreValue returns the weighted value without copying components.
//
// This helper is allocation-conscious for controller loops that only need the
// scalar score. It applies the same value and weight normalization as
// NewWeightedScore but does not retain diagnostic component storage.
func WeightedScoreValue(components []Component) float64 {
	var weightedSum float64
	var weightSum float64
	for _, component := range components {
		normalized := NewComponent(component.Name, component.Value, component.Weight)
		if normalized.Weight <= 0 {
			continue
		}
		weightedSum += normalized.Value * normalized.Weight
		weightSum += normalized.Weight
	}
	if weightSum == 0 {
		return 0
	}
	return numeric.Clamp01(weightedSum / weightSum)
}

// IsZero reports whether the score has no value and no effective weight.
func (s WeightedScore) IsZero() bool {
	if s.Value != 0 {
		return false
	}
	for _, component := range s.Components {
		if component.Weight > 0 && component.Value > 0 {
			return false
		}
	}
	return true
}

// DominantComponent returns the component with the largest weighted contribution.
func (s WeightedScore) DominantComponent() Component {
	var dominant Component
	var best float64
	for _, component := range s.Components {
		contribution := component.Value * component.Weight
		if contribution > best {
			best = contribution
			dominant = component
		}
	}
	return dominant
}
