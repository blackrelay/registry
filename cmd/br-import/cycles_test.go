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
	cycle, err = singleImportCycle("all")
	if err != nil {
		t.Fatal(err)
	}
	if cycle != nil {
		t.Fatalf("all import cycle = %#v, want nil", cycle)
	}
	cycle, err = singleImportCycle("5")
	if err != nil {
		t.Fatal(err)
	}
	if cycle == nil || *cycle != 5 {
		t.Fatalf("explicit import cycle = %#v, want 5", cycle)
	}
	if _, err := singleImportCycle("5,6"); err == nil {
		t.Fatal("multi-cycle import stamp was accepted")
	}
}
