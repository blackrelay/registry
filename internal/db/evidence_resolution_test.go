package db

import (
	"context"
	"testing"
	"time"

	"github.com/blackrelay/registry/internal/model"
)

func TestPostgresResolveEvidenceRelationsPromotesUnambiguousOwnerAndLocation(t *testing.T) {
	ctx := context.Background()
	pool := testPostgresPool(ctx, t)
	defer pool.Close()
	store := PostgresStore{Pool: pool}

	source := testResolutionSource(t, ctx, store)
	now := time.Date(2026, 6, 26, 10, 0, 0, 0, time.UTC)
	entities := []EntityFactSet{
		{
			Entity: testEntity(model.EntityTypeCharacter, "character:stillness:2112000001", "Owner", now),
			Facts: []EntityFactDraft{{
				Key:          "owner_cap_id",
				Value:        "0xowner",
				SourceID:     source.ID,
				Confidence:   model.ConfidenceVerified,
				Environment:  model.EnvironmentStillness,
				ReviewStatus: model.ReviewStatusReviewed,
			}},
		},
		{
			Entity: testEntity(model.EntityTypeSystem, "system:stillness:30001001", "NN0-Y-D5", now),
			Facts: []EntityFactDraft{{
				Key:          "location_hash",
				Value:        "loc-abc",
				SourceID:     source.ID,
				Confidence:   model.ConfidenceVerified,
				Environment:  model.EnvironmentStillness,
				ReviewStatus: model.ReviewStatusReviewed,
			}},
		},
		{
			Entity: testEntity(model.EntityTypeAssembly, "assembly:stillness:100", "Assembly 100", now),
			Facts: []EntityFactDraft{
				{
					Key:          "owner_cap_id",
					Value:        "0xowner",
					SourceID:     source.ID,
					Confidence:   model.ConfidenceVerified,
					Environment:  model.EnvironmentStillness,
					ReviewStatus: model.ReviewStatusReviewed,
				},
				{
					Key:          "location_hash",
					Value:        "loc-abc",
					SourceID:     source.ID,
					Confidence:   model.ConfidenceVerified,
					Environment:  model.EnvironmentStillness,
					ReviewStatus: model.ReviewStatusReviewed,
				},
			},
		},
	}
	if err := store.UpsertEventDerivationBatch(ctx, entities, nil, nil); err != nil {
		t.Fatal(err)
	}

	counts, err := store.ResolveEvidenceRelations(ctx, model.EnvironmentStillness)
	if err != nil {
		t.Fatal(err)
	}
	if counts.OwnershipRelations != 1 || counts.LocationRelations != 1 {
		t.Fatalf("unexpected evidence resolution counts %#v", counts)
	}

	current, err := store.ListCurrentEntities(ctx, CurrentEntityQuery{
		Type:        model.EntityTypeAssembly,
		Environment: model.EnvironmentStillness,
		OwnerID:     "character:stillness:2112000001",
		SystemID:    "system:stillness:30001001",
		Limit:       10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(current.Items) != 1 || current.Items[0].Entity.ID != "assembly:stillness:100" {
		t.Fatalf("assembly was not queryable by resolved owner and system: %#v", current.Items)
	}
}

func TestPostgresUpsertPreservesImportedSystemDisplayWhenPlaceholderArrives(t *testing.T) {
	ctx := context.Background()
	pool := testPostgresPool(ctx, t)
	defer pool.Close()
	store := PostgresStore{Pool: pool}

	source := testResolutionSource(t, ctx, store)
	now := time.Date(2026, 6, 26, 10, 0, 0, 0, time.UTC)
	if err := store.UpsertEventDerivationBatch(ctx, []EntityFactSet{{
		Entity: testEntity(model.EntityTypeSystem, "system:stillness:30016868", "I8V-PCH", now),
		Facts: []EntityFactDraft{{
			Key:          "system_id",
			Value:        "30016868",
			SourceID:     source.ID,
			Confidence:   model.ConfidenceVerified,
			Environment:  model.EnvironmentStillness,
			ReviewStatus: model.ReviewStatusReviewed,
		}},
	}}, nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertEventDerivationBatch(ctx, []EntityFactSet{{
		Entity: model.Entity{
			ID:          "system:stillness:30016868",
			Slug:        "system-30016868-stillness",
			Type:        model.EntityTypeSystem,
			Name:        "System 30016868",
			DisplayName: "System 30016868",
			Summary:     "Public on-chain solar system reference observed from Sui object data.",
			Environment: model.EnvironmentStillness,
			UpdatedAt:   now.Add(time.Minute),
		},
		Facts: []EntityFactDraft{{
			Key:          "item_id",
			Value:        "30016868",
			SourceID:     source.ID,
			Confidence:   model.ConfidenceVerified,
			Environment:  model.EnvironmentStillness,
			ReviewStatus: model.ReviewStatusReviewed,
		}},
	}}, nil, nil); err != nil {
		t.Fatal(err)
	}

	current, err := store.ListCurrentEntities(ctx, CurrentEntityQuery{
		Type:        model.EntityTypeSystem,
		Environment: model.EnvironmentStillness,
		SystemID:    "system:stillness:30016868",
		Limit:       10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(current.Items) != 1 {
		t.Fatalf("expected one system, got %#v", current.Items)
	}
	got := current.Items[0].Entity
	if got.Name != "I8V-PCH" || got.DisplayName != "I8V-PCH" {
		t.Fatalf("placeholder overwrote imported system display: %#v", got)
	}
}

func TestPostgresUpsertPreservesImportedCharacterDisplayWhenPlaceholderArrives(t *testing.T) {
	ctx := context.Background()
	pool := testPostgresPool(ctx, t)
	defer pool.Close()
	store := PostgresStore{Pool: pool}

	source := testResolutionSource(t, ctx, store)
	now := time.Date(2026, 6, 26, 10, 0, 0, 0, time.UTC)
	if err := store.UpsertEventDerivationBatch(ctx, []EntityFactSet{{
		Entity: testEntity(model.EntityTypeCharacter, "character:stillness:2112091476", "FC Jotunn", now),
		Facts: []EntityFactDraft{{
			Key:          "metadata_name",
			Value:        "FC Jotunn",
			SourceID:     source.ID,
			Confidence:   model.ConfidenceVerified,
			Environment:  model.EnvironmentStillness,
			ReviewStatus: model.ReviewStatusReviewed,
		}},
	}}, nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertEventDerivationBatch(ctx, []EntityFactSet{{
		Entity: model.Entity{
			ID:          "character:stillness:2112091476",
			Slug:        "character-2112091476-stillness",
			Type:        model.EntityTypeCharacter,
			Name:        "Character 2112091476",
			DisplayName: "Character 2112091476",
			Summary:     "Public on-chain character identity observed from Sui event data.",
			Environment: model.EnvironmentStillness,
			UpdatedAt:   now.Add(time.Minute),
		},
		Facts: []EntityFactDraft{{
			Key:          "character_id",
			Value:        "2112091476",
			SourceID:     source.ID,
			Confidence:   model.ConfidenceVerified,
			Environment:  model.EnvironmentStillness,
			ReviewStatus: model.ReviewStatusReviewed,
		}},
	}}, nil, nil); err != nil {
		t.Fatal(err)
	}

	current, err := store.ListCurrentEntities(ctx, CurrentEntityQuery{
		Type:        model.EntityTypeCharacter,
		Environment: model.EnvironmentStillness,
		Limit:       10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(current.Items) != 1 {
		t.Fatalf("expected one character, got %#v", current.Items)
	}
	got := current.Items[0].Entity
	if got.Name != "FC Jotunn" || got.DisplayName != "FC Jotunn" {
		t.Fatalf("placeholder overwrote imported character display: %#v", got)
	}
}

func TestPostgresResolveEvidenceRelationsSkipsAmbiguousOwnerCap(t *testing.T) {
	ctx := context.Background()
	pool := testPostgresPool(ctx, t)
	defer pool.Close()
	store := PostgresStore{Pool: pool}

	source := testResolutionSource(t, ctx, store)
	now := time.Date(2026, 6, 26, 10, 0, 0, 0, time.UTC)
	entities := []EntityFactSet{
		{
			Entity: testEntity(model.EntityTypeCharacter, "character:stillness:2112000001", "Owner A", now),
			Facts: []EntityFactDraft{{
				Key:          "owner_cap_id",
				Value:        "0xshared",
				SourceID:     source.ID,
				Confidence:   model.ConfidenceVerified,
				Environment:  model.EnvironmentStillness,
				ReviewStatus: model.ReviewStatusReviewed,
			}},
		},
		{
			Entity: testEntity(model.EntityTypeCharacter, "character:stillness:2112000002", "Owner B", now),
			Facts: []EntityFactDraft{{
				Key:          "owner_cap_id",
				Value:        "0xshared",
				SourceID:     source.ID,
				Confidence:   model.ConfidenceVerified,
				Environment:  model.EnvironmentStillness,
				ReviewStatus: model.ReviewStatusReviewed,
			}},
		},
		{
			Entity: testEntity(model.EntityTypeAssembly, "assembly:stillness:100", "Assembly 100", now),
			Facts: []EntityFactDraft{{
				Key:          "owner_cap_id",
				Value:        "0xshared",
				SourceID:     source.ID,
				Confidence:   model.ConfidenceVerified,
				Environment:  model.EnvironmentStillness,
				ReviewStatus: model.ReviewStatusReviewed,
			}},
		},
	}
	if err := store.UpsertEventDerivationBatch(ctx, entities, nil, nil); err != nil {
		t.Fatal(err)
	}

	counts, err := store.ResolveEvidenceRelations(ctx, model.EnvironmentStillness)
	if err != nil {
		t.Fatal(err)
	}
	if counts.OwnershipRelations != 0 {
		t.Fatalf("ambiguous owner cap should not be promoted: %#v", counts)
	}
}

func TestPostgresCurrentEntityQueryFiltersByEvidenceOwnerCapAndLocationHash(t *testing.T) {
	ctx := context.Background()
	pool := testPostgresPool(ctx, t)
	defer pool.Close()
	store := PostgresStore{Pool: pool}

	source := testResolutionSource(t, ctx, store)
	now := time.Date(2026, 6, 26, 10, 0, 0, 0, time.UTC)
	entities := []EntityFactSet{
		{
			Entity: testEntity(model.EntityTypeAssembly, "assembly:stillness:100", "Assembly 100", now),
			Facts: []EntityFactDraft{
				{
					Key:          "owner_cap_id",
					Value:        "0xowner",
					SourceID:     source.ID,
					Confidence:   model.ConfidenceVerified,
					Environment:  model.EnvironmentStillness,
					ReviewStatus: model.ReviewStatusReviewed,
				},
				{
					Key:          "location_hash",
					Value:        "loc-abc",
					SourceID:     source.ID,
					Confidence:   model.ConfidenceVerified,
					Environment:  model.EnvironmentStillness,
					ReviewStatus: model.ReviewStatusReviewed,
				},
			},
		},
		{
			Entity: testEntity(model.EntityTypeAssembly, "assembly:stillness:200", "Assembly 200", now.Add(-time.Minute)),
			Facts: []EntityFactDraft{
				{
					Key:          "owner_cap_id",
					Value:        "0xother",
					SourceID:     source.ID,
					Confidence:   model.ConfidenceVerified,
					Environment:  model.EnvironmentStillness,
					ReviewStatus: model.ReviewStatusReviewed,
				},
				{
					Key:          "location_hash",
					Value:        "loc-other",
					SourceID:     source.ID,
					Confidence:   model.ConfidenceVerified,
					Environment:  model.EnvironmentStillness,
					ReviewStatus: model.ReviewStatusReviewed,
				},
			},
		},
	}
	if err := store.UpsertEventDerivationBatch(ctx, entities, nil, nil); err != nil {
		t.Fatal(err)
	}

	current, err := store.ListCurrentEntities(ctx, CurrentEntityQuery{
		Type:         model.EntityTypeAssembly,
		Environment:  model.EnvironmentStillness,
		OwnerCapID:   "0xowner",
		LocationHash: "loc-abc",
		Limit:        10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(current.Items) != 1 || current.Items[0].Entity.ID != "assembly:stillness:100" {
		t.Fatalf("assembly was not queryable by owner cap and location hash evidence: %#v", current.Items)
	}
}

func TestPostgresSourceGapsCountsEvidenceOnlyAndUnresolvedKillmails(t *testing.T) {
	ctx := context.Background()
	pool := testPostgresPool(ctx, t)
	defer pool.Close()
	store := PostgresStore{Pool: pool}

	source := testResolutionSource(t, ctx, store)
	now := time.Date(2026, 6, 26, 10, 0, 0, 0, time.UTC)
	if err := store.UpsertEventDerivationBatch(ctx, []EntityFactSet{{
		Entity: testEntity(model.EntityTypeAssembly, "assembly:stillness:gap", "Gap Assembly", now),
		Facts: []EntityFactDraft{
			{Key: "owner_cap_id", Value: "0xowner", SourceID: source.ID, Confidence: model.ConfidenceVerified, Environment: model.EnvironmentStillness, ReviewStatus: model.ReviewStatusReviewed},
			{Key: "location_hash", Value: "loc-gap", SourceID: source.ID, Confidence: model.ConfidenceVerified, Environment: model.EnvironmentStillness, ReviewStatus: model.ReviewStatusReviewed},
		},
	}, {
		Entity: model.Entity{
			ID:          "tribe:stillness:42",
			Slug:        "tribe-42-stillness",
			Type:        model.EntityTypeTribe,
			Name:        "Tribe 42",
			DisplayName: "Tribe 42",
			Environment: model.EnvironmentStillness,
			UpdatedAt:   now,
		},
		Facts: []EntityFactDraft{
			{Key: "tribe_id", Value: "42", SourceID: source.ID, Confidence: model.ConfidenceVerified, Environment: model.EnvironmentStillness, ReviewStatus: model.ReviewStatusPublished},
		},
	}}, nil, []model.KillmailRaw{{
		ID:          "killmail:stillness:gap",
		Environment: model.EnvironmentStillness,
		OccurredAt:  now,
		SourceIDs:   []string{source.ID},
	}}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveSyncCursor(ctx, CursorStatus{
		ID:               "cursor:sui:sui-testnet:objects:range-blocked",
		Source:           "sui:sui-testnet:objects:range-blocked",
		Environment:      model.EnvironmentStillness,
		CursorKind:       "sui_object",
		EventsProcessed:  12,
		ErrorCount:       3,
		LastErrorSummary: "sui GraphQL returned errors: Request is outside consistent range",
		UpdatedAt:        now,
	}); err != nil {
		t.Fatal(err)
	}

	gaps, err := store.ListSourceGaps(ctx, model.EnvironmentStillness)
	if err != nil {
		t.Fatal(err)
	}
	seen := make(map[string]int64)
	for _, gap := range gaps {
		seen[gap.ID] = gap.Count
	}
	for _, id := range []string{
		"source-gap:stillness:ownership-evidence-only",
		"source-gap:stillness:location-evidence-only",
		"source-gap:stillness:unresolved-killmail-actors",
		"source-gap:stillness:sui-object-provider-range-blocked",
		"source-gap:stillness:tribe-identity-names",
		"source-gap:stillness:static-client-full-table-decoder",
	} {
		if seen[id] == 0 {
			t.Fatalf("expected source gap %s in %#v", id, gaps)
		}
	}
}

func TestPostgresCurrentEntityQueryFiltersStaticUniverseMembersByIncomingSystemRelation(t *testing.T) {
	ctx := context.Background()
	pool := testPostgresPool(ctx, t)
	defer pool.Close()
	store := PostgresStore{Pool: pool}

	source := testResolutionSource(t, ctx, store)
	now := time.Date(2026, 6, 26, 10, 0, 0, 0, time.UTC)
	entities := []EntityFactSet{
		{Entity: testEntity(model.EntityTypeSystem, "system:stillness:30001001", "NN0-Y-D5", now)},
		{Entity: testEntity(model.EntityTypeConstellation, "constellation:stillness:20000068", "Inner First", now.Add(-time.Minute))},
		{Entity: testEntity(model.EntityTypeRegion, "region:stillness:10000012", "Inner Stone Cluster", now.Add(-2*time.Minute))},
	}
	if err := store.UpsertEventDerivationBatch(ctx, entities, nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertRelations(ctx, []RelationDraft{
		{SubjectEntityID: "system:stillness:30001001", Predicate: "located_in", ObjectEntityID: "constellation:stillness:20000068", SourceID: source.ID, Confidence: model.ConfidenceVerified, Environment: model.EnvironmentStillness},
		{SubjectEntityID: "system:stillness:30001001", Predicate: "member_of_region", ObjectEntityID: "region:stillness:10000012", SourceID: source.ID, Confidence: model.ConfidenceVerified, Environment: model.EnvironmentStillness},
	}); err != nil {
		t.Fatal(err)
	}

	constellations, err := store.ListCurrentEntities(ctx, CurrentEntityQuery{
		Type:        model.EntityTypeConstellation,
		Environment: model.EnvironmentStillness,
		SystemID:    "system:stillness:30001001",
		Limit:       10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(constellations.Items) != 1 || constellations.Items[0].Entity.ID != "constellation:stillness:20000068" {
		t.Fatalf("constellation was not queryable by member system: %#v", constellations.Items)
	}
	regions, err := store.ListCurrentEntities(ctx, CurrentEntityQuery{
		Type:        model.EntityTypeRegion,
		Environment: model.EnvironmentStillness,
		SystemID:    "system:stillness:30001001",
		Limit:       10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(regions.Items) != 1 || regions.Items[0].Entity.ID != "region:stillness:10000012" {
		t.Fatalf("region was not queryable by member system: %#v", regions.Items)
	}
}

func testResolutionSource(t *testing.T, ctx context.Context, store PostgresStore) model.Source {
	t.Helper()
	source := model.Source{
		ID:          "source:sui:sui-testnet:graphql:objects",
		Kind:        model.SourceKindSuiObject,
		Title:       "Sui testnet GraphQL objects",
		Locator:     "fixture",
		Environment: model.EnvironmentStillness,
		CreatedAt:   time.Date(2026, 6, 26, 10, 0, 0, 0, time.UTC),
	}
	if err := store.EnsureSource(ctx, source); err != nil {
		t.Fatal(err)
	}
	return source
}
