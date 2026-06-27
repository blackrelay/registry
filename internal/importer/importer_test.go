package importer

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/blackrelay/registry/internal/artefacts"
	"github.com/blackrelay/registry/internal/db"
	"github.com/blackrelay/registry/internal/model"
)

func TestImportStaticEnemiesPromotesReviewedArtefactBackedCandidates(t *testing.T) {
	store := db.NewMemoryStore()
	fixture := filepath.Join("..", "..", "testdata", "fixtures", "static-enemies.reviewed.json")
	result, err := ImportStaticEnemies(context.Background(), store, artefacts.LocalStore{Root: filepath.Join(t.TempDir(), "artefacts")}, fixture, StaticEnemyOptions{
		Environment:     model.EnvironmentStillness,
		AllowedRootDirs: []string{filepath.Join("..", "..", "testdata"), "."},
	})
	if err != nil {
		t.Fatalf("ImportStaticEnemies returned error: %v", err)
	}
	if len(result.Candidates) != 26 {
		t.Fatalf("expected 26 candidates, got %d", len(result.Candidates))
	}
	entity, ok, err := store.GetEntity(context.Background(), "enemy:stillness:type:92096")
	if err != nil {
		t.Fatalf("GetEntity returned error: %v", err)
	}
	if !ok {
		t.Fatal("Caird enemy entity was not imported")
	}
	if entity.Name != "Caird" || entity.DisplayName != "Caird [NPC]" {
		t.Fatalf("unexpected Caird entity: %#v", entity)
	}
	if _, ok, _ := store.GetEntity(context.Background(), "enemy:stillness:type:92095"); ok {
		t.Fatal("unexpected unreviewed enemy entity was imported")
	}
	artefact, ok, err := store.GetArtefact(context.Background(), result.Artefact.ID)
	if err != nil || !ok {
		t.Fatalf("source artefact was not recorded: ok=%v err=%v", ok, err)
	}
	if artefact.RowCount != 26 {
		t.Fatalf("expected artefact row count 26, got %d", artefact.RowCount)
	}
}

func TestImportStaticEnemiesRejectsPathOutsideAllowedRoots(t *testing.T) {
	store := db.NewMemoryStore()
	allowed := t.TempDir()
	other := t.TempDir()
	fixture := filepath.Join(other, "static-enemies.reviewed.json")
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "fixtures", "static-enemies.reviewed.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fixture, data, 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = ImportStaticEnemies(context.Background(), store, artefacts.LocalStore{Root: filepath.Join(allowed, "artefacts")}, fixture, StaticEnemyOptions{
		Environment:     model.EnvironmentStillness,
		AllowedRootDirs: []string{allowed},
	})
	if err == nil {
		t.Fatal("ImportStaticEnemies accepted a path outside the allowed roots")
	}
}

func TestImportTribeIdentitiesPromotesReviewedArtefactBackedNames(t *testing.T) {
	store := db.NewMemoryStore()
	dir := t.TempDir()
	fixture := filepath.Join(dir, "tribe-identities.json")
	data := []byte(`{
	  "schemaVersion": "registry.tribe-identities.v1",
	  "environment": "stillness",
	  "source": {
	    "kind": "community_report",
	    "confidence": "reported",
	    "title": "Reviewed public tribe identity list",
	    "locator": "operator-reviewed-public-list",
	    "checkedAt": "2026-06-26T00:00:00Z",
	    "reviewStatus": "reviewed"
	  },
	  "tribes": [
	    {
	      "tribeId": "42",
	      "name": "Example Relay",
	      "tag": "ER",
	      "aliases": ["Relay Example"],
	      "description": "Example public tribe profile",
	      "url": "https://example.invalid/tribes/example-relay",
	      "confidence": "reported",
	      "sourceContext": "reviewed public profile"
	    }
	  ]
	}`)
	if err := os.WriteFile(fixture, data, 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := ImportTribeIdentities(context.Background(), store, artefacts.LocalStore{Root: filepath.Join(dir, "artefacts")}, fixture, TribeIdentityOptions{
		Environment:     model.EnvironmentStillness,
		AllowedRootDirs: []string{dir},
	})
	if err != nil {
		t.Fatalf("ImportTribeIdentities returned error: %v", err)
	}
	if result.RowsImported != 1 {
		t.Fatalf("expected 1 imported tribe identity, got %d", result.RowsImported)
	}
	entity, ok, err := store.GetEntity(context.Background(), "tribe:stillness:42")
	if err != nil {
		t.Fatalf("GetEntity returned error: %v", err)
	}
	if !ok {
		t.Fatal("reviewed tribe identity was not imported")
	}
	if entity.Name != "Example Relay" || entity.DisplayName != "Example Relay" {
		t.Fatalf("unexpected tribe entity: %#v", entity)
	}
	facts, err := store.ListEntityFacts(context.Background(), entity.ID)
	if err != nil {
		t.Fatalf("ListEntityFacts returned error: %v", err)
	}
	assertFact(t, facts, "tribe_id", "42")
	assertFact(t, facts, "display_name", "Example Relay")
	assertFact(t, facts, "tag", "ER")
	assertFact(t, facts, "description", "Example public tribe profile")
	assertFact(t, facts, "url", "https://example.invalid/tribes/example-relay")
	assertFact(t, facts, "source_context", "reviewed public profile")
	assertFact(t, facts, "source_artefact_id", result.Artefact.ID)
}

func assertFact(t *testing.T, facts []model.Fact, key string, want any) {
	t.Helper()
	for _, fact := range facts {
		if fact.Key == key && fact.Value == want {
			return
		}
	}
	t.Fatalf("missing fact %s=%v in %#v", key, want, facts)
}
