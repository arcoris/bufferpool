package pressure

// Level is a generic pressure severity level.
type Level uint8

const (
	// LevelNormal means no configured threshold is currently exceeded.
	LevelNormal Level = iota

	// LevelMedium means the medium threshold is exceeded.
	LevelMedium

	// LevelHigh means the high threshold is exceeded.
	LevelHigh

	// LevelCritical means the critical threshold is exceeded.
	LevelCritical
)

// String returns a stable diagnostic name for the level.
func (l Level) String() string {
	switch l {
	case LevelNormal:
		return "normal"
	case LevelMedium:
		return "medium"
	case LevelHigh:
		return "high"
	case LevelCritical:
		return "critical"
	default:
		return "unknown"
	}
}

// IsKnown reports whether l is one of the declared levels.
func (l Level) IsKnown() bool {
	return l == LevelNormal || l == LevelMedium || l == LevelHigh || l == LevelCritical
}
