package sui

import (
	"testing"
	"time"

	"github.com/blackrelay/registry/internal/db"
	"github.com/blackrelay/registry/internal/model"
)

func TestDeriveEntityFromCharacterObjectUsesMetadataAndCanonicalID(t *testing.T) {
	object := db.SuiObjectRecord{
		ID:          "object:0xabc:7",
		ObjectID:    "0xabc",
		Environment: model.EnvironmentStillness,
		TypeRepr:    testPackageID + "::character::Character",
		PackageID:   testPackageID,
		Module:      "character",
		TypeName:    "Character",
		Version:     "7",
		Digest:      "digest",
		SourceID:    "source:sui:sui-testnet:graphql:objects",
		ObservedAt:  time.Now().UTC(),
		Payload: map[string]any{
			"json": map[string]any{
				"key": map[string]any{
					"tenant":  "stillness",
					"item_id": "2112091476",
				},
				"metadata": map[string]any{
					"name":        "Tao",
					"description": "Public pilot profile",
					"url":         "https://example.invalid/characters/tao",
				},
				"tribe_id":          "42",
				"character_address": "0xwallet",
			},
		},
	}
	derived, ok := DeriveEntityFromObject(object)
	if !ok {
		t.Fatal("object was not derived")
	}
	if derived.Entity.ID != "character:stillness:2112091476" {
		t.Fatalf("unexpected entity id %s", derived.Entity.ID)
	}
	if derived.Entity.Name != "Tao" || derived.Entity.Type != model.EntityTypeCharacter {
		t.Fatalf("unexpected entity %#v", derived.Entity)
	}
	if !hasFact(derived.Facts, "metadata_name", "Tao") {
		t.Fatalf("missing metadata_name fact %#v", derived.Facts)
	}
	if !hasFact(derived.Facts, "metadata_description", "Public pilot profile") {
		t.Fatalf("missing metadata_description fact %#v", derived.Facts)
	}
	if !hasFact(derived.Facts, "metadata_url", "https://example.invalid/characters/tao") {
		t.Fatalf("missing metadata_url fact %#v", derived.Facts)
	}
	if !hasFact(derived.Facts, "character_address", "0xwallet") {
		t.Fatalf("missing character_address fact %#v", derived.Facts)
	}
}

func TestDeriveEntityFromCharacterObjectUsesNestedMetadataName(t *testing.T) {
	object := db.SuiObjectRecord{
		ID:          "object:0xprofile:8",
		ObjectID:    "0xprofile",
		Environment: model.EnvironmentStillness,
		TypeRepr:    testPackageID + "::character::PlayerProfile",
		PackageID:   testPackageID,
		Module:      "character",
		TypeName:    "PlayerProfile",
		SourceID:    "source:sui:sui-testnet:graphql:objects",
		ObservedAt:  time.Now().UTC(),
		Payload: map[string]any{
			"json": map[string]any{
				"character_id": "2112091476",
				"metadata": map[string]any{
					"fields": map[string]any{
						"name": "Nested Pilot",
					},
				},
			},
		},
	}

	derived, ok := DeriveEntityFromObject(object)
	if !ok {
		t.Fatal("object was not derived")
	}
	if derived.Entity.Name != "Nested Pilot" {
		t.Fatalf("nested metadata name was not used: %#v", derived.Entity)
	}
	if !hasFact(derived.Facts, "metadata_name", "Nested Pilot") {
		t.Fatalf("missing nested metadata_name fact %#v", derived.Facts)
	}
}

