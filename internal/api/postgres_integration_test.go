package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/blackrelay/registry/internal/db"
	"github.com/blackrelay/registry/internal/model"
	"github.com/blackrelay/registry/internal/staticdata"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresAPIServesCurrentFiltersAndDerivedState(t *testing.T) {
	ctx := context.Background()
	pool := testPostgresAPIPool(ctx, t)
	defer pool.Close()
	store := db.PostgresStore{Pool: pool}
	now := time.Date(2026, 6, 25, 14, 0, 0, 0, time.UTC)
	source := testAPISource(now)
	if err := store.EnsureSource(ctx, source); err != nil {
		t.Fatal(err)
	}
	entities := []model.Entity{
		testAPIEntity(model.EntityTypeCharacter, "character:stillness:tao", "Tao", now.Add(9*time.Minute)),
		testAPIEntity(model.EntityTypeCharacter, "character:stillness:other", "Other Pilot", now.Add(8*time.Minute)),
		testAPIEntity(model.EntityTypeTribe, "tribe:stillness:black-relay", "Black Relay", now.Add(7*time.Minute)),
		testAPIEntity(model.EntityTypeTribe, "tribe:stillness:other", "Other Tribe", now.Add(6*time.Minute)),
		testAPIEntity(model.EntityTypeAssembly, "assembly:stillness:100", "Assembly 100", now.Add(5*time.Minute)),
		testAPIEntity(model.EntityTypeResourceObject, "resource_object:stillness:owner-cap:0xcap", "Owner capability 0xcap", now.Add(4500*time.Millisecond)),
		testAPIEntity(model.EntityTypeResourceObject, "resource_object:stillness:location-hash:loc-1", "Location hash loc-1", now.Add(4400*time.Millisecond)),
		testAPIEntity(model.EntityTypeSystem, "system:stillness:30001001", "NN0-Y-D5", now.Add(4*time.Minute)),
		testAPIEntity(model.EntityTypeSystem, "system:stillness:30001002", "6RG-Y-T4", now.Add(3*time.Minute)),
		testAPIEntity(model.EntityTypeRoute, "route:stillness:nn0-to-6rg", "NN0-Y-D5 to 6RG-Y-T4", now.Add(2*time.Minute)),
		testAPIEntity(model.EntityTypeKillmail, "killmail:stillness:100", "Killmail 100", now.Add(time.Minute)),
	}
	for _, entity := range entities {
		facts := []db.EntityFactDraft{{
			Key:          "metadata_name",
			Value:        entity.DisplayName,
			SourceID:     source.ID,
			Confidence:   model.ConfidenceVerified,
			Environment:  model.EnvironmentStillness,
			ReviewStatus: model.ReviewStatusReviewed,
		}}
		switch entity.ID {
		case "character:stillness:tao":
			facts = append(facts,
				db.EntityFactDraft{Key: "metadata_description", Value: "Public pilot profile", SourceID: source.ID, Confidence: model.ConfidenceVerified, Environment: model.EnvironmentStillness, ReviewStatus: model.ReviewStatusReviewed},
				db.EntityFactDraft{Key: "metadata_url", Value: "https://example.invalid/characters/tao", SourceID: source.ID, Confidence: model.ConfidenceVerified, Environment: model.EnvironmentStillness, ReviewStatus: model.ReviewStatusReviewed},
			)
		case "tribe:stillness:black-relay":
			facts = append(facts,
				db.EntityFactDraft{Key: "tag", Value: "BR", SourceID: source.ID, Confidence: model.ConfidenceReported, Environment: model.EnvironmentStillness, ReviewStatus: model.ReviewStatusReviewed},
				db.EntityFactDraft{Key: "aliases", Value: []string{"Relay"}, SourceID: source.ID, Confidence: model.ConfidenceReported, Environment: model.EnvironmentStillness, ReviewStatus: model.ReviewStatusReviewed},
				db.EntityFactDraft{Key: "description", Value: "Reviewed public tribe profile", SourceID: source.ID, Confidence: model.ConfidenceReported, Environment: model.EnvironmentStillness, ReviewStatus: model.ReviewStatusReviewed},
				db.EntityFactDraft{Key: "url", Value: "https://example.invalid/tribes/black-relay", SourceID: source.ID, Confidence: model.ConfidenceReported, Environment: model.EnvironmentStillness, ReviewStatus: model.ReviewStatusReviewed},
			)
		case "assembly:stillness:100":
			facts = append(facts,
				db.EntityFactDraft{Key: "metadata_description", Value: "Public assembly profile", SourceID: source.ID, Confidence: model.ConfidenceVerified, Environment: model.EnvironmentStillness, ReviewStatus: model.ReviewStatusReviewed},
				db.EntityFactDraft{Key: "metadata_url", Value: "https://example.invalid/assemblies/100", SourceID: source.ID, Confidence: model.ConfidenceVerified, Environment: model.EnvironmentStillness, ReviewStatus: model.ReviewStatusReviewed},
				db.EntityFactDraft{Key: "owner_cap_id", Value: "0xcap", SourceID: source.ID, Confidence: model.ConfidenceVerified, Environment: model.EnvironmentStillness, ReviewStatus: model.ReviewStatusReviewed},
				db.EntityFactDraft{Key: "location_hash", Value: "loc-1", SourceID: source.ID, Confidence: model.ConfidenceVerified, Environment: model.EnvironmentStillness, ReviewStatus: model.ReviewStatusReviewed},
			)
		}
		if err := store.UpsertEntityFacts(ctx, entity, facts); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.UpsertRelations(ctx, []db.RelationDraft{
		{SubjectEntityID: "character:stillness:tao", Predicate: "belongs_to", ObjectEntityID: "tribe:stillness:black-relay", SourceID: source.ID, Confidence: model.ConfidenceVerified, Environment: model.EnvironmentStillness},
		{SubjectEntityID: "character:stillness:other", Predicate: "belongs_to", ObjectEntityID: "tribe:stillness:other", SourceID: source.ID, Confidence: model.ConfidenceVerified, Environment: model.EnvironmentStillness},
		{SubjectEntityID: "assembly:stillness:100", Predicate: "owned_by", ObjectEntityID: "character:stillness:tao", SourceID: source.ID, Confidence: model.ConfidenceVerified, Environment: model.EnvironmentStillness},
		{SubjectEntityID: "assembly:stillness:100", Predicate: "located_in", ObjectEntityID: "system:stillness:30001001", SourceID: source.ID, Confidence: model.ConfidenceProbable, Environment: model.EnvironmentStillness},
		{SubjectEntityID: "assembly:stillness:100", Predicate: "has_owner_cap", ObjectEntityID: "resource_object:stillness:owner-cap:0xcap", SourceID: source.ID, Confidence: model.ConfidenceVerified, Environment: model.EnvironmentStillness},
		{SubjectEntityID: "assembly:stillness:100", Predicate: "has_location_hash", ObjectEntityID: "resource_object:stillness:location-hash:loc-1", SourceID: source.ID, Confidence: model.ConfidenceVerified, Environment: model.EnvironmentStillness},
		{SubjectEntityID: "system:stillness:30001001", Predicate: "links_to", ObjectEntityID: "system:stillness:30001002", SourceID: source.ID, Confidence: model.ConfidenceVerified, Environment: model.EnvironmentStillness},
		{SubjectEntityID: "route:stillness:nn0-to-6rg", Predicate: "observed_between", ObjectEntityID: "system:stillness:30001001", SourceID: source.ID, Confidence: model.ConfidenceProbable, Environment: model.EnvironmentStillness},
		{SubjectEntityID: "killmail:stillness:100", Predicate: "victim", ObjectEntityID: "character:stillness:tao", SourceID: source.ID, Confidence: model.ConfidenceVerified, Environment: model.EnvironmentStillness},
		{SubjectEntityID: "killmail:stillness:100", Predicate: "reported_by", ObjectEntityID: "character:stillness:other", SourceID: source.ID, Confidence: model.ConfidenceVerified, Environment: model.EnvironmentStillness},
		{SubjectEntityID: "killmail:stillness:100", Predicate: "occurred_in", ObjectEntityID: "system:stillness:30001001", SourceID: source.ID, Confidence: model.ConfidenceVerified, Environment: model.EnvironmentStillness},
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertKillmail(ctx, model.KillmailRaw{
		ID:                "killmail:stillness:100",
		Environment:       model.EnvironmentStillness,
		OccurredAt:        now,
		SystemID:          "system:stillness:30001001",
		VictimCharacterID: "character:stillness:tao",
		KillerCharacterID: "character:stillness:other",
		SourceIDs:         []string{source.ID},
	}); err != nil {
		t.Fatal(err)
	}
	handler := Server{Store: store}.Handler()

	characters := getAPIData[model.CurrentEntity](t, handler, "/v1/current/characters?environment=stillness&tribe=tribe:stillness:black-relay&q=tao&has_activity=true")
	if len(characters) != 1 || characters[0].Entity.ID != "character:stillness:tao" {
		t.Fatalf("character filters returned wrong rows: %#v", characters)
	}
	if characters[0].Derived == nil || characters[0].Derived.Tribe == nil || characters[0].Derived.Tribe.DisplayName != "Black Relay" {
		t.Fatalf("character derived tribe state was not populated: %#v", characters[0].Derived)
	}
	if characters[0].Derived.PublicActivityCount == 0 {
		t.Fatalf("character public activity summary was not populated: %#v", characters[0].Derived)
	}
	if characters[0].Derived.Profile == nil ||
		characters[0].Derived.Profile.MetadataDescription != "Public pilot profile" ||
		characters[0].Derived.Profile.MetadataURL != "https://example.invalid/characters/tao" {
		t.Fatalf("character current profile was not populated: %#v", characters[0].Derived)
	}
	knownCharacters := getAPIData[model.CurrentEntity](t, handler, "/v1/current/characters?environment=stillness&profile=known&has_tribe=true")
	if len(knownCharacters) != 2 {
		t.Fatalf("known/profiled tribe member filter returned wrong rows: %#v", knownCharacters)
	}
	placeholderCharacters := getAPIData[model.CurrentEntity](t, handler, "/v1/current/characters?environment=stillness&profile=placeholder")
	if len(placeholderCharacters) != 0 {
		t.Fatalf("profile=placeholder should not return named PostgreSQL characters: %#v", placeholderCharacters)
	}

	tribes := getAPIData[model.CurrentEntity](t, handler, "/v1/current/tribes?environment=stillness")
	if len(tribes) < 1 || tribes[0].Entity.ID != "tribe:stillness:black-relay" {
		t.Fatalf("reviewed tribe should be returned before other tribe rows: %#v", tribes)
	}
	if tribes[0].Derived == nil || tribes[0].Derived.Profile == nil {
		t.Fatalf("tribe current profile was not populated: %#v", tribes[0].Derived)
	}
	if tribes[0].Derived.Profile.Tag != "BR" ||
		tribes[0].Derived.Profile.Description != "Reviewed public tribe profile" ||
		tribes[0].Derived.Profile.URL != "https://example.invalid/tribes/black-relay" ||
		len(tribes[0].Derived.Profile.Aliases) != 1 ||
		tribes[0].Derived.Profile.Aliases[0] != "Relay" {
		t.Fatalf("tribe current profile had wrong fields: %#v", tribes[0].Derived.Profile)
	}

	assemblies := getAPIData[model.CurrentEntity](t, handler, "/v1/current/assemblies?environment=stillness&owner=character:stillness:tao&system=system:stillness:30001001")
	if len(assemblies) != 1 || assemblies[0].Entity.ID != "assembly:stillness:100" {
		t.Fatalf("assembly filters returned wrong rows: %#v", assemblies)
	}
	if assemblies[0].Derived == nil || assemblies[0].Derived.Owner == nil || assemblies[0].Derived.System == nil {
		t.Fatalf("assembly owner/system derived state was not populated: %#v", assemblies[0].Derived)
	}
	if assemblies[0].Derived.Profile == nil ||
		assemblies[0].Derived.Profile.MetadataDescription != "Public assembly profile" ||
		assemblies[0].Derived.Profile.MetadataURL != "https://example.invalid/assemblies/100" {
		t.Fatalf("assembly current profile was not populated: %#v", assemblies[0].Derived)
	}
	withEvidence := getAPIData[model.CurrentEntity](t, handler, "/v1/current/assemblies?environment=stillness&has_owner_cap=true&has_location_hash=true&has_resolved_owner=true&has_resolved_system=true")
	if len(withEvidence) != 1 || withEvidence[0].Entity.ID != "assembly:stillness:100" {
		t.Fatalf("evidence boolean filters returned wrong PostgreSQL assembly rows: %#v", withEvidence)
	}
	withoutEvidence := getAPIData[model.CurrentEntity](t, handler, "/v1/current/assemblies?environment=stillness&has_owner_cap=false")
	if len(withoutEvidence) != 0 {
		t.Fatalf("has_owner_cap=false should not return evidence-backed PostgreSQL assemblies: %#v", withoutEvidence)
	}

	systems := getAPIData[model.CurrentEntity](t, handler, "/v1/current/systems?environment=stillness&connected_to=system:stillness:30001002&has_activity=true")
	if len(systems) != 1 || systems[0].Entity.ID != "system:stillness:30001001" {
		t.Fatalf("system filters returned wrong rows: %#v", systems)
	}
	if systems[0].Derived == nil || systems[0].Derived.ConnectedSystemCount == 0 || systems[0].Derived.KillmailCount == 0 {
		t.Fatalf("system connection/activity summary was not populated: %#v", systems[0].Derived)
	}

	edges := getAPIData[model.CurrentRelation](t, handler, "/v1/current/route-edges?environment=stillness&system=system:stillness:30001001")
	if len(edges) != 2 {
		t.Fatalf("route-edge system filter returned wrong rows: %#v", edges)
	}
}

func TestPostgresAPIKillmailFilters(t *testing.T) {
	ctx := context.Background()
	pool := testPostgresAPIPool(ctx, t)
	defer pool.Close()
	store := db.PostgresStore{Pool: pool}
	now := time.Date(2026, 6, 25, 15, 0, 0, 0, time.UTC)
	source := testAPISource(now)
	if err := store.EnsureSource(ctx, source); err != nil {
		t.Fatal(err)
	}
	staticSource := model.Source{ID: "source:static-client:stillness:reviewed-enemies", Kind: model.SourceKindStaticClientData, Title: "Reviewed enemies", Locator: "fixture", Environment: model.EnvironmentStillness, CreatedAt: now}
	staticArtefact := model.SourceArtefact{ID: "artefact:static-enemies", SourceID: staticSource.ID, Environment: model.EnvironmentStillness, SHA256: strings.Repeat("c", 64), ContentType: "application/json", ExtractedAt: now, ImporterName: "test", ReviewStatus: model.ReviewStatusReviewed}
	if err := store.RecordImport(ctx, "import:static-enemies", staticSource, staticArtefact, map[string]any{"fixture": true}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertStaticEnemy(ctx, "import:static-enemies", staticSource, staticArtefact, staticdata.EnemyCandidate{Name: "Caird", GroupID: 5033, TypeID: 92096, Confidence: string(model.ConfidenceProbable), Basis: "confirmed enemy group 5033"}); err != nil {
		t.Fatal(err)
	}
	for _, entity := range []model.Entity{
		testAPIEntity(model.EntityTypeSystem, "system:stillness:30001001", "NN0-Y-D5", now),
		testAPIEntity(model.EntityTypeSystem, "system:stillness:30001002", "6RG-Y-T4", now),
		testAPIEntity(model.EntityTypeCharacter, "character:stillness:victim", "Victim", now),
		testAPIEntity(model.EntityTypeCharacter, "character:stillness:killer", "Killer", now),
	} {
		if err := store.UpsertEntityFacts(ctx, entity, []db.EntityFactDraft{{Key: "metadata_name", Value: entity.DisplayName, SourceID: source.ID, Confidence: model.ConfidenceVerified, Environment: model.EnvironmentStillness, ReviewStatus: model.ReviewStatusReviewed}}); err != nil {
			t.Fatal(err)
		}
	}
	for _, raw := range []model.KillmailRaw{
		{ID: "killmail:stillness:npc", Environment: model.EnvironmentStillness, OccurredAt: now, SystemID: "system:stillness:30001001", VictimCharacterID: "character:stillness:victim", KillerTypeID: "92096", SourceIDs: []string{source.ID}},
		{ID: "killmail:stillness:player", Environment: model.EnvironmentStillness, OccurredAt: now.Add(-time.Hour), SystemID: "system:stillness:30001002", VictimCharacterID: "character:stillness:victim", KillerCharacterID: "character:stillness:killer", SourceIDs: []string{source.ID}},
		{ID: "killmail:stillness:fixture:caird", Environment: model.EnvironmentStillness, OccurredAt: now.Add(-2 * time.Hour), SystemID: "system:stillness:30001001", VictimCharacterID: "character:stillness:victim", KillerTypeID: "92096", SourceIDs: []string{"source:fixture:killmail"}},
	} {
		if err := store.UpsertKillmail(ctx, raw); err != nil {
			t.Fatal(err)
		}
	}
	handler := Server{Store: store}.Handler()

	npcKills := getAPIData[model.SemanticKillmail](t, handler, "/v1/killmails?environment=stillness&system=system:stillness:30001001&killer_type_id=92096&npc=true&from=2026-06-25T14:30:00Z&to=2026-06-25T15:30:00Z")
	if len(npcKills) != 1 || npcKills[0].ID != "killmail:stillness:npc" || npcKills[0].Killer.DisplayName != "Caird [NPC]" {
		t.Fatalf("killmail filters did not isolate NPC killmail: %#v", npcKills)
	}
	playerKills := getAPIData[model.SemanticKillmail](t, handler, "/v1/killmails?environment=stillness&killer=character:stillness:killer&npc=false")
	if len(playerKills) != 1 || playerKills[0].ID != "killmail:stillness:player" {
		t.Fatalf("killmail killer/npc=false filters did not isolate player killmail: %#v", playerKills)
	}
	liveKills := getAPIData[model.SemanticKillmail](t, handler, "/v1/killmails?environment=stillness&exclude_fixtures=true")
	for _, item := range liveKills {
		if item.ID == "killmail:stillness:fixture:caird" {
			t.Fatalf("fixture killmail leaked into fixture-excluded API results: %#v", liveKills)
		}
	}
	if len(liveKills) != 2 {
		t.Fatalf("expected only two non-fixture killmails, got %#v", liveKills)
	}
}

func testPostgresAPIPool(ctx context.Context, t *testing.T) *pgxpool.Pool {
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
	schema := fmt.Sprintf("registry_api_test_%d", time.Now().UnixNano())
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
	if err := applyPostgresAPIMigrations(ctx, pool); err != nil {
		pool.Close()
		t.Fatalf("apply test migrations: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func applyPostgresAPIMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	migrations, err := db.MigrationsFromDir("../../migrations")
	if err != nil {
		return err
	}
	for _, migration := range migrations {
		if _, err := pool.Exec(ctx, migration.SQL); err != nil {
			return fmt.Errorf("apply migration %s: %w", migration.Version, err)
		}
	}
	return nil
}

func testAPISource(now time.Time) model.Source {
	return model.Source{ID: "source:sui:sui-testnet:graphql", Kind: model.SourceKindSuiEvent, Title: "Sui testnet GraphQL", Locator: "fixture", Environment: model.EnvironmentStillness, CreatedAt: now}
}

func testAPIEntity(entityType model.EntityType, id, name string, updatedAt time.Time) model.Entity {
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

func getAPIData[T any](t *testing.T, handler http.Handler, path string) []T {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("%s returned %d body %s", path, res.Code, res.Body.String())
	}
	var body struct {
		Data []T `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return body.Data
}
