package sui

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/blackrelay/registry/internal/cursor"
	"github.com/blackrelay/registry/internal/db"
	"github.com/blackrelay/registry/internal/model"
)

func TestDeriveEntitiesFromKillmailEventCreatesParticipantsSystemAndRelations(t *testing.T) {
	event := db.EventRecord{
		ID:                "event:killtx:0",
		Kind:              "killmail.created",
		Environment:       model.EnvironmentStillness,
		OccurredAt:        time.Date(2026, 6, 24, 10, 0, 0, 0, time.UTC),
		PackageID:         testPackageID,
		Module:            "killmail",
		TransactionDigest: "killtx",
		SourceID:          "source:sui:sui-testnet:graphql",
		Payload: map[string]any{
			"json": map[string]any{
				"key":                      map[string]any{"tenant": "stillness", "item_id": "310"},
				"killer_id":                map[string]any{"tenant": "stillness", "item_id": "2112000355"},
				"victim_id":                map[string]any{"tenant": "stillness", "item_id": "2112000304"},
				"reported_by_character_id": map[string]any{"tenant": "stillness", "item_id": "2112000999"},
				"solar_system_id":          map[string]any{"tenant": "stillness", "item_id": "30001001"},
				"loss_type":                map[string]any{"@variant": "Ship"},
				"kill_timestamp":           float64(1_803_283_200_000),
			},
		},
	}

	derived := DeriveEntitiesFromEvent(event)
	if len(derived.Entities) != 5 {
		t.Fatalf("expected killmail, three character identities and a system, got %#v", derived.Entities)
	}
	if !hasDerivedEntity(derived.Entities, "killmail:stillness:310", model.EntityTypeKillmail) {
		t.Fatalf("missing killmail entity %#v", derived.Entities)
	}
	if !hasDerivedEntity(derived.Entities, "character:stillness:2112000304", model.EntityTypeCharacter) {
		t.Fatalf("missing victim character entity %#v", derived.Entities)
	}
	if !hasDerivedEntity(derived.Entities, "system:stillness:30001001", model.EntityTypeSystem) {
		t.Fatalf("missing system entity %#v", derived.Entities)
	}
	if len(derived.Killmails) != 1 {
		t.Fatalf("expected one killmail raw record, got %#v", derived.Killmails)
	}
	if derived.Killmails[0].VictimCharacterID != "character:stillness:2112000304" || derived.Killmails[0].KillerCharacterID != "character:stillness:2112000355" {
		t.Fatalf("killmail participants were not normalised: %#v", derived.Killmails[0])
	}
	if !hasRelation(derived.Relations, "killmail:stillness:310", "victim", "character:stillness:2112000304") {
		t.Fatalf("missing victim relation %#v", derived.Relations)
	}
	if !hasRelation(derived.Relations, "killmail:stillness:310", "killer", "character:stillness:2112000355") {
		t.Fatalf("missing killer relation %#v", derived.Relations)
	}
	if !hasRelation(derived.Relations, "killmail:stillness:310", "occurred_in", "system:stillness:30001001") {
		t.Fatalf("missing system relation %#v", derived.Relations)
	}
}

func TestDeriveEntitiesFromGateJumpEventCreatesRouteAndGateRelations(t *testing.T) {
	event := db.EventRecord{
		ID:          "event:gatejump:0",
		Kind:        "gate.jump",
		Environment: model.EnvironmentStillness,
		OccurredAt:  time.Date(2026, 6, 24, 11, 0, 0, 0, time.UTC),
		Module:      "gate",
		SourceID:    "source:sui:sui-testnet:graphql",
		Payload: map[string]any{
			"json": map[string]any{
				"source_gate_key":      map[string]any{"tenant": "stillness", "item_id": "100"},
				"destination_gate_key": map[string]any{"tenant": "stillness", "item_id": "200"},
				"character_id":         map[string]any{"tenant": "stillness", "item_id": "2112000304"},
			},
		},
	}

	derived := DeriveEntitiesFromEvent(event)
	if !hasDerivedEntity(derived.Entities, "route:stillness:100:200", model.EntityTypeRoute) {
		t.Fatalf("missing route entity %#v", derived.Entities)
	}
	if !hasRelation(derived.Relations, "gate:stillness:100", "links_to", "gate:stillness:200") {
		t.Fatalf("missing gate link relation %#v", derived.Relations)
	}
	if !hasRelation(derived.Relations, "route:stillness:100:200", "observed_between", "gate:stillness:100") {
		t.Fatalf("missing route source gate relation %#v", derived.Relations)
	}
}

