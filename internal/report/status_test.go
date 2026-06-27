package report

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/blackrelay/registry/internal/db"
	"github.com/blackrelay/registry/internal/exporter"
	"github.com/blackrelay/registry/internal/model"
)

func TestBuildIndexerStatusSummarisesCursorsAndExportManifest(t *testing.T) {
	store := db.NewMemoryStore()
	now := time.Date(2026, 6, 27, 3, 0, 0, 0, time.UTC)
	fresh := now.Add(-5 * time.Minute)
	stale := now.Add(-45 * time.Minute)
	if err := store.SaveSyncCursor(context.Background(), db.CursorStatus{
		ID:                   "cursor:sui:sui-testnet:events:original:0xworld:character",
		Source:               "sui:sui-testnet:events:original:0xworld:character",
		Environment:          model.EnvironmentStillness,
		CursorKind:           "sui_event",
		LastSuccessfulIngest: &fresh,
		EventsProcessed:      150,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveSyncCursor(context.Background(), db.CursorStatus{
		ID:                   "cursor:sui:sui-testnet:objects:original:0xworld::character::PlayerProfile",
		Source:               "sui:sui-testnet:objects:original:0xworld::character::PlayerProfile",
		Environment:          model.EnvironmentStillness,
		CursorKind:           "sui_object",
		LastSuccessfulIngest: &stale,
		EventsProcessed:      25,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveSyncCursor(context.Background(), db.CursorStatus{
		ID:               "cursor:sui:sui-testnet:objects:original:0xworld::assembly::Assembly",
		Source:           "sui:sui-testnet:objects:original:0xworld::assembly::Assembly",
		Environment:      model.EnvironmentStillness,
		CursorKind:       "sui_object",
		ErrorCount:       1,
		LastErrorSummary: "sui GraphQL returned errors: Request is outside consistent range",
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveSyncCursor(context.Background(), db.CursorStatus{
		ID:                   "cursor:registry:derive:sui-events:sui-testnet:module:killmail:stillness",
		Source:               "registry:derive:sui-events:sui-testnet:module:killmail",
		Environment:          model.EnvironmentStillness,
		CursorKind:           "sui_event_derivation",
		LastSuccessfulIngest: &fresh,
		EventsProcessed:      100,
	}); err != nil {
		t.Fatal(err)
	}

	manifestPath, bundleID := writeStatusExportManifest(t, now)
	got, err := BuildIndexerStatus(context.Background(), store, IndexerStatusOptions{
		Environment:        model.EnvironmentStillness,
		Now:                func() time.Time { return now },
		StaleAfter:         15 * time.Minute,
		ExportManifestPath: manifestPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.SchemaVersion != "registry.indexer_status.v1" {
		t.Fatalf("unexpected schema version %q", got.SchemaVersion)
	}
	if got.Status != IndexerStatusStale {
		t.Fatalf("stale non-provider-limited cursor should make the status stale, got %#v", got.Status)
	}
	if got.CursorCounts.Total != 4 || got.CursorCounts.Event != 1 || got.CursorCounts.Object != 2 || got.CursorCounts.Derivation != 1 {
		t.Fatalf("unexpected cursor kind counts %#v", got.CursorCounts)
	}
	if got.CursorCounts.RangeBlocked != 1 || got.CursorCounts.Stale != 1 || got.CursorCounts.MissingIngest != 0 {
		t.Fatalf("unexpected cursor health counts %#v", got.CursorCounts)
	}
	if got.CursorCounts.RowsProcessed != 275 {
		t.Fatalf("unexpected rows processed count %#v", got.CursorCounts)
	}
	if got.MaxCursorLagSeconds == nil || *got.MaxCursorLagSeconds != int64((45*time.Minute).Seconds()) {
		t.Fatalf("unexpected max cursor lag %#v", got.MaxCursorLagSeconds)
	}
	stream := findStatusStream(got.Streams, "cursor:sui:sui-testnet:events:original:0xworld:character")
	if stream.PackageID != "0xworld" || stream.ModuleName != "character" || stream.Network != "sui-testnet" {
		t.Fatalf("event stream source was not parsed: %#v", stream)
	}
	objectStream := findStatusStream(got.Streams, "cursor:sui:sui-testnet:objects:original:0xworld::character::PlayerProfile")
	if objectStream.TypeRepr != "0xworld::character::PlayerProfile" || objectStream.TypeName != "PlayerProfile" {
		t.Fatalf("object stream type was not parsed: %#v", objectStream)
	}
	if got.Export == nil {
		t.Fatal("expected export status")
	}
	if got.Export.BundleID != bundleID || got.Export.RowCounts["entities.jsonl"] != 42 || got.Export.RowCounts["killmails.jsonl"] != 7 {
		t.Fatalf("unexpected export status %#v", got.Export)
	}
}

func TestBuildIndexerStatusTreatsRangeBlockedObjectsAsProviderLimitedGaps(t *testing.T) {
	store := db.NewMemoryStore()
	now := time.Date(2026, 6, 27, 3, 0, 0, 0, time.UTC)
	fresh := now.Add(-5 * time.Minute)
	if err := store.SaveSyncCursor(context.Background(), db.CursorStatus{
		ID:                   "cursor:sui:sui-testnet:events:original:0xworld:killmail",
		Source:               "sui:sui-testnet:events:original:0xworld:killmail",
		Environment:          model.EnvironmentStillness,
		CursorKind:           "sui_event",
		LastSuccessfulIngest: &fresh,
		EventsProcessed:      20,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveSyncCursor(context.Background(), db.CursorStatus{
		ID:               "cursor:sui:sui-testnet:objects:original:0xworld::character::PlayerProfile",
		Source:           "sui:sui-testnet:objects:original:0xworld::character::PlayerProfile",
		Environment:      model.EnvironmentStillness,
		CursorKind:       "sui_object",
		ErrorCount:       1,
		LastErrorSummary: "sui GraphQL returned errors: Request is outside consistent range",
	}); err != nil {
		t.Fatal(err)
	}

	got, err := BuildIndexerStatus(context.Background(), store, IndexerStatusOptions{
		Environment: model.EnvironmentStillness,
		Now:         func() time.Time { return now },
		StaleAfter:  15 * time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != IndexerStatusOK {
		t.Fatalf("provider-limited object cursor should not make the indexer unhealthy, got %#v", got.Status)
	}
	if got.CursorCounts.RangeBlocked != 1 || got.CursorCounts.MissingIngest != 0 || got.CursorCounts.Errored != 0 {
		t.Fatalf("range-blocked object cursor was not separated from health failures: %#v", got.CursorCounts)
	}
	if len(got.Reasons) == 0 || got.Reasons[0] != "1 object cursor(s) are limited by the Sui provider range" {
		t.Fatalf("expected provider-limit reason, got %#v", got.Reasons)
	}
	stream := findStatusStream(got.Streams, "cursor:sui:sui-testnet:objects:original:0xworld::character::PlayerProfile")
	if !stream.ProviderRangeBlocked || stream.Status != model.CoverageStatusRangeBlocked {
		t.Fatalf("range-blocked stream should stay visible as provider-limited evidence: %#v", stream)
	}
}

func TestBuildIndexerStatusReportsDegradedWhenNoCursorsExist(t *testing.T) {
	got, err := BuildIndexerStatus(context.Background(), db.NewMemoryStore(), IndexerStatusOptions{
		Environment: model.EnvironmentStillness,
		Now:         func() time.Time { return time.Date(2026, 6, 27, 4, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != IndexerStatusDegraded {
		t.Fatalf("missing cursors should be degraded, got %#v", got.Status)
	}
	if got.CursorCounts.Total != 0 {
		t.Fatalf("unexpected cursor count %#v", got.CursorCounts)
	}
}

func writeStatusExportManifest(t *testing.T, generatedAt time.Time) (string, string) {
	t.Helper()
	dir := t.TempDir()
	manifest := exporter.ExportManifest{
		SchemaVersion:   "registry.export_manifest.v1",
		Registry:        "black-relay-registry",
		APIVersion:      "v1",
		GeneratedAt:     generatedAt,
		CycleScope:      "current",
		Cycles:          []int{6},
		IncludeUncycled: true,
		Files: []exporter.ExportFile{
			{Path: "entities.jsonl", ContentType: "application/x-ndjson", SHA256: "a", SizeBytes: 10, RowCount: 42},
			{Path: "killmails.jsonl", ContentType: "application/x-ndjson", SHA256: "b", SizeBytes: 20, RowCount: 7},
		},
		HighWaterMarks: map[string]exporter.ExportHighWaterMark{
			"entities":  {RowCount: 42, Complete: true},
			"killmails": {RowCount: 7, Complete: true},
		},
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "manifest.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	return path, hex.EncodeToString(sum[:])
}

func findStatusStream(streams []IndexerStreamStatus, id string) IndexerStreamStatus {
	for _, stream := range streams {
		if stream.CursorID == id {
			return stream
		}
	}
	return IndexerStreamStatus{}
}
