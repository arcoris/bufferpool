package pressure

const (
	// DefaultNormalSeverity represents no pressure. It should not contribute to
	// shrink, trim, or risk decisions by itself. This zero value is a structural
	// baseline rather than a heuristic.
	DefaultNormalSeverity = 0.0

	// DefaultMediumSeverity is a low but visible pressure signal. It encourages
	// observation and mild caution without implying urgent contraction. It is
	// intentionally below high severity to avoid overreacting to early pressure.
	// This is a tunable heuristic.
	DefaultMediumSeverity = 0.33

	// DefaultHighSeverity is a strong pressure signal. It should materially
	// affect waste and recommendation logic, but still remain below critical. It
	// encourages pressure-aware recommendations while preserving a separate
	// critical ceiling. This is a tunable heuristic.
	DefaultHighSeverity = 0.66

	// DefaultCriticalSeverity is the maximum generic pressure signal. It marks a
	// saturated severity projection and is a structural upper bound for the
	// normalized pressure scale.
	DefaultCriticalSeverity = 1.0
)

// Severity maps a pressure level to a normalized control-plane severity.
func Severity(level Level) float64 {
	switch level {
	case LevelNormal:
		return DefaultNormalSeverity
	case LevelMedium:
		return DefaultMediumSeverity
	case LevelHigh:
		return DefaultHighSeverity
	case LevelCritical:
		return DefaultCriticalSeverity
	default:
		return DefaultNormalSeverity
	}
}
