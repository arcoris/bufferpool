package numeric

import (
	"math"
	"testing"
)

func TestClamp(t *testing.T) {
	tests := []struct {
		name     string
		value    float64
		minValue float64
		maxValue float64
		want     float64
	}{
		{name: "below", value: -1, minValue: 0, maxValue: 2, want: 0},
		{name: "inside", value: 1, minValue: 0, maxValue: 2, want: 1},
		{name: "above", value: 3, minValue: 0, maxValue: 2, want: 2},
		{name: "swapped bounds", value: 3, minValue: 2, maxValue: 0, want: 2},
		{name: "nan value", value: math.NaN(), minValue: -1, maxValue: 1, want: 0},
		{name: "inf value", value: math.Inf(1), minValue: -1, maxValue: 1, want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Clamp(tt.value, tt.minValue, tt.maxValue); got != tt.want {
				t.Fatalf("Clamp() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestClamp01AndInvert01(t *testing.T) {
	if Clamp01(2) != 1 || Invert01(2) != 0 {
		t.Fatalf("Clamp01/Invert01 failed")
	}
}
