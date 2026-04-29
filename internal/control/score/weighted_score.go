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
	copied := make([]Component, len(components))
	copy(copied, components)
	values := make([]numeric.WeightedValue, 0, len(copied))
	for i, component := range copied {
		copied[i] = NewComponent(component.Name, component.Value, component.Weight)
		values = append(values, numeric.WeightedValue{Value: copied[i].Value, Weight: copied[i].Weight})
	}
	return WeightedScore{
		Value:      numeric.WeightedAverage(values),
		Components: copied,
	}
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
