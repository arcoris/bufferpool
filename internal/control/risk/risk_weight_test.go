package risk

import (
	"math"
	"testing"
)

func TestUsableRiskWeight(t *testing.T) {
	tests := []struct {
		name   string
		weight float64
		want   float64
	}{
		{name: "positive", weight: 1, want: 1},
		{name: "zero", weight: 0, want: 0},
		{name: "negative", weight: -1, want: 0},
		{name: "nan", weight: math.NaN(), want: 0},
		{name: "inf", weight: math.Inf(1), want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := usableRiskWeight(tt.weight); got != tt.want {
				t.Fatalf("usableRiskWeight() = %v, want %v", got, tt.want)
			}
		})
	}
}