func TestDeriveEntityFromObjectLeavesPreCurrentCycleUnlabelled(t *testing.T) {
	object := db.SuiObjectRecord{
		ID:          "object:0xcycle5:7",
		ObjectID:    "0xcycle5",
		Environment: model.EnvironmentStillness,
		TypeRepr:    testPackageID + "::character::Character",
		PackageID:   testPackageID,
		Module:      "character",
		TypeName:    "Character",
		Version:     "7",
		Digest:      "digest",
		SourceID:    "source:sui:sui-testnet:graphql:objects",
		ObservedAt:  time.Date(2026, 3, 11, 9, 0, 0, 0, time.UTC),
		Payload: map[string]any{
			"json": map[string]any{
				"key": map[string]any{
					"tenant":  "stillness",
					"item_id": "2112091476",
				},
				"character_id": "2112091476",
			},
		},
	}

	derived, ok := DeriveEntityFromObject(object)
	if !ok {
		t.Fatal("object was not derived")
	}
	if derived.Entity.Cycle != nil {
		t.Fatalf("pre-Cycle-6 object should not be cycle-labelled, got %#v", derived.Entity.Cycle)
	}
	if hasFactCycle(derived.Facts, "character_id", 5) {
		t.Fatalf("character_id fact should not carry Cycle 5, got %#v", derived.Facts)
	}
}

func TestDeriveGraphFromCharacterObjectCreatesTribeRelationAndEntity(t *testing.T) {
	object := db.SuiObjectRecord{
		ID:          "object:0xabc:7",
		ObjectID:    "0xabc",
		Environment: model.EnvironmentStillness,
		TypeRepr:    testPackageID + "::character::Character",
		PackageID:   testPackageID,
		Module:      "character",
		TypeName:    "Character",
		SourceID:    "source:sui:sui-testnet:graphql:objects",
		ObservedAt:  time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC),
		Payload: map[string]any{
			"json": map[string]any{
				"key": map[string]any{
					"tenant":  "stillness",
					"item_id": "2112091476",
				},
				"tribe_id": "42",
			},
		},
	}

	graph := DeriveGraphFromObject(object)
	if !hasObjectEntity(graph.Entities, "character:stillness:2112091476", model.EntityTypeCharacter) {
		t.Fatalf("missing character entity %#v", graph.Entities)
	}
	if !hasObjectEntity(graph.Entities, "tribe:stillness:42", model.EntityTypeTribe) {
		t.Fatalf("missing tribe entity %#v", graph.Entities)
	}
	if !hasObjectRelation(graph.Relations, "character:stillness:2112091476", "belongs_to", "tribe:stillness:42") {
		t.Fatalf("missing character tribe relation %#v", graph.Relations)
	}
}

func TestDeriveGraphFromGateObjectCreatesLinkedGateRelation(t *testing.T) {
	object := db.SuiObjectRecord{
		ID:          "object:0xgate:8",
		ObjectID:    "0xgate",
		Environment: model.EnvironmentStillness,
		TypeRepr:    testPackageID + "::gate::Gate",
		PackageID:   testPackageID,
		Module:      "gate",
		TypeName:    "Gate",
		SourceID:    "source:sui:sui-testnet:graphql:objects",
		ObservedAt:  time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC),
		Payload: map[string]any{
			"json": map[string]any{
				"key": map[string]any{
					"tenant":  "stillness",
					"item_id": "100",
				},
				"linked_gate_id": "200",
			},
		},
	}

	graph := DeriveGraphFromObject(object)
	if !hasObjectEntity(graph.Entities, "gate:stillness:100", model.EntityTypeGate) {
		t.Fatalf("missing source gate entity %#v", graph.Entities)
	}
	if !hasObjectEntity(graph.Entities, "gate:stillness:200", model.EntityTypeGate) {
		t.Fatalf("missing linked gate entity %#v", graph.Entities)
	}
	if !hasObjectRelation(graph.Relations, "gate:stillness:100", "links_to", "gate:stillness:200") {
		t.Fatalf("missing linked gate relation %#v", graph.Relations)
	}
}

