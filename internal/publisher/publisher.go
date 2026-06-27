package publisher

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/blackrelay/registry/internal/exporter"
)

type ObjectStore interface {
	PutObject(ctx context.Context, object Object) error
}

type Object struct {
	Key            string            `json:"key"`
	SourcePath     string            `json:"sourcePath,omitempty"`
	Body           []byte            `json:"-"`
	ContentType    string            `json:"contentType"`
	SHA256         string            `json:"sha256,omitempty"`
	SizeBytes      int64             `json:"sizeBytes,omitempty"`
	Metadata       map[string]string `json:"metadata,omitempty"`
	AllowOverwrite bool              `json:"allowOverwrite,omitempty"`
}

type Options struct {
	Prefix   string
	BundleID string
	Now      func() time.Time
}

type Result struct {
	SchemaVersion    string          `json:"schemaVersion"`
	PublishedAt      time.Time       `json:"publishedAt"`
	Registry         string          `json:"registry"`
	APIVersion       string          `json:"apiVersion"`
	Prefix           string          `json:"prefix,omitempty"`
	BundleID         string          `json:"bundleId"`
	ManifestKey      string          `json:"manifestKey"`
	ManifestSHA256   string          `json:"manifestSha256"`
	LatestPointerKey string          `json:"latestPointerKey"`
	Files            []PublishedFile `json:"files"`
}

type PublishedFile struct {
	Path        string `json:"path"`
	ObjectKey   string `json:"objectKey"`
	ContentType string `json:"contentType"`
	SHA256      string `json:"sha256"`
	SizeBytes   int64  `json:"sizeBytes"`
	RowCount    int    `json:"rowCount"`
}

type Pointer struct {
	SchemaVersion  string          `json:"schemaVersion"`
	PublishedAt    time.Time       `json:"publishedAt"`
	Registry       string          `json:"registry"`
	APIVersion     string          `json:"apiVersion"`
	BundleID       string          `json:"bundleId"`
	ManifestKey    string          `json:"manifestKey"`
	ManifestSHA256 string          `json:"manifestSha256"`
	Files          []PublishedFile `json:"files"`
}

