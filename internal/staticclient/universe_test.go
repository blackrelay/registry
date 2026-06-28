package staticclient

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/blackrelay/registry/internal/artefacts"
	"github.com/blackrelay/registry/internal/db"
	"github.com/blackrelay/registry/internal/model"
)

func TestImportUniverseCreatesSourceBackedSystemsRegionsAndRoutes(t *testing.T) {
	dir := t.TempDir()
	schemaDir := filepath.Join(dir, "fsd_binary_schema")
	if err := os.MkdirAll(schemaDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, filepath.Join(schemaDir, "regions.json"), `{
		"10000012": {
			"name": "Inner Stone Cluster",
			"regionID": "10000012",
			"center": [{"vector_schema": {}}, "4", "5", "6"]
		}
	}`)
	writeFixture(t, filepath.Join(schemaDir, "constellations.json"), `{
		"20000068": {
			"name": "Inner First",
			"constellationID": "20000068",
			"regionID": "10000012",
			"center": [{"vector_schema": {}}, "7", "8", "9"]
		}
	}`)
	writeFixture(t, filepath.Join(schemaDir, "systems.json"), `{
		"30001001": {
			"solarSystemID": "30001001",
			"name": "NN0-Y-D5",
			"regionID": "10000012",
			"constellationID": "20000068",
			"center": [{"vector_schema": {}}, "1", "2", "3"],
			"securityClass": "D1",
			"sunTypeID": "45031"
		},
		"30001002": {
			"solarSystemID": "30001002",
			"name": "6RG-Y-T4",
			"regionID": "10000012",
			"constellationID": "20000068",
			"center": [{"vector_schema": {}}, "10", "11", "12"],
			"securityClass": "D1"
		}
	}`)
	writeFixture(t, filepath.Join(schemaDir, "jumps.json"), `{
		"Type: FSD List": [
			{
				"jump": {
					"jumpID": "3535",
					"stargateID": "60000001",
					"fromSystemID": "30001001",
					"toSystemID": "30001002",
					"jumpType": "0"
				}
			}
		]
	}`)

	store := db.NewMemoryStore()
	result, err := ImportUniverse(context.Background(), store, artefacts.LocalStore{Root: filepath.Join(dir, "artefacts")}, dir, UniverseOptions{
		Environment:     model.EnvironmentStillness,
		AllowedRootDirs: []string{dir},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.SystemsImported != 2 || result.RegionsImported != 1 || result.RoutesImported != 1 {
		t.Fatalf("unexpected import counts: %#v", result)
	}
	if len(result.Artefacts) != 4 {
		t.Fatalf("expected four source artefacts, got %d", len(result.Artefacts))
	}

	system := store.Entities["system:stillness:30001001"]
	if system.Name != "NN0-Y-D5" || system.Type != model.EntityTypeSystem {
		t.Fatalf("system was not imported correctly: %#v", system)
	}
	region := store.Entities["region:stillness:10000012"]
	if region.Name != "Inner Stone Cluster" || region.Type != model.EntityTypeRegion {
		t.Fatalf("region was not imported correctly: %#v", region)
	}
	constellation := store.Entities["constellation:stillness:20000068"]
	if constellation.Name != "Inner First" || constellation.Type != model.EntityTypeConstellation {
		t.Fatalf("constellation was not imported correctly: %#v", constellation)
	}
	route := store.Entities["route:stillness:30001001:30001002"]
	if route.Name != "NN0-Y-D5 to 6RG-Y-T4" || route.Type != model.EntityTypeRoute {
		t.Fatalf("route was not imported correctly: %#v", route)
	}
	if !hasFact(store.Facts[system.ID], "region_name", "Inner Stone Cluster") {
		t.Fatalf("system facts did not include region name: %#v", store.Facts[system.ID])
	}
	if !hasFact(store.Facts[system.ID], "constellation_name", "Inner First") {
		t.Fatalf("system facts did not include constellation name: %#v", store.Facts[system.ID])
	}
	if !hasFact(store.Facts[region.ID], "constellation_count", 1) || !hasFact(store.Facts[region.ID], "system_count", 2) {
		t.Fatalf("region facts did not include hierarchy counts: %#v", store.Facts[region.ID])
	}
	if !hasFact(store.Facts[constellation.ID], "system_count", 2) {
		t.Fatalf("constellation facts did not include system count: %#v", store.Facts[constellation.ID])
	}
	if !hasRelation(store.Relations, constellation.ID, "located_in", region.ID) {
		t.Fatalf("constellation region relation was not imported: %#v", store.Relations)
	}
	if !hasRelation(store.Relations, system.ID, "located_in", constellation.ID) {
		t.Fatalf("system constellation relation was not imported: %#v", store.Relations)
	}
	if !hasRelation(store.Relations, "system:stillness:30001001", "links_to", "system:stillness:30001002") {
		t.Fatalf("system link relation was not imported: %#v", store.Relations)
	}
	if !hasRelation(store.Relations, route.ID, "observed_between", "system:stillness:30001001") ||
		!hasRelation(store.Relations, route.ID, "observed_between", "system:stillness:30001002") {
		t.Fatalf("route endpoint relations were not imported: %#v", store.Relations)
	}
}

func TestUpsertUniverseBulkStoreWritesEntitiesBeforeRelations(t *testing.T) {
	store := &orderingBulkStore{seen: make(map[string]struct{})}
	entities := make([]db.EntityFactSet, 0, bulkChunkSize+1)
	for index := 0; index < bulkChunkSize; index++ {
		entities = append(entities, db.EntityFactSet{Entity: model.Entity{
			ID:          "system:stillness:" + string(rune('a'+index%26)),
			Environment: model.EnvironmentStillness,
		}})
	}
	entities = append(entities, db.EntityFactSet{Entity: model.Entity{ID: "route:stillness:1:2", Environment: model.EnvironmentStillness}})
	relations := []db.RelationDraft{
		{SubjectEntityID: "route:stillness:1:2", Predicate: "observed_between", ObjectEntityID: "system:stillness:a", SourceID: "source", Environment: model.EnvironmentStillness},
	}
	if err := upsertUniverse(context.Background(), store, entities, relations); err != nil {
		t.Fatal(err)
	}
}

func writeFixture(t *testing.T, path, data string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func hasFact(facts []model.Fact, key string, value any) bool {
	for _, fact := range facts {
		if fact.Key == key && fact.Value == value {
			return true
		}
	}
	return false
}

func hasRelation(relations map[string]db.RelationDraft, subject, predicate, object string) bool {
	for _, relation := range relations {
		if relation.SubjectEntityID == subject && relation.Predicate == predicate && relation.ObjectEntityID == object {
			return true
		}
	}
	return false
}

type orderingBulkStore struct {
	seen map[string]struct{}
}

func (s *orderingBulkStore) RecordImport(context.Context, string, model.Source, model.SourceArtefact, map[string]any) error {
	return nil
}

func (s *orderingBulkStore) UpsertEntityFacts(context.Context, model.Entity, []db.EntityFactDraft) error {
	return nil
}

func (s *orderingBulkStore) UpsertRelations(context.Context, []db.RelationDraft) error {
	return nil
}

func (s *orderingBulkStore) UpsertEventDerivationBatch(_ context.Context, entities []db.EntityFactSet, relations []db.RelationDraft, _ []model.KillmailRaw) error {
	for _, entity := range entities {
		s.seen[entity.Entity.ID] = struct{}{}
	}
	for _, relation := range relations {
		if _, ok := s.seen[relation.SubjectEntityID]; !ok {
			return errors.New("relation subject was written before entity")
		}
		if _, ok := s.seen[relation.ObjectEntityID]; !ok {
			return errors.New("relation object was written before entity")
		}
	}
	return nil
}
