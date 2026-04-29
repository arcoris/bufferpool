package score

import "testing"

func TestComponent(t *testing.T) {
	component := NewComponent("x", 2, -1)
	if component.Name != "x" || component.Value != 1 || component.Weight != 0 {
		t.Fatalf("NewComponent() = %+v", component)
	}
}
