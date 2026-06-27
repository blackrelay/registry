package killmail

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/blackrelay/registry/internal/db"
	"github.com/blackrelay/registry/internal/model"
	"github.com/blackrelay/registry/internal/resolver"
	"github.com/blackrelay/registry/internal/staticdata"
)

func TestSemanticKillmailResolvesNPCKiller(t *testing.T) {
	store := db.NewMemoryStore()
	source := model.Source{ID: "source:static-client:stillness:reviewed-enemies", Kind: model.SourceKindStaticClientData, Title: "fixture", Locator: "fixture", Environment: model.EnvironmentStillness}
	artefact := model.SourceArtefact{ID: "artefact:fixture", SourceID: source.ID, Environment: model.EnvironmentStillness, SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}
	if err := store.RecordImport(context.Background(), "import:fixture", source, artefact, nil); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertStaticEnemy(context.Background(), "import:fixture", source, artefact, staticdata.EnemyCandidate{Name: "Caird", GroupID: 5033, TypeID: 92096, Confidence: string(model.ConfidenceProbable), Basis: "confirmed enemy group 5033"}); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 6, 24, 10, 0, 0, 0, time.UTC)
	store.Entities["character:stillness:victim"] = model.Entity{ID: "character:stillness:victim", Slug: "fixture-victim", Type: model.EntityTypeCharacter, Name: "Fixture Victim", Environment: model.EnvironmentStillness, UpdatedAt: now}
	store.Entities["system:stillness:nn0-y-d5"] = model.Entity{ID: "system:stillness:nn0-y-d5", Slug: "nn0-y-d5", Type: model.EntityTypeSystem, Name: "NN0-Y-D5", Environment: model.EnvironmentStillness, UpdatedAt: now}
	service := Service{Resolver: resolver.Resolver{Store: store}}
	semantic := service.Semantic(context.Background(), model.KillmailRaw{
		ID:                "killmail:stillness:fixture:caird",
		Environment:       model.EnvironmentStillness,
		OccurredAt:        now,
		SystemID:          "system:stillness:nn0-y-d5",
		VictimCharacterID: "character:stillness:victim",
		KillerTypeID:      "92096",
		SourceIDs:         []string{"source:fixture"},
	})
	if semantic.Killer.EntityType != model.EntityTypeEnemy {
		t.Fatalf("expected enemy killer, got %#v", semantic.Killer)
	}
	if semantic.Killer.DisplayName != "Caird [NPC]" {
		t.Fatalf("unexpected killer display name %q", semantic.Killer.DisplayName)
	}
	if !semantic.Killer.IsNPC {
		t.Fatalf("expected killer to be marked as NPC: %#v", semantic.Killer)
	}
	if semantic.SummaryText != "Caird [NPC] killed Fixture Victim" {
		t.Fatalf("unexpected summary text %q", semantic.SummaryText)
	}
	if semantic.System.DisplayName != "NN0-Y-D5" {
		t.Fatalf("unexpected system resolution %#v", semantic.System)
	}
}