func TestDeriveEntitiesFromAssemblyEventPreservesMetadataDescriptionAndURL(t *testing.T) {
	event := db.EventRecord{
		ID:          "event:assembly:0",
		Kind:        "assembly.created",
		Environment: model.EnvironmentStillness,
		OccurredAt:  time.Date(2026, 6, 24, 11, 0, 0, 0, time.UTC),
		Module:      "assembly",
		SourceID:    "source:sui:sui-testnet:graphql",
		Payload: map[string]any{
			"json": map[string]any{
				"assembly_key": map[string]any{"tenant": "stillness", "item_id": "100"},
				"metadata": map[string]any{
					"name":        "Assembly One",
					"description": "Public assembly profile",
					"url":         "https://example.invalid/assemblies/100",
				},
			},
		},
	}

	derived := DeriveEntitiesFromEvent(event)
	var assembly *DerivedEventEntity
	for i := range derived.Entities {
		if derived.Entities[i].Entity.ID == "assembly:stillness:100" {
			assembly = &derived.Entities[i]
			break
		}
	}
	if assembly == nil {
		t.Fatalf("missing assembly entity %#v", derived.Entities)
	}
	if !hasEventFact(assembly.Facts, "metadata_name", "Assembly One") {
		t.Fatalf("missing metadata_name fact %#v", assembly.Facts)
	}
	if !hasEventFact(assembly.Facts, "metadata_description", "Public assembly profile") {
		t.Fatalf("missing metadata_description fact %#v", assembly.Facts)
	}
	if !hasEventFact(assembly.Facts, "metadata_url", "https://example.invalid/assemblies/100") {
		t.Fatalf("missing metadata_url fact %#v", assembly.Facts)
	}
}

func TestDeriveEntitiesFromInventoryV2EventCreatesItemStorageAndCharacterRelations(t *testing.T) {
	event := db.EventRecord{
		ID:          "event:inventory-v2:0",
		Kind:        "inventory.item.deposited.v2",
		Environment: model.EnvironmentStillness,
		OccurredAt:  time.Date(2026, 6, 25, 10, 0, 0, 0, time.UTC),
		Module:      "storage_unit",
		SourceID:    "source:sui:sui-testnet:graphql",
		Payload: map[string]any{
			"json": map[string]any{
				"item_id":       "0",
				"type_id":       "81972",
				"quantity":      float64(1),
				"assembly_id":   "0xstorage",
				"inventory_key": "0xinventory",
				"assembly_key": map[string]any{
					"tenant":  "stillness",
					"item_id": "1000000259415",
				},
				"character_id": "0xcharacter",
				"character_key": map[string]any{
					"tenant":  "stillness",
					"item_id": "2112092707",
				},
			},
		},
	}

	derived := DeriveEntitiesFromEvent(event)
	if !hasDerivedEntity(derived.Entities, "item:stillness:type:81972", model.EntityTypeItem) {
		t.Fatalf("missing item type entity %#v", derived.Entities)
	}
	if !hasDerivedEntity(derived.Entities, "storage:stillness:1000000259415", model.EntityTypeStorage) {
		t.Fatalf("missing storage entity %#v", derived.Entities)
	}
	if !hasDerivedEntity(derived.Entities, "character:stillness:2112092707", model.EntityTypeCharacter) {
		t.Fatalf("missing character entity %#v", derived.Entities)
	}
	if !hasRelation(derived.Relations, "item:stillness:type:81972", "deposited_into", "storage:stillness:1000000259415") {
		t.Fatalf("missing item deposit relation %#v", derived.Relations)
	}
	if !hasRelation(derived.Relations, "character:stillness:2112092707", "deposited", "item:stillness:type:81972") {
		t.Fatalf("missing character deposit relation %#v", derived.Relations)
	}
	var item *DerivedEventEntity
	for i := range derived.Entities {
		if derived.Entities[i].Entity.ID == "item:stillness:type:81972" {
			item = &derived.Entities[i]
			break
		}
	}
	if item == nil || !hasEventFact(item.Facts, "inventory_action", "deposited") || !hasEventFact(item.Facts, "type_id", "81972") {
		t.Fatalf("missing item inventory facts %#v", item)
	}
}

