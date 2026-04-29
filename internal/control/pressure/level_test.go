package pressure

import "testing"

func TestLevel(t *testing.T) {
	tests := []struct {
		level Level
		name  string
		known bool
	}{
		{level: LevelNormal, name: "normal", known: true},
		{level: LevelMedium, name: "medium", known: true},
		{level: LevelHigh, name: "high", known: true},
		{level: LevelCritical, name: "critical", known: true},
		{level: Level(99), name: "unknown"},
	}
	for _, tt := range tests {
		if tt.level.String() != tt.name || tt.level.IsKnown() != tt.known {
			t.Fatalf("level %d string/known mismatch", tt.level)
		}
	}
}
