package main

import (
	"testing"

	"github.com/blackrelay/registry/internal/db"
	"github.com/blackrelay/registry/internal/model"
)

func TestResolveEvidenceOutputIncludesBridgeCounts(t *testing.T) {
	output := resolveEvidenceOutput(model.EnvironmentStillness, "sui-testnet", db.EvidenceRelationResolutionCounts{
		OwnershipRelations: 7,
		LocationRelations:  11,
	})
	if output["environment"] != model.EnvironmentStillness {
		t.Fatalf("environment was not preserved: %#v", output)
	}
	if output["network"] != "sui-testnet" {
		t.Fatalf("network was not preserved: %#v", output)
	}
	if output["ownershipRelations"] != int64(7) || output["locationRelations"] != int64(11) {
		t.Fatalf("relation counts were not exposed: %#v", output)
	}
}
