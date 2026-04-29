package decision

import "testing"

func TestAction(t *testing.T) {
	action := NewAction(KindGrow, 2, 2, "grow")
	if action.Confidence != 1 || action.TargetIndex != 2 {
		t.Fatalf("NewAction() = %+v", action)
	}
}
