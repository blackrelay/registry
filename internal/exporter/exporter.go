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
	"github.com/blackrelay/registry/internal/killmail"
	"github.com/blackrelay/registry/internal/model"
	"github.com/blackrelay/registry/internal/resolver"
	"github.com/blackrelay/registry/internal/sui"
)

type Store interface {
	ListEntities(ctx context.Context, query db.EntityQuery) (db.EntityPage, error)
	ListCurrentEntities(ctx context.Context, query db.CurrentEntityQuery) (db.CurrentEntityPage, error)
	ListCurrentRelations(ctx context.Context, query db.CurrentRelationQuery) (db.CurrentRelationPage, error)
	GetEntity(ctx context.Context, idOrSlug string) (model.Entity, bool, error)
	ListEntityFacts(ctx context.Context, entityID string) ([]model.Fact, error)
	ListEntityRelations(ctx context.Context, entityID string) ([]model.Relation, error)
	ListEntitySources(ctx context.Context, entityID string) ([]model.Source, error)
	ListEvents(ctx context.Context, query db.EventQuery) (db.EventPage, error)
	ListKillmailRaw(ctx context.Context, query db.KillmailQuery) ([]model.KillmailRaw, string, error)
	ResolveCharacter(ctx context.Context, idOrName string, environment model.Environment) (model.ResolvedValue, bool, error)
	ResolveEnemyType(ctx context.Context, typeID string, environment model.Environment) (model.ResolvedValue, bool, error)
	ResolveSystem(ctx context.Context, idOrName string, environment model.Environment) (model.ResolvedValue, bool, error)
	ListSourceArtefactsPage(ctx context.Context, query db.SourceArtefactQuery) (db.SourceArtefactPage, error)
	ListSuiObjects(ctx context.Context, query db.SuiObjectQuery) (db.SuiObjectPage, error)
	ListFreshness(ctx context.Context) ([]db.FreshnessStatus, error)
	ListCursors(ctx context.Context) ([]db.CursorStatus, error)
	ListSourceGaps(ctx context.Context, environment model.Environment) ([]model.SourceGap, error)
	ListSources(ctx context.Context, limit int) ([]model.Source, error)
	ListSourcesPage(ctx context.Context, query db.SourceQuery) (db.SourcePage, error)
	ExportDatabaseIdentity(ctx context.Context) (db.DatabaseIdentity, error)
}

type entitySourceExport struct {
	EntityID string       `json:"entityId"`
	Source   model.Source `json:"source"`
}

