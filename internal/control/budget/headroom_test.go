package budget

import "testing"

func TestHeadroom(t *testing.T) {
	if Headroom(5, 10) != 5 || Headroom(10, 10) != 0 || Headroom(11, 10) != 0 || Headroom(1, 0) != 0 {
		t.Fatalf("Headroom failed")
	}
}
