package db

import (
	"testing"

	"github.com/blackrelay/registry/internal/model"
)

func TestDeriveCurrentEntityExposesEvidenceOnlyOwnerCapAndLocationHash(t *testing.T) {
	item := model.CurrentEntity{
		Entity: model.Entity{
			ID:          "assembly:stillness:100",
			Type:        model.EntityTypeAssembly,
			Environment: model.EnvironmentStillness,
		},
		OutgoingRelations: []model.CurrentRelation{
			{
				SubjectEntityID:   "assembly:stillness:100",
				Predicate:         "has_owner_cap",
				ObjectEntityID:    "resource_object:stillness:owner-cap:0xowner",
				ObjectEntityType:  model.EntityTypeResourceObject,
				ObjectDisplayName: "Owner capability 0xowner",
			},
			{
				SubjectEntityID:   "assembly:stillness:100",
				Predicate:         "has_location_hash",
				ObjectEntityID:    "resource_object:stillness:location-hash:loc-abc",
				ObjectEntityType:  model.EntityTypeResourceObject,
				ObjectDisplayName: "Location hash loc-abc",
			},
		},
	}

	deriveCurrentEntity(&item)

	if item.Derived == nil {
		t.Fatal("expected derived state")
	}
	if item.Derived.OwnerCap == nil || item.Derived.OwnerCap.EntityID != "resource_object:stillness:owner-cap:0xowner" {
		t.Fatalf("owner capability evidence was not exposed: %#v", item.Derived)
	}
	if item.Derived.LocationHash == nil || item.Derived.LocationHash.EntityID != "resource_object:stillness:location-hash:loc-abc" {
		t.Fatalf("location hash evidence was not exposed: %#v", item.Derived)
	}
	if item.Derived.Owner != nil || item.Derived.System != nil {
		t.Fatalf("evidence-only relations should not be promoted to owner/system: %#v", item.Derived)
	}
}

func TestCurrentEntityMatchesProfileAndEvidenceBooleanFilters(t *testing.T) {
	withProfile := model.CurrentEntity{
		Entity: model.Entity{
			ID:          "character:stillness:2112091476",
			Type:        model.EntityTypeCharacter,
			Name:        "FC Jotunn",
			DisplayName: "FC Jotunn",
			Environment: model.EnvironmentStillness,
		},
		Facts: map[string]any{"metadata_name": "FC Jotunn"},
		OutgoingRelations: []model.CurrentRelation{{
			SubjectEntityID:  "character:stillness:2112091476",
			Predicate:        "belongs_to",
			ObjectEntityID:   "tribe:stillness:99",
			ObjectEntityType: model.EntityTypeTribe,
		}},
		IncomingRelations: []model.CurrentRelation{{
			SubjectEntityID:   "killmail:stillness:1",
			SubjectEntityType: model.EntityTypeKillmail,
			Predicate:         "victim",
			ObjectEntityID:    "character:stillness:2112091476",
		}},
	}
	deriveCurrentEntity(&withProfile)
	if !currentEntityMatchesQuery(withProfile, CurrentEntityQuery{ProfileState: "known"}) {
		t.Fatalf("profile=known did not match current profile: %#v", withProfile.Derived)
	}
	if currentEntityMatchesQuery(withProfile, CurrentEntityQuery{ProfileState: "placeholder"}) {
		t.Fatalf("profile=placeholder matched named character")
	}
	yes := true
	no := false
	if !currentEntityMatchesQuery(withProfile, CurrentEntityQuery{HasTribe: &yes, HasActivity: &yes}) {
		t.Fatalf("has_tribe/has_activity filters did not match derived character state: %#v", withProfile.Derived)
	}
	if currentEntityMatchesQuery(withProfile, CurrentEntityQuery{HasTribe: &no}) {
		t.Fatalf("has_tribe=false matched a character with a tribe")
	}

	placeholder := model.CurrentEntity{
		Entity: model.Entity{
			ID:          "character:stillness:42",
			Type:        model.EntityTypeCharacter,
			Name:        "Character 42",
			DisplayName: "Character 42",
			Environment: model.EnvironmentStillness,
		},
		Facts: map[string]any{"character_id": "42"},
	}
	deriveCurrentEntity(&placeholder)
	if !currentEntityMatchesQuery(placeholder, CurrentEntityQuery{ProfileState: "placeholder"}) {
		t.Fatalf("profile=placeholder did not match placeholder character")
	}
	if currentEntityMatchesQuery(placeholder, CurrentEntityQuery{ProfileState: "known"}) {
		t.Fatalf("profile=known matched placeholder character")
	}

	assembly := model.CurrentEntity{
		Entity: model.Entity{
			ID:          "assembly:stillness:100",
			Type:        model.EntityTypeAssembly,
			Name:        "Assembly 100",
			DisplayName: "Assembly 100",
			Environment: model.EnvironmentStillness,
		},
		OutgoingRelations: []model.CurrentRelation{
			{SubjectEntityID: "assembly:stillness:100", Predicate: "has_owner_cap", ObjectEntityID: "resource_object:stillness:owner-cap:0xcap", ObjectEntityType: model.EntityTypeResourceObject},
			{SubjectEntityID: "assembly:stillness:100", Predicate: "has_location_hash", ObjectEntityID: "resource_object:stillness:location-hash:loc-1", ObjectEntityType: model.EntityTypeResourceObject},
			{SubjectEntityID: "assembly:stillness:100", Predicate: "owned_by", ObjectEntityID: "character:stillness:2112091476", ObjectEntityType: model.EntityTypeCharacter},
			{SubjectEntityID: "assembly:stillness:100", Predicate: "located_in", ObjectEntityID: "system:stillness:30001001", ObjectEntityType: model.EntityTypeSystem},
		},
	}
	deriveCurrentEntity(&assembly)
	if !currentEntityMatchesQuery(assembly, CurrentEntityQuery{
		HasOwnerCap:       &yes,
		HasLocationHash:   &yes,
		HasResolvedOwner:  &yes,
		HasResolvedSystem: &yes,
	}) {
		t.Fatalf("evidence boolean filters did not match derived assembly state: %#v", assembly.Derived)
	}
	if currentEntityMatchesQuery(assembly, CurrentEntityQuery{HasResolvedOwner: &no}) {
		t.Fatalf("has_resolved_owner=false matched assembly with resolved owner")
	}
}
