package score

import "testing"

func TestUsefulness(t *testing.T) {
	high := Usefulness(UsefulnessInput{HitRatio: 1, RetainRatio: 1, AllocationAvoidance: 1, ActivityScore: 1})
	if high.Value != 1 {
		t.Fatalf("high usefulness = %+v", high)
	}
	lowHit := Usefulness(UsefulnessInput{RetainRatio: 1, AllocationAvoidance: 1, ActivityScore: 1})
	if lowHit.Value >= high.Value {
		t.Fatalf("low hit score should reduce usefulness: %+v", lowHit)
	}
	penalized := Usefulness(UsefulnessInput{HitRatio: 1, RetainRatio: 1, AllocationAvoidance: 1, ActivityScore: 1, DropPenalty: 1})
	if penalized.Value >= high.Value {
		t.Fatalf("drop penalty should reduce usefulness: %+v", penalized)
	}
	if !Usefulness(UsefulnessInput{}).IsZero() {
		t.Fatalf("zero usefulness should be zero")
	}
}
