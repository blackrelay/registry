package db

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/blackrelay/registry/internal/model"
	"github.com/blackrelay/registry/internal/staticdata"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresCountKillmailResolutionMatchesSemanticCases(t *testing.T) {
	ctx := context.Background()
	pool := testPostgresPool(ctx, t)
	defer pool.Close()
	store := PostgresStore{Pool: pool}

	now := time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)
	source := model.Source{
		ID:          "source:sui:sui-testnet:graphql",
		Kind:        model.SourceKindSuiEvent,
		Title:       "Sui testnet GraphQL",
		Locator:     "fixture",
		Environment: model.EnvironmentStillness,
		CreatedAt:   now,
	}
	if err := store.EnsureSource(ctx, source); err != nil {
		t.Fatal(err)
	}
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
		SHA256:       strings.Repeat("a", 64),
		ContentType:  "application/json",
		ExtractedAt:  now,
		ImporterName: "test",
		ReviewStatus: model.ReviewStatusReviewed,
	}
	if err := store.RecordImport(ctx, "import:static-enemies", staticSource, staticArtefact, map[string]any{"fixture": true}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertStaticEnemy(ctx, "import:static-enemies", staticSource, staticArtefact, staticdata.EnemyCandidate{
		Name:       "Caird",
		GroupID:    5033,
		TypeID:     92096,
		Confidence: string(model.ConfidenceProbable),
		Basis:      "confirmed enemy group 5033",
	}); err != nil {
		t.Fatal(err)
	}

	for _, entity := range []model.Entity{
		testEntity(model.EntityTypeSystem, "system:stillness:30001001", "NN0-Y-D5", now),
		testEntity(model.EntityTypeSystem, "system:stillness:30001002", "6RG-Y-T4", now),
		testEntity(model.EntityTypeSystem, "system:stillness:30001003", "L6M-Y-M4", now),
		testEntity(model.EntityTypeCharacter, "character:stillness:victim", "Victim", now),
		testEntity(model.EntityTypeCharacter, "character:stillness:killer", "Killer", now),
		testEntity(model.EntityTypeCharacter, "character:stillness:reporter", "Reporter", now),
		testEntity(model.EntityTypeCharacter, "character:stillness:raw-victim", "Raw Victim", now),
		testEntity(model.EntityTypeCharacter, "character:stillness:raw-reporter", "Raw Reporter", now),
		testEntity(model.EntityTypeCharacter, "character:stillness:graph-victim", "Graph Victim", now),
		testEntity(model.EntityTypeCharacter, "character:stillness:graph-killer", "Graph Killer", now),
		testEntity(model.EntityTypeCharacter, "character:stillness:graph-reporter", "Graph Reporter", now),
	} {
		if err := store.UpsertEntityFacts(ctx, entity, []EntityFactDraft{{
			Key:          "metadata_name",
			Value:        entity.DisplayName,
			SourceID:     source.ID,
			Confidence:   model.ConfidenceVerified,
			Environment:  model.EnvironmentStillness,
			ReviewStatus: model.ReviewStatusReviewed,
		}}); err != nil {
			t.Fatal(err)
		}
	}

	for _, raw := range []model.KillmailRaw{
		{
			ID:                  "killmail:stillness:raw-player",
			Environment:         model.EnvironmentStillness,
			OccurredAt:          now,
			SystemID:            "system:stillness:30001001",
			VictimCharacterID:   "character:stillness:victim",
			KillerCharacterID:   "character:stillness:killer",
			ReporterCharacterID: "character:stillness:reporter",
			SourceIDs:           []string{source.ID},
		},
		{
			ID:                  "killmail:stillness:npc",
			Environment:         model.EnvironmentStillness,
			OccurredAt:          now.Add(time.Second),
			SystemID:            "system:stillness:30001001",
			VictimCharacterID:   "character:stillness:victim",
			KillerTypeID:        "92096",
			ReporterCharacterID: "character:stillness:reporter",
			SourceIDs:           []string{source.ID},
		},
		{
			ID:                  "killmail:stillness:npc-with-killer-character-id",
			Environment:         model.EnvironmentStillness,
			OccurredAt:          now.Add(1500 * time.Millisecond),
			SystemID:            "system:stillness:30001001",
			VictimCharacterID:   "character:stillness:victim",
			KillerCharacterID:   "character:stillness:killer",
			KillerTypeID:        "92096",
			ReporterCharacterID: "character:stillness:reporter",
			SourceIDs:           []string{source.ID},
		},
		{
			ID:          "killmail:stillness:raw-payload",
			Environment: model.EnvironmentStillness,
			OccurredAt:  now.Add(2 * time.Second),
			Raw: map[string]any{
				"event": map[string]any{
					"json": map[string]any{
						"killer_type_id": "92096",
						"victim_id":      map[string]any{"tenant": "stillness", "item_id": "raw-victim"},
						"reported_by_character_id": map[string]any{
							"tenant":  "stillness",
							"item_id": "raw-reporter",
						},
						"solar_system_id": map[string]any{"tenant": "stillness", "item_id": "30001002"},
					},
				},
			},
			SourceIDs: []string{source.ID},
		},
		{
			ID:          "killmail:stillness:graph",
			Environment: model.EnvironmentStillness,
			OccurredAt:  now.Add(3 * time.Second),
			SourceIDs:   []string{source.ID},
		},
		{
			ID:          "killmail:stillness:unresolved",
			Environment: model.EnvironmentStillness,
			OccurredAt:  now.Add(4 * time.Second),
			SourceIDs:   []string{source.ID},
		},
	} {
		if err := store.UpsertKillmail(ctx, raw); err != nil {
			t.Fatal(err)
		}
	}
	killmailEntity := testEntity(model.EntityTypeKillmail, "killmail:stillness:graph", "Graph Killmail", now)
	if err := store.UpsertEntityFacts(ctx, killmailEntity, []EntityFactDraft{{
		Key:          "source",
		Value:        "test",
		SourceID:     source.ID,
		Confidence:   model.ConfidenceVerified,
		Environment:  model.EnvironmentStillness,
		ReviewStatus: model.ReviewStatusReviewed,
	}}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertRelations(ctx, []RelationDraft{
		{SubjectEntityID: "killmail:stillness:graph", Predicate: "occurred_in", ObjectEntityID: "system:stillness:30001003", SourceID: source.ID, Confidence: model.ConfidenceVerified, Environment: model.EnvironmentStillness},
		{SubjectEntityID: "killmail:stillness:graph", Predicate: "victim", ObjectEntityID: "character:stillness:graph-victim", SourceID: source.ID, Confidence: model.ConfidenceVerified, Environment: model.EnvironmentStillness},
		{SubjectEntityID: "killmail:stillness:graph", Predicate: "killer", ObjectEntityID: "character:stillness:graph-killer", SourceID: source.ID, Confidence: model.ConfidenceVerified, Environment: model.EnvironmentStillness},
		{SubjectEntityID: "killmail:stillness:graph", Predicate: "reported_by", ObjectEntityID: "character:stillness:graph-reporter", SourceID: source.ID, Confidence: model.ConfidenceVerified, Environment: model.EnvironmentStillness},
	}); err != nil {
		t.Fatal(err)
	}

	counts, err := store.CountKillmailResolution(ctx, model.EnvironmentStillness)
	if err != nil {
		t.Fatal(err)
	}
	if counts.Total != 6 {
		t.Fatalf("unexpected total %#v", counts)
	}
	if counts.ResolvedSystems != 5 || counts.UnresolvedSystems != 1 {
		t.Fatalf("unexpected system counts %#v", counts)
	}
	if counts.ResolvedVictims != 5 || counts.UnresolvedVictims != 1 {
		t.Fatalf("unexpected victim counts %#v", counts)
	}
	if counts.ResolvedKillers != 5 || counts.UnresolvedKillers != 1 {
		t.Fatalf("unexpected killer counts %#v", counts)
	}
	if counts.ResolvedReporters != 5 || counts.UnresolvedReporters != 1 {
		t.Fatalf("unexpected reporter counts %#v", counts)
	}
	if counts.CharacterKillers != 2 || counts.NPCKillers != 3 {
		t.Fatalf("unexpected killer type counts %#v", counts)
	}
}

func TestPostgresCountRegistryRowsReportsSeededTotalsAndGroups(t *testing.T) {
	ctx := context.Background()
	pool := testPostgresPool(ctx, t)
	defer pool.Close()
	store := PostgresStore{Pool: pool}

	now := time.Date(2026, 6, 25, 13, 0, 0, 0, time.UTC)
	source := model.Source{
		ID:          "source:static-client:stillness:reviewed-enemies",
		Kind:        model.SourceKindStaticClientData,
		Title:       "Reviewed enemies",
		Locator:     "fixture",
		Environment: model.EnvironmentStillness,
		CreatedAt:   now,
	}
	artefact := model.SourceArtefact{
		ID:           "artefact:static-enemies",
		SourceID:     source.ID,
		Environment:  model.EnvironmentStillness,
		SHA256:       strings.Repeat("b", 64),
		ContentType:  "application/json",
		ExtractedAt:  now,
		ImporterName: "test",
		ReviewStatus: model.ReviewStatusReviewed,
	}
	if err := store.RecordImport(ctx, "import:static-enemies", source, artefact, map[string]any{"fixture": true}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertStaticEnemy(ctx, "import:static-enemies", source, artefact, staticdata.EnemyCandidate{
		Name:       "Caird",
		GroupID:    5033,
		TypeID:     92096,
		Confidence: string(model.ConfidenceProbable),
		Basis:      "confirmed enemy group 5033",
	}); err != nil {
		t.Fatal(err)
	}
	character := testEntity(model.EntityTypeCharacter, "character:stillness:2112091476", "Tao", now.Add(time.Minute))
	if err := store.UpsertEntityFacts(ctx, character, []EntityFactDraft{{
		Key:          "character_address",
		Value:        "0xabc",
		SourceID:     source.ID,
		Confidence:   model.ConfidenceVerified,
		Environment:  model.EnvironmentStillness,
		ReviewStatus: model.ReviewStatusReviewed,
	}}); err != nil {
		t.Fatal(err)
	}
	system := testEntity(model.EntityTypeSystem, "system:stillness:30001001", "NN0-Y-D5", now.Add(2*time.Minute))
	if err := store.UpsertEntityFacts(ctx, system, nil); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertRelations(ctx, []RelationDraft{{
		SubjectEntityID: character.ID,
		Predicate:       "owned_by",
		ObjectEntityID:  "enemy:stillness:type:92096",
		SourceID:        source.ID,
		Confidence:      model.ConfidenceVerified,
		Environment:     model.EnvironmentStillness,
	}}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertSuiEvent(ctx, EventRecord{
		ID:          "event:stillness:character:1",
		Kind:        "character.created",
		Environment: model.EnvironmentStillness,
		OccurredAt:  now,
		PackageID:   "0xabc",
		Module:      "character",
		SourceID:    source.ID,
		Payload:     map[string]any{"module": "character"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertSuiObject(ctx, SuiObjectRecord{
		ID:          "object:stillness:player-profile:1",
		ObjectID:    "0xprofile",
		Environment: model.EnvironmentStillness,
		TypeRepr:    "0xabc::character::PlayerProfile",
		PackageID:   "0xabc",
		Module:      "character",
		TypeName:    "PlayerProfile",
		Version:     "1",
		Digest:      "digest",
		SourceID:    source.ID,
		Payload:     map[string]any{"type": "PlayerProfile"},
		ObservedAt:  now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertKillmail(ctx, model.KillmailRaw{
		ID:                "killmail:stillness:fixture:caird",
		Environment:       model.EnvironmentStillness,
		OccurredAt:        now,
		SystemID:          system.ID,
		VictimCharacterID: character.ID,
		KillerTypeID:      "92096",
		SourceIDs:         []string{source.ID},
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveSyncCursor(ctx, CursorStatus{
		ID:              "cursor:sui:sui-testnet:events:stillness:character",
		Source:          "sui:sui-testnet:events:stillness:character",
		Environment:     model.EnvironmentStillness,
		CursorValue:     "cursor",
		CursorKind:      "sui_event",
		EventsProcessed: 1,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateReview(ctx, ReviewDraft{TargetKind: "source_artefact", TargetID: artefact.ID, Notes: "fixture"}); err != nil {
		t.Fatal(err)
	}

	snapshot, err := store.CountRegistryRows(ctx, model.EnvironmentStillness)
	if err != nil {
		t.Fatal(err)
	}
	expected := RegistryRowCounts{
		Sources:         1,
		SourceArtefacts: 1,
		Imports:         1,
		Reviews:         1,
		RawSuiEvents:    1,
		RawSuiObjects:   1,
		Entities:        3,
		Facts:           6,
		Relations:       1,
		Killmails:       1,
		SearchTerms:     3,
		SyncCursors:     1,
		PlayerProfiles:  1,
	}
	if snapshot.Counts != expected {
		t.Fatalf("unexpected row counts\nwant: %#v\n got: %#v", expected, snapshot.Counts)
	}
	if snapshot.EntitiesByType[model.EntityTypeEnemy] != 1 || snapshot.EntitiesByType[model.EntityTypeCharacter] != 1 || snapshot.EntitiesByType[model.EntityTypeSystem] != 1 {
		t.Fatalf("unexpected entities by type %#v", snapshot.EntitiesByType)
	}
	if snapshot.EventsByModule["character"] != 1 {
		t.Fatalf("unexpected events by module %#v", snapshot.EventsByModule)
	}
	if snapshot.SuiObjectsByType["PlayerProfile"] != 1 {
		t.Fatalf("unexpected objects by type %#v", snapshot.SuiObjectsByType)
	}
	if snapshot.RelationsByPredicate["owned_by"] != 1 {
		t.Fatalf("unexpected relations by predicate %#v", snapshot.RelationsByPredicate)
	}
}

func TestPostgresListEventsScansCycle(t *testing.T) {
	ctx := context.Background()
	pool := testPostgresPool(ctx, t)
	defer pool.Close()
	store := PostgresStore{Pool: pool}

	cycle := 6
	if err := store.UpsertSuiEvent(ctx, EventRecord{
		ID:          "event:cycle:6",
		Kind:        "character.created",
		Environment: model.EnvironmentStillness,
		OccurredAt:  time.Date(2026, 6, 25, 9, 0, 0, 0, time.UTC),
		Cycle:       &cycle,
		PackageID:   "0xabc",
		Module:      "character",
		Payload:     map[string]any{"cycle": 6},
	}); err != nil {
		t.Fatal(err)
	}

	page, err := store.ListEvents(ctx, EventQuery{
		Environment: model.EnvironmentStillness,
		Cycle:       &cycle,
		Limit:       10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 {
		t.Fatalf("expected one event, got %#v", page.Items)
	}
	if page.Items[0].Cycle == nil || *page.Items[0].Cycle != 6 {
		t.Fatalf("expected scanned cycle 6, got %#v", page.Items[0].Cycle)
	}
}

func TestPostgresListSuiObjectsFiltersByObservedCycle(t *testing.T) {
	ctx := context.Background()
	pool := testPostgresPool(ctx, t)
	defer pool.Close()
	store := PostgresStore{Pool: pool}

	for _, object := range []SuiObjectRecord{
		{
			ID:          "object:cycle5",
			ObjectID:    "0x5",
			Environment: model.EnvironmentStillness,
			TypeRepr:    "0xabc::character::PlayerProfile",
			ObservedAt:  time.Date(2026, 6, 24, 10, 0, 0, 0, time.UTC),
		},
		{
			ID:          "object:cycle6",
			ObjectID:    "0x6",
			Environment: model.EnvironmentStillness,
			TypeRepr:    "0xabc::character::PlayerProfile",
			ObservedAt:  time.Date(2026, 6, 25, 10, 0, 0, 0, time.UTC),
		},
	} {
		if err := store.UpsertSuiObject(ctx, object); err != nil {
			t.Fatal(err)
		}
	}

	page, err := store.ListSuiObjects(ctx, SuiObjectQuery{
		Environment: model.EnvironmentStillness,
		Cycles:      []int{6},
		Limit:       10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := objectIDs(page.Items); strings.Join(got, ",") != "object:cycle6" {
		t.Fatalf("unexpected object cycle scope %v", got)
	}
}

func TestPostgresListKillmailRawFiltersByCycleWindow(t *testing.T) {
	ctx := context.Background()
	pool := testPostgresPool(ctx, t)
	defer pool.Close()
	store := PostgresStore{Pool: pool}

	source := model.Source{
		ID:          "source:sui:sui-testnet:graphql",
		Kind:        model.SourceKindSuiEvent,
		Title:       "Sui testnet GraphQL",
		Locator:     "fixture",
		Environment: model.EnvironmentStillness,
		CreatedAt:   time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC),
	}
	if err := store.EnsureSource(ctx, source); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []model.KillmailRaw{
		{
			ID:          "killmail:cycle5",
			Environment: model.EnvironmentStillness,
			OccurredAt:  time.Date(2026, 6, 24, 10, 0, 0, 0, time.UTC),
			SourceIDs:   []string{source.ID},
		},
		{
			ID:          "killmail:cycle6",
			Environment: model.EnvironmentStillness,
			OccurredAt:  time.Date(2026, 6, 25, 10, 0, 0, 0, time.UTC),
			SourceIDs:   []string{source.ID},
		},
	} {
		if err := store.UpsertKillmail(ctx, raw); err != nil {
			t.Fatal(err)
		}
	}

	items, _, err := store.ListKillmailRaw(ctx, KillmailQuery{
		Environment: model.EnvironmentStillness,
		Cycles:      []int{6},
		Limit:       10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := rawKillmailIDs(items); strings.Join(got, ",") != "killmail:cycle6" {
		t.Fatalf("unexpected killmail cycle scope %v", got)
	}
}

func rawKillmailIDs(items []model.KillmailRaw) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.ID)
	}
	return out
}

func testPostgresPool(ctx context.Context, t *testing.T) *pgxpool.Pool {
	t.Helper()
	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		databaseURL = "postgres://blackrelay:blackrelay@127.0.0.1:5432/blackrelay_registry?sslmode=disable"
	}
	connectCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	adminPool, err := pgxpool.New(connectCtx, databaseURL)
	if err != nil {
		t.Skipf("PostgreSQL is not available: %v", err)
	}
	if err := adminPool.Ping(connectCtx); err != nil {
		adminPool.Close()
		t.Skipf("PostgreSQL is not available: %v", err)
	}
	schema := fmt.Sprintf("registry_test_%d", time.Now().UnixNano())
	quotedSchema := pgx.Identifier{schema}.Sanitize()
	if _, err := adminPool.Exec(ctx, "CREATE SCHEMA "+quotedSchema); err != nil {
		adminPool.Close()
		t.Fatalf("create test schema: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, _ = adminPool.Exec(cleanupCtx, "DROP SCHEMA IF EXISTS "+quotedSchema+" CASCADE")
		adminPool.Close()
	})

	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatalf("parse test database URL: %v", err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(connectCtx, cfg)
	if err != nil {
		t.Fatalf("connect test schema: %v", err)
	}
	if err := applyTestMigrations(ctx, pool); err != nil {
		pool.Close()
		t.Fatalf("apply test migrations: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func applyTestMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	if _, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
		  version text PRIMARY KEY,
		  applied_at timestamptz NOT NULL DEFAULT now()
		)
	`); err != nil {
		return err
	}
	migrations, err := MigrationsFromDir("../../migrations")
	if err != nil {
		return err
	}
	for _, migration := range migrations {
		if err := applyMigration(ctx, pool, migration); err != nil {
			return err
		}
	}
	return nil
}

func testEntity(entityType model.EntityType, id, name string, updatedAt time.Time) model.Entity {
	return model.Entity{
		ID:          id,
		Slug:        strings.ReplaceAll(id, ":", "-"),
		Type:        entityType,
		Name:        name,
		DisplayName: name,
		Environment: model.EnvironmentStillness,
		UpdatedAt:   updatedAt,
	}
}
