package pressure

import (
	"math"
	"testing"
)

func TestSeverity(t *testing.T) {
	if Severity(LevelNormal) != DefaultNormalSeverity ||
		Severity(LevelMedium) != DefaultMediumSeverity ||
		Severity(LevelHigh) != DefaultHighSeverity ||
		Severity(LevelCritical) != DefaultCriticalSeverity ||
		Severity(Level(99)) != DefaultNormalSeverity {
		t.Fatalf("Severity returned unexpected value")
	}
	for name, value := range map[string]float64{
		"normal":   DefaultNormalSeverity,
		"medium":   DefaultMediumSeverity,
		"high":     DefaultHighSeverity,
		"critical": DefaultCriticalSeverity,
	} {
		if value < 0 || value > 1 || math.IsNaN(value) || math.IsInf(value, 0) {
			t.Fatalf("default pressure severity %s = %v, want finite [0,1]", name, value)
		}
	}
}
