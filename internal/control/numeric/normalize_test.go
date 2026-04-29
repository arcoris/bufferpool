package numeric

import "testing"

func TestNormalize(t *testing.T) {
	tests := []struct {
		name string
		got  float64
		want float64
	}{
		{name: "zero limit", got: NormalizeToLimit(1, 0), want: 0},
		{name: "below limit", got: NormalizeToLimit(5, 10), want: 0.5},
		{name: "equal limit", got: NormalizeToLimit(10, 10), want: 1},
		{name: "above limit", got: NormalizeToLimit(20, 10), want: 1},
		{name: "float limit", got: NormalizeFloatToLimit(2, 4), want: 0.5},
		{name: "range", got: NormalizeToRange(15, 10, 20), want: 0.5},
		{name: "invalid range", got: NormalizeToRange(15, 20, 10), want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Fatalf("got %v, want %v", tt.got, tt.want)
			}
		})
	}
}
