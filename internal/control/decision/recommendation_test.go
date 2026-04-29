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
		if !tt.kind.IsKnown() || tt.kind.String() == "unknown" {
			t.Fatalf("kind helpers failed for %v", tt.kind)
		}
	}
	if Kind(99).IsKnown() || Kind(99).String() != "unknown" {
		t.Fatalf("unknown kind helpers failed")
	}
}

func BenchmarkControlDecisionRecommendation(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = NewRecommendation(KindGrow, 0.75, "benchmark")
	}
}
