package smooth

import (
	"math"
	"testing"
)

func TestEWMA(t *testing.T) {
	var avg EWMA
	avg = avg.Update(0.2, 10)
	if !avg.Initialized || avg.Value != 10 {
		t.Fatalf("first update = %+v", avg)
	}
	avg = avg.Update(0.5, 20)
	if avg.Value != 15 {
		t.Fatalf("second update = %+v, want 15", avg)
	}
	if got := avg.Update(1, 40).Value; got != 40 {
		t.Fatalf("alpha 1 update = %v, want 40", got)
	}
	if got := avg.Update(0, 40).Value; got != avg.Value {
		t.Fatalf("alpha 0 update changed value to %v", got)
	}
	if got := avg.Update(2, 40).Value; got != 40 {
		t.Fatalf("alpha >1 update = %v, want 40", got)
	}
	if got := (EWMA{}).Update(0.2, math.NaN()).Value; got != 0 {
		t.Fatalf("non-finite update = %v, want 0", got)
	}
	avg.UpdateInPlace(1, 30)
	if avg.Value != 30 {
		t.Fatalf("UpdateInPlace = %+v", avg)
	}
}

func TestEWMASmoother(t *testing.T) {
	smoother, err := NewEWMASmoother(AlphaConfig{})
	if err != nil {
		t.Fatalf("NewEWMASmoother(default) error = %v", err)
	}
	state := smoother.Update(EWMA{}, 10)
	if !state.Initialized || state.Value != 10 {
		t.Fatalf("first smoother update = %+v", state)
	}
	state = smoother.Update(state, 20)
	if state.Value != 12 {
		t.Fatalf("second smoother update = %+v, want 12", state)
	}
	if _, err := NewEWMASmoother(AlphaConfig{Alpha: math.NaN()}); err == nil {
		t.Fatalf("NewEWMASmoother accepted non-finite alpha")
	}
	if got := MustNewEWMASmoother(AlphaConfig{Alpha: 1}).Update(state, 40).Value; got != 40 {
		t.Fatalf("MustNewEWMASmoother alpha=1 update = %v, want 40", got)
	}
}

func BenchmarkControlSmoothEWMAUpdate(b *testing.B) {
	b.ReportAllocs()
	avg := EWMA{}
	for i := 0; i < b.N; i++ {
		avg = avg.Update(0.2, float64(i))
	}
	_ = avg
}

func BenchmarkControlSmoothEWMASmootherUpdate(b *testing.B) {
	b.ReportAllocs()
	smoother := MustNewEWMASmoother(AlphaConfig{Alpha: 0.2})
	avg := EWMA{}
	for i := 0; i < b.N; i++ {
		avg = smoother.Update(avg, float64(i))
	}
	_ = avg
}
