package report

import (
	"context"
	"testing"
	"time"

	"github.com/blackrelay/registry/internal/db"
	"github.com/blackrelay/registry/internal/model"
	"github.com/blackrelay/registry/internal/staticdata"
)

func TestBuildKillmailAuditExcludesFixturesAndSamplesUnresolvedRows(t *testing.T) {
	store := db.NewMemoryStore()
	now := time.Date(2026, 6, 25, 13, 0, 0, 0, time.UTC)
	staticSource := model.Source{
		ID:          "source:static-client:stillness:reviewed-enemies",
		Kind:        model.SourceKindStaticClientData,
		Title:       "Reviewed enemies",
		Locator:     "fixture",
		Environment: model.EnvironmentStillness,
		CreatedAt:   now,
	}
	staticArtefact := model.SourceArtefact{
		ID:           "artefact:static-enemies",
		SourceID:     staticSource.ID,
		Environment:  model.EnvironmentStillness,
		SHA256:       "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
		ContentType:  "application/json",
		ExtractedAt:  now,
		ImporterName: "test",
	}
	if err := store.RecordImport(context.Background(), "import:static-enemies", staticSource, staticArtefact, nil); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertStaticEnemy(context.Background(), "import:static-enemies", staticSource, staticArtefact, staticdata.EnemyCandidate{
		Name:       "Caird",
		GroupID:    5033,
		TypeID:     92096,
		Confidence: string(model.ConfidenceProbable),
		Basis:      "confirmed enemy group 5033",
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertKillmail(context.Background(), model.KillmailRaw{
		ID:           "killmail:stillness:fixture:caird",
		Environment:  model.EnvironmentStillness,
		OccurredAt:   now,
		KillerTypeID: "92096",
		SourceIDs:    []string{"source:fixture:killmail"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertKillmail(context.Background(), model.KillmailRaw{
		ID:          "killmail:stillness:live-unresolved",
		Environment: model.EnvironmentStillness,
		OccurredAt:  now.Add(-time.Minute),
		SourceIDs:   []string{"source:sui:sui-testnet:graphql"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertKillmail(context.Background(), model.KillmailRaw{
		ID:                "killmail:stillness:raw-killer-id",
		Environment:       model.EnvironmentStillness,
		OccurredAt:        now.Add(-2 * time.Minute),
		KillerCharacterID: "92096",
		SourceIDs:         []string{"source:sui:sui-testnet:graphql"},
		Raw: map[string]any{
			"killer_id": map[string]any{"tenant": "stillness", "item_id": "92096"},
		},
	}); err != nil {
		t.Fatal(err)
	}

	got, err := BuildKillmailAudit(context.Background(), store, KillmailAuditOptions{
		Environment:     model.EnvironmentStillness,
		ExcludeFixtures: true,
		SampleLimit:     2,
		Now:             func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Counts.Total != 2 || got.Counts.NPCKillers != 0 || got.Counts.UnresolvedKillers != 2 {
		t.Fatalf("unexpected fixture-excluded counts %#v", got.Counts)
	}
	if got.Evidence.RawKillerIDsWithoutTypeIDs != 1 || got.Evidence.RawKillerIDsWarnedAsNotStaticNPCType != 1 {
		t.Fatalf("raw killer evidence was not counted separately: %#v", got.Evidence)
	}
	if len(got.Samples.UnresolvedKillers) != 2 {
		t.Fatalf("unexpected unresolved killer samples %#v", got.Samples.UnresolvedKillers)
	}
}

func TestBuildKillmailAuditSamplesPreferResolvedDisplayNames(t *testing.T) {
	store := db.NewMemoryStore()
	now := time.Date(2026, 6, 25, 13, 30, 0, 0, time.UTC)
	character := testAuditEntity(model.EntityTypeCharacter, "character:stillness:2112091476", "Character 2112091476", now)
	if err := store.UpsertEntityFacts(context.Background(), character, []db.EntityFactDraft{
		{
			Key:          "metadata_name",
			Value:        "Specter",
			SourceID:     "source:sui:sui-testnet:graphql:objects",
			Confidence:   model.ConfidenceVerified,
			Environment:  model.EnvironmentStillness,
			ReviewStatus: model.ReviewStatusReviewed,
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertKillmail(context.Background(), model.KillmailRaw{
		ID:                "killmail:stillness:resolved-killer",
		Environment:       model.EnvironmentStillness,
		OccurredAt:        now,
		KillerCharacterID: "character:stillness:2112091476",
		SourceIDs:         []string{"source:sui:sui-testnet:graphql"},
	}); err != nil {
		t.Fatal(err)
	}

	got, err := BuildKillmailAudit(context.Background(), store, KillmailAuditOptions{
		Environment: model.EnvironmentStillness,
		SampleLimit: 1,
		Now:         func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Counts.CharacterKillers != 1 {
		t.Fatalf("unexpected audit counts %#v", got.Counts)
	}
	if len(got.Samples.CharacterKillers) != 1 || got.Samples.CharacterKillers[0].Killer != "Specter" {
		t.Fatalf("sample did not use resolved display name %#v", got.Samples.CharacterKillers)
	}
}

func TestBuildTribeIdentityEvidenceAuditSeparatesMembershipFromIdentity(t *testing.T) {
	store := db.NewMemoryStore()
	now := time.Date(2026, 6, 26, 9, 0, 0, 0, time.UTC)
	if err := store.UpsertSuiEvent(context.Background(), db.EventRecord{
		ID:          "event:membership-only",
		Kind:        "character.updated",
		Environment: model.EnvironmentStillness,
		OccurredAt:  now,
		Module:      "character",
		Payload: map[string]any{
			"character_id": "2112091476",
			"tribe_id":     "42",
			"metadata": map[string]any{
				"name":        "FC Jotunn",
				"description": "Character profile, not tribe profile",
				"url":         "https://example.invalid/character/fc-jotunn",
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertSuiEvent(context.Background(), db.EventRecord{
		ID:          "event:tribe-identity",
		Kind:        "notify.tribe_join",
		Environment: model.EnvironmentStillness,
		OccurredAt:  now.Add(time.Minute),
		Module:      "character",
		Payload: map[string]any{
			"tribe": map[string]any{
				"id":          "42",
				"name":        "Black Relay",
				"ticker":      "BR",
				"description": "Public tribe profile",
				"url":         "https://example.invalid/tribes/black-relay",
			},
		},
	}); err != nil {
		t.Fatal(err)
	}

	got, err := BuildTribeIdentityEvidenceAudit(context.Background(), store, TribeIdentityEvidenceAuditOptions{
		Environment: model.EnvironmentStillness,
		SampleLimit: 5,
		Now:         func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Counts.EventsScanned != 2 || got.Counts.ObjectsScanned != 0 {
		t.Fatalf("unexpected scan counts %#v", got.Counts)
	}
	if got.Counts.RowsWithTribeID != 2 || got.Counts.RowsWithOnlyMembership != 1 {
		t.Fatalf("membership-only evidence was not counted correctly: %#v", got.Counts)
	}
	if got.Counts.CandidateIdentityRows != 1 || got.Counts.RowsWithTribeName != 1 || got.Counts.RowsWithTribeTicker != 1 || got.Counts.RowsWithProfileText != 1 {
		t.Fatalf("identity/profile evidence was not counted correctly: %#v", got.Counts)
	}
	if len(got.Samples.MembershipOnly) != 1 || got.Samples.MembershipOnly[0].ID != "event:membership-only" {
		t.Fatalf("unexpected membership samples %#v", got.Samples.MembershipOnly)
	}
	if len(got.Samples.IdentityCandidates) != 1 || got.Samples.IdentityCandidates[0].TribeName != "Black Relay" || got.Samples.IdentityCandidates[0].TribeTicker != "BR" {
		t.Fatalf("unexpected identity samples %#v", got.Samples.IdentityCandidates)
	}
}

func TestBuildTribeIdentityEvidenceAuditDetectsObjectCorpAlias(t *testing.T) {
	store := db.NewMemoryStore()
	now := time.Date(2026, 6, 26, 9, 30, 0, 0, time.UTC)
	if err := store.UpsertSuiObject(context.Background(), db.SuiObjectRecord{
		ID:          "object:profile",
		ObjectID:    "0xprofile",
		Environment: model.EnvironmentStillness,
		TypeRepr:    "0xworld::character::PlayerProfile",
		PackageID:   "0xworld",
		Module:      "character",
		TypeName:    "PlayerProfile",
		ObservedAt:  now,
		Payload: map[string]any{
			"corpId": "99",
		},
	}); err != nil {
		t.Fatal(err)
	}

	got, err := BuildTribeIdentityEvidenceAudit(context.Background(), store, TribeIdentityEvidenceAuditOptions{
		Environment:    model.EnvironmentStillness,
		ObjectTypeName: "PlayerProfile",
		SampleLimit:    5,
		Now:            func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Counts.ObjectsScanned != 1 || got.Counts.RowsWithTribeID != 1 || got.Counts.RowsWithOnlyMembership != 1 {
		t.Fatalf("corpId alias was not treated as membership evidence: %#v", got.Counts)
	}
	if len(got.Samples.MembershipOnly) != 1 || got.Samples.MembershipOnly[0].TribeID != "99" {
		t.Fatalf("unexpected corpId sample %#v", got.Samples.MembershipOnly)
	}
}

func TestBuildCurrentStateAuditCountsDomainCoverage(t *testing.T) {
	store := db.NewMemoryStore()
	now := time.Date(2026, 6, 25, 14, 0, 0, 0, time.UTC)
	sourceID := "source:sui:sui-testnet:graphql"
	for _, entity := range []model.Entity{
		testAuditEntity(model.EntityTypeCharacter, "character:stillness:tao", "Tao", now),
		testAuditEntity(model.EntityTypeCharacter, "character:stillness:other", "Other", now),
		testAuditEntity(model.EntityTypeTribe, "tribe:stillness:black-relay", "Black Relay", now),
		testAuditEntity(model.EntityTypeAssembly, "assembly:stillness:100", "Assembly 100", now),
		testAuditEntity(model.EntityTypeGate, "gate:stillness:100", "Gate 100", now),
		testAuditEntity(model.EntityTypeGate, "gate:stillness:200", "Gate 200", now),
		testAuditEntity(model.EntityTypeStorage, "storage:stillness:300", "Storage 300", now),
		testAuditEntity(model.EntityTypeTurret, "turret:stillness:400", "Turret 400", now),
		testAuditEntity(model.EntityTypeSystem, "system:stillness:30001001", "NN0-Y-D5", now),
		testAuditEntity(model.EntityTypeSystem, "system:stillness:30001002", "6RG-Y-T4", now),
		testAuditEntity(model.EntityTypeRoute, "route:stillness:one", "Route One", now),
		testAuditEntity(model.EntityTypeKillmail, "killmail:stillness:100", "Killmail 100", now),
		testAuditEntity(model.EntityTypeResourceObject, "resource_object:stillness:owner-cap:0xcap", "Owner capability 0xcap", now),
		testAuditEntity(model.EntityTypeResourceObject, "resource_object:stillness:location-hash:loc-1", "Location hash loc-1", now),
	} {
		if err := store.UpsertEntityFacts(context.Background(), entity, nil); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.UpsertRelations(context.Background(), []db.RelationDraft{
		{SubjectEntityID: "character:stillness:tao", Predicate: "belongs_to", ObjectEntityID: "tribe:stillness:black-relay", SourceID: sourceID, Confidence: model.ConfidenceVerified, Environment: model.EnvironmentStillness},
		{SubjectEntityID: "assembly:stillness:100", Predicate: "owned_by", ObjectEntityID: "character:stillness:tao", SourceID: sourceID, Confidence: model.ConfidenceVerified, Environment: model.EnvironmentStillness},
		{SubjectEntityID: "assembly:stillness:100", Predicate: "located_in", ObjectEntityID: "system:stillness:30001001", SourceID: sourceID, Confidence: model.ConfidenceVerified, Environment: model.EnvironmentStillness},
		{SubjectEntityID: "storage:stillness:300", Predicate: "owned_by", ObjectEntityID: "character:stillness:tao", SourceID: sourceID, Confidence: model.ConfidenceVerified, Environment: model.EnvironmentStillness},
		{SubjectEntityID: "turret:stillness:400", Predicate: "located_in", ObjectEntityID: "system:stillness:30001001", SourceID: sourceID, Confidence: model.ConfidenceVerified, Environment: model.EnvironmentStillness},
		{SubjectEntityID: "gate:stillness:100", Predicate: "links_to", ObjectEntityID: "gate:stillness:200", SourceID: sourceID, Confidence: model.ConfidenceVerified, Environment: model.EnvironmentStillness},
		{SubjectEntityID: "system:stillness:30001001", Predicate: "links_to", ObjectEntityID: "system:stillness:30001002", SourceID: sourceID, Confidence: model.ConfidenceVerified, Environment: model.EnvironmentStillness},
		{SubjectEntityID: "route:stillness:one", Predicate: "observed_between", ObjectEntityID: "system:stillness:30001001", SourceID: sourceID, Confidence: model.ConfidenceVerified, Environment: model.EnvironmentStillness},
		{SubjectEntityID: "killmail:stillness:100", Predicate: "victim", ObjectEntityID: "character:stillness:tao", SourceID: sourceID, Confidence: model.ConfidenceVerified, Environment: model.EnvironmentStillness},
		{SubjectEntityID: "killmail:stillness:100", Predicate: "occurred_in", ObjectEntityID: "system:stillness:30001001", SourceID: sourceID, Confidence: model.ConfidenceVerified, Environment: model.EnvironmentStillness},
		{SubjectEntityID: "assembly:stillness:100", Predicate: "has_owner_cap", ObjectEntityID: "resource_object:stillness:owner-cap:0xcap", SourceID: sourceID, Confidence: model.ConfidenceVerified, Environment: model.EnvironmentStillness},
		{SubjectEntityID: "assembly:stillness:100", Predicate: "has_location_hash", ObjectEntityID: "resource_object:stillness:location-hash:loc-1", SourceID: sourceID, Confidence: model.ConfidenceVerified, Environment: model.EnvironmentStillness},
		{SubjectEntityID: "storage:stillness:300", Predicate: "has_owner_cap", ObjectEntityID: "resource_object:stillness:owner-cap:0xcap", SourceID: sourceID, Confidence: model.ConfidenceVerified, Environment: model.EnvironmentStillness},
		{SubjectEntityID: "storage:stillness:300", Predicate: "has_location_hash", ObjectEntityID: "resource_object:stillness:location-hash:loc-1", SourceID: sourceID, Confidence: model.ConfidenceVerified, Environment: model.EnvironmentStillness},
		{SubjectEntityID: "turret:stillness:400", Predicate: "has_owner_cap", ObjectEntityID: "resource_object:stillness:owner-cap:0xcap", SourceID: sourceID, Confidence: model.ConfidenceVerified, Environment: model.EnvironmentStillness},
		{SubjectEntityID: "turret:stillness:400", Predicate: "has_location_hash", ObjectEntityID: "resource_object:stillness:location-hash:loc-1", SourceID: sourceID, Confidence: model.ConfidenceVerified, Environment: model.EnvironmentStillness},
	}); err != nil {
		t.Fatal(err)
	}

	got, err := BuildCurrentStateAudit(context.Background(), store, CurrentStateAuditOptions{
		Environment: model.EnvironmentStillness,
		Now:         func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Counts.Characters != 2 || got.Counts.CharactersWithTribe != 1 || got.Counts.CharactersWithActivity != 1 {
		t.Fatalf("unexpected character audit counts %#v", got.Counts)
	}
	if got.Counts.AssembliesWithOwner != 1 || got.Counts.AssembliesWithSystem != 1 {
		t.Fatalf("unexpected assembly audit counts %#v", got.Counts)
	}
	if got.Counts.AssembliesWithOwnerCap != 1 || got.Counts.AssembliesWithLocationHash != 1 {
		t.Fatalf("unexpected assembly evidence counts %#v", got.Counts)
	}
	if got.Counts.StorageWithOwnerCap != 1 || got.Counts.StorageWithLocationHash != 1 {
		t.Fatalf("unexpected storage evidence counts %#v", got.Counts)
	}
	if got.Counts.TurretsWithOwnerCap != 1 || got.Counts.TurretsWithLocationHash != 1 {
		t.Fatalf("unexpected turret evidence counts %#v", got.Counts)
	}
	if got.Counts.GatesWithLinkedGate != 2 {
		t.Fatalf("unexpected gate audit counts %#v", got.Counts)
	}
	if got.Counts.SystemsWithActivity != 1 || got.Counts.SystemsWithConnections != 2 {
		t.Fatalf("unexpected system audit counts %#v", got.Counts)
	}
	if got.Counts.OwnershipRelations != 2 || got.Counts.RouteEdgeRelations != 3 {
		t.Fatalf("unexpected relation audit counts %#v", got.Counts)
	}
	if got.Counts.OwnerCapRelations != 3 || got.Counts.LocationHashRelations != 3 {
		t.Fatalf("unexpected evidence relation audit counts %#v", got.Counts)
	}
}

func TestBuildCharacterProfileAuditCountsKnownAndPlaceholderProfiles(t *testing.T) {
	store := db.NewMemoryStore()
	now := time.Date(2026, 6, 25, 14, 30, 0, 0, time.UTC)
	sourceID := "source:sui:sui-testnet:graphql:objects"
	if err := store.UpsertEntityFacts(context.Background(), testAuditEntity(model.EntityTypeCharacter, "character:stillness:2112091476", "FC Jotunn", now), []db.EntityFactDraft{
		{Key: "metadata_name", Value: "FC Jotunn", SourceID: sourceID, Confidence: model.ConfidenceVerified, Environment: model.EnvironmentStillness, ReviewStatus: model.ReviewStatusReviewed},
		{Key: "metadata_description", Value: "Public pilot profile", SourceID: sourceID, Confidence: model.ConfidenceVerified, Environment: model.EnvironmentStillness, ReviewStatus: model.ReviewStatusReviewed},
		{Key: "metadata_url", Value: "https://example.invalid/characters/2112091476", SourceID: sourceID, Confidence: model.ConfidenceVerified, Environment: model.EnvironmentStillness, ReviewStatus: model.ReviewStatusReviewed},
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertEntityFacts(context.Background(), testAuditEntity(model.EntityTypeCharacter, "character:stillness:42", "Character 42", now.Add(-time.Minute)), []db.EntityFactDraft{
		{Key: "character_id", Value: "42", SourceID: sourceID, Confidence: model.ConfidenceVerified, Environment: model.EnvironmentStillness, ReviewStatus: model.ReviewStatusReviewed},
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertEntityFacts(context.Background(), testAuditEntity(model.EntityTypeTribe, "tribe:stillness:99", "Named Tribe", now), nil); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertEntityFacts(context.Background(), testAuditEntity(model.EntityTypeKillmail, "killmail:stillness:1", "Killmail 1", now), nil); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertRelations(context.Background(), []db.RelationDraft{
		{SubjectEntityID: "character:stillness:2112091476", Predicate: "belongs_to", ObjectEntityID: "tribe:stillness:99", SourceID: sourceID, Confidence: model.ConfidenceVerified, Environment: model.EnvironmentStillness},
		{SubjectEntityID: "killmail:stillness:1", Predicate: "victim", ObjectEntityID: "character:stillness:2112091476", SourceID: sourceID, Confidence: model.ConfidenceVerified, Environment: model.EnvironmentStillness},
	}); err != nil {
		t.Fatal(err)
	}

	got, err := BuildCharacterProfileAudit(context.Background(), store, CharacterProfileAuditOptions{
		Environment: model.EnvironmentStillness,
		SampleLimit: 1,
		Now:         func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.SchemaVersion != "registry.character-profile-audit.v1" {
		t.Fatalf("unexpected schema version %q", got.SchemaVersion)
	}
	if got.Counts.Characters != 2 ||
		got.Counts.NamedCharacters != 1 ||
		got.Counts.PlaceholderDisplayNames != 1 ||
		got.Counts.WithMetadataName != 1 ||
		got.Counts.WithMetadataDescription != 1 ||
		got.Counts.WithMetadataURL != 1 ||
		got.Counts.WithTribe != 1 ||
		got.Counts.WithActivity != 1 {
		t.Fatalf("unexpected character profile audit counts %#v", got.Counts)
	}
	if len(got.Samples.PlaceholderDisplayNames) != 1 || got.Samples.PlaceholderDisplayNames[0].EntityID != "character:stillness:42" {
		t.Fatalf("placeholder samples were not captured: %#v", got.Samples.PlaceholderDisplayNames)
	}
}

func TestBuildEvidenceBridgeAuditReportsEvidenceAndResolvedBridgeCoverage(t *testing.T) {
	store := db.NewMemoryStore()
	now := time.Date(2026, 6, 25, 15, 0, 0, 0, time.UTC)
	sourceID := "source:sui:sui-testnet:graphql"
	for _, entity := range []model.Entity{
		testAuditEntity(model.EntityTypeCharacter, "character:stillness:owner", "Owner", now),
		testAuditEntity(model.EntityTypeSystem, "system:stillness:30001001", "NN0-Y-D5", now),
		testAuditEntity(model.EntityTypeAssembly, "assembly:stillness:100", "Assembly 100", now),
		testAuditEntity(model.EntityTypeAssembly, "assembly:stillness:200", "Assembly 200", now.Add(-time.Minute)),
		testAuditEntity(model.EntityTypeResourceObject, "resource_object:stillness:owner-cap:0xcap", "Owner capability 0xcap", now),
		testAuditEntity(model.EntityTypeResourceObject, "resource_object:stillness:location-hash:loc-1", "Location hash loc-1", now),
	} {
		if err := store.UpsertEntityFacts(context.Background(), entity, nil); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.UpsertEntityFacts(context.Background(), store.Entities["character:stillness:owner"], []db.EntityFactDraft{
		{Key: "owner_cap_id", Value: "0xcap", SourceID: sourceID, Confidence: model.ConfidenceVerified, Environment: model.EnvironmentStillness, ReviewStatus: model.ReviewStatusReviewed},
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertEntityFacts(context.Background(), store.Entities["system:stillness:30001001"], []db.EntityFactDraft{
		{Key: "location_hash", Value: "loc-1", SourceID: sourceID, Confidence: model.ConfidenceVerified, Environment: model.EnvironmentStillness, ReviewStatus: model.ReviewStatusReviewed},
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertEntityFacts(context.Background(), store.Entities["assembly:stillness:100"], []db.EntityFactDraft{
		{Key: "owner_cap_id", Value: "0xcap", SourceID: sourceID, Confidence: model.ConfidenceVerified, Environment: model.EnvironmentStillness, ReviewStatus: model.ReviewStatusReviewed},
		{Key: "location_hash", Value: "loc-1", SourceID: sourceID, Confidence: model.ConfidenceVerified, Environment: model.EnvironmentStillness, ReviewStatus: model.ReviewStatusReviewed},
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertEntityFacts(context.Background(), store.Entities["assembly:stillness:200"], []db.EntityFactDraft{
		{Key: "owner_cap_id", Value: "0xunresolved", SourceID: sourceID, Confidence: model.ConfidenceVerified, Environment: model.EnvironmentStillness, ReviewStatus: model.ReviewStatusReviewed},
		{Key: "location_hash", Value: "loc-unresolved", SourceID: sourceID, Confidence: model.ConfidenceVerified, Environment: model.EnvironmentStillness, ReviewStatus: model.ReviewStatusReviewed},
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertRelations(context.Background(), []db.RelationDraft{
		{SubjectEntityID: "assembly:stillness:100", Predicate: "has_owner_cap", ObjectEntityID: "resource_object:stillness:owner-cap:0xcap", SourceID: sourceID, Confidence: model.ConfidenceVerified, Environment: model.EnvironmentStillness},
		{SubjectEntityID: "assembly:stillness:100", Predicate: "has_location_hash", ObjectEntityID: "resource_object:stillness:location-hash:loc-1", SourceID: sourceID, Confidence: model.ConfidenceVerified, Environment: model.EnvironmentStillness},
		{SubjectEntityID: "assembly:stillness:100", Predicate: "owned_by", ObjectEntityID: "character:stillness:owner", SourceID: sourceID, Confidence: model.ConfidenceVerified, Environment: model.EnvironmentStillness},
		{SubjectEntityID: "assembly:stillness:100", Predicate: "located_in", ObjectEntityID: "system:stillness:30001001", SourceID: sourceID, Confidence: model.ConfidenceVerified, Environment: model.EnvironmentStillness},
	}); err != nil {
		t.Fatal(err)
	}

	got, err := BuildEvidenceBridgeAudit(context.Background(), store, EvidenceBridgeAuditOptions{
		Environment: model.EnvironmentStillness,
		SampleLimit: 2,
		Now:         func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.SchemaVersion != "registry.evidence-bridge-audit.v1" {
		t.Fatalf("unexpected schema version %q", got.SchemaVersion)
	}
	if got.Counts.InfrastructureWithOwnerCap != 2 ||
		got.Counts.InfrastructureWithLocationHash != 2 ||
		got.Counts.InfrastructureWithResolvedOwner != 1 ||
		got.Counts.InfrastructureWithResolvedSystem != 1 ||
		got.Counts.CharactersWithOwnerCap != 1 ||
		got.Counts.SystemsWithLocationHash != 1 ||
		got.Counts.UniqueOwnerCapValues != 2 ||
		got.Counts.UniqueLocationHashValues != 2 {
		t.Fatalf("unexpected evidence bridge counts %#v", got.Counts)
	}
	if len(got.Samples.UnresolvedOwnerCaps) != 1 || got.Samples.UnresolvedOwnerCaps[0].Value != "0xunresolved" {
		t.Fatalf("unresolved owner-cap samples were not captured: %#v", got.Samples.UnresolvedOwnerCaps)
	}
	if len(got.Samples.UnresolvedLocationHashes) != 1 || got.Samples.UnresolvedLocationHashes[0].Value != "loc-unresolved" {
		t.Fatalf("unresolved location-hash samples were not captured: %#v", got.Samples.UnresolvedLocationHashes)
	}
}

func testAuditEntity(entityType model.EntityType, id, name string, updatedAt time.Time) model.Entity {
	return model.Entity{
		ID:          id,
		Slug:        id,
		Type:        entityType,
		Name:        name,
		DisplayName: name,
		Environment: model.EnvironmentStillness,
		UpdatedAt:   updatedAt,
	}
}
