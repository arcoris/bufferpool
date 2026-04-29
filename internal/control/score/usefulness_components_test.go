package score

import "testing"

func TestUsefulnessComponentNames(t *testing.T) {
	if ComponentUsefulnessHitRatio == "" ||
		ComponentUsefulnessRetainRatio == "" ||
		ComponentUsefulnessAllocationAvoidance == "" ||
		ComponentUsefulnessActivity == "" {
		t.Fatalf("usefulness component names must be non-empty")
	}
}
