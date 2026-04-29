package smooth

import (
	"math"
	"testing"
	"time"
)

func TestAlphaConfig(t *testing.T) {
	if got := (AlphaConfig{}).Normalize().Alpha; got != DefaultAlpha {
		t.Fatalf("Normalize alpha = %v, want %v", got, DefaultAlpha)
	}
	valid := AlphaConfig{Alpha: 1}
	if err := valid.Validate(); err != nil {
		t.Fatalf("Validate alpha=1: %v", err)
	}
	invalid := []AlphaConfig{{Alpha: -1}, {Alpha: 0}, {Alpha: 2}, {Alpha: math.NaN()}, {Alpha: math.Inf(1)}}
	for _, cfg := range invalid {
		if err := cfg.Validate(); err == nil {
			t.Fatalf("Validate(%+v) succeeded, want error", cfg)
		}
	}
	alpha := AlphaFromHalfLife(time.Second, time.Second)
	if alpha != 0.5 {
		t.Fatalf("AlphaFromHalfLife = %v, want 0.5", alpha)
	}
	if AlphaFromHalfLife(0, time.Second) != 0 || AlphaFromHalfLife(time.Second, 0) != 0 {
		t.Fatalf("AlphaFromHalfLife non-positive duration should return zero")
	}
}