func TestDeriveGraphFromGateObjectCompactsLongHexGateNames(t *testing.T) {
	object := db.SuiObjectRecord{
		ID:          "object:0xgate:9",
		ObjectID:    "0xgate",
		Environment: model.EnvironmentStillness,
		TypeRepr:    testPackageID + "::gate::Gate",
		PackageID:   testPackageID,
		Module:      "gate",
		TypeName:    "Gate",
		SourceID:    "source:sui:sui-testnet:graphql:objects",
		ObservedAt:  time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC),
		Payload: map[string]any{
			"json": map[string]any{
				"key": map[string]any{
					"tenant":  "stillness",
					"item_id": "0x00132c79dc203b2dc04bd84a31c04130e6af21990c7d4ed7f16247ea255db190",
				},
			},
		},
	}

	graph := DeriveGraphFromObject(object)
	for _, entity := range graph.Entities {
		if entity.Entity.Type == model.EntityTypeGate {
			if entity.Entity.DisplayName != "Gate 00132c79dc20" {
				t.Fatalf("long gate identity was not compacted: %#v", entity.Entity)
			}
			return
		}
	}
	t.Fatalf("missing gate entity %#v", graph.Entities)
}

func TestDeriveGraphFromGateObjectPromotesExplicitSystemAndCoordinates(t *testing.T) {
	object := db.SuiObjectRecord{
		ID:          "object:0xgate:10",
		ObjectID:    "0xgate",
		Environment: model.EnvironmentStillness,
		TypeRepr:    testPackageID + "::gate::Gate",
		PackageID:   testPackageID,
		Module:      "gate",
		TypeName:    "Gate",
		SourceID:    "source:sui:sui-testnet:graphql:objects",
		ObservedAt:  time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC),
		Payload: map[string]any{
			"json": map[string]any{
				"key": map[string]any{
					"tenant":  "stillness",
					"item_id": "100",
				},
				"location": map[string]any{
					"solar_system_id": map[string]any{"tenant": "stillness", "item_id": "30001001"},
					"x":               "12.5",
					"y":               "-3.25",
					"z":               "8",
				},
			},
		},
	}

	graph := DeriveGraphFromObject(object)
	var gate *DerivedObjectEntity
	for i := range graph.Entities {
		if graph.Entities[i].Entity.ID == "gate:stillness:100" {
			gate = &graph.Entities[i]
			break
		}
	}
	if gate == nil {
		t.Fatalf("missing gate entity %#v", graph.Entities)
	}
	if !hasFact(gate.Facts, "solar_system_id", "30001001") || !hasFact(gate.Facts, "x", "12.5") || !hasFact(gate.Facts, "y", "-3.25") || !hasFact(gate.Facts, "z", "8") {
		t.Fatalf("gate location facts were not preserved: %#v", gate.Facts)
	}
	if !hasObjectRelation(graph.Relations, "gate:stillness:100", "located_in", "system:stillness:30001001") {
		t.Fatalf("missing gate system relation %#v", graph.Relations)
	}
}

func TestDeriveGraphFromInfrastructureObjectCreatesCapAndLocationEvidenceOnly(t *testing.T) {
	object := db.SuiObjectRecord{
		ID:          "object:0xassembly:8",
		ObjectID:    "0xassembly",
		Environment: model.EnvironmentStillness,
		TypeRepr:    testPackageID + "::assembly::Assembly",
		PackageID:   testPackageID,
		Module:      "assembly",
		TypeName:    "Assembly",
		SourceID:    "source:sui:sui-testnet:graphql:objects",
		ObservedAt:  time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC),
		Payload: map[string]any{
			"json": map[string]any{
				"key": map[string]any{
					"tenant":  "stillness",
					"item_id": "100",
				},
				"owner_cap_id": "0xownercap",
				"location": map[string]any{
					"location_hash": "loc-abc",
				},
			},
		},
	}

	graph := DeriveGraphFromObject(object)
	if !hasObjectEntity(graph.Entities, "assembly:stillness:100", model.EntityTypeAssembly) {
		t.Fatalf("missing assembly entity %#v", graph.Entities)
	}
	if !hasObjectEntity(graph.Entities, "resource_object:stillness:owner-cap:0xownercap", model.EntityTypeResourceObject) {
		t.Fatalf("missing owner-cap evidence entity %#v", graph.Entities)
	}
	if !hasObjectEntity(graph.Entities, "resource_object:stillness:location-hash:loc-abc", model.EntityTypeResourceObject) {
		t.Fatalf("missing location-hash evidence entity %#v", graph.Entities)
	}
	if !hasObjectRelation(graph.Relations, "assembly:stillness:100", "has_owner_cap", "resource_object:stillness:owner-cap:0xownercap") {
		t.Fatalf("missing owner-cap evidence relation %#v", graph.Relations)
	}
	if !hasObjectRelation(graph.Relations, "assembly:stillness:100", "has_location_hash", "resource_object:stillness:location-hash:loc-abc") {
		t.Fatalf("missing location-hash evidence relation %#v", graph.Relations)
	}
	if hasObjectRelation(graph.Relations, "assembly:stillness:100", "owned_by", "resource_object:stillness:owner-cap:0xownercap") {
		t.Fatalf("owner cap should not be promoted to owned_by relation %#v", graph.Relations)
	}
	if hasObjectRelation(graph.Relations, "assembly:stillness:100", "located_in", "resource_object:stillness:location-hash:loc-abc") {
		t.Fatalf("location hash should not be promoted to located_in relation %#v", graph.Relations)
	}
}