func TestSemanticKillmailPrefersResolvedEnemyTypeWhenRawPayloadAlsoContainsKillerCharacterID(t *testing.T) {
	store := db.NewMemoryStore()
	source := model.Source{ID: "source:static-client:stillness:reviewed-enemies", Kind: model.SourceKindStaticClientData, Title: "fixture", Locator: "fixture", Environment: model.EnvironmentStillness}
	artefact := model.SourceArtefact{ID: "artefact:fixture", SourceID: source.ID, Environment: model.EnvironmentStillness, SHA256: "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"}
	if err := store.RecordImport(context.Background(), "import:fixture", source, artefact, nil); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertStaticEnemy(context.Background(), "import:fixture", source, artefact, staticdata.EnemyCandidate{Name: "Mycena", GroupID: 5130, TypeID: 94167, Confidence: string(model.ConfidenceProbable), Basis: "confirmed enemy group 5130"}); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 6, 25, 10, 0, 0, 0, time.UTC)
	store.Entities["character:stillness:94167"] = model.Entity{ID: "character:stillness:94167", Slug: "character-94167-stillness", Type: model.EntityTypeCharacter, Name: "Character 94167", DisplayName: "Character 94167", Environment: model.EnvironmentStillness, UpdatedAt: now}
	store.Entities["character:stillness:victim"] = model.Entity{ID: "character:stillness:victim", Slug: "victim", Type: model.EntityTypeCharacter, Name: "Victim", DisplayName: "Victim", Environment: model.EnvironmentStillness, UpdatedAt: now}
	service := Service{Resolver: resolver.Resolver{Store: store}, GraphStore: store}

	semantic := service.Semantic(context.Background(), model.KillmailRaw{
		ID:          "killmail:stillness:npc-with-killer-id",
		Environment: model.EnvironmentStillness,
		OccurredAt:  now,
		Raw: map[string]any{
			"event": map[string]any{
				"json": map[string]any{
					"killer_type_id": "94167",
					"killer_id":      map[string]any{"tenant": "stillness", "item_id": "94167"},
					"victim_id":      map[string]any{"tenant": "stillness", "item_id": "victim"},
				},
			},
		},
	})

	if semantic.Killer.EntityType != model.EntityTypeEnemy || semantic.Killer.DisplayName != "Mycena [NPC]" || !semantic.Killer.IsNPC {
		t.Fatalf("expected resolved NPC killer to win over raw killer_id, got %#v", semantic.Killer)
	}
	if semantic.SummaryText != "Mycena [NPC] killed Victim" {
		t.Fatalf("unexpected summary text %q", semantic.SummaryText)
	}
}

func TestSemanticKillmailDoesNotTreatRawKillerItemIDAsNPCType(t *testing.T) {
	store := db.NewMemoryStore()
	source := model.Source{ID: "source:static-client:enemies:stillness", Kind: model.SourceKindStaticClientData, Title: "fixture", Locator: "fixture", Environment: model.EnvironmentStillness}
	artefact := model.SourceArtefact{ID: "artefact:fixture-mycena", SourceID: source.ID, Environment: model.EnvironmentStillness, SHA256: "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"}
	if err := store.RecordImport(context.Background(), "import:fixture-mycena", source, artefact, nil); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertStaticEnemy(context.Background(), "import:fixture-mycena", source, artefact, staticdata.EnemyCandidate{Name: "Mycena", GroupID: 5130, TypeID: 94167, Confidence: string(model.ConfidenceProbable), Basis: "confirmed enemy group 5130"}); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 6, 25, 10, 0, 0, 0, time.UTC)
	store.Entities["character:stillness:victim"] = model.Entity{ID: "character:stillness:victim", Slug: "victim", Type: model.EntityTypeCharacter, Name: "Victim", DisplayName: "Victim", Environment: model.EnvironmentStillness, UpdatedAt: now}
	service := Service{Resolver: resolver.Resolver{Store: store}, GraphStore: store}

	semantic := service.Semantic(context.Background(), model.KillmailRaw{
		ID:          "killmail:stillness:npc-item-id",
		Environment: model.EnvironmentStillness,
		OccurredAt:  now,
		Raw: map[string]any{
			"object": map[string]any{
				"json": map[string]any{
					"killer_id": map[string]any{
						"tenant":  "stillness",
						"item_id": "94167",
					},
					"victim_id": map[string]any{
						"tenant":  "stillness",
						"item_id": "victim",
					},
				},
			},
		},
	})

	if semantic.Killer.EntityType != model.EntityTypeCharacter || semantic.Killer.IsNPC {
		t.Fatalf("expected raw killer_id to remain a character lookup, got %#v", semantic.Killer)
	}
	if semantic.Killer.EntityID != "" || semantic.Killer.Confidence != model.ConfidenceUnknown {
		t.Fatalf("unresolved raw killer_id should not be promoted as sourced evidence: %#v", semantic.Killer)
	}
	if !containsWarning(semantic.Killer.Warnings, "killer_id is not a static NPC type id") {
		t.Fatalf("expected explicit killer_id warning, got %#v", semantic.Killer.Warnings)
	}
}

