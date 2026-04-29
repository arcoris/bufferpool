package budget

import "testing"

func TestUsage(t *testing.T) {
	tests := []struct {
		name        string
		current     uint64
		limit       uint64
		utilization float64
		over        bool
	}{
		{name: "zero limit", current: 10},
		{name: "under", current: 5, limit: 10, utilization: 0.5},
		{name: "equal", current: 10, limit: 10, utilization: 1},
		{name: "over", current: 20, limit: 10, utilization: 2, over: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewUsage(tt.current, tt.limit)
			if got.Utilization != tt.utilization || got.OverLimit != tt.over {
				t.Fatalf("NewUsage() = %+v", got)
			}
		})
	}
}
