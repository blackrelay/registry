package sui

import (
	"context"
	"testing"
	"time"

	"github.com/blackrelay/registry/internal/db"
	"github.com/blackrelay/registry/internal/model"
)

func TestBuildObjectShapeAuditCollectsDeterministicKeyPaths(t *testing.T) {
	store := db.NewMemoryStore()
	observedAt := time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)
	for _, object := range []db.SuiObjectRecord{
		{
			ID:          "object:assembly:one",
			ObjectID:    "0xassembly1",
			Environment: model.EnvironmentStillness,
			TypeRepr:    testPackageID + "::assembly::Assembly",
			PackageID:   testPackageID,
			Module:      "assembly",
			TypeName:    "Assembly",
			SourceID:    "source:sui:sui-testnet:graphql:objects",
			ObservedAt:  observedAt,
			Payload: map[string]any{
				"json": map[string]any{
					"key":          map[string]any{"tenant": "stillness", "item_id": "100"},
					"metadata":     map[string]any{"name": "Assembly One"},
					"owner_cap_id": "0xcap1",
					"location":     map[string]any{"location_hash": "loc-one"},
				},
			},
		},
		{
			ID:          "object:assembly:two",
			ObjectID:    "0xassembly2",
			Environment: model.EnvironmentStillness,
			TypeRepr:    testPackageID + "::assembly::Assembly",
			PackageID:   testPackageID,
			Module:      "assembly",
			TypeName:    "Assembly",
			SourceID:    "source:sui:sui-testnet:graphql:objects",
			ObservedAt:  observedAt.Add(time.Second),
			Payload: map[string]any{
				"json": map[string]any{
					"key":          map[string]any{"tenant": "stillness", "item_id": "101"},
					"owner_cap_id": "0xcap2",
					"status":       map[string]any{"@variant": "Active"},
				},
			},
		},
		{
			ID:          "object:profile",
			ObjectID:    "0xprofile",
			Environment: model.EnvironmentStillness,
			TypeRepr:    testPackageID + "::character::PlayerProfile",
			PackageID:   testPackageID,
			Module:      "character",
			TypeName:    "PlayerProfile",
			SourceID:    "source:sui:sui-testnet:graphql:objects",
			ObservedAt:  observedAt,
			Payload:     map[string]any{"json": map[string]any{"character_id": "2112091476"}},
		},
	} {
		if err := store.UpsertSuiObject(context.Background(), object); err != nil {
			t.Fatal(err)
		}
	}

	got, err := BuildObjectShapeAudit(context.Background(), store, ObjectShapeAuditOptions{
		Environment: model.EnvironmentStillness,
		TypeName:    "Assembly",
		Limit:       10,
		SampleLimit: 1,
		Now:         func() time.Time { return observedAt },
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.ObjectsScanned != 2 {
		t.Fatalf("expected two assembly objects, got %#v", got)
	}
	if len(got.Samples) != 1 || got.Samples[0].ObjectID != "0xassembly1" {
		t.Fatalf("unexpected samples %#v", got.Samples)
	}
	want := []string{
		"key.item_id",
		"key.tenant",
		"location.location_hash",
		"metadata.name",
		"owner_cap_id",
		"status.@variant",
	}
	if gotPaths := keyPathNames(got.KeyPaths); !sameStrings(gotPaths, want) {
		t.Fatalf("unexpected key paths\n got: %#v\nwant: %#v", gotPaths, want)
	}
}

func keyPathNames(items []ObjectShapeKeyPath) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.Path)
	}
	return out
}

func sameStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
