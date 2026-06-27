package main

import "testing"

func TestIndexerCycleScopeDefaultsToCurrentAndAllowsArchiveOptIn(t *testing.T) {
	scope, err := indexerCycleScope("")
	if err != nil {
		t.Fatal(err)
	}
	if len(scope.Cycles) != 1 || scope.Cycles[0] != 6 || !scope.IncludeUncycled {
		t.Fatalf("default cycle scope = %#v, want current cycle plus unlabelled rows", scope)
	}

	scope, err = indexerCycleScope("all")
	if err != nil {
		t.Fatal(err)
	}
	if !scope.All() {
		t.Fatalf("all cycle scope = %#v, want unrestricted archive scope", scope)
	}

	scope, err = indexerCycleScope("5,6")
	if err != nil {
		t.Fatal(err)
	}
	if len(scope.Cycles) != 2 || scope.Cycles[0] != 5 || scope.Cycles[1] != 6 || scope.IncludeUncycled {
		t.Fatalf("explicit cycle scope = %#v, want strict cycles 5 and 6", scope)
	}
}
