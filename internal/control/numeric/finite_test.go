package numeric

import (
	"math"
	"testing"
)

func TestFinite(t *testing.T) {
	if !IsFinite(1) || IsFinite(math.NaN()) || IsFinite(math.Inf(1)) || IsFinite(math.Inf(-1)) {
		t.Fatalf("IsFinite returned unexpected result")
	}
	if FiniteOrZero(math.NaN()) != 0 || FiniteOrZero(math.Inf(1)) != 0 || FiniteOrZero(2) != 2 {
		t.Fatalf("FiniteOrZero returned unexpected result")
	}
}
