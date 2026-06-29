package db

import (
	"context"
	"testing"
	"time"

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

func TestDeriveCurrentEntityExposesStaticUniverseHierarchy(t *testing.T) {
	item := model.CurrentEntity{
		Entity: model.Entity{
			ID:          "system:stillness:30001001",
			Type:        model.EntityTypeSystem,
			Environment: model.EnvironmentStillness,
		},
		OutgoingRelations: []model.CurrentRelation{
			{
				SubjectEntityID:   "system:stillness:30001001",
				Predicate:         "located_in",
				ObjectEntityID:    "constellation:stillness:20000001",
				ObjectEntityType:  model.EntityTypeConstellation,
				ObjectDisplayName: "C-20000001",
				SubjectEntityType: model.EntityTypeSystem,
			},
			{
				SubjectEntityID:   "system:stillness:30001001",
				Predicate:         "member_of_region",
				ObjectEntityID:    "region:stillness:10000001",
				ObjectEntityType:  model.EntityTypeRegion,
				ObjectDisplayName: "000-0Y-0",
			},
		},
	}

	deriveCurrentEntity(&item)

	if item.Derived == nil {
		t.Fatal("expected derived static universe state")
	}
	if item.Derived.Constellation == nil || item.Derived.Constellation.DisplayName != "C-20000001" {
		t.Fatalf("constellation was not exposed: %#v", item.Derived)
	}
	if item.Derived.Region == nil || item.Derived.Region.DisplayName != "000-0Y-0" {
		t.Fatalf("region was not exposed: %#v", item.Derived)
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

func TestCurrentEntityMatchesBareAndCanonicalTribeIDs(t *testing.T) {
	item := model.CurrentEntity{
		Entity: model.Entity{
			ID:          "character:stillness:2112092610",
			Type:        model.EntityTypeCharacter,
			Name:        "Hei Warden",
			DisplayName: "Hei Warden",
			Environment: model.EnvironmentStillness,
		},
		OutgoingRelations: []model.CurrentRelation{{
			SubjectEntityID:  "character:stillness:2112092610",
			Predicate:        "belongs_to",
			ObjectEntityID:   "tribe:stillness:1000167",
			ObjectEntityType: model.EntityTypeTribe,
		}},
	}
	deriveCurrentEntity(&item)

	if !currentEntityMatchesQuery(item, CurrentEntityQuery{TribeID: "1000167"}) {
		t.Fatalf("bare tribe ID did not match canonical tribe relation")
	}

	item.OutgoingRelations[0].ObjectEntityID = "1000167"
	deriveCurrentEntity(&item)
	if !currentEntityMatchesQuery(item, CurrentEntityQuery{TribeID: "tribe:stillness:1000167"}) {
		t.Fatalf("canonical tribe ID did not match bare tribe relation")
	}
}

func TestDedupeCurrentCharacterIdentitiesPreservesDistinctCharacterIDs(t *testing.T) {
	now := time.Date(2026, 6, 27, 13, 12, 29, 0, time.UTC)
	const characterAddress = "0xdff1ca19cea48a7d452cd0d79ebed10398bb90178aa7d2a4726e99e3344b5c78"
	items := []model.CurrentEntity{
		{
			Entity: model.Entity{
				ID:          "character:stillness:2112092421",
				Type:        model.EntityTypeCharacter,
				Name:        "Hei Warden",
				DisplayName: "Hei Warden",
				Environment: model.EnvironmentStillness,
				Cycle:       intPtr(5),
				UpdatedAt:   now,
			},
			Facts: map[string]any{
				"character_address": characterAddress,
				"object_id":         "0x782282305a916627bb9a96e89d24224c6bb2d4db14a85f2208311a91e65c3bf7",
			},
			OutgoingRelations: []model.CurrentRelation{{
				ID:                "relation:legacy-character-tribe",
				SubjectEntityID:   "character:stillness:2112092421",
				SubjectEntityType: model.EntityTypeCharacter,
				Predicate:         "belongs_to",
				ObjectEntityID:    "tribe:stillness:1000167",
				ObjectEntityType:  model.EntityTypeTribe,
			}},
			SourceIDs: []string{"source:sui-object:legacy"},
		},
		{
			Entity: model.Entity{
				ID:          "character:stillness:2112092610",
				Type:        model.EntityTypeCharacter,
				Name:        "Hei Warden",
				DisplayName: "Hei Warden",
				Environment: model.EnvironmentStillness,
				Cycle:       intPtr(6),
				UpdatedAt:   now.Add(-2 * time.Hour),
			},
			Facts: map[string]any{
				"character_address": characterAddress,
				"source_event_kind": "character.created",
				"source_event_id":   "event:character-created",
			},
			OutgoingRelations: []model.CurrentRelation{{
				ID:                "relation:event-character-tribe",
				SubjectEntityID:   "character:stillness:2112092610",
				SubjectEntityType: model.EntityTypeCharacter,
				Predicate:         "belongs_to",
				ObjectEntityID:    "tribe:stillness:1000167",
				ObjectEntityType:  model.EntityTypeTribe,
			}},
			SourceIDs: []string{"source:sui-event:cycle-6"},
		},
	}

	deduped := dedupeCurrentEntities(items, CurrentEntityQuery{Type: model.EntityTypeCharacter})

	if len(deduped) != 2 {
		t.Fatalf("character ids with the same address and name must stay distinct, got %#v", deduped)
	}
	if deduped[0].Entity.ID != "character:stillness:2112092421" || deduped[1].Entity.ID != "character:stillness:2112092610" {
		t.Fatalf("dedupe changed character identity rows: %#v", deduped)
	}
}

func TestCurrentCycleCharacterQueryRejectsObjectOnlyRows(t *testing.T) {
	objectOnly := model.CurrentEntity{
		Entity: model.Entity{
			ID:          "character:stillness:2112077591",
			Type:        model.EntityTypeCharacter,
			Name:        "Cassius",
			DisplayName: "Cassius",
			Environment: model.EnvironmentStillness,
			Cycle:       intPtr(6),
		},
		Facts: map[string]any{
			"character_address": "0xf09dfb4627f9144213d3c9a0390933b5febbe2f2bc959404d309d0538ea4fec4",
			"metadata_name":     "Cassius",
			"package_id":        "0x28b497559d65ab320d9da4613bf2498d5946b2c0ae3597ccfda3072ce127448c",
		},
	}
	deriveCurrentEntity(&objectOnly)
	if currentEntityMatchesQuery(objectOnly, CurrentEntityQuery{
		Type:            model.EntityTypeCharacter,
		Environment:     model.EnvironmentStillness,
		Cycles:          []int{6},
		IncludeUncycled: true,
	}) {
		t.Fatalf("Cycle-scoped character query matched old object-only character row")
	}

	eventBacked := objectOnly
	eventBacked.Facts = map[string]any{
		"character_address": "0xf09dfb4627f9144213d3c9a0390933b5febbe2f2bc959404d309d0538ea4fec4",
		"metadata_name":     "Cassius",
		"source_event_kind": "character.created",
		"source_event_id":   "event:character-created",
	}
	deriveCurrentEntity(&eventBacked)
	if !currentEntityMatchesQuery(eventBacked, CurrentEntityQuery{
		Type:            model.EntityTypeCharacter,
		Environment:     model.EnvironmentStillness,
		Cycles:          []int{6},
		IncludeUncycled: true,
	}) {
		t.Fatalf("Cycle-scoped character query rejected event-backed character row")
	}
}

func TestCurrentCycleTribeQueryRejectsPlaceholderAndNPCCorpRows(t *testing.T) {
	query := CurrentEntityQuery{
		Type:        model.EntityTypeTribe,
		Environment: model.EnvironmentStillness,
		Cycles:      []int{6},
	}
	publicTribe := model.CurrentEntity{
		Entity: model.Entity{
			ID:          "tribe:stillness:1000167",
			Type:        model.EntityTypeTribe,
			Name:        "Clonebank 86",
			DisplayName: "Clonebank 86",
			Environment: model.EnvironmentStillness,
			Cycle:       intPtr(6),
		},
		Facts: map[string]any{"tribe_id": "1000167", "tag": "CO86"},
	}
	deriveCurrentEntity(&publicTribe)
	if !currentEntityMatchesQuery(publicTribe, query) {
		t.Fatalf("Cycle-scoped tribe query rejected public tribe row")
	}

	placeholder := publicTribe
	placeholder.Entity.ID = "tribe:stillness:98000539"
	placeholder.Entity.Name = "Tribe 98000539"
	placeholder.Entity.DisplayName = "Tribe 98000539"
	placeholder.Facts = map[string]any{"tribe_id": "98000539"}
	deriveCurrentEntity(&placeholder)
	if currentEntityMatchesQuery(placeholder, query) {
		t.Fatalf("Cycle-scoped tribe query matched placeholder tribe row")
	}
	if !currentEntityMatchesQuery(placeholder, CurrentEntityQuery{Type: model.EntityTypeTribe}) {
		t.Fatalf("unscoped tribe query should still be able to inspect raw placeholder evidence")
	}

	npcCorp := publicTribe
	npcCorp.Entity.ID = "tribe:stillness:1000167:npc"
	npcCorp.Entity.Name = "NPC Corp 1000167"
	npcCorp.Entity.DisplayName = "NPC Corp 1000167"
	npcCorp.Facts = map[string]any{"tribe_id": "1000167", "tag": "C086"}
	deriveCurrentEntity(&npcCorp)
	if currentEntityMatchesQuery(npcCorp, query) {
		t.Fatalf("Cycle-scoped tribe query matched NPC corp tribe row")
	}
}

func TestDedupeCurrentTribeIdentitiesPrefersNamedProfileRow(t *testing.T) {
	now := time.Date(2026, 6, 28, 10, 30, 45, 0, time.UTC)
	items := []model.CurrentEntity{
		{
			Entity: model.Entity{
				ID:          "tribe:stillness:1000167",
				Type:        model.EntityTypeTribe,
				Name:        "Clonebank 86",
				DisplayName: "Clonebank 86",
				Environment: model.EnvironmentStillness,
				Cycle:       intPtr(6),
				UpdatedAt:   now,
			},
			Facts: map[string]any{
				"tribe_id": "1000167",
				"tag":      "CO86",
			},
			SourceIDs: []string{"source:world-api:tribes"},
		},
		{
			Entity: model.Entity{
				ID:          "tribe:liminality:1000167",
				Type:        model.EntityTypeTribe,
				Name:        "Tribe 1000167",
				DisplayName: "Tribe 1000167",
				Environment: model.EnvironmentStillness,
				Cycle:       intPtr(6),
				UpdatedAt:   now.Add(-time.Hour),
			},
			Facts: map[string]any{
				"tribe_id":           "1000167",
				"source_event_kind":  "character.created",
				"transaction_digest": "ZEXV842KCQhXa3jhynE6EbYpigUZ88Q9amEfMKpa9e4",
			},
			IncomingRelations: []model.CurrentRelation{{
				ID:                 "relation:character:liminality:2112000001:belongs_to:tribe:liminality:1000167",
				SubjectEntityID:    "character:liminality:2112000001",
				SubjectEntityType:  model.EntityTypeCharacter,
				SubjectDisplayName: "fingolfin",
				Predicate:          "belongs_to",
				ObjectEntityID:     "tribe:liminality:1000167",
				ObjectEntityType:   model.EntityTypeTribe,
				ObjectDisplayName:  "Tribe 1000167",
			}},
			SourceIDs: []string{"source:sui:sui-testnet:graphql"},
		},
	}

	deduped := dedupeCurrentEntities(items, CurrentEntityQuery{Type: model.EntityTypeTribe})

	if len(deduped) != 1 {
		t.Fatalf("expected one current tribe identity, got %#v", deduped)
	}
	if deduped[0].Entity.ID != "tribe:stillness:1000167" || deduped[0].Entity.DisplayName != "Clonebank 86" {
		t.Fatalf("expected named World API row to win, got %#v", deduped[0].Entity)
	}
	if deduped[0].Facts["source_event_kind"] != "character.created" {
		t.Fatalf("event evidence was not retained: %#v", deduped[0].Facts)
	}
	if !containsString(deduped[0].SourceIDs, "source:world-api:tribes") || !containsString(deduped[0].SourceIDs, "source:sui:sui-testnet:graphql") {
		t.Fatalf("source evidence was not merged: %#v", deduped[0].SourceIDs)
	}
}

func TestMemoryCurrentSystemUpgradesPlaceholderToStaticName(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	now := time.Date(2026, 6, 27, 12, 0, 0, 0, time.UTC)
	placeholder := model.Entity{
		ID:          "system:stillness:30001001",
		Slug:        "system-30001001-stillness",
		Type:        model.EntityTypeSystem,
		Name:        "System 30001001",
		DisplayName: "System 30001001",
		Environment: model.EnvironmentStillness,
		Cycle:       intPtr(6),
		UpdatedAt:   now,
	}
	if err := store.UpsertEntityFacts(ctx, placeholder, nil); err != nil {
		t.Fatalf("insert placeholder system: %v", err)
	}
	named := placeholder
	named.Name = "NN0-Y-D5"
	named.DisplayName = "NN0-Y-D5"
	named.Summary = "Static-client solar system metadata."
	named.UpdatedAt = now.Add(time.Minute)
	if err := store.UpsertEntityFacts(ctx, named, []EntityFactDraft{{
		Key:          "system_id",
		Value:        "30001001",
		SourceID:     "source:static-universe:stillness",
		Confidence:   model.ConfidenceVerified,
		Environment:  model.EnvironmentStillness,
		Cycle:        intPtr(6),
		ReviewStatus: model.ReviewStatusPublished,
	}}); err != nil {
		t.Fatalf("insert named system: %v", err)
	}

	page, err := store.ListCurrentEntities(ctx, CurrentEntityQuery{
		Type:        model.EntityTypeSystem,
		Environment: model.EnvironmentStillness,
		Cycles:      []int{6},
		Limit:       10,
	})
	if err != nil {
		t.Fatalf("list current systems: %v", err)
	}
	if len(page.Items) != 1 {
		t.Fatalf("expected one system, got %#v", page.Items)
	}
	if page.Items[0].Entity.DisplayName != "NN0-Y-D5" {
		t.Fatalf("system placeholder was not upgraded: %#v", page.Items[0].Entity)
	}
}

func intPtr(value int) *int {
	return &value
}
