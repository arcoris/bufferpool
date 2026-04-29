package budget

import (
	"math"
	"testing"
)

func TestAllocation(t *testing.T) {
	if ProportionalShare(100, 0, 1) != 0 || ProportionalShare(100, 1, 0) != 0 {
		t.Fatalf("zero score share failed")
	}
	if ProportionalShare(100, 1, 4) != 25 {
		t.Fatalf("normal share failed")
	}
	if ProportionalShareByWeight(100, 1, 4) != 25 {
		t.Fatalf("integer share failed")
	}
	if ProportionalShareByWeight(math.MaxUint64, math.MaxUint64-1, math.MaxUint64) != math.MaxUint64-1 {
		t.Fatalf("huge integer share failed")
	}
	if ProportionalShareByWeight(100, 0, 4) != 0 || ProportionalShareByWeight(100, 1, 0) != 0 {
		t.Fatalf("zero integer weight share failed")
	}
	if ProportionalShareByWeight(100, 5, 4) != 100 {
		t.Fatalf("weight above sum should cap at total")
	}
	if ClampTarget(5, 10, 20) != 10 || ClampTarget(25, 10, 20) != 20 || ClampTarget(15, 10, 20) != 15 {
		t.Fatalf("ClampTarget failed")
	}
	if ClampTarget(5, 20, 10) != 10 {
		t.Fatalf("ClampTarget inverted bounds failed")
	}
}

func BenchmarkControlBudgetProportionalShare(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = ProportionalShare(1<<30, float64(i%1024+1), 2048)
	}
}

func BenchmarkControlBudgetProportionalShareByWeight(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = ProportionalShareByWeight(1<<30, uint64(i%1024+1), 2048)
	}
}
