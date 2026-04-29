package pressure

const (
	// DefaultNormalSeverity represents no pressure. It should not contribute to
	// shrink, trim, or risk decisions by itself.
	DefaultNormalSeverity = 0.0

	// DefaultMediumSeverity is a low but visible pressure signal. It encourages
	// observation and mild caution without implying urgent contraction.
	DefaultMediumSeverity = 0.33

	// DefaultHighSeverity is a strong pressure signal. It should materially
	// affect waste and recommendation logic, but still remain below critical.
	DefaultHighSeverity = 0.66

	// DefaultCriticalSeverity is the maximum generic pressure signal.
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
