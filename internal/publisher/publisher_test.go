package publisher

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/blackrelay/registry/internal/db"
	"github.com/blackrelay/registry/internal/exporter"
	"github.com/blackrelay/registry/internal/model"
)

func TestPublishVerifiedExportWritesImmutableBundleAndLatestPointerLast(t *testing.T) {
	store := db.NewMemoryStore()
	now := time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)
	store.Entities["enemy:stillness:type:92096"] = model.Entity{
		ID:          "enemy:stillness:type:92096",
		Slug:        "enemy-caird-92096-stillness",
		Type:        model.EntityTypeEnemy,
		Name:        "Caird",
		DisplayName: "Caird [NPC]",
		Environment: model.EnvironmentStillness,
		UpdatedAt:   now,
	}
	exportDir := t.TempDir()
	if _, err := exporter.WritePublicExport(context.Background(), store, exportDir, exporter.ExportOptions{
		RegistryID: "frontier-community-registry",
		APIVersion: "v1-community",
		Now:        func() time.Time { return now },
	}); err != nil {
		t.Fatal(err)
	}

	objectStore := newMemoryObjectStore()
	result, err := PublishVerifiedExport(context.Background(), exportDir, objectStore, Options{
		Prefix: "registry",
		Now:    func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.SchemaVersion != "registry.export_publish.v1" {
		t.Fatalf("unexpected schema version %q", result.SchemaVersion)
	}
	if result.Registry != "frontier-community-registry" || result.APIVersion != "v1-community" {
		t.Fatalf("result did not preserve export identity: %#v", result)
	}
	if result.BundleID == "" || result.ManifestSHA256 == "" {
		t.Fatalf("result did not record bundle identity: %#v", result)
	}
	wantManifestKey := "registry/bundles/" + result.BundleID + "/manifest.json"
	if result.ManifestKey != wantManifestKey {
		t.Fatalf("unexpected manifest key %q", result.ManifestKey)
	}
	if result.LatestPointerKey != "registry/latest/manifest.json" {
		t.Fatalf("unexpected latest pointer key %q", result.LatestPointerKey)
	}
	if len(result.Files) != 5 {
		t.Fatalf("expected catalog, manifest and three data files, got %#v", result.Files)
	}
	if objectStore.order[len(objectStore.order)-1] != result.LatestPointerKey {
		t.Fatalf("latest pointer was not written last: %#v", objectStore.order)
	}
	if _, ok := objectStore.objects[wantManifestKey]; !ok {
		t.Fatalf("manifest was not published to immutable bundle key")
	}

	var pointer Pointer
	if err := json.Unmarshal(objectStore.objects[result.LatestPointerKey], &pointer); err != nil {
		t.Fatal(err)
	}
	if pointer.BundleID != result.BundleID || pointer.ManifestKey != result.ManifestKey {
		t.Fatalf("latest pointer did not reference bundle: %#v", pointer)
	}
	if pointer.ManifestSHA256 != result.ManifestSHA256 {
		t.Fatalf("latest pointer did not record manifest checksum: %#v", pointer)
	}
}

func TestPublishVerifiedExportRefusesTamperedExportWithoutWrites(t *testing.T) {
	store := db.NewMemoryStore()
	now := time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)
	store.Entities["enemy:stillness:type:92096"] = model.Entity{
		ID:          "enemy:stillness:type:92096",
		Slug:        "enemy-caird-92096-stillness",
		Type:        model.EntityTypeEnemy,
		Name:        "Caird",
		DisplayName: "Caird [NPC]",
		Environment: model.EnvironmentStillness,
		UpdatedAt:   now,
	}
	exportDir := t.TempDir()
	if _, err := exporter.WritePublicExport(context.Background(), store, exportDir, exporter.ExportOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(exportDir, "entities.jsonl"), []byte("tampered\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	objectStore := newMemoryObjectStore()
	if _, err := PublishVerifiedExport(context.Background(), exportDir, objectStore, Options{Prefix: "registry"}); err == nil {
		t.Fatal("expected tampered export to be rejected")
	}
	if len(objectStore.objects) != 0 {
		t.Fatalf("tampered export wrote objects: %#v", objectStore.order)
	}
}

func TestPublishVerifiedExportRejectsUnsafePrefixBeforeWrites(t *testing.T) {
	store := db.NewMemoryStore()
	exportDir := t.TempDir()
	if _, err := exporter.WritePublicExport(context.Background(), store, exportDir, exporter.ExportOptions{}); err != nil {
		t.Fatal(err)
	}

	objectStore := newMemoryObjectStore()
	if _, err := PublishVerifiedExport(context.Background(), exportDir, objectStore, Options{Prefix: "../registry"}); err == nil {
		t.Fatal("expected unsafe prefix to be rejected")
	}
	if len(objectStore.objects) != 0 {
		t.Fatalf("unsafe prefix wrote objects before failing: %#v", objectStore.order)
	}
}

type memoryObjectStore struct {
	objects map[string][]byte
	order   []string
}

func newMemoryObjectStore() *memoryObjectStore {
	return &memoryObjectStore{objects: map[string][]byte{}}
}

func (s *memoryObjectStore) PutObject(ctx context.Context, object Object) error {
	_ = ctx
	data, err := os.ReadFile(object.SourcePath)
	if object.SourcePath == "" {
		data = object.Body
	} else if err != nil {
		return err
	}
	if _, exists := s.objects[object.Key]; exists && !object.AllowOverwrite {
		return os.ErrExist
	}
	s.objects[object.Key] = append([]byte(nil), data...)
	s.order = append(s.order, object.Key)
	return nil
}