func TestSemanticKillmailKeepsKnownCharacterWhenKillerItemIDAlsoMatchesNPCType(t *testing.T) {
	store := db.NewMemoryStore()
	source := model.Source{ID: "source:static-client:enemies:stillness", Kind: model.SourceKindStaticClientData, Title: "fixture", Locator: "fixture", Environment: model.EnvironmentStillness}
	artefact := model.SourceArtefact{ID: "artefact:fixture-mycena", SourceID: source.ID, Environment: model.EnvironmentStillness, SHA256: "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"}
	if err := store.RecordImport(context.Background(), "import:fixture-mycena", source, artefact, nil); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertStaticEnemy(context.Background(), "import:fixture-mycena", source, artefact, staticdata.EnemyCandidate{Name: "Mycena", GroupID: 5130, TypeID: 94167, Confidence: string(model.ConfidenceProbable), Basis: "confirmed enemy group 5130"}); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 6, 25, 10, 0, 0, 0, time.UTC)
	store.Entities["character:stillness:94167"] = model.Entity{ID: "character:stillness:94167", Slug: "character-94167-stillness", Type: model.EntityTypeCharacter, Name: "Known Pilot", DisplayName: "Known Pilot", Environment: model.EnvironmentStillness, UpdatedAt: now}
	store.Entities["character:stillness:victim"] = model.Entity{ID: "character:stillness:victim", Slug: "victim", Type: model.EntityTypeCharacter, Name: "Victim", DisplayName: "Victim", Environment: model.EnvironmentStillness, UpdatedAt: now}
	service := Service{Resolver: resolver.Resolver{Store: store}, GraphStore: store}

	semantic := service.Semantic(context.Background(), model.KillmailRaw{
		ID:          "killmail:stillness:known-character-item-id",
		Environment: model.EnvironmentStillness,
		OccurredAt:  now,
		Raw: map[string]any{
			"object": map[string]any{
				"json": map[string]any{
					"killer_id": map[string]any{
						"tenant":  "stillness",
						"item_id": "94167",
					},
					"victim_id": map[string]any{
						"tenant":  "stillness",
						"item_id": "victim",
					},
				},
			},
		},
	})

	if semantic.Killer.EntityType != model.EntityTypeCharacter || semantic.Killer.DisplayName != "Known Pilot" || semantic.Killer.IsNPC {
		t.Fatalf("expected known character to win over enemy fallback, got %#v", semantic.Killer)
	}
}

func TestSemanticKillmailReturnsUnresolvedKiller(t *testing.T) {
	service := Service{Resolver: resolver.Resolver{Store: db.NewMemoryStore()}}
	semantic := service.Semantic(context.Background(), model.KillmailRaw{
		ID:           "killmail:stillness:fixture:unknown",
		Environment:  model.EnvironmentStillness,
		OccurredAt:   time.Date(2026, 6, 24, 10, 0, 0, 0, time.UTC),
		KillerTypeID: "999999",
	})
	if semantic.Killer.EntityType != model.EntityTypeUnknown {
		t.Fatalf("expected unknown killer, got %#v", semantic.Killer)
	}
	if len(semantic.Killer.Warnings) == 0 {
		t.Fatal("expected unresolved killer warning")
	}
}