func TestDeriveGraphFromRiftObjectCreatesSiteAndLocationEvidence(t *testing.T) {
	object := db.SuiObjectRecord{
		ID:          "object:0xrift:8",
		ObjectID:    "0xrift",
		Environment: model.EnvironmentStillness,
		TypeRepr:    testPackageID + "::rift::Rift",
		PackageID:   testPackageID,
		Module:      "rift",
		TypeName:    "Rift",
		SourceID:    "source:sui:sui-testnet:graphql:objects",
		ObservedAt:  time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC),
		Payload: map[string]any{
			"json": map[string]any{
				"key": map[string]any{
					"tenant":  "stillness",
					"item_id": "9001",
				},
				"location": map[string]any{
					"location_hash": "0xabc123",
				},
			},
		},
	}

	graph := DeriveGraphFromObject(object)
	if !hasObjectEntity(graph.Entities, "site:stillness:9001", model.EntityTypeSite) {
		t.Fatalf("missing rift site entity %#v", graph.Entities)
	}
	if !hasObjectEntity(graph.Entities, "resource_object:stillness:location-hash:0xabc123", model.EntityTypeResourceObject) {
		t.Fatalf("missing location-hash evidence entity %#v", graph.Entities)
	}
	if !hasObjectRelation(graph.Relations, "site:stillness:9001", "has_location_hash", "resource_object:stillness:location-hash:0xabc123") {
		t.Fatalf("missing rift location-hash evidence relation %#v", graph.Relations)
	}
	if hasObjectRelation(graph.Relations, "site:stillness:9001", "located_in", "resource_object:stillness:location-hash:0xabc123") {
		t.Fatalf("rift location hash should not be promoted to located_in relation %#v", graph.Relations)
	}
}

