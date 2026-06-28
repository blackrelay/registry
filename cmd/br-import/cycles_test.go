package main

import "testing"

func TestImportCycleFlagDefaultsToCurrentAndRejectsMultiCycleStamp(t *testing.T) {
	cycle, err := singleImportCycle("")
	if err != nil {
		t.Fatal(err)
	}
	if cycle == nil || *cycle != 6 {
		t.Fatalf("default import cycle = %#v, want 6", cycle)
	}
	cycle, err = singleImportCycle("6")
	if err != nil {
		t.Fatal(err)
	}
	if cycle == nil || *cycle != 6 {
		t.Fatalf("explicit import cycle = %#v, want 6", cycle)
	}
	for _, value := range []string{"all", "5", "5,6"} {
		if _, err := singleImportCycle(value); err == nil {
			t.Fatalf("unsupported import cycle %q was accepted", value)
		}
	}
}
