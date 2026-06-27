package exporter

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/blackrelay/registry/internal/db"
	"github.com/blackrelay/registry/internal/model"
)

type Store interface {
	ListEntities(ctx context.Context, query db.EntityQuery) (db.EntityPage, error)
	ListEvents(ctx context.Context, query db.EventQuery) (db.EventPage, error)
	ListKillmailRaw(ctx context.Context, query db.KillmailQuery) ([]model.KillmailRaw, string, error)
	ListSuiObjects(ctx context.Context, query db.SuiObjectQuery) (db.SuiObjectPage, error)
	ListSources(ctx context.Context, limit int) ([]model.Source, error)
	ListSourcesPage(ctx context.Context, query db.SourceQuery) (db.SourcePage, error)
	ExportDatabaseIdentity(ctx context.Context) (db.DatabaseIdentity, error)
}

type ExportOptions struct {
	Limit             int
	PageSize          int
	IncludeEvents     bool
	IncludeSuiObjects bool
	CycleScope        string
	Cycles            []int
	IncludeUncycled   bool
	RegistryID        string
	APIVersion        string
	Now               func() time.Time
}

type ExportResult struct {
	SchemaVersion   string    `json:"schemaVersion"`
	Registry        string    `json:"registry"`
	APIVersion      string    `json:"apiVersion"`
	GeneratedAt     time.Time `json:"generatedAt"`
	CycleScope      string    `json:"cycleScope"`
	Cycles          []int     `json:"cycles,omitempty"`
	IncludeUncycled bool      `json:"includeUncycled,omitempty"`
	EntityCount     int       `json:"entityCount"`
	KillmailCount   int       `json:"killmailCount"`
	SourceCount     int       `json:"sourceCount"`
	EventCount      int       `json:"eventCount,omitempty"`
	SuiObjectCount  int       `json:"suiObjectCount,omitempty"`
	Files           []string  `json:"files"`
}

type ExportManifest struct {
	SchemaVersion   string                         `json:"schemaVersion"`
	Registry        string                         `json:"registry"`
	APIVersion      string                         `json:"apiVersion"`
	GeneratedAt     time.Time                      `json:"generatedAt"`
	CycleScope      string                         `json:"cycleScope"`
	Cycles          []int                          `json:"cycles,omitempty"`
	IncludeUncycled bool                           `json:"includeUncycled,omitempty"`
	Database        db.DatabaseIdentity            `json:"database"`
	Files           []ExportFile                   `json:"files"`
	HighWaterMarks  map[string]ExportHighWaterMark `json:"highWaterMarks"`
}

type ExportFile struct {
	Path        string `json:"path"`
	ContentType string `json:"contentType"`
	SHA256      string `json:"sha256"`
	SizeBytes   int64  `json:"sizeBytes"`
	RowCount    int    `json:"rowCount"`
}

type ExportHighWaterMark struct {
	RowCount    int        `json:"rowCount"`
	Complete    bool       `json:"complete"`
	CursorOrder string     `json:"cursorOrder"`
	FirstID     string     `json:"firstId,omitempty"`
	FirstTime   *time.Time `json:"firstTime,omitempty"`
	LastID      string     `json:"lastId,omitempty"`
	LastTime    *time.Time `json:"lastTime,omitempty"`
	NextCursor  string     `json:"nextCursor,omitempty"`
}

type collectionExport struct {
	RowCount    int
	FirstID     string
	FirstTime   time.Time
	LastID      string
	LastTime    time.Time
	NextCursor  string
	Complete    bool
	CursorOrder string
}

