package worldapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/blackrelay/registry/internal/artefacts"
	"github.com/blackrelay/registry/internal/db"
	"github.com/blackrelay/registry/internal/model"
)

const (
	datahubImporter = "br-import-datahub-types"
	worldImporter   = "br-import-world-systems"
	tribeImporter   = "br-import-world-tribes"
	maxSnapshotSize = 100 << 20
)

type MetadataOptions struct {
	Environment     model.Environment
	AllowedRootDirs []string
	SourceURL       string
	SourceTitle     string
	Notes           string
	Cycle           *int
}

type MetadataResult struct {
	Source       model.Source         `json:"source"`
	Artefact     model.SourceArtefact `json:"artefact"`
	ImportID     string               `json:"importId"`
	RowsImported int                  `json:"rowsImported"`
}

type FetchResult struct {
	Path      string `json:"path"`
	SourceURL string `json:"sourceUrl"`
	SizeBytes int64  `json:"sizeBytes"`
}

type MetadataStore interface {
	RecordImport(ctx context.Context, importID string, source model.Source, artefact model.SourceArtefact, summary map[string]any) error
	UpsertEntityFacts(ctx context.Context, entity model.Entity, facts []db.EntityFactDraft) error
}

func ImportDatahubTypes(ctx context.Context, store MetadataStore, artefactStore artefacts.Store, inputPath string, opts MetadataOptions) (MetadataResult, error) {
	return importMetadataRows(ctx, store, artefactStore, inputPath, opts, metadataImportSpec{
		sourceID:     "source:datahub:types",
		sourceKind:   model.SourceKindDatahub,
		sourceTitle:  "Public Datahub type metadata",
		artefactKind: "datahub_types",
		importerName: datahubImporter,
		contentType:  "application/json",
		rowToEntity:  datahubTypeEntity,
	})
}

func FetchSnapshot(ctx context.Context, sourceURL, outputPath string) (FetchResult, error) {
	if err := validatePublicSourceURL(sourceURL); err != nil {
		return FetchResult{}, err
	}
	if strings.TrimSpace(outputPath) == "" {
		return FetchResult{}, errors.New("snapshot output path is required")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return FetchResult{}, err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return FetchResult{}, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return FetchResult{}, fmt.Errorf("fetch snapshot returned HTTP %d", response.StatusCode)
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return FetchResult{}, err
	}
	tmp := outputPath + ".tmp"
	file, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return FetchResult{}, err
	}
	size, copyErr := io.Copy(file, io.LimitReader(response.Body, maxSnapshotSize+1))
	closeErr := file.Close()
	if copyErr != nil {
		_ = os.Remove(tmp)
		return FetchResult{}, copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return FetchResult{}, closeErr
	}
	if size > maxSnapshotSize {
		_ = os.Remove(tmp)
		return FetchResult{}, fmt.Errorf("snapshot exceeds maximum size of %d bytes", maxSnapshotSize)
	}
	if err := os.Rename(tmp, outputPath); err != nil {
		_ = os.Remove(tmp)
		return FetchResult{}, err
	}
	return FetchResult{Path: outputPath, SourceURL: sourceURL, SizeBytes: size}, nil
}

func ImportWorldSystems(ctx context.Context, store MetadataStore, artefactStore artefacts.Store, inputPath string, opts MetadataOptions) (MetadataResult, error) {
	return importMetadataRows(ctx, store, artefactStore, inputPath, opts, metadataImportSpec{
		sourceID:     "source:world-api:systems",
		sourceKind:   model.SourceKindWorldAPI,
		sourceTitle:  "Public World API solar system metadata",
		artefactKind: "world_systems",
		importerName: worldImporter,
		contentType:  "application/json",
		rowToEntity:  worldSystemEntity,
	})
}

func ImportWorldTribes(ctx context.Context, store MetadataStore, artefactStore artefacts.Store, inputPath string, opts MetadataOptions) (MetadataResult, error) {
	return importMetadataRows(ctx, store, artefactStore, inputPath, opts, metadataImportSpec{
		sourceID:     "source:world-api:tribes",
		sourceKind:   model.SourceKindWorldAPI,
		sourceTitle:  "Public World API tribe metadata",
		artefactKind: "world_tribes",
		importerName: tribeImporter,
		contentType:  "application/json",
		rowToEntity:  worldTribeEntity,
	})
}

type metadataImportSpec struct {
	sourceID     string
	sourceKind   model.SourceKind
	sourceTitle  string
	artefactKind string
	importerName string
	contentType  string
	rowToEntity  func(model.Environment, model.Source, map[string]any) (model.Entity, []db.EntityFactDraft, bool)
}