func TestDeriveEntitiesFromRiftBroadcastCreatesSiteSystemAndLocationEvidence(t *testing.T) {
	event := db.EventRecord{
		ID:          "event:rift:0",
		Kind:        "rift.location.broadcast",
		Environment: model.EnvironmentStillness,
		OccurredAt:  time.Date(2026, 6, 25, 10, 0, 0, 0, time.UTC),
		Module:      "rift",
		SourceID:    "source:sui:sui-testnet:graphql",
		Payload: map[string]any{
			"json": map[string]any{
				"rift_id":       "0xrift",
				"location_hash": "0xabc123",
				"solarsystem":   float64(30001001),
				"x":             "12.5",
				"y":             "-3.25",
				"z":             "8",
				"rift_key": map[string]any{
					"tenant":  "stillness",
					"item_id": "9001",
				},
			},
		},
	}

	derived := DeriveEntitiesFromEvent(event)
	if !hasDerivedEntity(derived.Entities, "site:stillness:9001", model.EntityTypeSite) {
		t.Fatalf("missing rift site entity %#v", derived.Entities)
	}
	if !hasDerivedEntity(derived.Entities, "system:stillness:30001001", model.EntityTypeSystem) {
		t.Fatalf("missing broadcast system entity %#v", derived.Entities)
	}
	if !hasDerivedEntity(derived.Entities, "resource_object:stillness:location-hash:0xabc123", model.EntityTypeResourceObject) {
		t.Fatalf("missing rift location hash evidence entity %#v", derived.Entities)
	}
	if !hasRelation(derived.Relations, "site:stillness:9001", "located_in", "system:stillness:30001001") {
		t.Fatalf("missing rift system relation %#v", derived.Relations)
	}
	if !hasRelation(derived.Relations, "site:stillness:9001", "has_location_hash", "resource_object:stillness:location-hash:0xabc123") {
		t.Fatalf("missing rift location hash relation %#v", derived.Relations)
	}
}

func TestDeriveEntitiesFromEventAssignsTimestampCycle(t *testing.T) {
	event := db.EventRecord{
		ID:          "event:cycle6:0",
		Kind:        "character.created",
		Environment: model.EnvironmentStillness,
		OccurredAt:  time.Date(2026, 6, 25, 9, 0, 0, 0, time.UTC),
		Module:      "character",
		SourceID:    "source:sui:sui-testnet:graphql",
		Payload: map[string]any{
			"json": map[string]any{
				"key":               map[string]any{"tenant": "stillness", "item_id": "2112091476"},
				"character_id":      "2112091476",
				"tribe_id":          "42",
				"character_address": "0xwallet",
			},
		},
	}

	derived := DeriveEntitiesFromEvent(event)
	var character *DerivedEventEntity
	for i := range derived.Entities {
		if derived.Entities[i].Entity.ID == "character:stillness:2112091476" {
			character = &derived.Entities[i]
			break
		}
	}
	if character == nil {
		t.Fatalf("missing character entity %#v", derived.Entities)
	}
	if character.Entity.Cycle == nil || *character.Entity.Cycle != 6 {
		t.Fatalf("expected character cycle 6, got %#v", character.Entity.Cycle)
	}
	if !hasFactCycle(character.Facts, "character_id", 6) {
		t.Fatalf("expected character_id fact to carry cycle 6, got %#v", character.Facts)
	}
}