func TestDeriveGraphFromKillmailObjectCreatesRelationsAndRawKillmail(t *testing.T) {
	object := db.SuiObjectRecord{
		ID:          "object:0xkill:8",
		ObjectID:    "0xkill",
		Environment: model.EnvironmentStillness,
		TypeRepr:    testPackageID + "::killmail::Killmail",
		PackageID:   testPackageID,
		Module:      "killmail",
		TypeName:    "Killmail",
		SourceID:    "source:sui:sui-testnet:graphql:objects",
		ObservedAt:  time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC),
		Payload: map[string]any{
			"json": map[string]any{
				"key":                      map[string]any{"tenant": "stillness", "item_id": "310"},
				"killer_id":                map[string]any{"tenant": "stillness", "item_id": "2112000355"},
				"victim_id":                map[string]any{"tenant": "stillness", "item_id": "2112000304"},
				"reported_by_character_id": map[string]any{"tenant": "stillness", "item_id": "2112000999"},
				"solar_system_id":          map[string]any{"tenant": "stillness", "item_id": "30001001"},
				"killer_type_id":           "92096",
				"loss_type":                map[string]any{"@variant": "Ship"},
				"kill_timestamp":           float64(1_803_283_200_000),
			},
		},
	}

	graph := DeriveGraphFromObject(object)
	if !hasObjectEntity(graph.Entities, "killmail:stillness:310", model.EntityTypeKillmail) {
		t.Fatalf("missing killmail entity %#v", graph.Entities)
	}
	if !hasObjectRelation(graph.Relations, "killmail:stillness:310", "victim", "character:stillness:2112000304") {
		t.Fatalf("missing victim relation %#v", graph.Relations)
	}
	if !hasObjectRelation(graph.Relations, "killmail:stillness:310", "occurred_in", "system:stillness:30001001") {
		t.Fatalf("missing system relation %#v", graph.Relations)
	}
	if len(graph.Killmails) != 1 {
		t.Fatalf("expected one raw killmail, got %#v", graph.Killmails)
	}
	if graph.Killmails[0].VictimCharacterID != "character:stillness:2112000304" {
		t.Fatalf("victim was not normalised in raw killmail %#v", graph.Killmails[0])
	}
	if graph.Killmails[0].KillerTypeID != "92096" {
		t.Fatalf("killer type was not preserved in raw killmail %#v", graph.Killmails[0])
	}
}

func TestDeriveEntityFromNetworkNodeMapsToAssemblyWithExactMoveTypeFact(t *testing.T) {
	object := db.SuiObjectRecord{
		ID:          "object:0xnode:9",
		ObjectID:    "0xnode",
		Environment: model.EnvironmentStillness,
		TypeRepr:    testPackageID + "::network_node::NetworkNode",
		PackageID:   testPackageID,
		Module:      "network_node",
		TypeName:    "NetworkNode",
		Version:     "9",
		SourceID:    "source:sui:sui-testnet:graphql:objects",
		Payload: map[string]any{
			"json": map[string]any{
				"key": map[string]any{
					"tenant":  "stillness",
					"item_id": "999",
				},
			},
		},
	}
	derived, ok := DeriveEntityFromObject(object)
	if !ok {
		t.Fatal("object was not derived")
	}
	if derived.Entity.Type != model.EntityTypeAssembly {
		t.Fatalf("unexpected entity type %s", derived.Entity.Type)
	}
	if !hasFact(derived.Facts, "object_type", testPackageID+"::network_node::NetworkNode") {
		t.Fatalf("missing exact object_type fact %#v", derived.Facts)
	}
}

func TestDeriveEntityFromObjectRejectsUnknownType(t *testing.T) {
	object := db.SuiObjectRecord{
		ObjectID:    "0xabc",
		Environment: model.EnvironmentStillness,
		TypeRepr:    testPackageID + "::inventory::Inventory",
		PackageID:   testPackageID,
		Module:      "inventory",
		TypeName:    "Inventory",
		SourceID:    "source:sui:sui-testnet:graphql:objects",
		Payload:     map[string]any{"json": map[string]any{}},
	}
	if _, ok := DeriveEntityFromObject(object); ok {
		t.Fatal("unknown object type should not derive a domain entity")
	}
}

func hasObjectEntity(entities []DerivedObjectEntity, id string, entityType model.EntityType) bool {
	for _, entity := range entities {
		if entity.Entity.ID == id && entity.Entity.Type == entityType {
			return true
		}
	}
	return false
}

func hasObjectRelation(relations []db.RelationDraft, subject, predicate, object string) bool {
	for _, relation := range relations {
		if relation.SubjectEntityID == subject && relation.Predicate == predicate && relation.ObjectEntityID == object {
			return true
		}
	}
	return false
}

func hasFact(facts []db.EntityFactDraft, key string, value any) bool {
	for _, fact := range facts {
		if fact.Key == key && fact.Value == value {
			return true
		}
	}
	return false
}
