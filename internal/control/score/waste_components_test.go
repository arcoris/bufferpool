package score

import "testing"

func TestWasteComponentNames(t *testing.T) {
	if ComponentWasteLowHit == "" ||
		ComponentWasteRetainedPressure == "" ||
		ComponentWasteLowActivity == "" ||
		ComponentWasteDrop == "" {
		t.Fatalf("waste component names must be non-empty")
	}
}
