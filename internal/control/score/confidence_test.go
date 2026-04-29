package score

import "testing"

func TestConfidence(t *testing.T) {
	if ConfidenceFromScore(-1) != 0 || ConfidenceFromScore(2) != 1 {
		t.Fatalf("ConfidenceFromScore clamp failed")
	}
	if ConfidenceFromGap(0.8, 0.3) != 0.5 {
		t.Fatalf("ConfidenceFromGap failed")
	}
	if ConfidenceFromGap(0.5, 0.5) != 0 {
		t.Fatalf("equal candidates should have zero gap confidence")
	}
}