func WritePublicExport(ctx context.Context, store Store, outputDir string, options ExportOptions) (ExportResult, error) {
	if options.PageSize <= 0 {
		options.PageSize = 200
	}
	if options.PageSize > 200 {
		options.PageSize = 200
	}
	now := time.Now().UTC()
	if options.Now != nil {
		now = options.Now().UTC()
	}
	identity := model.ResponseMeta(options.RegistryID, options.APIVersion)
	cycleScope := exportCycleScopeLabel(options)
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return ExportResult{}, err
	}
	entityExport, err := writeEntityExport(ctx, store, filepath.Join(outputDir, "entities.jsonl"), options)
	if err != nil {
		return ExportResult{}, err
	}
	killmailExport, err := writeKillmailExport(ctx, store, filepath.Join(outputDir, "killmails.jsonl"), options)
	if err != nil {
		return ExportResult{}, err
	}
	sourceExport, err := writeSourceExport(ctx, store, filepath.Join(outputDir, "sources.jsonl"), options)
	if err != nil {
		return ExportResult{}, err
	}
	fileRows := map[string]int{
		"entities.jsonl":  entityExport.RowCount,
		"killmails.jsonl": killmailExport.RowCount,
		"sources.jsonl":   sourceExport.RowCount,
	}
	highWaterMarks := map[string]ExportHighWaterMark{
		"entities":  highWaterMark(entityExport),
		"killmails": highWaterMark(killmailExport),
		"sources":   highWaterMark(sourceExport),
	}
	dataFiles := []string{"entities.jsonl", "killmails.jsonl", "sources.jsonl"}
	eventExport := collectionExport{}
	if options.IncludeEvents {
		var err error
		eventExport, err = writeEventExport(ctx, store, filepath.Join(outputDir, "events.jsonl"), options)
		if err != nil {
			return ExportResult{}, err
		}
		dataFiles = append(dataFiles, "events.jsonl")
		fileRows["events.jsonl"] = eventExport.RowCount
		highWaterMarks["events"] = highWaterMark(eventExport)
	}
	suiObjectExport := collectionExport{}
	if options.IncludeSuiObjects {
		var err error
		suiObjectExport, err = writeSuiObjectExport(ctx, store, filepath.Join(outputDir, "sui_objects.jsonl"), options)
		if err != nil {
			return ExportResult{}, err
		}
		dataFiles = append(dataFiles, "sui_objects.jsonl")
		fileRows["sui_objects.jsonl"] = suiObjectExport.RowCount
		highWaterMarks["sui_objects"] = highWaterMark(suiObjectExport)
	}
	result := ExportResult{
		SchemaVersion:   "registry.export.v1",
		Registry:        identity.Registry,
		APIVersion:      identity.APIVersion,
		GeneratedAt:     now,
		CycleScope:      cycleScope,
		Cycles:          append([]int(nil), options.Cycles...),
		IncludeUncycled: options.IncludeUncycled,
		EntityCount:     entityExport.RowCount,
		KillmailCount:   killmailExport.RowCount,
		SourceCount:     sourceExport.RowCount,
		EventCount:      eventExport.RowCount,
		SuiObjectCount:  suiObjectExport.RowCount,
		Files:           append([]string{"catalog.json", "manifest.json"}, dataFiles...),
	}
	if err := writeJSON(filepath.Join(outputDir, "catalog.json"), result); err != nil {
		return ExportResult{}, err
	}
	database, err := store.ExportDatabaseIdentity(ctx)
	if err != nil {
		return ExportResult{}, err
	}
	fileRows["catalog.json"] = 1
	manifestFiles := []ExportFile{}
	for _, name := range append([]string{"catalog.json"}, dataFiles...) {
		metadata, err := exportFileMetadata(filepath.Join(outputDir, name), name, fileRows[name])
		if err != nil {
			return ExportResult{}, err
		}
		manifestFiles = append(manifestFiles, metadata)
	}
	manifest := ExportManifest{
		SchemaVersion:   "registry.export_manifest.v1",
		Registry:        identity.Registry,
		APIVersion:      identity.APIVersion,
		GeneratedAt:     now,
		CycleScope:      cycleScope,
		Cycles:          append([]int(nil), options.Cycles...),
		IncludeUncycled: options.IncludeUncycled,
		Database:        database,
		Files:           manifestFiles,
		HighWaterMarks:  highWaterMarks,
	}
	if err := writeJSON(filepath.Join(outputDir, "manifest.json"), manifest); err != nil {
		return ExportResult{}, err
	}
	return result, nil
}

