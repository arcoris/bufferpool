package activity

import "testing"

func TestIdle(t *testing.T) {
	if IsIdle(0, 10, IdleConfig{Threshold: 0.1}) {
		t.Fatalf("zero quiet windows should disable idle detection")
	}
	if IsIdle(0.05, 1, IdleConfig{Threshold: 0.1, QuietWindows: 2}) {
		t.Fatalf("not enough quiet windows should not be idle")
	}
	if !IsIdle(0.05, 2, IdleConfig{Threshold: 0.1, QuietWindows: 2}) {
		t.Fatalf("quiet windows should be idle")
	}
	if IsIdle(0.2, 2, IdleConfig{Threshold: 0.1, QuietWindows: 2}) {
		t.Fatalf("hot window should not be idle")
	}
}
