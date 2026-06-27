package staticclient

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/blackrelay/registry/internal/artefacts"
	"github.com/blackrelay/registry/internal/db"
	"github.com/blackrelay/registry/internal/model"
)

func TestImportRecipesPromotesSourceBackedRecipeBlueprintAndItemPlaceholders(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "static-client-recipes.json")
	writeFixture(t, path, `{
		"recipes": [
			{
				"recipeId": "reflex",
				"name": "Reflex",
				"outputTypeId": 1001,
				"outputQuantity": 1,
				"blueprintTypeId": 75001,
				"facilityTypeId": 70001,
				"inputs": [
					{"typeId": 3001, "quantity": 3},
					{"typeId": 3002, "quantity": 1}
				],
				"sourceContext": "fixture recipe row"
			}
		]
	}`)

	store := db.NewMemoryStore()
	result, err := ImportRecipes(context.Background(), store, artefacts.LocalStore{Root: filepath.Join(dir, "artefacts")}, path, RecipeImportOptions{
		Environment:     model.EnvironmentStillness,
		AllowedRootDirs: []string{dir},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.RowsImported != 1 {
		t.Fatalf("expected one imported recipe, got %#v", result)
	}
	assertStaticTypeEntity(t, store, "recipe:stillness:reflex", model.EntityTypeRecipe)
	assertStaticTypeEntity(t, store, "blueprint:stillness:type:75001", model.EntityTypeBlueprint)
	assertStaticTypeEntity(t, store, "item:stillness:type:1001", model.EntityTypeItem)
	assertStaticTypeEntity(t, store, "item:stillness:type:3001", model.EntityTypeItem)
	assertStaticTypeEntity(t, store, "structure:stillness:type:70001", model.EntityTypeStructure)
	if !hasFact(store.Facts["recipe:stillness:reflex"], "output_quantity", 1) {
		t.Fatalf("recipe output quantity fact was not recorded: %#v", store.Facts["recipe:stillness:reflex"])
	}
	if !hasFactDeep(store.Facts["recipe:stillness:reflex"], "output", map[string]any{"typeId": 1001, "quantity": 1}) {
		t.Fatalf("recipe output fact was not recorded: %#v", store.Facts["recipe:stillness:reflex"])
	}
	if !hasFactDeep(store.Facts["recipe:stillness:reflex"], "inputs", []map[string]any{
		{"typeId": 3001, "quantity": 3},
		{"typeId": 3002, "quantity": 1},
	}) {
		t.Fatalf("recipe inputs fact was not recorded deterministically: %#v", store.Facts["recipe:stillness:reflex"])
	}
	predicates := make(map[string]int)
	for _, relation := range store.Relations {
		if relation.SubjectEntityID == "recipe:stillness:reflex" {
			predicates[relation.Predicate]++
		}
	}
	for _, predicate := range []string{"produces", "requires_input", "uses_facility", "uses_blueprint"} {
		if predicates[predicate] == 0 {
			t.Fatalf("missing %s relation for recipe: %#v", predicate, store.Relations)
		}
	}
	if _, ok := store.Artefacts[result.Artefact.ID]; !ok {
		t.Fatalf("source artefact was not recorded: %#v", store.Artefacts)
	}
}

func TestImportRecipesRejectsMalformedReviewedRows(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "static-client-recipes.json")
	writeFixture(t, path, `{
		"schemaVersion": "registry.static-client-recipes.v1",
		"environment": "stillness",
		"recipes": [
			{
				"recipeId": "broken",
				"name": "Broken",
				"outputTypeId": 0,
				"outputQuantity": 1,
				"inputs": [{"typeId": 3001, "quantity": 1}]
			}
		]
	}`)

	store := db.NewMemoryStore()
	_, err := ImportRecipes(context.Background(), store, artefacts.LocalStore{Root: filepath.Join(dir, "artefacts")}, path, RecipeImportOptions{
		Environment:     model.EnvironmentStillness,
		AllowedRootDirs: []string{dir},
	})
	if err == nil {
		t.Fatal("expected malformed recipe row to be rejected")
	}
	if len(store.Artefacts) != 0 || len(store.Entities) != 0 {
		t.Fatalf("malformed recipe import should not register evidence or entities: artefacts=%#v entities=%#v", store.Artefacts, store.Entities)
	}
}

func hasFactDeep(facts []model.Fact, key string, value any) bool {
	for _, fact := range facts {
		if fact.Key == key && reflect.DeepEqual(fact.Value, value) {
			return true
		}
	}
	return false
}
