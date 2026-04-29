package series

// SampleScope describes whether an observation covers a full unit or a subset.
type SampleScope uint8

const (
	// SampleScopeUnset is the zero value and means the scope was not specified.
	SampleScopeUnset SampleScope = iota

	// SampleScopeFull means the sample covers the complete observed unit.
	SampleScopeFull

	// SampleScopeSelected means the sample covers a selected subset.
	SampleScopeSelected
)

// String returns a stable diagnostic name.
func (s SampleScope) String() string {
	switch s {
	case SampleScopeUnset:
		return "unset"
	case SampleScopeFull:
		return "full"
	case SampleScopeSelected:
		return "selected"
	default:
		return "unknown"
	}
}

// IsKnown reports whether s is one of the declared values.
func (s SampleScope) IsKnown() bool {
	return s == SampleScopeUnset || s == SampleScopeFull || s == SampleScopeSelected
}
