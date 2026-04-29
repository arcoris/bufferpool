package stability

import "testing"

func TestConsecutiveState(t *testing.T) {
	var state ConsecutiveState
	state = state.Update(true).Update(true)
	if state.Count != 2 || !state.Reached(2) {
		t.Fatalf("consecutive state = %+v", state)
	}
	if state.Reached(0) {
		t.Fatalf("required zero should not trigger")
	}
	state = state.Update(false)
	if state.Count != 0 {
		t.Fatalf("false condition should reset: %+v", state)
	}
}
