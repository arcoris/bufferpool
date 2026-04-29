package smooth

import "testing"

func TestNamedEWMA(t *testing.T) {
	named := NamedEWMA{Name: "hit_ratio", EWMA: EWMA{Initialized: true, Value: 0.5}}
	if named.Name != "hit_ratio" || !named.EWMA.Initialized || named.EWMA.Value != 0.5 {
		t.Fatalf("NamedEWMA = %+v", named)
	}
}