func importMetadataRows(ctx context.Context, store MetadataStore, artefactStore artefacts.Store, inputPath string, opts MetadataOptions, spec metadataImportSpec) (MetadataResult, error) {
	if store == nil {
		return MetadataResult{}, errors.New("store is required")
	}
	if artefactStore == nil {
		return MetadataResult{}, errors.New("artefact store is required")
	}
	if err := validatePublicSourceURL(opts.SourceURL); err != nil {
		return MetadataResult{}, err
	}
	environment := opts.Environment
	if environment == "" {
		environment = model.EnvironmentStillness
	}
	data, err := os.ReadFile(inputPath)
	if err != nil {
		return MetadataResult{}, err
	}
	rows, err := decodeRows(data)
	if err != nil {
		return MetadataResult{}, err
	}
	source := model.Source{
		ID:          spec.sourceID + ":" + string(environment),
		Kind:        spec.sourceKind,
		Title:       firstNonEmpty(opts.SourceTitle, spec.sourceTitle),
		Locator:     inputPath,
		URL:         opts.SourceURL,
		Environment: environment,
		Cycle:       opts.Cycle,
		Metadata: map[string]any{
			"rowCount": len(rows),
		},
		CreatedAt: time.Now().UTC(),
	}
	artefact, err := artefactStore.RegisterFile(ctx, inputPath, artefacts.RegisterMeta{
		SourceID:        source.ID,
		SourceKind:      spec.sourceKind,
		Kind:            spec.artefactKind,
		ArtefactKind:    spec.artefactKind,
		Environment:     environment,
		ContentType:     spec.contentType,
		RowCount:        int64(len(rows)),
		ImporterName:    spec.importerName,
		Cycle:           opts.Cycle,
		ReviewStatus:    model.ReviewStatusReviewed,
		Notes:           opts.Notes,
		AllowedRootDirs: opts.AllowedRootDirs,
	})
	if err != nil {
		return MetadataResult{}, err
	}
	importID := fmt.Sprintf("import:%s:%s:%s", spec.artefactKind, environment, artefact.SHA256[:12])
	if err := store.RecordImport(ctx, importID, source, artefact, map[string]any{
		"rowCount":   len(rows),
		"sourceKind": source.Kind,
	}); err != nil {
		return MetadataResult{}, err
	}
	var imported int
	for _, row := range rows {
		entity, facts, ok := spec.rowToEntity(environment, source, row)
		if !ok {
			continue
		}
		entity.Cycle = opts.Cycle
		for i := range facts {
			facts[i].Cycle = opts.Cycle
		}
		if err := store.UpsertEntityFacts(ctx, entity, facts); err != nil {
			return MetadataResult{}, err
		}
		imported++
	}
	return MetadataResult{Source: source, Artefact: artefact, ImportID: importID, RowsImported: imported}, nil
}

func datahubTypeEntity(environment model.Environment, source model.Source, row map[string]any) (model.Entity, []db.EntityFactDraft, bool) {
	itemID := fieldString(row, "itemId", "typeId", "id")
	name := fieldString(row, "name", "typeName")
	if itemID == "" || name == "" {
		return model.Entity{}, nil, false
	}
	entity := model.Entity{
		ID:          fmt.Sprintf("item:%s:%s", environment, itemID),
		Slug:        slugify("item-" + itemID + "-" + string(environment)),
		Type:        model.EntityTypeItem,
		Name:        name,
		DisplayName: name,
		Summary:     "Public Datahub type metadata.",
		Environment: environment,
		UpdatedAt:   time.Now().UTC(),
	}
	facts := metadataFacts(source, environment, map[string]any{
		"item_id":     itemID,
		"type_id":     fieldString(row, "typeId"),
		"group_id":    fieldString(row, "groupId"),
		"category":    fieldString(row, "category", "categoryName"),
		"description": fieldString(row, "description"),
	})
	return entity, facts, true
}

