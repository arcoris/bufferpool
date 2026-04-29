package activity

import "testing"

func TestActivityState(t *testing.T) {
	var state ActivityState
	state = state.Update(0.05, 0.1)
	state = state.Update(0.05, 0.1)
	if state.QuietWindows != 2 {
		t.Fatalf("quiet windows = %+v", state)
	}
	state = state.Update(0.5, 0.1)
	if state.QuietWindows != 0 {
		t.Fatalf("hot update should reset quiet windows: %+v", state)
	}
}
