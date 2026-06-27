package worldapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/blackrelay/registry/internal/artefacts"
	"github.com/blackrelay/registry/internal/db"
	"github.com/blackrelay/registry/internal/model"
)

func TestImportDatahubTypesCreatesSourceBackedItemEntities(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "types.json")
	if err := os.WriteFile(path, []byte(`[
		{"itemId":"42","name":"Reflex","groupId":7,"category":"material","description":"Recovered material"}
	]`), 0o644); err != nil {
		t.Fatal(err)
	}
	store := db.NewMemoryStore()
	result, err := ImportDatahubTypes(context.Background(), store, artefacts.LocalStore{Root: filepath.Join(dir, "artefacts")}, path, MetadataOptions{
		Environment:     model.EnvironmentStillness,
		AllowedRootDirs: []string{dir},
		SourceURL:       "https://datahub.evefrontier.com/types.json",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.RowsImported != 1 {
		t.Fatalf("unexpected import count %d", result.RowsImported)
	}
	entity, ok := store.Entities["item:stillness:42"]
	if !ok {
		t.Fatalf("item entity was not imported: %#v", store.Entities)
	}
	if entity.Name != "Reflex" || entity.Type != model.EntityTypeItem {
		t.Fatalf("unexpected entity %#v", entity)
	}
	artefact, ok := store.Artefacts[result.Artefact.ID]
	if !ok || artefact.SourceKind != model.SourceKindDatahub {
		t.Fatalf("datahub artefact was not recorded with provenance: %#v", store.Artefacts)
	}
}

func TestImportWorldSystemsCreatesSystemEntities(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "systems.json")
	if err := os.WriteFile(path, []byte(`[
		{"systemId":"30001001","name":"NN0-Y-D5","region":"Inner Stone Cluster"}
	]`), 0o644); err != nil {
		t.Fatal(err)
	}
	store := db.NewMemoryStore()
	result, err := ImportWorldSystems(context.Background(), store, artefacts.LocalStore{Root: filepath.Join(dir, "artefacts")}, path, MetadataOptions{
		Environment:     model.EnvironmentStillness,
		AllowedRootDirs: []string{dir},
		SourceURL:       "https://world-api.evefrontier.com/systems.json",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.RowsImported != 1 {
		t.Fatalf("unexpected import count %d", result.RowsImported)
	}
	entity, ok := store.Entities["system:stillness:30001001"]
	if !ok {
		t.Fatalf("system entity was not imported: %#v", store.Entities)
	}
	if entity.Name != "NN0-Y-D5" || entity.Type != model.EntityTypeSystem {
		t.Fatalf("unexpected entity %#v", entity)
	}
}

func TestImportWorldTribesCreatesReviewedTribeProfiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tribes.json")
	if err := os.WriteFile(path, []byte(`{
		"tribes": [
			{
				"tribeId": "42",
				"name": "Black Relay",
				"ticker": "BR",
				"description": "Public profile",
				"url": "https://example.invalid/tribes/black-relay",
				"memberCount": 12,
				"foundedAt": 1770000000
			}
		]
	}`), 0o644); err != nil {
		t.Fatal(err)
	}
	store := db.NewMemoryStore()
	result, err := ImportWorldTribes(context.Background(), store, artefacts.LocalStore{Root: filepath.Join(dir, "artefacts")}, path, MetadataOptions{
		Environment:     model.EnvironmentStillness,
		AllowedRootDirs: []string{dir},
		SourceURL:       "https://world-api-stillness.live.pub.evefrontier.com/v2/tribes",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.RowsImported != 1 {
		t.Fatalf("unexpected import count %d", result.RowsImported)
	}
	entity, ok := store.Entities["tribe:stillness:42"]
	if !ok {
		t.Fatalf("tribe entity was not imported: %#v", store.Entities)
	}
	if entity.Name != "Black Relay" || entity.Type != model.EntityTypeTribe {
		t.Fatalf("unexpected entity %#v", entity)
	}
	facts := store.Facts[entity.ID]
	assertWorldFact(t, facts, "display_name", "Black Relay")
	assertWorldFact(t, facts, "tag", "BR")
	assertWorldFact(t, facts, "description", "Public profile")
	assertWorldFact(t, facts, "url", "https://example.invalid/tribes/black-relay")
	assertWorldFact(t, facts, "member_count", "12")
	assertWorldFact(t, facts, "founded_at", "1770000000")
	artefact, ok := store.Artefacts[result.Artefact.ID]
	if !ok || artefact.SourceKind != model.SourceKindWorldAPI || artefact.ArtefactKind != "world_tribes" {
		t.Fatalf("world tribe artefact was not recorded with provenance: %#v", store.Artefacts)
	}
}

func TestImportWorldTribesAcceptsCorporationAliases(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tribes.json")
	if err := os.WriteFile(path, []byte(`[
		{"corpId":"99","corpName":"Outer Coalition","corpTicker":"OC"}
	]`), 0o644); err != nil {
		t.Fatal(err)
	}
	store := db.NewMemoryStore()
	result, err := ImportWorldTribes(context.Background(), store, artefacts.LocalStore{Root: filepath.Join(dir, "artefacts")}, path, MetadataOptions{
		Environment:     model.EnvironmentStillness,
		AllowedRootDirs: []string{dir},
		SourceURL:       "https://world-api-stillness.live.pub.evefrontier.com/v2/tribes",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.RowsImported != 1 {
		t.Fatalf("unexpected import count %d", result.RowsImported)
	}
	entity, ok := store.Entities["tribe:stillness:99"]
	if !ok || entity.DisplayName != "Outer Coalition" {
		t.Fatalf("corporation alias row was not imported: %#v", store.Entities)
	}
	assertWorldFact(t, store.Facts[entity.ID], "tag", "OC")
}

func TestImportWorldTribesAcceptsLiveStillnessFieldNames(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tribes.json")
	if err := os.WriteFile(path, []byte(`{
		"data": [
			{
				"id": 98000547,
				"name": "Algorithmic Warfare",
				"nameShort": "AWAR",
				"description": "EVE Frontier founder tribe.",
				"taxRate": 0.11,
				"tribeUrl": "https://awar.dev"
			}
		],
		"metadata": {"total": 1}
	}`), 0o644); err != nil {
		t.Fatal(err)
	}
	store := db.NewMemoryStore()
	result, err := ImportWorldTribes(context.Background(), store, artefacts.LocalStore{Root: filepath.Join(dir, "artefacts")}, path, MetadataOptions{
		Environment:     model.EnvironmentStillness,
		AllowedRootDirs: []string{dir},
		SourceURL:       "https://world-api-stillness.live.pub.evefrontier.com/v2/tribes",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.RowsImported != 1 {
		t.Fatalf("unexpected import count %d", result.RowsImported)
	}
	entity, ok := store.Entities["tribe:stillness:98000547"]
	if !ok {
		t.Fatalf("live Stillness tribe row was not imported: %#v", store.Entities)
	}
	facts := store.Facts[entity.ID]
	assertWorldFact(t, facts, "tag", "AWAR")
	assertWorldFact(t, facts, "description", "EVE Frontier founder tribe.")
	assertWorldFact(t, facts, "url", "https://awar.dev")
	assertWorldFact(t, facts, "tax_rate", "0.11")
}

func TestImportWorldTribesStampsCycleOnCurrentSnapshotRows(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tribes.json")
	if err := os.WriteFile(path, []byte(`[
		{"id": 98000547, "name": "Algorithmic Warfare", "nameShort": "AWAR"}
	]`), 0o644); err != nil {
		t.Fatal(err)
	}
	cycle := 6
	store := db.NewMemoryStore()
	result, err := ImportWorldTribes(context.Background(), store, artefacts.LocalStore{Root: filepath.Join(dir, "artefacts")}, path, MetadataOptions{
		Environment:     model.EnvironmentStillness,
		AllowedRootDirs: []string{dir},
		SourceURL:       "https://world-api-stillness.live.pub.evefrontier.com/v2/tribes",
		Cycle:           &cycle,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Source.Cycle == nil || *result.Source.Cycle != 6 {
		t.Fatalf("source cycle = %#v, want 6", result.Source.Cycle)
	}
	if result.Artefact.Cycle == nil || *result.Artefact.Cycle != 6 {
		t.Fatalf("artefact cycle = %#v, want 6", result.Artefact.Cycle)
	}
	entity := store.Entities["tribe:stillness:98000547"]
	if entity.Cycle == nil || *entity.Cycle != 6 {
		t.Fatalf("entity cycle = %#v, want 6", entity.Cycle)
	}
	for _, fact := range store.Facts[entity.ID] {
		if fact.Cycle == nil || *fact.Cycle != 6 {
			t.Fatalf("fact %s cycle = %#v, want 6", fact.Key, fact.Cycle)
		}
	}
}

func TestMetadataImportRejectsPrivateEVEFrontierHosts(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "types.json")
	if err := os.WriteFile(path, []byte(`[]`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := ImportDatahubTypes(context.Background(), db.NewMemoryStore(), artefacts.LocalStore{Root: filepath.Join(dir, "artefacts")}, path, MetadataOptions{
		Environment:     model.EnvironmentStillness,
		AllowedRootDirs: []string{dir},
		SourceURL:       "https://data.priv.evefrontier.com/types.json",
	})
	if err == nil {
		t.Fatal("private source host was accepted")
	}
}

func assertWorldFact(t *testing.T, facts []model.Fact, key string, want any) {
	t.Helper()
	for _, fact := range facts {
		if fact.Key == key {
			if fact.Value != want {
				t.Fatalf("fact %s = %#v, want %#v", key, fact.Value, want)
			}
			if fact.SourceID == "" || fact.Confidence != model.ConfidenceVerified || fact.ReviewStatus != model.ReviewStatusReviewed {
				t.Fatalf("fact %s missing provenance: %#v", key, fact)
			}
			return
		}
	}
	t.Fatalf("missing fact %s in %#v", key, facts)
}

func TestFetchSnapshotWritesLocalEvidenceFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"itemId":"42","name":"Reflex"}]`))
	}))
	defer server.Close()
	dir := t.TempDir()
	target := filepath.Join(dir, "snapshot.json")
	result, err := FetchSnapshot(context.Background(), server.URL+"/types.json", target)
	if err != nil {
		t.Fatal(err)
	}
	if result.SizeBytes == 0 || result.Path != target {
		t.Fatalf("unexpected fetch result %#v", result)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `[{"itemId":"42","name":"Reflex"}]` {
		t.Fatalf("unexpected snapshot data %s", data)
	}
}
