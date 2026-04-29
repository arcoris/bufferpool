package decision

import "testing"

func TestRecommendation(t *testing.T) {
	tests := []struct {
		kind       Kind
		actionable bool
	}{
		{kind: KindNone},
		{kind: KindObserve},
		{kind: KindGrow, actionable: true},
		{kind: KindShrink, actionable: true},
		{kind: KindTrim, actionable: true},
		{kind: KindInvestigate, actionable: true},
	}
	for _, tt := range tests {
		recommendation := NewRecommendation(tt.kind, 2, "test")
		if recommendation.Confidence != 1 || recommendation.IsActionable() != tt.actionable {
			t.Fatalf("recommendation = %+v", recommendation)
		}
	}
}
