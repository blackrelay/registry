package main

import (
	"testing"

	"github.com/blackrelay/registry/internal/model"
	"github.com/blackrelay/registry/internal/sui"
)

func TestRangeBlockedObjectAuditOutputIsCompactAndStable(t *testing.T) {
	output := rangeBlockedObjectAuditOutput(model.EnvironmentStillness, "sui-testnet", []sui.ObjectTypeTarget{{
		Environment: model.EnvironmentStillness,
		Network:     "sui-testnet",
		PackageName: "world",
		PackageID:   "0xworld",
		Role:        sui.PackageRolePublished,
		ModuleName:  "character",
		TypeName:    "PlayerProfile",
		TypeRepr:    "0xworld::character::PlayerProfile",
	}})
	if output.SchemaVersion != "registry.range-blocked-objects.v1" {
		t.Fatalf("unexpected schema version %q", output.SchemaVersion)
	}
	if output.Environment != model.EnvironmentStillness || output.Network != "sui-testnet" || output.Count != 1 {
		t.Fatalf("environment/network/count were not preserved: %#v", output)
	}
	if len(output.Targets) != 1 {
		t.Fatalf("expected one target: %#v", output.Targets)
	}
	target := output.Targets[0]
	if target.PackageName != "world" || target.PackageID != "0xworld" || target.Role != sui.PackageRolePublished || target.ModuleName != "character" || target.TypeName != "PlayerProfile" || target.TypeRepr != "0xworld::character::PlayerProfile" {
		t.Fatalf("target identity was not preserved compactly: %#v", target)
	}
}