func writeSourceExport(ctx context.Context, store Store, path string, options ExportOptions) (collectionExport, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return collectionExport{}, err
	}
	defer file.Close()
	encoder := json.NewEncoder(file)
	cursor := ""
	progress := newCollectionExport("created_at DESC, id DESC")
	for {
		limit := nextPageSize(options.Limit, options.PageSize, progress.RowCount)
		if limit == 0 {
			break
		}
		page, err := store.ListSourcesPage(ctx, db.SourceQuery{
			Cycles:          options.Cycles,
			IncludeUncycled: options.IncludeUncycled,
			Limit:           limit,
			Cursor:          cursor,
		})
		if err != nil {
			return progress, err
		}
		for _, item := range page.Items {
			if err := encoder.Encode(item); err != nil {
				return progress, err
			}
			progress.observe(item.ID, item.CreatedAt)
		}
		progress.NextCursor = page.NextCursor
		progress.Complete = page.NextCursor == ""
		if page.NextCursor == "" || len(page.Items) == 0 {
			break
		}
		cursor = page.NextCursor
	}
	return progress, nil
}

func writeEntityExport(ctx context.Context, store Store, path string, options ExportOptions) (collectionExport, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return collectionExport{}, err
	}
	defer file.Close()
	encoder := json.NewEncoder(file)
	cursor := ""
	progress := newCollectionExport("updated_at DESC, id DESC")
	for {
		limit := nextPageSize(options.Limit, options.PageSize, progress.RowCount)
		if limit == 0 {
			break
		}
		page, err := store.ListEntities(ctx, db.EntityQuery{
			Cycles:          options.Cycles,
			IncludeUncycled: options.IncludeUncycled,
			Limit:           limit,
			Cursor:          cursor,
		})
		if err != nil {
			return progress, err
		}
		for _, item := range page.Items {
			if err := encoder.Encode(item); err != nil {
				return progress, err
			}
			progress.observe(item.ID, item.UpdatedAt)
		}
		progress.NextCursor = page.NextCursor
		progress.Complete = page.NextCursor == ""
		if page.NextCursor == "" || len(page.Items) == 0 {
			break
		}
		cursor = page.NextCursor
	}
	return progress, nil
}

func writeKillmailExport(ctx context.Context, store Store, path string, options ExportOptions) (collectionExport, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return collectionExport{}, err
	}
	defer file.Close()
	encoder := json.NewEncoder(file)
	cursor := ""
	progress := newCollectionExport("occurred_at DESC, id DESC")
	for {
		limit := nextPageSize(options.Limit, options.PageSize, progress.RowCount)
		if limit == 0 {
			break
		}
		items, nextCursor, err := store.ListKillmailRaw(ctx, db.KillmailQuery{
			Cycles:          options.Cycles,
			IncludeUncycled: options.IncludeUncycled,
			Limit:           limit,
			Cursor:          cursor,
		})
		if err != nil {
			return progress, err
		}
		for _, item := range items {
			if err := encoder.Encode(item); err != nil {
				return progress, err
			}
			progress.observe(item.ID, item.OccurredAt)
		}
		progress.NextCursor = nextCursor
		progress.Complete = nextCursor == ""
		if nextCursor == "" || len(items) == 0 {
			break
		}
		cursor = nextCursor
	}
	return progress, nil
}

