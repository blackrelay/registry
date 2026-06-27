package report

import (
	"context"
	"testing"
	"time"

	"github.com/blackrelay/registry/internal/db"
	"github.com/blackrelay/registry/internal/model"
	"github.com/blackrelay/registry/internal/staticdata"
)

func TestBuildCountsIndexedDataAndSemanticKillmailResolution(t *testing.T) {
	store := db.NewMemoryStore()
	now := time.Date(2026, 6, 25, 10, 30, 0, 0, time.UTC)
	source := model.Source{
		ID:          "source:static-client:stillness:reviewed-enemies",
		Kind:        model.SourceKindStaticClientData,
		Title:       "Reviewed enemies",
		Locator:     "fixture",
		Environment: model.EnvironmentStillness,
		CreatedAt:   now,
	}
	artefact := model.SourceArtefact{
		ID:           "artefact:fixture",
		SourceID:     source.ID,
		Environment:  model.EnvironmentStillness,
		SHA256:       "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		ContentType:  "application/json",
		ExtractedAt:  now,
		ImporterName: "test",
	}
	if err := store.RecordImport(context.Background(), "import:fixture", source, artefact, nil); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertStaticEnemy(context.Background(), "import:fixture", source, artefact, staticdata.EnemyCandidate{
		Name:       "Caird",
		GroupID:    5033,
		TypeID:     92096,
		Confidence: string(model.ConfidenceProbable),
		Basis:      "confirmed enemy group 5033",
	}); err != nil {
		t.Fatal(err)
	}
	character := model.Entity{
		ID:          "character:stillness:2112091476",
		Slug:        "character-2112091476-stillness",
		Type:        model.EntityTypeCharacter,
		Name:        "Tao",
		DisplayName: "Tao",
		Environment: model.EnvironmentStillness,
		UpdatedAt:   now,
	}
	system := model.Entity{
		ID:          "system:stillness:30001001",
		Slug:        "system-30001001-stillness",
		Type:        model.EntityTypeSystem,
		Name:        "NN0-Y-D5",
		DisplayName: "NN0-Y-D5",
		Environment: model.EnvironmentStillness,
		UpdatedAt:   now,
	}
	if err := store.UpsertEntityFacts(context.Background(), character, nil); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertEntityFacts(context.Background(), system, nil); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertSuiEvent(context.Background(), db.EventRecord{
		ID:          "event:killmail",
		Kind:        "killmail.created",
		Environment: model.EnvironmentStillness,
		OccurredAt:  now,
		Module:      "killmail",
		SourceID:    "source:sui",
		Payload:     map[string]any{"json": map[string]any{}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertSuiObject(context.Background(), db.SuiObjectRecord{
		ID:          "object:profile",
		ObjectID:    "0xprofile",
		Environment: model.EnvironmentStillness,
		Module:      "character",
		TypeName:    "PlayerProfile",
		TypeRepr:    "0xabc::character::PlayerProfile",
		ObservedAt:  now,
		Payload:     map[string]any{"json": map[string]any{}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertKillmail(context.Background(), model.KillmailRaw{
		ID:                "killmail:stillness:resolved",
		Environment:       model.EnvironmentStillness,
		OccurredAt:        now,
		SystemID:          system.ID,
		VictimCharacterID: character.ID,
		KillerTypeID:      "92096",
		ReporterName:      "Reporter Without Entity",
		SourceIDs:         []string{"source:sui"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertKillmail(context.Background(), model.KillmailRaw{
		ID:          "killmail:stillness:unresolved",
		Environment: model.EnvironmentStillness,
		OccurredAt:  now.Add(-time.Minute),
	}); err != nil {
		t.Fatal(err)
	}

	got, err := Build(context.Background(), store, Options{
		Environment: model.EnvironmentStillness,
		Now:         func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Counts.RawSuiEvents != 1 || got.Counts.RawSuiObjects != 1 || got.Counts.Killmails != 2 {
		t.Fatalf("unexpected raw counts %#v", got.Counts)
	}
	if got.Counts.PlayerProfiles != 1 {
		t.Fatalf("expected player profile count, got %#v", got.Counts)
	}
	if got.EntitiesByType[model.EntityTypeCharacter] != 1 || got.EntitiesByType[model.EntityTypeEnemy] != 1 || got.EntitiesByType[model.EntityTypeSystem] != 1 {
		t.Fatalf("unexpected entity type counts %#v", got.EntitiesByType)
	}
	if got.EventsByModule["killmail"] != 1 {
		t.Fatalf("unexpected event module counts %#v", got.EventsByModule)
	}
	if got.SuiObjectsByType["PlayerProfile"] != 1 {
		t.Fatalf("unexpected object type counts %#v", got.SuiObjectsByType)
	}
	if got.Killmails.Total != 2 || got.Killmails.NPCKillers != 1 || got.Killmails.UnresolvedKillers != 1 {
		t.Fatalf("unexpected killmail resolution counts %#v", got.Killmails)
	}
	if got.Killmails.UnresolvedSystems != 1 || got.Killmails.UnresolvedVictims != 1 || got.Killmails.UnresolvedReporters != 2 {
		t.Fatalf("unexpected unresolved actor counts %#v", got.Killmails)
	}
	if !hasSourceGap(got.SourceGaps, "source-gap:stillness:unresolved-killmail-actors") {
		t.Fatalf("expected unresolved killmail source gap in report: %#v", got.SourceGaps)
	}
	if !hasSourceGap(got.SourceGaps, "source-gap:stillness:static-client-full-table-decoder") {
		t.Fatalf("expected native static-client decoder source gap in report: %#v", got.SourceGaps)
	}
}

func TestBuildUsesFastKillmailResolutionCounterWhenAvailable(t *testing.T) {
	store := &fastReportStore{
		MemoryStore: db.NewMemoryStore(),
		counts: db.KillmailResolutionCounts{
			Total:             42,
			ResolvedKillers:   41,
			UnresolvedKillers: 1,
			CharacterKillers:  40,
			NPCKillers:        1,
		},
	}
	got, err := Build(context.Background(), store, Options{
		Environment: model.EnvironmentStillness,
		Now:         func() time.Time { return time.Date(2026, 6, 25, 11, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Killmails.Total != 42 || got.Killmails.CharacterKillers != 40 || got.Killmails.NPCKillers != 1 {
		t.Fatalf("Build did not use the fast killmail resolution counts: %#v", got.Killmails)
	}
	if store.listKillmailRawCalls != 0 {
		t.Fatalf("fast report store should not page raw killmails, called %d time(s)", store.listKillmailRawCalls)
	}
}

func TestBuildCanExcludeFixtureKillmailsFromSemanticCounts(t *testing.T) {
	store := db.NewMemoryStore()
	now := time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)
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
		SHA256:       "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
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
		ID:          "killmail:stillness:live",
		Environment: model.EnvironmentStillness,
		OccurredAt:  now.Add(-time.Minute),
		SourceIDs:   []string{"source:sui:sui-testnet:graphql"},
	}); err != nil {
		t.Fatal(err)
	}

	got, err := Build(context.Background(), store, Options{
		Environment:     model.EnvironmentStillness,
		ExcludeFixtures: true,
		Now:             func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	if !got.ExcludeFixtures {
		t.Fatalf("report did not echo fixture exclusion: %#v", got)
	}
	if got.Counts.Killmails != 2 {
		t.Fatalf("raw row counts should remain unfiltered evidence totals, got %#v", got.Counts)
	}
	if got.Killmails.Total != 1 || got.Killmails.NPCKillers != 0 || got.Killmails.UnresolvedKillers != 1 {
		t.Fatalf("fixture killmail was not excluded from semantic counts: %#v", got.Killmails)
	}
}

type fastReportStore struct {
	*db.MemoryStore
	counts               db.KillmailResolutionCounts
	listKillmailRawCalls int
}

func (s *fastReportStore) CountKillmailResolution(ctx context.Context, environment model.Environment) (db.KillmailResolutionCounts, error) {
	_ = ctx
	_ = environment
	return s.counts, nil
}

func (s *fastReportStore) ListKillmailRaw(ctx context.Context, query db.KillmailQuery) ([]model.KillmailRaw, string, error) {
	s.listKillmailRawCalls++
	return s.MemoryStore.ListKillmailRaw(ctx, query)
}

func hasSourceGap(gaps []model.SourceGap, id string) bool {
	for _, gap := range gaps {
		if gap.ID == id {
			return true
		}
	}
	return false
}
