package report

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/blackrelay/registry/internal/db"
	"github.com/blackrelay/registry/internal/model"
)

func TestBuildSystemReconciliationComparesStaticUniverseWithRegistrySystems(t *testing.T) {
	dir := t.TempDir()
	schemaDir := filepath.Join(dir, "fsd_binary_schema")
	if err := os.MkdirAll(schemaDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(schemaDir, "systems.json"), []byte(`{
		"30001001": {"solarSystemID": 30001001, "name": "NN0-Y-D5"},
		"30001002": {"solarSystemID": 30001002, "name": "6RG-Y-T4"}
	}`), 0o644); err != nil {
		t.Fatal(err)
	}

	store := db.NewMemoryStore()
	now := time.Date(2026, 6, 25, 17, 0, 0, 0, time.UTC)
	for _, entity := range []model.Entity{
		{ID: "system:stillness:30001001", Slug: "system-30001001-stillness", Type: model.EntityTypeSystem, Name: "NN0-Y-D5", DisplayName: "NN0-Y-D5", Environment: model.EnvironmentStillness, UpdatedAt: now},
		{ID: "system:stillness:30009999", Slug: "system-30009999-stillness", Type: model.EntityTypeSystem, Name: "Extra", DisplayName: "Extra", Environment: model.EnvironmentStillness, UpdatedAt: now.Add(-time.Minute)},
	} {
		if err := store.UpsertEntityFacts(context.Background(), entity, []db.EntityFactDraft{{
			Key:          "solar_system_id",
			Value:        entity.ID[len("system:stillness:"):],
			SourceID:     "source:test",
			Confidence:   model.ConfidenceVerified,
			Environment:  model.EnvironmentStillness,
			ReviewStatus: model.ReviewStatusReviewed,
		}}); err != nil {
			t.Fatal(err)
		}
	}

	got, err := BuildSystemReconciliation(context.Background(), store, dir, SystemReconciliationOptions{
		Environment: model.EnvironmentStillness,
		Now:         func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.SourceSystemCount != 2 || got.RegistrySystemCount != 2 || got.MatchedSystemCount != 1 {
		t.Fatalf("unexpected reconciliation counts: %#v", got)
	}
	if len(got.MissingInRegistry) != 1 || got.MissingInRegistry[0] != "30001002" {
		t.Fatalf("unexpected missing-in-registry systems: %#v", got.MissingInRegistry)
	}
	if len(got.MissingInSource) != 1 || got.MissingInSource[0] != "30009999" {
		t.Fatalf("unexpected missing-in-source systems: %#v", got.MissingInSource)
	}
}
