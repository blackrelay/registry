package artefacts

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/blackrelay/registry/internal/model"
)

func TestLocalStoreRegistersSHA256Artefact(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "candidate.json")
	if err := os.WriteFile(input, []byte(`{"ok":true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	store := LocalStore{
		Root: filepath.Join(dir, "artefacts"),
		Now:  func() time.Time { return time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC) },
	}
	artefact, err := store.RegisterFile(context.Background(), input, RegisterMeta{
		SourceID:        "source:static-enemies",
		Kind:            "static_enemy_candidates",
		Environment:     model.EnvironmentStillness,
		ContentType:     "application/json",
		ImporterName:    "br-import",
		ReviewStatus:    model.ReviewStatusReviewed,
		AllowedRootDirs: []string{dir},
	})
	if err != nil {
		t.Fatalf("RegisterFile returned error: %v", err)
	}
	if artefact.SHA256 != "4062edaf750fb8074e7e83e0c9028c94e32468a8b6f1614774328ef045150f93" {
		t.Fatalf("unexpected hash %s", artefact.SHA256)
	}
	if _, err := os.Stat(artefact.PathOrURI); err != nil {
		t.Fatalf("artefact copy missing: %v", err)
	}
}

func TestLocalStoreRejectsDisallowedPath(t *testing.T) {
	dir := t.TempDir()
	other := t.TempDir()
	input := filepath.Join(other, "candidate.json")
	if err := os.WriteFile(input, []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	store := LocalStore{Root: filepath.Join(dir, "artefacts")}
	_, err := store.RegisterFile(context.Background(), input, RegisterMeta{
		SourceID:        "source:static-enemies",
		Kind:            "static_enemy_candidates",
		Environment:     model.EnvironmentStillness,
		ContentType:     "application/json",
		ImporterName:    "br-import",
		ReviewStatus:    model.ReviewStatusReviewed,
		AllowedRootDirs: []string{dir},
	})
	if err == nil {
		t.Fatal("RegisterFile accepted a path outside the allowed roots")
	}
}