func TestDeriveEntitiesFromEventRejectsMismatchedTenant(t *testing.T) {
	event := db.EventRecord{
		ID:          "event:mismatched-tenant:0",
		Kind:        "character.created",
		Environment: model.EnvironmentStillness,
		OccurredAt:  time.Date(2026, 6, 25, 9, 0, 0, 0, time.UTC),
		Module:      "character",
		SourceID:    "source:sui:sui-testnet:graphql",
		Payload: map[string]any{
			"json": map[string]any{
				"assembly_key":      map[string]any{"tenant": "liminality", "item_id": "2112000001"},
				"character_id":      "0xcharacter",
				"tribe_id":          "1000167",
				"character_address": "0xwallet",
			},
		},
	}

	derived := DeriveEntitiesFromEvent(event)
	if len(derived.Entities) != 0 || len(derived.Relations) != 0 || len(derived.Killmails) != 0 {
		t.Fatalf("mismatched tenant event derived data: %#v", derived)
	}
}

func TestDeriveEntitiesFromCharacterCreatedEventUsesMetadataProfile(t *testing.T) {
	event := db.EventRecord{
		ID:          "event:character-profile:0",
		Kind:        "character.created",
		Environment: model.EnvironmentStillness,
		OccurredAt:  time.Date(2026, 6, 25, 9, 0, 0, 0, time.UTC),
		Module:      "character",
		SourceID:    "source:sui:sui-testnet:graphql",
		Payload: map[string]any{
			"json": map[string]any{
				"key":               map[string]any{"tenant": "stillness", "item_id": "2112091476"},
				"character_id":      "2112091476",
				"tribe_id":          "42",
				"character_address": "0xwallet",
				"metadata": map[string]any{
					"name":        "FC Jotunn",
					"description": "Public character profile",
					"url":         "https://example.invalid/characters/fc-jotunn",
				},
			},
		},
	}

	derived := DeriveEntitiesFromEvent(event)
	var character *DerivedEventEntity
	for i := range derived.Entities {
		if derived.Entities[i].Entity.ID == "character:stillness:2112091476" {
			character = &derived.Entities[i]
			break
		}
	}
	if character == nil {
		t.Fatalf("missing character entity %#v", derived.Entities)
	}
	if character.Entity.Name != "FC Jotunn" || character.Entity.DisplayName != "FC Jotunn" {
		t.Fatalf("character metadata name was not used: %#v", character.Entity)
	}
	if !hasEventFact(character.Facts, "metadata_name", "FC Jotunn") {
		t.Fatalf("missing metadata_name fact %#v", character.Facts)
	}
	if !hasEventFact(character.Facts, "metadata_description", "Public character profile") {
		t.Fatalf("missing metadata_description fact %#v", character.Facts)
	}
	if !hasEventFact(character.Facts, "metadata_url", "https://example.invalid/characters/fc-jotunn") {
		t.Fatalf("missing metadata_url fact %#v", character.Facts)
	}
}

