package decision

import "testing"

func TestEvaluation(t *testing.T) {
	evaluation := NewEvaluation(2, NewRecommendation(KindObserve, 0.5, "observe"))
	if evaluation.Score != 1 || evaluation.Recommendation.Kind != KindObserve {
		t.Fatalf("NewEvaluation() = %+v", evaluation)
	}
}
