package pressure

import "testing"

func TestThresholds(t *testing.T) {
	tests := []struct {
		name       string
		value      uint64
		thresholds Thresholds
		want       Level
	}{
		{name: "none", value: 10, want: LevelNormal},
		{name: "only medium below", value: 9, thresholds: Thresholds{Medium: 10}, want: LevelNormal},
		{name: "only medium", value: 10, thresholds: Thresholds{Medium: 10}, want: LevelMedium},
		{name: "only high", value: 20, thresholds: Thresholds{High: 20}, want: LevelHigh},
		{name: "only critical", value: 30, thresholds: Thresholds{Critical: 30}, want: LevelCritical},
		{name: "full chain high", value: 20, thresholds: Thresholds{Medium: 10, High: 20, Critical: 30}, want: LevelHigh},
		{name: "full chain critical", value: 31, thresholds: Thresholds{Medium: 10, High: 20, Critical: 30}, want: LevelCritical},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Classify(tt.value, tt.thresholds); got != tt.want {
				t.Fatalf("Classify() = %v, want %v", got, tt.want)
			}
			classifier, err := NewClassifier(tt.thresholds)
			if err != nil {
				t.Fatalf("NewClassifier() error = %v", err)
			}
			if got := classifier.Classify(tt.value); got != tt.want {
				t.Fatalf("Classifier.Classify() = %v, want %v", got, tt.want)
			}
		})
	}
	invalid := []Thresholds{{Medium: 20, High: 10}, {High: 30, Critical: 20}, {Medium: 30, Critical: 20}}
	for _, thresholds := range invalid {
		if err := thresholds.Validate(); err == nil {
			t.Fatalf("Validate(%+v) succeeded, want error", thresholds)
		}
		if _, err := NewClassifier(thresholds); err == nil {
			t.Fatalf("NewClassifier(%+v) succeeded, want error", thresholds)
		}
	}
	if err := (Thresholds{Medium: 10, Critical: 20}).Validate(); err != nil {
		t.Fatalf("valid partial thresholds: %v", err)
	}
	_ = MustNewClassifier(Thresholds{Medium: 10})
}