func worldSystemEntity(environment model.Environment, source model.Source, row map[string]any) (model.Entity, []db.EntityFactDraft, bool) {
	systemID := fieldString(row, "systemId", "solarSystemId", "id")
	name := fieldString(row, "name", "systemName")
	if systemID == "" || name == "" {
		return model.Entity{}, nil, false
	}
	entity := model.Entity{
		ID:          fmt.Sprintf("system:%s:%s", environment, systemID),
		Slug:        slugify("system-" + systemID + "-" + string(environment)),
		Type:        model.EntityTypeSystem,
		Name:        name,
		DisplayName: name,
		Summary:     "Public World API solar system metadata.",
		Environment: environment,
		UpdatedAt:   time.Now().UTC(),
	}
	facts := metadataFacts(source, environment, map[string]any{
		"system_id": systemID,
		"region":    fieldString(row, "region", "regionName"),
		"x":         fieldString(row, "x"),
		"y":         fieldString(row, "y"),
		"z":         fieldString(row, "z"),
	})
	return entity, facts, true
}

func worldTribeEntity(environment model.Environment, source model.Source, row map[string]any) (model.Entity, []db.EntityFactDraft, bool) {
	tribeID := fieldString(row, "tribeId", "tribeID", "tribe_id", "id", "corpId", "corpID", "corp_id", "corporationId", "corporationID", "corporation_id")
	name := fieldString(row, "name", "tribeName", "tribe_name", "corpName", "corp_name", "corporationName", "corporation_name")
	if tribeID == "" || name == "" {
		return model.Entity{}, nil, false
	}
	entity := model.Entity{
		ID:          fmt.Sprintf("tribe:%s:%s", environment, tribeID),
		Slug:        slugify("tribe-" + tribeID + "-" + string(environment)),
		Type:        model.EntityTypeTribe,
		Name:        name,
		DisplayName: name,
		Summary:     "Public World API tribe metadata.",
		Environment: environment,
		UpdatedAt:   time.Now().UTC(),
	}
	facts := metadataFacts(source, environment, map[string]any{
		"tribe_id":       tribeID,
		"display_name":   name,
		"tag":            fieldString(row, "tag", "ticker", "nameShort", "name_short", "tribeTicker", "tribe_ticker", "corpTicker", "corp_ticker", "corporationTicker", "corporation_ticker"),
		"description":    fieldString(row, "description", "tribeDescription", "tribe_description", "corpDescription", "corp_description"),
		"url":            fieldString(row, "url", "uri", "tribeUrl", "tribeURL", "tribe_url", "website", "websiteUrl", "websiteURL", "homeUrl", "homeURL"),
		"member_count":   fieldString(row, "memberCount", "member_count", "members", "membersCount"),
		"founded_at":     fieldString(row, "foundedAt", "founded_at", "createdAt", "created_at"),
		"tax_rate":       fieldString(row, "taxRate", "tax_rate"),
		"source_context": "Public World API tribe metadata row.",
	})
	return entity, facts, true
}

func metadataFacts(source model.Source, environment model.Environment, values map[string]any) []db.EntityFactDraft {
	facts := make([]db.EntityFactDraft, 0, len(values))
	for key, value := range values {
		if value == nil || value == "" {
			continue
		}
		facts = append(facts, db.EntityFactDraft{
			Key:          key,
			Value:        value,
			SourceID:     source.ID,
			Confidence:   model.ConfidenceVerified,
			Environment:  environment,
			ReviewStatus: model.ReviewStatusReviewed,
		})
	}
	return facts
}

func decodeRows(data []byte) ([]map[string]any, error) {
	var rows []map[string]any
	if err := json.Unmarshal(data, &rows); err == nil {
		return rows, nil
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, err
	}
	for _, key := range []string{"data", "items", "types", "systems", "tribes", "rows"} {
		if raw, ok := envelope[key]; ok {
			if err := json.Unmarshal(raw, &rows); err != nil {
				return nil, err
			}
			return rows, nil
		}
	}
	return nil, errors.New("metadata JSON must be an array or contain a data/items/types/systems/rows array")
}

func validatePublicSourceURL(rawURL string) error {
	if strings.TrimSpace(rawURL) == "" {
		return nil
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return err
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "priv.evefrontier.com" || strings.HasSuffix(host, ".priv.evefrontier.com") {
		return fmt.Errorf("private EVE Frontier host is not allowed: %s", host)
	}
	return nil
}

func fieldString(row map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := row[key]
		if !ok {
			continue
		}
		switch item := value.(type) {
		case string:
			if item != "" {
				return item
			}
		case float64:
			if item == float64(int64(item)) {
				return fmt.Sprint(int64(item))
			}
			return fmt.Sprint(item)
		case int:
			return fmt.Sprint(item)
		case int64:
			return fmt.Sprint(item)
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

var slugReplacePattern = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(value string) string {
	slug := strings.ToLower(value)
	slug = slugReplacePattern.ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-")
	if slug == "" {
		return "metadata-row"
	}
	return slug
}
