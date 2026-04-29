package series

import "testing"

func TestWindowMeta(t *testing.T) {
	var zero WindowMeta
	if zero.Scope != SampleScopeUnset || zero.TotalCount != 0 {
		t.Fatalf("zero WindowMeta = %+v", zero)
	}
	full := WindowMeta{Generation: 1, PolicyGeneration: 2, Scope: SampleScopeFull, TotalCount: 3, SampledCount: 3}
	if full.Scope != SampleScopeFull || full.TotalCount != full.SampledCount {
		t.Fatalf("full WindowMeta = %+v", full)
	}
	selected := WindowMeta{Scope: SampleScopeSelected, TotalCount: 3, SampledCount: 1}
	if selected.Scope != SampleScopeSelected || selected.TotalCount <= selected.SampledCount {
		t.Fatalf("selected WindowMeta = %+v", selected)
	}
}
