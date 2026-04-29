package pressure

// Severity maps a pressure level to a normalized control-plane severity.
func Severity(level Level) float64 {
	switch level {
	case LevelNormal:
		return 0
	case LevelMedium:
		return 0.33
	case LevelHigh:
		return 0.66
	case LevelCritical:
		return 1
	default:
		return 0
	}
}
