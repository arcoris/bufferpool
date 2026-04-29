package rate

import "testing"

func TestOperationRates(t *testing.T) {
	rates := NewOperationRates(3, 1, 1, 4, 2, 2, 4)
	if rates.HitRatio != 0.75 || rates.MissRatio != 0.25 || rates.AllocationRatio != 0.25 ||
		rates.RetainRatio != 0.5 || rates.DropRatio != 0.5 {
		t.Fatalf("NewOperationRates() = %+v", rates)
	}
	zero := NewOperationRates(0, 0, 1, 0, 0, 0, 0)
	if zero.HitRatio != 0 || zero.AllocationRatio != 0 || zero.RetainRatio != 0 {
		t.Fatalf("zero-denominator rates = %+v", zero)
	}
}