func writeEventExport(ctx context.Context, store Store, path string, options ExportOptions) (collectionExport, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return collectionExport{}, err
	}
	defer file.Close()
	encoder := json.NewEncoder(file)
	cursor := ""
	progress := newCollectionExport("occurred_at DESC, id DESC")
	for {
		limit := nextPageSize(options.Limit, options.PageSize, progress.RowCount)
		if limit == 0 {
			break
		}
		page, err := store.ListEvents(ctx, db.EventQuery{
			Cycles:          options.Cycles,
			IncludeUncycled: options.IncludeUncycled,
			Limit:           limit,
			Cursor:          cursor,
		})
		if err != nil {
			return progress, err
		}
		for _, item := range page.Items {
			if err := encoder.Encode(item); err != nil {
				return progress, err
			}
			progress.observe(item.ID, item.OccurredAt)
		}
		progress.NextCursor = page.NextCursor
		progress.Complete = page.NextCursor == ""
		if page.NextCursor == "" || len(page.Items) == 0 {
			break
		}
		cursor = page.NextCursor
	}
	return progress, nil
}

func writeSuiObjectExport(ctx context.Context, store Store, path string, options ExportOptions) (collectionExport, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return collectionExport{}, err
	}
	defer file.Close()
	encoder := json.NewEncoder(file)
	cursor := ""
	progress := newCollectionExport("observed_at ASC, id ASC")
	for {
		limit := nextPageSize(options.Limit, options.PageSize, progress.RowCount)
		if limit == 0 {
			break
		}
		page, err := store.ListSuiObjects(ctx, db.SuiObjectQuery{
			Cycles:          options.Cycles,
			IncludeUncycled: options.IncludeUncycled,
			Limit:           limit,
			Cursor:          cursor,
		})
		if err != nil {
			return progress, err
		}
		for _, item := range page.Items {
			if err := encoder.Encode(item); err != nil {
				return progress, err
			}
			progress.observe(item.ID, item.ObservedAt)
		}
		progress.NextCursor = page.NextCursor
		progress.Complete = page.NextCursor == ""
		if page.NextCursor == "" || len(page.Items) == 0 {
			break
		}
		cursor = page.NextCursor
	}
	return progress, nil
}

func nextPageSize(limit, pageSize, count int) int {
	if limit <= 0 {
		return pageSize
	}
	remaining := limit - count
	if remaining <= 0 {
		return 0
	}
	if remaining < pageSize {
		return remaining
	}
	return pageSize
}

func writeJSON(path string, value any) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func newCollectionExport(order string) collectionExport {
	return collectionExport{Complete: true, CursorOrder: order}
}

func (s *collectionExport) observe(id string, timestamp time.Time) {
	if s.RowCount == 0 {
		s.FirstID = id
		s.FirstTime = timestamp
	}
	s.RowCount++
	s.LastID = id
	s.LastTime = timestamp
}

func highWaterMark(progress collectionExport) ExportHighWaterMark {
	mark := ExportHighWaterMark{
		RowCount:    progress.RowCount,
		Complete:    progress.Complete,
		CursorOrder: progress.CursorOrder,
		FirstID:     progress.FirstID,
		LastID:      progress.LastID,
		NextCursor:  progress.NextCursor,
	}
	if !progress.FirstTime.IsZero() {
		first := progress.FirstTime
		mark.FirstTime = &first
	}
	if !progress.LastTime.IsZero() {
		last := progress.LastTime
		mark.LastTime = &last
	}
	return mark
}

func exportCycleScopeLabel(options ExportOptions) string {
	if options.CycleScope != "" {
		return options.CycleScope
	}
	if len(options.Cycles) == 0 {
		return "all"
	}
	return "custom"
}

func exportFileMetadata(path, name string, rowCount int) (ExportFile, error) {
	file, err := os.Open(path)
	if err != nil {
		return ExportFile{}, err
	}
	defer file.Close()
	hash := sha256.New()
	size, err := io.Copy(hash, file)
	if err != nil {
		return ExportFile{}, err
	}
	return ExportFile{
		Path:        name,
		ContentType: contentTypeForExportFile(name),
		SHA256:      hex.EncodeToString(hash.Sum(nil)),
		SizeBytes:   size,
		RowCount:    rowCount,
	}, nil
}

func contentTypeForExportFile(name string) string {
	if filepath.Ext(name) == ".jsonl" {
		return "application/x-ndjson"
	}
	return "application/json"
}