func TestSemanticKillmailUsesDerivedRelationsWhenRawFieldsAreSparse(t *testing.T) {
	store := db.NewMemoryStore()
	now := time.Date(2026, 6, 24, 10, 0, 0, 0, time.UTC)
	source := model.Source{ID: "source:sui:sui-testnet:graphql", Kind: model.SourceKindSuiEvent, Title: "Sui", Locator: "fixture", Environment: model.EnvironmentStillness}
	store.Sources[source.ID] = source
	store.Entities["killmail:stillness:310"] = model.Entity{ID: "killmail:stillness:310", Slug: "killmail-310-stillness", Type: model.EntityTypeKillmail, Name: "Killmail 310", Environment: model.EnvironmentStillness, UpdatedAt: now}
	store.Entities["character:stillness:victim"] = model.Entity{ID: "character:stillness:victim", Slug: "victim", Type: model.EntityTypeCharacter, Name: "Fixture Victim", Environment: model.EnvironmentStillness, UpdatedAt: now}
	store.Entities["character:stillness:killer"] = model.Entity{ID: "character:stillness:killer", Slug: "killer", Type: model.EntityTypeCharacter, Name: "Fixture Killer", Environment: model.EnvironmentStillness, UpdatedAt: now}
	store.Entities["system:stillness:nn0-y-d5"] = model.Entity{ID: "system:stillness:nn0-y-d5", Slug: "nn0-y-d5", Type: model.EntityTypeSystem, Name: "NN0-Y-D5", Environment: model.EnvironmentStillness, UpdatedAt: now}
	if err := store.UpsertRelations(context.Background(), []db.RelationDraft{
		{SubjectEntityID: "killmail:stillness:310", Predicate: "victim", ObjectEntityID: "character:stillness:victim", SourceID: source.ID, Confidence: model.ConfidenceVerified, Environment: model.EnvironmentStillness},
		{SubjectEntityID: "killmail:stillness:310", Predicate: "killer", ObjectEntityID: "character:stillness:killer", SourceID: source.ID, Confidence: model.ConfidenceVerified, Environment: model.EnvironmentStillness},
		{SubjectEntityID: "killmail:stillness:310", Predicate: "occurred_in", ObjectEntityID: "system:stillness:nn0-y-d5", SourceID: source.ID, Confidence: model.ConfidenceVerified, Environment: model.EnvironmentStillness},
	}); err != nil {
		t.Fatal(err)
	}

	service := Service{Resolver: resolver.Resolver{Store: store}, GraphStore: store}
	semantic := service.Semantic(context.Background(), model.KillmailRaw{
		ID:          "killmail:stillness:310",
		Environment: model.EnvironmentStillness,
		OccurredAt:  now,
	})
	if semantic.Victim.DisplayName != "Fixture Victim" {
		t.Fatalf("victim was not resolved from graph: %#v", semantic.Victim)
	}
	if semantic.Killer.DisplayName != "Fixture Killer" {
		t.Fatalf("killer was not resolved from graph: %#v", semantic.Killer)
	}
	if semantic.System.DisplayName != "NN0-Y-D5" {
		t.Fatalf("system was not resolved from graph: %#v", semantic.System)
	}
	if len(semantic.Sources) != 1 || semantic.Sources[0] != source.ID {
		t.Fatalf("source IDs did not include graph provenance: %#v", semantic.Sources)
	}
}

