package main

import "testing"

func TestIndexerCycleScopeDefaultsToCurrentAndRejectsArchiveOptIn(t *testing.T) {
	scope, err := indexerCycleScope("")
	if err != nil {
		t.Fatal(err)
	}
	if len(scope.Cycles) != 1 || scope.Cycles[0] != 6 || !scope.IncludeUncycled {
		t.Fatalf("default cycle scope = %#v, want current cycle plus unlabelled rows", scope)
	}

	scope, err = indexerCycleScope("6")
	if err != nil {
		t.Fatal(err)
	}
	if len(scope.Cycles) != 1 || scope.Cycles[0] != 6 || scope.IncludeUncycled {
		t.Fatalf("explicit cycle scope = %#v, want strict current cycle", scope)
	}

	for _, value := range []string{"all", "5", "5,6"} {
		if _, err := indexerCycleScope(value); err == nil {
			t.Fatalf("unsupported cycle scope %q was accepted", value)
		}
	}
}
