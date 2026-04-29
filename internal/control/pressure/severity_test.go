package pressure

import "testing"

func TestSeverity(t *testing.T) {
	if Severity(LevelNormal) != 0 || Severity(LevelMedium) != 0.33 || Severity(LevelHigh) != 0.66 ||
		Severity(LevelCritical) != 1 || Severity(Level(99)) != 0 {
		t.Fatalf("Severity returned unexpected value")
	}
}
