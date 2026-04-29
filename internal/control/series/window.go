package series

// WindowMeta is generic metadata for a windowed observation.
type WindowMeta struct {
	// Generation is the current observed state generation.
	Generation uint64

	// PolicyGeneration is the current observed policy generation.
	PolicyGeneration uint64

	// Scope describes whether the sample covers a full unit or selected subset.
	Scope SampleScope

	// TotalCount is the total number of observed children in the unit.
	TotalCount int

	// SampledCount is the number of children represented by this window.
	SampledCount int
}
