package pressure

import "testing"

func TestSeverity(t *testing.T) {
	if Severity(LevelNormal) != DefaultNormalSeverity ||
		Severity(LevelMedium) != DefaultMediumSeverity ||
		Severity(LevelHigh) != DefaultHighSeverity ||
		Severity(LevelCritical) != DefaultCriticalSeverity ||
		Severity(Level(99)) != DefaultNormalSeverity {
		t.Fatalf("Severity returned unexpected value")
	}
}