func PublishVerifiedExport(ctx context.Context, exportDir string, store ObjectStore, options Options) (Result, error) {
	if strings.TrimSpace(exportDir) == "" {
		return Result{}, errors.New("export directory is required")
	}
	if store == nil {
		return Result{}, errors.New("object store is required")
	}
	verification, err := exporter.VerifyPublicExport(exportDir)
	if err != nil {
		return Result{}, err
	}
	if !verification.Valid {
		return Result{}, fmt.Errorf("export verification failed: %s", strings.Join(verification.Errors, "; "))
	}
	manifestPath := filepath.Join(exportDir, "manifest.json")
	manifestBytes, err := os.ReadFile(manifestPath)
	if err != nil {
		return Result{}, err
	}
	manifestSum := sha256.Sum256(manifestBytes)
	manifestSHA256 := hex.EncodeToString(manifestSum[:])
	var manifest exporter.ExportManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return Result{}, fmt.Errorf("decode manifest: %w", err)
	}
	now := time.Now().UTC()
	if options.Now != nil {
		now = options.Now().UTC()
	}
	bundleID := strings.TrimSpace(options.BundleID)
	if bundleID == "" {
		bundleID = manifestSHA256
	}
	if err := validateBundleID(bundleID); err != nil {
		return Result{}, err
	}
	prefix := normalizePrefix(options.Prefix)
	if err := validateObjectKeyPrefix(prefix); err != nil {
		return Result{}, err
	}
	result := Result{
		SchemaVersion:    "registry.export_publish.v1",
		PublishedAt:      now,
		Registry:         manifest.Registry,
		APIVersion:       manifest.APIVersion,
		Prefix:           prefix,
		BundleID:         bundleID,
		ManifestSHA256:   manifestSHA256,
		ManifestKey:      objectKey(prefix, "bundles", bundleID, "manifest.json"),
		LatestPointerKey: objectKey(prefix, "latest", "manifest.json"),
	}
	for _, file := range manifest.Files {
		published := PublishedFile{
			Path:        file.Path,
			ObjectKey:   objectKey(prefix, "bundles", bundleID, file.Path),
			ContentType: file.ContentType,
			SHA256:      file.SHA256,
			SizeBytes:   file.SizeBytes,
			RowCount:    file.RowCount,
		}
		if err := store.PutObject(ctx, Object{
			Key:         published.ObjectKey,
			SourcePath:  filepath.Join(exportDir, file.Path),
			ContentType: file.ContentType,
			SHA256:      file.SHA256,
			SizeBytes:   file.SizeBytes,
			Metadata:    objectMetadata(manifest, bundleID),
		}); err != nil {
			return Result{}, err
		}
		result.Files = append(result.Files, published)
	}
	manifestFile := PublishedFile{
		Path:        "manifest.json",
		ObjectKey:   result.ManifestKey,
		ContentType: "application/json",
		SHA256:      manifestSHA256,
		SizeBytes:   int64(len(manifestBytes)),
		RowCount:    1,
	}
	if err := store.PutObject(ctx, Object{
		Key:         manifestFile.ObjectKey,
		SourcePath:  manifestPath,
		ContentType: manifestFile.ContentType,
		SHA256:      manifestFile.SHA256,
		SizeBytes:   manifestFile.SizeBytes,
		Metadata:    objectMetadata(manifest, bundleID),
	}); err != nil {
		return Result{}, err
	}
	result.Files = append(result.Files, manifestFile)
	pointer := Pointer{
		SchemaVersion:  "registry.export_publish_pointer.v1",
		PublishedAt:    now,
		Registry:       manifest.Registry,
		APIVersion:     manifest.APIVersion,
		BundleID:       bundleID,
		ManifestKey:    result.ManifestKey,
		ManifestSHA256: manifestSHA256,
		Files:          result.Files,
	}
	pointerBytes, err := json.MarshalIndent(pointer, "", "  ")
	if err != nil {
		return Result{}, err
	}
	pointerBytes = append(pointerBytes, '\n')
	if err := store.PutObject(ctx, Object{
		Key:            result.LatestPointerKey,
		Body:           pointerBytes,
		ContentType:    "application/json",
		AllowOverwrite: true,
		Metadata:       objectMetadata(manifest, bundleID),
	}); err != nil {
		return Result{}, err
	}
	return result, nil
}

func objectMetadata(manifest exporter.ExportManifest, bundleID string) map[string]string {
	return map[string]string{
		"registry":   manifest.Registry,
		"apiVersion": manifest.APIVersion,
		"bundleID":   bundleID,
	}
}

func normalizePrefix(prefix string) string {
	prefix = strings.TrimSpace(prefix)
	prefix = strings.Trim(prefix, "/\\")
	if prefix == "." {
		return ""
	}
	return strings.ReplaceAll(prefix, "\\", "/")
}

func objectKey(parts ...string) string {
	clean := make([]string, 0, len(parts))
	for _, part := range parts {
		part = normalizePrefix(part)
		if part != "" {
			clean = append(clean, part)
		}
	}
	return path.Join(clean...)
}

func validateBundleID(bundleID string) error {
	if bundleID == "" {
		return errors.New("bundle id is required")
	}
	if strings.ContainsAny(bundleID, `/\`) || strings.Contains(bundleID, "..") {
		return fmt.Errorf("unsafe bundle id %q", bundleID)
	}
	return nil
}

func validateObjectKeyPrefix(prefix string) error {
	if prefix == "" {
		return nil
	}
	if strings.Contains(prefix, "\x00") {
		return fmt.Errorf("unsafe object key prefix %q: NUL bytes are not allowed", prefix)
	}
	for _, part := range strings.Split(prefix, "/") {
		if part == "" || part == "." || part == ".." {
			return fmt.Errorf("unsafe object key prefix %q", prefix)
		}
	}
	return nil
}
