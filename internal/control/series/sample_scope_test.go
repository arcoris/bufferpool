package series

import "testing"

func TestSampleScope(t *testing.T) {
	if SampleScopeUnset.String() != "unset" || SampleScopeFull.String() != "full" || SampleScopeSelected.String() != "selected" {
		t.Fatalf("unexpected sample scope strings")
	}
	if !SampleScopeUnset.IsKnown() || !SampleScopeFull.IsKnown() || !SampleScopeSelected.IsKnown() {
		t.Fatalf("declared scopes should be known")
	}
	if SampleScope(99).IsKnown() || SampleScope(99).String() != "unknown" {
		t.Fatalf("unknown scope handling failed")
	}
}