func TestRunEventDerivationStoresGraphAndCursor(t *testing.T) {
	store := db.NewMemoryStore()
	event := db.EventRecord{
		ID:          "event:killtx:0",
		Kind:        "killmail.created",
		Environment: model.EnvironmentStillness,
		OccurredAt:  time.Date(2026, 6, 24, 10, 0, 0, 0, time.UTC),
		Module:      "killmail",
		SourceID:    "source:sui:sui-testnet:graphql",
		Payload: map[string]any{
			"json": map[string]any{
				"key":             map[string]any{"tenant": "stillness", "item_id": "310"},
				"killer_id":       map[string]any{"tenant": "stillness", "item_id": "2112000355"},
				"victim_id":       map[string]any{"tenant": "stillness", "item_id": "2112000304"},
				"solar_system_id": map[string]any{"tenant": "stillness", "item_id": "30001001"},
			},
		},
	}
	if err := store.UpsertSuiEvent(context.Background(), event); err != nil {
		t.Fatal(err)
	}

	summary, err := RunEventDerivation(context.Background(), store, EventDerivationOptions{
		Environment: model.EnvironmentStillness,
		Network:     "sui-testnet",
		BatchSize:   1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if summary.EventsScanned != 1 || summary.EntitiesDerived != 4 || summary.RelationsDerived != 3 || summary.KillmailsDerived != 1 {
		t.Fatalf("unexpected summary %#v", summary)
	}
	if _, ok := store.Entities["killmail:stillness:310"]; !ok {
		t.Fatalf("killmail entity was not stored: %#v", store.Entities)
	}
	if _, ok := store.Killmails["killmail:stillness:310"]; !ok {
		t.Fatalf("killmail raw record was not stored: %#v", store.Killmails)
	}
	if len(store.Relations) != 3 {
		t.Fatalf("relations were not stored: %#v", store.Relations)
	}
	if _, ok, err := store.GetSyncCursor(context.Background(), "cursor:registry:derive:sui-events:sui-testnet:stillness"); err != nil || !ok {
		t.Fatalf("derive cursor was not saved ok=%v err=%v", ok, err)
	}
}

func TestRunEventDerivationResumesForwardToNewerEvents(t *testing.T) {
	store := db.NewMemoryStore()
	older := db.EventRecord{
		ID:          "event:old:0",
		Kind:        "killmail.created",
		Environment: model.EnvironmentStillness,
		OccurredAt:  time.Date(2026, 6, 24, 10, 0, 0, 0, time.UTC),
		Module:      "killmail",
		SourceID:    "source:sui:sui-testnet:graphql",
		Payload: map[string]any{
			"json": map[string]any{
				"key":             map[string]any{"tenant": "stillness", "item_id": "310"},
				"victim_id":       map[string]any{"tenant": "stillness", "item_id": "2112000304"},
				"solar_system_id": map[string]any{"tenant": "stillness", "item_id": "30001001"},
			},
		},
	}
	newer := older
	newer.ID = "event:new:0"
	newer.OccurredAt = older.OccurredAt.Add(time.Minute)
	newer.Payload = map[string]any{
		"json": map[string]any{
			"key":             map[string]any{"tenant": "stillness", "item_id": "311"},
			"victim_id":       map[string]any{"tenant": "stillness", "item_id": "2112000305"},
			"solar_system_id": map[string]any{"tenant": "stillness", "item_id": "30001001"},
		},
	}
	if err := store.UpsertSuiEvent(context.Background(), older); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertSuiEvent(context.Background(), newer); err != nil {
		t.Fatal(err)
	}
	cursorValue, err := cursorForEvent(older)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveSyncCursor(context.Background(), db.CursorStatus{
		ID:              DeriveEventsCursorID("sui-testnet", model.EnvironmentStillness),
		Source:          DeriveEventsCursorSource("sui-testnet"),
		Environment:     model.EnvironmentStillness,
		CursorKind:      "sui_event_derivation",
		CursorValue:     cursorValue,
		EventsProcessed: 1,
	}); err != nil {
		t.Fatal(err)
	}

	summary, err := RunEventDerivation(context.Background(), store, EventDerivationOptions{
		Environment: model.EnvironmentStillness,
		Network:     "sui-testnet",
		BatchSize:   10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if summary.EventsScanned != 1 {
		t.Fatalf("expected only the newer event to be scanned, got %#v", summary)
	}
	if _, ok := store.Killmails["killmail:stillness:311"]; !ok {
		t.Fatalf("newer killmail was not derived: %#v", store.Killmails)
	}
	if _, ok := store.Killmails["killmail:stillness:310"]; ok {
		t.Fatalf("older killmail should have been skipped by cursor: %#v", store.Killmails)
	}
}

func TestRunEventDerivationCanTargetOneModule(t *testing.T) {
	store := db.NewMemoryStore()
	killmail := db.EventRecord{
		ID:          "event:killmail:0",
		Kind:        "killmail.created",
		Environment: model.EnvironmentStillness,
		OccurredAt:  time.Date(2026, 6, 24, 10, 0, 0, 0, time.UTC),
		Module:      "killmail",
		SourceID:    "source:sui:sui-testnet:graphql",
		Payload: map[string]any{
			"json": map[string]any{
				"key":             map[string]any{"tenant": "stillness", "item_id": "310"},
				"victim_id":       map[string]any{"tenant": "stillness", "item_id": "2112000304"},
				"solar_system_id": map[string]any{"tenant": "stillness", "item_id": "30001001"},
			},
		},
	}
	networkNode := db.EventRecord{
		ID:          "event:network-node:0",
		Kind:        "network_node.created",
		Environment: model.EnvironmentStillness,
		OccurredAt:  killmail.OccurredAt.Add(time.Minute),
		Module:      "network_node",
		SourceID:    "source:sui:sui-testnet:graphql",
		Payload: map[string]any{
			"json": map[string]any{
				"network_node_key": map[string]any{"tenant": "stillness", "item_id": "910"},
			},
		},
	}
	if err := store.UpsertSuiEvent(context.Background(), killmail); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertSuiEvent(context.Background(), networkNode); err != nil {
		t.Fatal(err)
	}

	summary, err := RunEventDerivation(context.Background(), store, EventDerivationOptions{
		Environment: model.EnvironmentStillness,
		Network:     "sui-testnet",
		BatchSize:   10,
		Modules:     []string{"killmail"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if summary.EventsScanned != 1 || summary.KillmailsDerived != 1 {
		t.Fatalf("expected only killmail events to be derived, got %#v", summary)
	}
	if _, ok := store.Entities["assembly:stillness:910"]; ok {
		t.Fatalf("network_node entity should not have been derived by killmail-only run: %#v", store.Entities)
	}
	cursorID := DeriveEventsModuleCursorID("sui-testnet", model.EnvironmentStillness, "killmail")
	if _, ok, err := store.GetSyncCursor(context.Background(), cursorID); err != nil || !ok {
		t.Fatalf("module derive cursor was not saved ok=%v err=%v", ok, err)
	}
}

func TestRunEventDerivationUsesBulkStorePerPage(t *testing.T) {
	store := &bulkEventStore{MemoryStore: db.NewMemoryStore()}
	for _, event := range []db.EventRecord{
		{
			ID:          "event:killmail:1",
			Kind:        "killmail.created",
			Environment: model.EnvironmentStillness,
			OccurredAt:  time.Date(2026, 6, 24, 10, 0, 0, 0, time.UTC),
			Module:      "killmail",
			SourceID:    "source:sui:sui-testnet:graphql",
			Payload: map[string]any{
				"json": map[string]any{
					"key":             map[string]any{"tenant": "stillness", "item_id": "310"},
					"victim_id":       map[string]any{"tenant": "stillness", "item_id": "2112000304"},
					"solar_system_id": map[string]any{"tenant": "stillness", "item_id": "30001001"},
				},
			},
		},
		{
			ID:          "event:killmail:2",
			Kind:        "killmail.created",
			Environment: model.EnvironmentStillness,
			OccurredAt:  time.Date(2026, 6, 24, 10, 1, 0, 0, time.UTC),
			Module:      "killmail",
			SourceID:    "source:sui:sui-testnet:graphql",
			Payload: map[string]any{
				"json": map[string]any{
					"key":             map[string]any{"tenant": "stillness", "item_id": "311"},
					"victim_id":       map[string]any{"tenant": "stillness", "item_id": "2112000305"},
					"solar_system_id": map[string]any{"tenant": "stillness", "item_id": "30001001"},
				},
			},
		},
	} {
		if err := store.UpsertSuiEvent(context.Background(), event); err != nil {
			t.Fatal(err)
		}
	}

	summary, err := RunEventDerivation(context.Background(), store, EventDerivationOptions{
		Environment: model.EnvironmentStillness,
		Network:     "sui-testnet",
		BatchSize:   10,
		Modules:     []string{"killmail"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if summary.EventsScanned != 2 || summary.KillmailsDerived != 2 {
		t.Fatalf("unexpected summary %#v", summary)
	}
	if store.bulkCalls != 1 {
		t.Fatalf("expected one bulk write for one derivation page, got %d", store.bulkCalls)
	}
	if store.killmailCount != 2 {
		t.Fatalf("expected two killmails in the bulk write, got %d", store.killmailCount)
	}
}

func TestRunEventDerivationHonoursLargeInternalBatchSize(t *testing.T) {
	store := db.NewMemoryStore()
	for i := 0; i < 250; i++ {
		event := db.EventRecord{
			ID:          fmt.Sprintf("event:generic:%03d", i),
			Kind:        "status.updated",
			Environment: model.EnvironmentStillness,
			OccurredAt:  time.Date(2026, 6, 24, 10, 0, 0, 0, time.UTC).Add(time.Duration(i) * time.Second),
			Module:      "status",
			SourceID:    "source:sui:sui-testnet:graphql",
			Payload:     map[string]any{"json": map[string]any{}},
		}
		if err := store.UpsertSuiEvent(context.Background(), event); err != nil {
			t.Fatal(err)
		}
	}

	summary, err := RunEventDerivation(context.Background(), store, EventDerivationOptions{
		Environment: model.EnvironmentStillness,
		Network:     "sui-testnet",
		BatchSize:   500,
		Modules:     []string{"status"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if summary.Batches != 1 || summary.EventsScanned != 250 {
		t.Fatalf("expected one large internal batch, got %#v", summary)
	}
}

type bulkEventStore struct {
	*db.MemoryStore
	bulkCalls     int
	killmailCount int
}

func (s *bulkEventStore) UpsertEventDerivationBatch(ctx context.Context, entities []db.EntityFactSet, relations []db.RelationDraft, killmails []model.KillmailRaw) error {
	s.bulkCalls++
	s.killmailCount += len(killmails)
	for _, item := range entities {
		if err := s.MemoryStore.UpsertEntityFacts(ctx, item.Entity, item.Facts); err != nil {
			return err
		}
	}
	for _, raw := range killmails {
		if err := s.MemoryStore.UpsertKillmail(ctx, raw); err != nil {
			return err
		}
	}
	return s.MemoryStore.UpsertRelations(ctx, relations)
}

func hasDerivedEntity(entities []DerivedEventEntity, id string, entityType model.EntityType) bool {
	for _, entity := range entities {
		if entity.Entity.ID == id && entity.Entity.Type == entityType {
			return true
		}
	}
	return false
}

func cursorForEvent(event db.EventRecord) (string, error) {
	return cursor.Encode(cursor.Keyset{Time: event.OccurredAt, ID: event.ID})
}

func hasRelation(relations []db.RelationDraft, subject, predicate, object string) bool {
	for _, relation := range relations {
		if relation.SubjectEntityID == subject && relation.Predicate == predicate && relation.ObjectEntityID == object {
			return true
		}
	}
	return false
}

func hasFactCycle(facts []db.EntityFactDraft, key string, cycle int) bool {
	for _, fact := range facts {
		if fact.Key == key && fact.Cycle != nil && *fact.Cycle == cycle {
			return true
		}
	}
	return false
}

func hasEventFact(facts []db.EntityFactDraft, key string, value any) bool {
	for _, fact := range facts {
		if fact.Key == key && fact.Value == value {
			return true
		}
	}
	return false
}