func TestSemanticKillmailPrefersSourcedCharacterMetadataName(t *testing.T) {
	store := db.NewMemoryStore()
	source := model.Source{ID: "source:sui:stillness:objects", Kind: model.SourceKindSuiObject, Title: "Sui objects", Locator: "fixture", Environment: model.EnvironmentStillness}
	store.Sources[source.ID] = source
	character := model.Entity{
		ID:          "character:stillness:2112090868",
		Slug:        "character-2112090868-stillness",
		Type:        model.EntityTypeCharacter,
		Name:        "Character 2112090868",
		DisplayName: "Character 2112090868",
		Environment: model.EnvironmentStillness,
		UpdatedAt:   time.Now().UTC(),
	}
	if err := store.UpsertEntityFacts(context.Background(), character, []db.EntityFactDraft{{
		Key:          "metadata_name",
		Value:        "Ven Unit",
		SourceID:     source.ID,
		Confidence:   model.ConfidenceVerified,
		Environment:  model.EnvironmentStillness,
		ReviewStatus: model.ReviewStatusReviewed,
	}}); err != nil {
		t.Fatal(err)
	}
	service := Service{Resolver: resolver.Resolver{Store: store}, GraphStore: store}
	semantic := service.Semantic(context.Background(), model.KillmailRaw{
		ID:                "killmail:stillness:28796",
		Environment:       model.EnvironmentStillness,
		OccurredAt:        time.Now().UTC(),
		KillerCharacterID: "character:stillness:2112090868",
	})

	if semantic.Killer.DisplayName != "Ven Unit" {
		t.Fatalf("expected sourced character name, got %#v", semantic.Killer)
	}
	if semantic.Killer.Confidence != model.ConfidenceVerified {
		t.Fatalf("expected sourced confidence, got %#v", semantic.Killer)
	}
	if len(semantic.Killer.SourceIDs) != 1 || semantic.Killer.SourceIDs[0] != source.ID {
		t.Fatalf("expected sourced character provenance, got %#v", semantic.Killer.SourceIDs)
	}
}

func TestSemanticKillmailUsesRawPayloadIDsWhenColumnsAreSparse(t *testing.T) {
	store := db.NewMemoryStore()
	source := model.Source{ID: "source:static-client:stillness:reviewed-enemies", Kind: model.SourceKindStaticClientData, Title: "fixture", Locator: "fixture", Environment: model.EnvironmentStillness}
	artefact := model.SourceArtefact{ID: "artefact:fixture", SourceID: source.ID, Environment: model.EnvironmentStillness, SHA256: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}
	if err := store.RecordImport(context.Background(), "import:fixture", source, artefact, nil); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertStaticEnemy(context.Background(), "import:fixture", source, artefact, staticdata.EnemyCandidate{Name: "Caird", GroupID: 5033, TypeID: 92096, Confidence: string(model.ConfidenceProbable), Basis: "confirmed enemy group 5033"}); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 6, 24, 10, 0, 0, 0, time.UTC)
	store.Entities["character:stillness:2112000304"] = model.Entity{ID: "character:stillness:2112000304", Slug: "character-2112000304-stillness", Type: model.EntityTypeCharacter, Name: "Victim", DisplayName: "Victim", Environment: model.EnvironmentStillness, UpdatedAt: now}
	store.Entities["system:stillness:30001001"] = model.Entity{ID: "system:stillness:30001001", Slug: "system-30001001-stillness", Type: model.EntityTypeSystem, Name: "NN0-Y-D5", DisplayName: "NN0-Y-D5", Environment: model.EnvironmentStillness, UpdatedAt: now}

	service := Service{Resolver: resolver.Resolver{Store: store}, GraphStore: store}
	semantic := service.Semantic(context.Background(), model.KillmailRaw{
		ID:          "killmail:stillness:raw:310",
		Environment: model.EnvironmentStillness,
		OccurredAt:  now,
		Raw: map[string]any{
			"event": map[string]any{
				"json": map[string]any{
					"killer_type_id":  "92096",
					"victim_id":       map[string]any{"tenant": "stillness", "item_id": "2112000304"},
					"solar_system_id": map[string]any{"tenant": "stillness", "item_id": "30001001"},
				},
			},
		},
	})

	if semantic.Killer.DisplayName != "Caird [NPC]" {
		t.Fatalf("killer was not resolved from raw payload: %#v", semantic.Killer)
	}
	if semantic.Victim.DisplayName != "Victim" {
		t.Fatalf("victim was not resolved from raw payload: %#v", semantic.Victim)
	}
	if semantic.System.DisplayName != "NN0-Y-D5" {
		t.Fatalf("system was not resolved from raw payload: %#v", semantic.System)
	}
}

func containsWarning(warnings []string, substring string) bool {
	for _, warning := range warnings {
		if strings.Contains(warning, substring) {
			return true
		}
	}
	return false
}
