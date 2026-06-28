package staticclient

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/blackrelay/registry/internal/artefacts"
	"github.com/blackrelay/registry/internal/db"
	"github.com/blackrelay/registry/internal/model"
)

func TestImportTypesPromotesStaticClientRowsAsSourceBackedEntities(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "static-client-types.json")
	writeFixture(t, path, `{
		"candidates": [
			{"groupId":4000,"name":"Reflex","typeId":1001,"typeNameId":2001,"description":"processed resource","wreckTypeId":81610},
			{"groupId":27,"name":"Rifter Frame","typeId":1002,"typeNameId":2002},
			{"groupId":7000,"name":"Frontier Gate Structure","typeId":1003,"typeNameId":2003},
			{"groupId":8000,"name":"Hydrated Sulfide Matrix","typeId":1004,"typeNameId":2004}
		]
	}`)
	store := db.NewMemoryStore()
	result, err := ImportTypes(context.Background(), store, artefacts.LocalStore{Root: filepath.Join(dir, "artefacts")}, path, TypeImportOptions{
		Environment:     model.EnvironmentStillness,
		AllowedRootDirs: []string{dir},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.RowsImported != 4 {
		t.Fatalf("expected four imported type rows, got %#v", result)
	}
	assertStaticTypeEntity(t, store, "item:stillness:type:1001", model.EntityTypeItem)
	assertStaticTypeEntity(t, store, "ship:stillness:type:1002", model.EntityTypeShip)
	assertStaticTypeEntity(t, store, "structure:stillness:type:1003", model.EntityTypeStructure)
	assertStaticTypeEntity(t, store, "material:stillness:type:1004", model.EntityTypeMaterial)
	if !hasFact(store.Facts["item:stillness:type:1001"], "wreck_type_id", 81610) {
		t.Fatalf("static type facts did not preserve wreck type id: %#v", store.Facts["item:stillness:type:1001"])
	}
	if _, ok := store.Artefacts[result.Artefact.ID]; !ok {
		t.Fatalf("source artefact was not recorded: %#v", store.Artefacts)
	}
}

func TestImportTypesClassifiesFromStableGroupAndCategoryMetadata(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "static-client-types.json")
	writeFixture(t, path, `{
		"candidates": [
			{"categoryName":"Ship","groupId":9000,"groupName":"Assault Ships","name":"Name Without Ship Keyword","typeId":2001},
			{"categoryName":"Deployable","groupId":9001,"groupName":"Smart Assemblies","name":"Name Without Structure Keyword","typeId":2002},
			{"categoryName":"Inventory","groupId":9002,"groupName":"Raw Materials","name":"Plain Resource Name","typeId":2003},
			{"groupId":9003,"name":"Plain Commodity","typeId":2004,"typeNameId":0,"wreckTypeId":0}
		]
	}`)
	store := db.NewMemoryStore()
	result, err := ImportTypes(context.Background(), store, artefacts.LocalStore{Root: filepath.Join(dir, "artefacts")}, path, TypeImportOptions{
		Environment:     model.EnvironmentStillness,
		AllowedRootDirs: []string{dir},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.RowsImported != 4 {
		t.Fatalf("expected four imported type rows, got %#v", result)
	}
	assertStaticTypeEntity(t, store, "ship:stillness:type:2001", model.EntityTypeShip)
	assertStaticTypeEntity(t, store, "structure:stillness:type:2002", model.EntityTypeStructure)
	assertStaticTypeEntity(t, store, "material:stillness:type:2003", model.EntityTypeMaterial)
	assertStaticTypeEntity(t, store, "item:stillness:type:2004", model.EntityTypeItem)
	if hasFact(store.Facts["item:stillness:type:2004"], "type_name_id", 0) || hasFact(store.Facts["item:stillness:type:2004"], "wreck_type_id", 0) {
		t.Fatalf("zero-valued optional IDs should not be stored as facts: %#v", store.Facts["item:stillness:type:2004"])
	}
}

func TestImportTypesTrimsStaticClientNameWrapperQuotes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "static-client-types.json")
	writeFixture(t, path, `{
		"candidates": [
			{"groupId":9003,"name":"'Analytical Mind'","typeId":3001},
			{"groupId":9003,"name":"'Augmented' Acolyte","typeId":3002}
		]
	}`)
	store := db.NewMemoryStore()
	_, err := ImportTypes(context.Background(), store, artefacts.LocalStore{Root: filepath.Join(dir, "artefacts")}, path, TypeImportOptions{
		Environment:     model.EnvironmentStillness,
		AllowedRootDirs: []string{dir},
	})
	if err != nil {
		t.Fatal(err)
	}
	entity := store.Entities["item:stillness:type:3001"]
	if entity.Name != "Analytical Mind" || entity.DisplayName != "Analytical Mind" {
		t.Fatalf("static-client wrapper quotes were not trimmed: %#v", entity)
	}
	entity = store.Entities["item:stillness:type:3002"]
	if entity.Name != "Augmented Acolyte" || entity.DisplayName != "Augmented Acolyte" {
		t.Fatalf("static-client paired name quotes were not trimmed: %#v", entity)
	}
}

func assertStaticTypeEntity(t *testing.T, store *db.MemoryStore, id string, entityType model.EntityType) {
	t.Helper()
	entity := store.Entities[id]
	if entity.ID != id || entity.Type != entityType {
		t.Fatalf("expected %s entity %s, got %#v", entityType, id, entity)
	}
}
