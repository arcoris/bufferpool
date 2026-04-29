package budget

import "testing"

func TestDeficit(t *testing.T) {
	if Deficit(11, 10) != 1 || Deficit(10, 10) != 0 || Deficit(5, 10) != 0 || Deficit(1, 0) != 0 {
		t.Fatalf("Deficit failed")
	}
}
