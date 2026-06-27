package exporter

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/blackrelay/registry/internal/db"
	"github.com/blackrelay/registry/internal/model"
)

func TestWritePublicExportWritesCatalogAndJSONLFiles(t *testing.T) {
	store := db.NewMemoryStore()
	now := time.Date(2026, 6, 24, 10, 0, 0, 0, time.UTC)
	store.Sources["source:fixture"] = model.Source{ID: "source:fixture", Kind: model.SourceKindSuiEvent, Title: "Fixture", Locator: "fixture", Environment: model.EnvironmentStillness, CreatedAt: now}
	store.Entities["enemy:stillness:type:92096"] = model.Entity{ID: "enemy:stillness:type:92096", Slug: "enemy-caird-92096-stillness", Type: model.EntityTypeEnemy, Name: "Caird", DisplayName: "Caird [NPC]", Environment: model.EnvironmentStillness, UpdatedAt: now}
	if err := store.UpsertKillmail(context.Background(), model.KillmailRaw{ID: "killmail:stillness:310", Environment: model.EnvironmentStillness, OccurredAt: now, SourceIDs: []string{"source:fixture"}}); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	result, err := WritePublicExport(context.Background(), store, dir, ExportOptions{Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	if result.EntityCount != 1 || result.KillmailCount != 1 || result.SourceCount != 1 {
		t.Fatalf("unexpected result %#v", result)
	}
	for _, name := range []string{
		"catalog.json",
		"entities.jsonl",
		"killmails.jsonl",
		"sources.jsonl",
		"facts.jsonl",
		"relations.jsonl",
		"entity_sources.jsonl",
		"source_artefacts.jsonl",
		"current_entities.jsonl",
		"current_relations.jsonl",
		"ops_freshness.json",
		"ops_cursors.json",
		"ops_sui_coverage.json",
		"ops_source_gaps.json",
	} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("missing %s: %v", name, err)
		}
	}
	data, err := os.ReadFile(filepath.Join(dir, "entities.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "Caird") {
		t.Fatalf("entity export did not include entity data: %s", data)
	}
}

func TestWritePublicExportWritesAuditableManifest(t *testing.T) {
	store := db.NewMemoryStore()
	now := time.Date(2026, 6, 24, 10, 0, 0, 0, time.UTC)
	store.Sources["source:fixture"] = model.Source{ID: "source:fixture", Kind: model.SourceKindSuiEvent, Title: "Fixture", Locator: "fixture", Environment: model.EnvironmentStillness, CreatedAt: now}
	store.Entities["enemy:stillness:type:92096"] = model.Entity{ID: "enemy:stillness:type:92096", Slug: "enemy-caird-92096-stillness", Type: model.EntityTypeEnemy, Name: "Caird", DisplayName: "Caird [NPC]", Environment: model.EnvironmentStillness, UpdatedAt: now}
	if err := store.UpsertKillmail(context.Background(), model.KillmailRaw{ID: "killmail:stillness:310", Environment: model.EnvironmentStillness, OccurredAt: now, SourceIDs: []string{"source:fixture"}}); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	result, err := WritePublicExport(context.Background(), store, dir, ExportOptions{Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	if !containsFile(result.Files, "manifest.json") {
		t.Fatalf("catalog files did not include manifest.json: %#v", result.Files)
	}
	var manifest ExportManifest
	readJSONFile(t, filepath.Join(dir, "manifest.json"), &manifest)
	if manifest.SchemaVersion != "registry.export_manifest.v1" {
		t.Fatalf("unexpected manifest schema version %q", manifest.SchemaVersion)
	}
	if manifest.GeneratedAt != now {
		t.Fatalf("unexpected manifest generatedAt %s", manifest.GeneratedAt)
	}
	if manifest.Database.Engine != "memory" {
		t.Fatalf("expected memory database identity, got %#v", manifest.Database)
	}
	entities := manifestFile(t, manifest, "entities.jsonl")
	if entities.RowCount != 1 {
		t.Fatalf("expected entity row count 1, got %#v", entities)
	}
	if entities.SHA256 != fileSHA256(t, filepath.Join(dir, "entities.jsonl")) {
		t.Fatalf("entity checksum mismatch: %#v", entities)
	}
	if entities.SizeBytes <= 0 {
		t.Fatalf("entity size was not recorded: %#v", entities)
	}
	if manifest.HighWaterMarks["entities"].RowCount != 1 {
		t.Fatalf("entity high-water mark missing row count: %#v", manifest.HighWaterMarks["entities"])
	}
	if manifest.HighWaterMarks["entities"].FirstID != "enemy:stillness:type:92096" || manifest.HighWaterMarks["entities"].LastID != "enemy:stillness:type:92096" {
		t.Fatalf("entity high-water mark did not record exported ids: %#v", manifest.HighWaterMarks["entities"])
	}
}

func TestWritePublicExportRecordsConfiguredInstanceIdentity(t *testing.T) {
	store := db.NewMemoryStore()
	dir := t.TempDir()
	result, err := WritePublicExport(context.Background(), store, dir, ExportOptions{
		RegistryID: "frontier-community-registry",
		APIVersion: "v1-community",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Registry != "frontier-community-registry" || result.APIVersion != "v1-community" {
		t.Fatalf("catalog result did not record configured identity: %#v", result)
	}
	var catalog ExportResult
	readJSONFile(t, filepath.Join(dir, "catalog.json"), &catalog)
	if catalog.Registry != "frontier-community-registry" || catalog.APIVersion != "v1-community" {
		t.Fatalf("catalog file did not record configured identity: %#v", catalog)
	}
	var manifest ExportManifest
	readJSONFile(t, filepath.Join(dir, "manifest.json"), &manifest)
	if manifest.Registry != "frontier-community-registry" || manifest.APIVersion != "v1-community" {
		t.Fatalf("manifest did not record configured identity: %#v", manifest)
	}
}

func TestWritePublicExportAppliesCycleScope(t *testing.T) {
	store := db.NewMemoryStore()
	cycle5 := 5
	cycle6 := 6
	cycle5Time := time.Date(2026, 3, 12, 10, 0, 0, 0, time.UTC)
	cycle6Time := time.Date(2026, 6, 26, 10, 0, 0, 0, time.UTC)
	store.Sources["source:cycle5"] = model.Source{ID: "source:cycle5", Kind: model.SourceKindSuiEvent, Title: "Cycle 5", Locator: "fixture", Environment: model.EnvironmentStillness, Cycle: &cycle5, CreatedAt: cycle5Time}
	store.Sources["source:cycle6"] = model.Source{ID: "source:cycle6", Kind: model.SourceKindSuiEvent, Title: "Cycle 6", Locator: "fixture", Environment: model.EnvironmentStillness, Cycle: &cycle6, CreatedAt: cycle6Time}
	store.Sources["source:unlabelled"] = model.Source{ID: "source:unlabelled", Kind: model.SourceKindCommunityReport, Title: "Unlabelled", Locator: "fixture", Environment: model.EnvironmentStillness, CreatedAt: cycle6Time.Add(time.Minute)}
	store.Entities["character:stillness:cycle5"] = model.Entity{ID: "character:stillness:cycle5", Slug: "cycle5", Type: model.EntityTypeCharacter, Name: "Cycle 5", Environment: model.EnvironmentStillness, Cycle: &cycle5, UpdatedAt: cycle5Time}
	store.Entities["character:stillness:cycle6"] = model.Entity{ID: "character:stillness:cycle6", Slug: "cycle6", Type: model.EntityTypeCharacter, Name: "Cycle 6", Environment: model.EnvironmentStillness, Cycle: &cycle6, UpdatedAt: cycle6Time}
	store.Entities["character:stillness:unlabelled"] = model.Entity{ID: "character:stillness:unlabelled", Slug: "unlabelled", Type: model.EntityTypeCharacter, Name: "Unlabelled", Environment: model.EnvironmentStillness, UpdatedAt: cycle6Time.Add(time.Minute)}
	for _, item := range []model.KillmailRaw{
		{ID: "killmail:stillness:cycle5", Environment: model.EnvironmentStillness, OccurredAt: cycle5Time, SourceIDs: []string{"source:cycle5"}},
		{ID: "killmail:stillness:cycle6", Environment: model.EnvironmentStillness, OccurredAt: cycle6Time, SourceIDs: []string{"source:cycle6"}},
	} {
		if err := store.UpsertKillmail(context.Background(), item); err != nil {
			t.Fatal(err)
		}
	}
	for _, item := range []db.EventRecord{
		{ID: "event:cycle5", Kind: "fixture", Environment: model.EnvironmentStillness, OccurredAt: cycle5Time, Cycle: &cycle5, Payload: map[string]any{"kind": "fixture"}},
		{ID: "event:cycle6", Kind: "fixture", Environment: model.EnvironmentStillness, OccurredAt: cycle6Time, Cycle: &cycle6, Payload: map[string]any{"kind": "fixture"}},
		{ID: "event:unlabelled", Kind: "fixture", Environment: model.EnvironmentStillness, OccurredAt: cycle6Time.Add(time.Minute), Payload: map[string]any{"kind": "fixture"}},
	} {
		if err := store.UpsertSuiEvent(context.Background(), item); err != nil {
			t.Fatal(err)
		}
	}
	for _, item := range []db.SuiObjectRecord{
		{ID: "sui-object:cycle5", ObjectID: "0x5", Environment: model.EnvironmentStillness, TypeRepr: "0x2::fixture::Object", ObservedAt: cycle5Time, Payload: map[string]any{"kind": "fixture"}},
		{ID: "sui-object:cycle6", ObjectID: "0x6", Environment: model.EnvironmentStillness, TypeRepr: "0x2::fixture::Object", ObservedAt: cycle6Time, Payload: map[string]any{"kind": "fixture"}},
	} {
		if err := store.UpsertSuiObject(context.Background(), item); err != nil {
			t.Fatal(err)
		}
	}

	currentDir := t.TempDir()
	result, err := WritePublicExport(context.Background(), store, currentDir, ExportOptions{
		CycleScope:        "current",
		Cycles:            []int{6},
		IncludeUncycled:   true,
		IncludeEvents:     true,
		IncludeSuiObjects: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.CycleScope != "current" || len(result.Cycles) != 1 || result.Cycles[0] != 6 || !result.IncludeUncycled {
		t.Fatalf("catalog did not record cycle scope: %#v", result)
	}
	if result.EntityCount != 2 || result.KillmailCount != 1 || result.SourceCount != 2 || result.EventCount != 2 || result.SuiObjectCount != 1 {
		t.Fatalf("unexpected scoped export counts: %#v", result)
	}
	assertFileContains(t, filepath.Join(currentDir, "entities.jsonl"), "character:stillness:cycle6")
	assertFileContains(t, filepath.Join(currentDir, "entities.jsonl"), "character:stillness:unlabelled")
	assertFileExcludes(t, filepath.Join(currentDir, "entities.jsonl"), "character:stillness:cycle5")
	assertFileContains(t, filepath.Join(currentDir, "events.jsonl"), "event:cycle6")
	assertFileContains(t, filepath.Join(currentDir, "events.jsonl"), "event:unlabelled")
	assertFileExcludes(t, filepath.Join(currentDir, "events.jsonl"), "event:cycle5")
	assertFileContains(t, filepath.Join(currentDir, "sui_objects.jsonl"), "sui-object:cycle6")
	assertFileExcludes(t, filepath.Join(currentDir, "sui_objects.jsonl"), "sui-object:cycle5")
	var manifest ExportManifest
	readJSONFile(t, filepath.Join(currentDir, "manifest.json"), &manifest)
	if manifest.CycleScope != "current" || len(manifest.Cycles) != 1 || manifest.Cycles[0] != 6 || !manifest.IncludeUncycled {
		t.Fatalf("manifest did not record cycle scope: %#v", manifest)
	}

	allDir := t.TempDir()
	allResult, err := WritePublicExport(context.Background(), store, allDir, ExportOptions{
		CycleScope:        "all",
		IncludeEvents:     true,
		IncludeSuiObjects: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if allResult.EntityCount != 3 || allResult.KillmailCount != 2 || allResult.SourceCount != 3 || allResult.EventCount != 3 || allResult.SuiObjectCount != 2 {
		t.Fatalf("unexpected all-cycle export counts: %#v", allResult)
	}
}

func TestWritePublicExportManifestIncludesRawCorpusWatermarks(t *testing.T) {
	store := db.NewMemoryStore()
	now := time.Date(2026, 6, 24, 10, 0, 0, 0, time.UTC)
	if err := store.UpsertSuiEvent(context.Background(), db.EventRecord{
		ID:          "event:stillness:001",
		Kind:        "killmail.created",
		Environment: model.EnvironmentStillness,
		OccurredAt:  now,
		Payload:     map[string]any{"kind": "fixture"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertSuiObject(context.Background(), db.SuiObjectRecord{
		ID:          "sui-object:stillness:001",
		ObjectID:    "0x1",
		Environment: model.EnvironmentStillness,
		TypeRepr:    "0x2::fixture::Object",
		ObservedAt:  now,
		Payload:     map[string]any{"kind": "fixture"},
	}); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if _, err := WritePublicExport(context.Background(), store, dir, ExportOptions{IncludeEvents: true, IncludeSuiObjects: true}); err != nil {
		t.Fatal(err)
	}
	var manifest ExportManifest
	readJSONFile(t, filepath.Join(dir, "manifest.json"), &manifest)
	if manifestFile(t, manifest, "events.jsonl").RowCount != 1 {
		t.Fatalf("events file metadata missing row count: %#v", manifest.Files)
	}
	if manifestFile(t, manifest, "sui_objects.jsonl").RowCount != 1 {
		t.Fatalf("sui_objects file metadata missing row count: %#v", manifest.Files)
	}
	if manifest.HighWaterMarks["events"].FirstID != "event:stillness:001" {
		t.Fatalf("events high-water mark missing id: %#v", manifest.HighWaterMarks["events"])
	}
	if manifest.HighWaterMarks["sui_objects"].FirstID != "sui-object:stillness:001" {
		t.Fatalf("sui object high-water mark missing id: %#v", manifest.HighWaterMarks["sui_objects"])
	}
}

func TestVerifyPublicExportAcceptsGeneratedExport(t *testing.T) {
	store := db.NewMemoryStore()
	now := time.Date(2026, 6, 24, 10, 0, 0, 0, time.UTC)
	store.Sources["source:fixture"] = model.Source{ID: "source:fixture", Kind: model.SourceKindSuiEvent, Title: "Fixture", Locator: "fixture", Environment: model.EnvironmentStillness, CreatedAt: now}
	store.Entities["enemy:stillness:type:92096"] = model.Entity{ID: "enemy:stillness:type:92096", Slug: "enemy-caird-92096-stillness", Type: model.EntityTypeEnemy, Name: "Caird", DisplayName: "Caird [NPC]", Environment: model.EnvironmentStillness, UpdatedAt: now}
	if err := store.UpsertKillmail(context.Background(), model.KillmailRaw{ID: "killmail:stillness:310", Environment: model.EnvironmentStillness, OccurredAt: now, SourceIDs: []string{"source:fixture"}}); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if _, err := WritePublicExport(context.Background(), store, dir, ExportOptions{}); err != nil {
		t.Fatal(err)
	}
	result, err := VerifyPublicExport(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Valid {
		t.Fatalf("expected export to verify: %#v", result)
	}
	for _, name := range []string{"facts.jsonl", "relations.jsonl", "source_artefacts.jsonl", "ops_sui_coverage.json"} {
		if !verifyResultContainsFile(result, name) {
			t.Fatalf("expected %s to be verified, got %#v", name, result.Files)
		}
	}
}

func TestVerifyPublicExportRejectsTamperedFile(t *testing.T) {
	store := db.NewMemoryStore()
	now := time.Date(2026, 6, 24, 10, 0, 0, 0, time.UTC)
	store.Entities["enemy:stillness:type:92096"] = model.Entity{ID: "enemy:stillness:type:92096", Slug: "enemy-caird-92096-stillness", Type: model.EntityTypeEnemy, Name: "Caird", DisplayName: "Caird [NPC]", Environment: model.EnvironmentStillness, UpdatedAt: now}
	dir := t.TempDir()
	if _, err := WritePublicExport(context.Background(), store, dir, ExportOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "entities.jsonl"), []byte("tampered\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := VerifyPublicExport(dir)
	if err != nil {
		t.Fatal(err)
	}
	if result.Valid {
		t.Fatalf("tampered export verified successfully: %#v", result)
	}
	entities := verifyFile(t, result, "entities.jsonl")
	if entities.Valid {
		t.Fatalf("tampered entities file was marked valid: %#v", entities)
	}
	if entities.ExpectedSHA256 == entities.ActualSHA256 {
		t.Fatalf("expected checksum mismatch, got %#v", entities)
	}
}

func TestVerifyPublicExportRejectsTruncatedJSONL(t *testing.T) {
	store := db.NewMemoryStore()
	now := time.Date(2026, 6, 24, 10, 0, 0, 0, time.UTC)
	for i := 0; i < 2; i++ {
		id := fmt.Sprintf("character:stillness:%03d", i)
		store.Entities[id] = model.Entity{ID: id, Slug: id, Type: model.EntityTypeCharacter, Name: id, DisplayName: id, Environment: model.EnvironmentStillness, UpdatedAt: now.Add(time.Duration(i) * time.Second)}
	}
	dir := t.TempDir()
	if _, err := WritePublicExport(context.Background(), store, dir, ExportOptions{}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "entities.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	firstLine, _, _ := bytes.Cut(data, []byte("\n"))
	if err := os.WriteFile(filepath.Join(dir, "entities.jsonl"), append(firstLine, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := VerifyPublicExport(dir)
	if err != nil {
		t.Fatal(err)
	}
	if result.Valid {
		t.Fatalf("truncated export verified successfully: %#v", result)
	}
	entities := verifyFile(t, result, "entities.jsonl")
	if entities.ExpectedRowCount != 2 || entities.ActualRowCount != 1 {
		t.Fatalf("expected row count mismatch, got %#v", entities)
	}
}

func TestVerifyPublicExportRejectsMissingFile(t *testing.T) {
	store := db.NewMemoryStore()
	now := time.Date(2026, 6, 24, 10, 0, 0, 0, time.UTC)
	store.Entities["enemy:stillness:type:92096"] = model.Entity{ID: "enemy:stillness:type:92096", Slug: "enemy-caird-92096-stillness", Type: model.EntityTypeEnemy, Name: "Caird", DisplayName: "Caird [NPC]", Environment: model.EnvironmentStillness, UpdatedAt: now}
	dir := t.TempDir()
	if _, err := WritePublicExport(context.Background(), store, dir, ExportOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "entities.jsonl")); err != nil {
		t.Fatal(err)
	}
	result, err := VerifyPublicExport(dir)
	if err != nil {
		t.Fatal(err)
	}
	if result.Valid {
		t.Fatalf("export with missing file verified successfully: %#v", result)
	}
	entities := verifyFile(t, result, "entities.jsonl")
	if entities.Valid {
		t.Fatalf("missing file was marked valid: %#v", entities)
	}
	if len(entities.Errors) == 0 {
		t.Fatalf("missing file did not record an error: %#v", entities)
	}
}

func TestVerifyPublicExportRejectsUnsafeManifestPath(t *testing.T) {
	dir := t.TempDir()
	manifest := ExportManifest{
		SchemaVersion: "registry.export_manifest.v1",
		GeneratedAt:   time.Date(2026, 6, 24, 10, 0, 0, 0, time.UTC),
		Database:      db.DatabaseIdentity{Engine: "memory"},
		Files: []ExportFile{{
			Path:        "..\\outside.jsonl",
			ContentType: "application/x-ndjson",
			SHA256:      strings.Repeat("a", 64),
			SizeBytes:   1,
			RowCount:    1,
		}},
		HighWaterMarks: map[string]ExportHighWaterMark{},
	}
	if err := writeJSON(filepath.Join(dir, "manifest.json"), manifest); err != nil {
		t.Fatal(err)
	}
	result, err := VerifyPublicExport(dir)
	if err != nil {
		t.Fatal(err)
	}
	if result.Valid {
		t.Fatalf("unsafe manifest path verified successfully: %#v", result)
	}
	if len(result.Errors) == 0 || !strings.Contains(result.Errors[0], "unsafe") {
		t.Fatalf("expected unsafe path error, got %#v", result)
	}
}

func TestVerifyPublicExportRejectsDuplicateAndWrongContentTypeManifestEntries(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "entities.jsonl")
	if err := os.WriteFile(path, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := ExportManifest{
		SchemaVersion: "registry.export_manifest.v1",
		GeneratedAt:   time.Date(2026, 6, 24, 10, 0, 0, 0, time.UTC),
		Database:      db.DatabaseIdentity{Engine: "memory"},
		Files: []ExportFile{{
			Path:        "entities.jsonl",
			ContentType: "application/json",
			SHA256:      fileSHA256(t, path),
			SizeBytes:   3,
			RowCount:    1,
		}, {
			Path:        "entities.jsonl",
			ContentType: "application/x-ndjson",
			SHA256:      fileSHA256(t, path),
			SizeBytes:   3,
			RowCount:    1,
		}},
		HighWaterMarks: map[string]ExportHighWaterMark{},
	}
	if err := writeJSON(filepath.Join(dir, "manifest.json"), manifest); err != nil {
		t.Fatal(err)
	}
	result, err := VerifyPublicExport(dir)
	if err != nil {
		t.Fatal(err)
	}
	if result.Valid {
		t.Fatalf("duplicate/wrong-content-type manifest verified successfully: %#v", result)
	}
	joined := strings.Join(result.Errors, "\n")
	if !strings.Contains(joined, "duplicate") || !strings.Contains(joined, "content type") {
		t.Fatalf("expected duplicate and content-type errors, got %#v", result.Errors)
	}
}

func TestVerifyPublicExportRejectsInvalidJSONLRowShape(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "killmails.jsonl")
	row := []byte(`{"id":"killmail:stillness:missing-source"}` + "\n")
	if err := os.WriteFile(path, row, 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := ExportManifest{
		SchemaVersion: "registry.export_manifest.v1",
		GeneratedAt:   time.Date(2026, 6, 24, 10, 0, 0, 0, time.UTC),
		Database:      db.DatabaseIdentity{Engine: "memory"},
		Files: []ExportFile{{
			Path:        "killmails.jsonl",
			ContentType: "application/x-ndjson",
			SHA256:      fileSHA256(t, path),
			SizeBytes:   int64(len(row)),
			RowCount:    1,
		}},
		HighWaterMarks: map[string]ExportHighWaterMark{},
	}
	if err := writeJSON(filepath.Join(dir, "manifest.json"), manifest); err != nil {
		t.Fatal(err)
	}
	result, err := VerifyPublicExport(dir)
	if err != nil {
		t.Fatal(err)
	}
	if result.Valid {
		t.Fatalf("invalid JSONL shape verified successfully: %#v", result)
	}
	killmails := verifyFile(t, result, "killmails.jsonl")
	if killmails.Valid || !strings.Contains(strings.Join(killmails.Errors, "\n"), "missing required field") {
		t.Fatalf("expected row shape error, got %#v", killmails)
	}
}

func TestWritePublicExportDrainsPastPublicPageCaps(t *testing.T) {
	store := db.NewMemoryStore()
	now := time.Date(2026, 6, 24, 10, 0, 0, 0, time.UTC)
	store.Sources["source:fixture"] = model.Source{ID: "source:fixture", Kind: model.SourceKindSuiEvent, Title: "Fixture", Locator: "fixture", Environment: model.EnvironmentStillness, CreatedAt: now}
	for i := 0; i < 225; i++ {
		id := fmt.Sprintf("character:stillness:%03d", i)
		store.Entities[id] = model.Entity{
			ID:          id,
			Slug:        fmt.Sprintf("character-%03d", i),
			Type:        model.EntityTypeCharacter,
			Name:        fmt.Sprintf("Character %03d", i),
			DisplayName: fmt.Sprintf("Character %03d", i),
			Environment: model.EnvironmentStillness,
			UpdatedAt:   now.Add(time.Duration(i) * time.Second),
		}
		if err := store.UpsertKillmail(context.Background(), model.KillmailRaw{
			ID:          fmt.Sprintf("killmail:stillness:%03d", i),
			Environment: model.EnvironmentStillness,
			OccurredAt:  now.Add(time.Duration(i) * time.Second),
			SourceIDs:   []string{"source:fixture"},
		}); err != nil {
			t.Fatal(err)
		}
	}
	dir := t.TempDir()
	result, err := WritePublicExport(context.Background(), store, dir, ExportOptions{Limit: 1000})
	if err != nil {
		t.Fatal(err)
	}
	if result.EntityCount != 225 || result.KillmailCount != 225 {
		t.Fatalf("expected export to drain 225 rows, got %#v", result)
	}
}

func containsFile(files []string, name string) bool {
	for _, file := range files {
		if file == name {
			return true
		}
	}
	return false
}

func verifyResultContainsFile(result VerifyResult, name string) bool {
	for _, file := range result.Files {
		if file.Path == name {
			return true
		}
	}
	return false
}

func assertFileContains(t *testing.T, path, needle string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), needle) {
		t.Fatalf("%s did not contain %q:\n%s", path, needle, data)
	}
}

func assertFileExcludes(t *testing.T, path, needle string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), needle) {
		t.Fatalf("%s unexpectedly contained %q:\n%s", path, needle, data)
	}
}

func readJSONFile[T any](t *testing.T, path string, target *T) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, target); err != nil {
		t.Fatal(err)
	}
}

func manifestFile(t *testing.T, manifest ExportManifest, path string) ExportFile {
	t.Helper()
	for _, file := range manifest.Files {
		if file.Path == path {
			return file
		}
	}
	t.Fatalf("manifest did not include %s: %#v", path, manifest.Files)
	return ExportFile{}
}

func fileSHA256(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func verifyFile(t *testing.T, result VerifyResult, path string) VerifyFile {
	t.Helper()
	for _, file := range result.Files {
		if file.Path == path {
			return file
		}
	}
	t.Fatalf("verify result did not include %s: %#v", path, result.Files)
	return VerifyFile{}
}

func TestWritePublicExportDrainsSourcesPastBoundedListCap(t *testing.T) {
	store := db.NewMemoryStore()
	now := time.Date(2026, 6, 24, 10, 0, 0, 0, time.UTC)
	for i := 0; i < 5001; i++ {
		id := fmt.Sprintf("source:%04d", i)
		store.Sources[id] = model.Source{
			ID:          id,
			Kind:        model.SourceKindSuiEvent,
			Title:       fmt.Sprintf("Source %04d", i),
			Locator:     "fixture",
			Environment: model.EnvironmentStillness,
			CreatedAt:   now.Add(time.Duration(i) * time.Second),
		}
	}
	dir := t.TempDir()
	result, err := WritePublicExport(context.Background(), store, dir, ExportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if result.SourceCount != 5001 {
		t.Fatalf("expected all sources to be exported, got %#v", result)
	}
}

func TestWritePublicExportIncludesRawCorpusOnlyWhenRequested(t *testing.T) {
	store := db.NewMemoryStore()
	now := time.Date(2026, 6, 24, 10, 0, 0, 0, time.UTC)
	if err := store.UpsertSuiEvent(context.Background(), db.EventRecord{
		ID:          "event:stillness:001",
		Kind:        "killmail.created",
		Environment: model.EnvironmentStillness,
		OccurredAt:  now,
		Payload:     map[string]any{"kind": "fixture"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertSuiObject(context.Background(), db.SuiObjectRecord{
		ID:          "sui-object:stillness:001",
		ObjectID:    "0x1",
		Environment: model.EnvironmentStillness,
		TypeRepr:    "0x2::fixture::Object",
		ObservedAt:  now,
		Payload:     map[string]any{"kind": "fixture"},
	}); err != nil {
		t.Fatal(err)
	}
	defaultDir := t.TempDir()
	defaultResult, err := WritePublicExport(context.Background(), store, defaultDir, ExportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if defaultResult.EventCount != 0 || defaultResult.SuiObjectCount != 0 {
		t.Fatalf("raw corpus should be opt-in, got %#v", defaultResult)
	}
	if _, err := os.Stat(filepath.Join(defaultDir, "events.jsonl")); !os.IsNotExist(err) {
		t.Fatalf("events.jsonl should not exist by default, stat err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(defaultDir, "sui_objects.jsonl")); !os.IsNotExist(err) {
		t.Fatalf("sui_objects.jsonl should not exist by default, stat err=%v", err)
	}
	rawDir := t.TempDir()
	rawResult, err := WritePublicExport(context.Background(), store, rawDir, ExportOptions{IncludeEvents: true, IncludeSuiObjects: true})
	if err != nil {
		t.Fatal(err)
	}
	if rawResult.EventCount != 1 || rawResult.SuiObjectCount != 1 {
		t.Fatalf("expected raw corpus counts, got %#v", rawResult)
	}
	for _, name := range []string{"events.jsonl", "sui_objects.jsonl"} {
		if _, err := os.Stat(filepath.Join(rawDir, name)); err != nil {
			t.Fatalf("missing %s: %v", name, err)
		}
	}
}