type killmailExportRow struct {
	model.KillmailRaw
	Kind        string              `json:"kind"`
	System      model.ResolvedValue `json:"system"`
	Victim      model.ResolvedValue `json:"victim"`
	Killer      model.ResolvedValue `json:"killer"`
	Reporter    model.ResolvedValue `json:"reporter"`
	SummaryText string              `json:"summaryText,omitempty"`
	Sources     []string            `json:"sources,omitempty"`
	Warnings    []string            `json:"warnings,omitempty"`
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
	FactCount       int       `json:"factCount"`
	RelationCount   int       `json:"relationCount"`
	ArtefactCount   int       `json:"artefactCount"`
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
	factExport, relationExport, entitySourceExport, err := writeEntityProvenanceExports(ctx, store, outputDir, options)
	if err != nil {
		return ExportResult{}, err
	}
	artefactExport, err := writeSourceArtefactExport(ctx, store, filepath.Join(outputDir, "source_artefacts.jsonl"), options)
	if err != nil {
		return ExportResult{}, err
	}
	currentEntityExport, err := writeCurrentEntityExport(ctx, store, filepath.Join(outputDir, "current_entities.jsonl"), options)
	if err != nil {
		return ExportResult{}, err
	}
	currentRelationExport, err := writeCurrentRelationExport(ctx, store, filepath.Join(outputDir, "current_relations.jsonl"), options)
	if err != nil {
		return ExportResult{}, err
	}
	opsRows, err := writeOpsDocuments(ctx, store, outputDir)
	if err != nil {
		return ExportResult{}, err
	}
	fileRows := map[string]int{
		"entities.jsonl":          entityExport.RowCount,
		"killmails.jsonl":         killmailExport.RowCount,
		"sources.jsonl":           sourceExport.RowCount,
		"facts.jsonl":             factExport.RowCount,
		"relations.jsonl":         relationExport.RowCount,
		"entity_sources.jsonl":    entitySourceExport.RowCount,
		"source_artefacts.jsonl":  artefactExport.RowCount,
		"current_entities.jsonl":  currentEntityExport.RowCount,
		"current_relations.jsonl": currentRelationExport.RowCount,
		"ops_freshness.json":      opsRows["ops_freshness.json"],
		"ops_cursors.json":        opsRows["ops_cursors.json"],
		"ops_sui_coverage.json":   opsRows["ops_sui_coverage.json"],
		"ops_source_gaps.json":    opsRows["ops_source_gaps.json"],
	}
	highWaterMarks := map[string]ExportHighWaterMark{
		"entities":          highWaterMark(entityExport),
		"killmails":         highWaterMark(killmailExport),
		"sources":           highWaterMark(sourceExport),
		"facts":             highWaterMark(factExport),
		"relations":         highWaterMark(relationExport),
		"entity_sources":    highWaterMark(entitySourceExport),
		"source_artefacts":  highWaterMark(artefactExport),
		"current_entities":  highWaterMark(currentEntityExport),
		"current_relations": highWaterMark(currentRelationExport),
	}
	dataFiles := []string{
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
	}
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
		FactCount:       factExport.RowCount,
		RelationCount:   relationExport.RowCount,
		ArtefactCount:   artefactExport.RowCount,
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
			return closeCollectionExport(file, progress, err)
		}
		for _, item := range page.Items {
			if err := encoder.Encode(item); err != nil {
				return closeCollectionExport(file, progress, err)
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
	return closeCollectionExport(file, progress, nil)
}

func writeEntityExport(ctx context.Context, store Store, path string, options ExportOptions) (collectionExport, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return collectionExport{}, err
	}
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
			return closeCollectionExport(file, progress, err)
		}
		for _, item := range page.Items {
			if err := encoder.Encode(item); err != nil {
				return closeCollectionExport(file, progress, err)
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
	return closeCollectionExport(file, progress, nil)
}

func writeKillmailExport(ctx context.Context, store Store, path string, options ExportOptions) (collectionExport, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return collectionExport{}, err
	}
	encoder := json.NewEncoder(file)
	semanticService := killmail.Service{
		Resolver:   resolver.Resolver{Store: store},
		GraphStore: store,
	}
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
			return closeCollectionExport(file, progress, err)
		}
		for _, item := range items {
			semantic := semanticService.Semantic(ctx, item)
			if err := encoder.Encode(killmailExportRow{
				KillmailRaw: item,
				Kind:        semantic.Kind,
				System:      semantic.System,
				Victim:      semantic.Victim,
				Killer:      semantic.Killer,
				Reporter:    semantic.Reporter,
				SummaryText: semantic.SummaryText,
				Sources:     semantic.Sources,
				Warnings:    semantic.Warnings,
			}); err != nil {
				return closeCollectionExport(file, progress, err)
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
	return closeCollectionExport(file, progress, nil)
}

func writeEventExport(ctx context.Context, store Store, path string, options ExportOptions) (collectionExport, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return collectionExport{}, err
	}
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
			return closeCollectionExport(file, progress, err)
		}
		for _, item := range page.Items {
			if err := encoder.Encode(item); err != nil {
				return closeCollectionExport(file, progress, err)
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
	return closeCollectionExport(file, progress, nil)
}

func writeSuiObjectExport(ctx context.Context, store Store, path string, options ExportOptions) (collectionExport, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return collectionExport{}, err
	}
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
			return closeCollectionExport(file, progress, err)
		}
		for _, item := range page.Items {
			if err := encoder.Encode(item); err != nil {
				return closeCollectionExport(file, progress, err)
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
	return closeCollectionExport(file, progress, nil)
}

func writeEntityProvenanceExports(ctx context.Context, store Store, outputDir string, options ExportOptions) (collectionExport, collectionExport, collectionExport, error) {
	factFile, err := os.OpenFile(filepath.Join(outputDir, "facts.jsonl"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return collectionExport{}, collectionExport{}, collectionExport{}, err
	}
	relationFile, err := os.OpenFile(filepath.Join(outputDir, "relations.jsonl"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return collectionExport{}, collectionExport{}, collectionExport{}, closeFile(factFile, err)
	}
	entitySourceFile, err := os.OpenFile(filepath.Join(outputDir, "entity_sources.jsonl"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		_ = closeFile(relationFile, nil)
		return collectionExport{}, collectionExport{}, collectionExport{}, closeFile(factFile, err)
	}
	factEncoder := json.NewEncoder(factFile)
	relationEncoder := json.NewEncoder(relationFile)
	entitySourceEncoder := json.NewEncoder(entitySourceFile)
	factProgress := newCollectionExport("entity_id ASC, key ASC, source_id ASC")
	relationProgress := newCollectionExport("subject_entity_id ASC, predicate ASC, object_entity_id ASC, source_id ASC")
	entitySourceProgress := newCollectionExport("entity_id ASC, source_id ASC")
	seenRelations := map[string]struct{}{}
	seenEntitySources := map[string]struct{}{}
	cursor := ""
	for {
		limit := nextPageSize(options.Limit, options.PageSize, factProgress.RowCount)
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
			return factProgress, relationProgress, entitySourceProgress, closeThree(factFile, relationFile, entitySourceFile, err)
		}
		for _, entity := range page.Items {
			facts, err := store.ListEntityFacts(ctx, entity.ID)
			if err != nil {
				return factProgress, relationProgress, entitySourceProgress, closeThree(factFile, relationFile, entitySourceFile, err)
			}
			for _, fact := range facts {
				if err := factEncoder.Encode(fact); err != nil {
					return factProgress, relationProgress, entitySourceProgress, closeThree(factFile, relationFile, entitySourceFile, err)
				}
				factProgress.observe(fact.EntityID+":"+fact.Key+":"+fact.SourceID, time.Time{})
			}
			relations, err := store.ListEntityRelations(ctx, entity.ID)
			if err != nil {
				return factProgress, relationProgress, entitySourceProgress, closeThree(factFile, relationFile, entitySourceFile, err)
			}
			for _, relation := range relations {
				key := relationExportKey(relation)
				if _, ok := seenRelations[key]; ok {
					continue
				}
				seenRelations[key] = struct{}{}
				if err := relationEncoder.Encode(relation); err != nil {
					return factProgress, relationProgress, entitySourceProgress, closeThree(factFile, relationFile, entitySourceFile, err)
				}
				relationProgress.observe(key, time.Time{})
			}
			sources, err := store.ListEntitySources(ctx, entity.ID)
			if err != nil {
				return factProgress, relationProgress, entitySourceProgress, closeThree(factFile, relationFile, entitySourceFile, err)
			}
			for _, source := range sources {
				key := entity.ID + ":" + source.ID
				if _, ok := seenEntitySources[key]; ok {
					continue
				}
				seenEntitySources[key] = struct{}{}
				item := entitySourceExport{EntityID: entity.ID, Source: source}
				if err := entitySourceEncoder.Encode(item); err != nil {
					return factProgress, relationProgress, entitySourceProgress, closeThree(factFile, relationFile, entitySourceFile, err)
				}
				entitySourceProgress.observe(key, time.Time{})
			}
		}
		factProgress.NextCursor = page.NextCursor
		relationProgress.NextCursor = page.NextCursor
		entitySourceProgress.NextCursor = page.NextCursor
		complete := page.NextCursor == ""
		factProgress.Complete = complete
		relationProgress.Complete = complete
		entitySourceProgress.Complete = complete
		if page.NextCursor == "" || len(page.Items) == 0 {
			break
		}
		cursor = page.NextCursor
	}
	return factProgress, relationProgress, entitySourceProgress, closeThree(factFile, relationFile, entitySourceFile, nil)
}

func writeSourceArtefactExport(ctx context.Context, store Store, path string, options ExportOptions) (collectionExport, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return collectionExport{}, err
	}
	encoder := json.NewEncoder(file)
	cursor := ""
	progress := newCollectionExport("created_at DESC, id DESC")
	for {
		limit := nextPageSize(options.Limit, options.PageSize, progress.RowCount)
		if limit == 0 {
			break
		}
		page, err := store.ListSourceArtefactsPage(ctx, db.SourceArtefactQuery{
			Cycles:          options.Cycles,
			IncludeUncycled: options.IncludeUncycled,
			Limit:           limit,
			Cursor:          cursor,
		})
		if err != nil {
			return closeCollectionExport(file, progress, err)
		}
		for _, item := range page.Items {
			if err := encoder.Encode(item); err != nil {
				return closeCollectionExport(file, progress, err)
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
	return closeCollectionExport(file, progress, nil)
}

func writeCurrentEntityExport(ctx context.Context, store Store, path string, options ExportOptions) (collectionExport, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return collectionExport{}, err
	}
	encoder := json.NewEncoder(file)
	progress := newCollectionExport("type ASC, display_name ASC, entity_id ASC")
	for _, entityType := range currentEntityExportTypes() {
		cursor := ""
		for {
			limit := nextPageSize(options.Limit, options.PageSize, progress.RowCount)
			if limit == 0 {
				return closeCollectionExport(file, progress, nil)
			}
			page, err := store.ListCurrentEntities(ctx, db.CurrentEntityQuery{
				Type:            entityType,
				Cycles:          options.Cycles,
				IncludeUncycled: options.IncludeUncycled,
				Limit:           limit,
				Cursor:          cursor,
			})
			if err != nil {
				return closeCollectionExport(file, progress, err)
			}
			for _, item := range page.Items {
				if err := encoder.Encode(item); err != nil {
					return closeCollectionExport(file, progress, err)
				}
				progress.observe(string(entityType)+":"+item.Entity.ID, item.Entity.UpdatedAt)
			}
			progress.NextCursor = page.NextCursor
			progress.Complete = page.NextCursor == ""
			if page.NextCursor == "" || len(page.Items) == 0 {
				break
			}
			cursor = page.NextCursor
		}
	}
	progress.NextCursor = ""
	progress.Complete = true
	return closeCollectionExport(file, progress, nil)
}

func writeCurrentRelationExport(ctx context.Context, store Store, path string, options ExportOptions) (collectionExport, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return collectionExport{}, err
	}
	encoder := json.NewEncoder(file)
	progress := newCollectionExport("predicate ASC, subject_entity_id ASC, object_entity_id ASC")
	for _, predicates := range [][]string{{"owned_by"}, {"links_to", "observed_between"}} {
		cursor := ""
		for {
			limit := nextPageSize(options.Limit, options.PageSize, progress.RowCount)
			if limit == 0 {
				return closeCollectionExport(file, progress, nil)
			}
			page, err := store.ListCurrentRelations(ctx, db.CurrentRelationQuery{
				Predicates:      predicates,
				Cycles:          options.Cycles,
				IncludeUncycled: options.IncludeUncycled,
				Limit:           limit,
				Cursor:          cursor,
			})
			if err != nil {
				return closeCollectionExport(file, progress, err)
			}
			for _, item := range page.Items {
				if err := encoder.Encode(item); err != nil {
					return closeCollectionExport(file, progress, err)
				}
				progress.observe(nonEmpty(item.ID, relationExportKey(model.Relation{
					SubjectEntityID: item.SubjectEntityID,
					Predicate:       item.Predicate,
					ObjectEntityID:  item.ObjectEntityID,
					SourceID:        item.SourceID,
				})), item.CreatedAt)
			}
			progress.NextCursor = page.NextCursor
			progress.Complete = page.NextCursor == ""
			if page.NextCursor == "" || len(page.Items) == 0 {
				break
			}
			cursor = page.NextCursor
		}
	}
	progress.NextCursor = ""
	progress.Complete = true
	return closeCollectionExport(file, progress, nil)
}

func writeOpsDocuments(ctx context.Context, store Store, outputDir string) (map[string]int, error) {
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return nil, err
	}
	freshness, err := store.ListFreshness(ctx)
	if err != nil {
		return nil, err
	}
	cursors, err := store.ListCursors(ctx)
	if err != nil {
		return nil, err
	}
	sourceGaps, err := store.ListSourceGaps(ctx, model.EnvironmentStillness)
	if err != nil {
		return nil, err
	}
	documents := map[string]any{
		"ops_freshness.json":    map[string]any{"data": freshness},
		"ops_cursors.json":      map[string]any{"data": cursors},
		"ops_sui_coverage.json": map[string]any{"data": sui.CursorCoverageSummary(cursors)},
		"ops_source_gaps.json":  map[string]any{"data": sourceGaps},
	}
	rows := make(map[string]int, len(documents))
	for name, body := range documents {
		path := filepath.Join(outputDir, name)
		if err := writeJSON(path, body); err != nil {
			return nil, err
		}
		rows[name] = 1
	}
	return rows, nil
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
		return closeFile(file, err)
	}
	return closeFile(file, nil)
}

func closeCollectionExport(file *os.File, progress collectionExport, err error) (collectionExport, error) {
	if closeErr := file.Close(); err == nil && closeErr != nil {
		return progress, closeErr
	}
	return progress, err
}

func closeFile(file *os.File, err error) error {
	if closeErr := file.Close(); err == nil && closeErr != nil {
		return closeErr
	}
	return err
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

func relationExportKey(relation model.Relation) string {
	return relation.SubjectEntityID + ":" + relation.Predicate + ":" + relation.ObjectEntityID + ":" + relation.SourceID
}

func nonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func currentEntityExportTypes() []model.EntityType {
	return []model.EntityType{
		model.EntityTypeCharacter,
		model.EntityTypeTribe,
		model.EntityTypeAssembly,
		model.EntityTypeGate,
		model.EntityTypeStorage,
		model.EntityTypeTurret,
		model.EntityTypeRegion,
		model.EntityTypeConstellation,
		model.EntityTypeItem,
		model.EntityTypeMaterial,
		model.EntityTypeEnemy,
		model.EntityTypeRecipe,
		model.EntityTypeBlueprint,
		model.EntityTypeShip,
		model.EntityTypeStructure,
		model.EntityTypeSystem,
		model.EntityTypeRoute,
	}
}

func closeThree(first, second, third *os.File, err error) error {
	err = closeFile(third, err)
	err = closeFile(second, err)
	return closeFile(first, err)
}
