package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/blackrelay/registry/internal/config"
	"github.com/blackrelay/registry/internal/cursor"
	cyclepkg "github.com/blackrelay/registry/internal/cycles"
	"github.com/blackrelay/registry/internal/db"
	"github.com/blackrelay/registry/internal/model"
	"github.com/blackrelay/registry/internal/report"
	"github.com/blackrelay/registry/internal/sui"
	"github.com/jackc/pgx/v5"
)

type listFlag []string

func (f *listFlag) String() string {
	return strings.Join(*f, ",")
}

func (f *listFlag) Set(value string) error {
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			*f = append(*f, part)
		}
	}
	return nil
}

type rangeBlockedObjectAudit struct {
	SchemaVersion string                 `json:"schemaVersion"`
	Environment   model.Environment      `json:"environment"`
	Network       string                 `json:"network"`
	Count         int                    `json:"count"`
	Targets       []sui.ObjectTypeTarget `json:"targets"`
}

type characterObjectRepairSummary struct {
	Environment      model.Environment `json:"environment"`
	Network          string            `json:"network"`
	Endpoint         string            `json:"endpoint"`
	Candidates       int               `json:"candidates"`
	ObjectsFetched   int64             `json:"objectsFetched"`
	ObjectsReplayed  int64             `json:"objectsReplayed"`
	ObjectsMissing   int64             `json:"objectsMissing"`
	EntitiesDerived  int64             `json:"entitiesDerived"`
	RelationsDerived int64             `json:"relationsDerived"`
	KillmailsDerived int64             `json:"killmailsDerived"`
	Errors           int64             `json:"errors"`
	StartedAt        time.Time         `json:"startedAt"`
	FinishedAt       time.Time         `json:"finishedAt"`
}

type characterObjectRef struct {
	EntityID        string
	ObjectID        string
	HasStoredObject bool
}

func main() {
	cfg := config.Load()
	manifestPath := flag.String("manifest", "testdata/fixtures/sui-packages.stillness.json", "Sui package manifest path")
	mode := flag.String("mode", "plan", "indexer mode: plan, audit, audit-stillness, report, status, audit-killmails, audit-current-state, audit-character-profiles, audit-tribe-identity-evidence, audit-evidence-bridges, audit-object-shapes, audit-systems, audit-range-blocked-objects, events, objects, retry-range-blocked-objects, repair-character-objects, all, derive-events, derive-objects or resolve-evidence")
	environment := flag.String("environment", string(model.EnvironmentStillness), "registry environment")
	network := flag.String("network", "sui-testnet", "Sui network from the package manifest")
	endpoint := flag.String("endpoint", "https://graphql.testnet.sui.io/graphql", "Sui GraphQL endpoint")
	cycleValue := flag.String("cycles", "current", "cycle scope: current or 6")
	databaseURL := flag.String("database-url", cfg.DatabaseURL, "PostgreSQL connection string")
	staticUniversePath := flag.String("static-universe-path", "", "static-client universe extraction directory for audit-systems")
	exportManifestPath := flag.String("export-manifest", "", "optional public export manifest path included in status output")
	first := flag.Int("first", 50, "items per GraphQL page")
	maxPages := flag.Int("max-pages", 1, "maximum pages per stream; zero means full backfill")
	maxBatches := flag.Int("max-batches", 0, "maximum derive batches; zero means all available rows")
	deriveBatchSize := flag.Int("derive-batch-size", 1000, "stored Sui object rows per derive batch")
	concurrency := flag.Int("concurrency", 8, "maximum concurrent Sui streams")
	checkpointFrom := flag.Uint64("checkpoint-from", 0, "first checkpoint for explicit event-type shards")
	checkpointTo := flag.Uint64("checkpoint-to", 0, "last checkpoint for explicit event-type shards")
	checkpointShardSize := flag.Uint64("checkpoint-shard-size", 0, "checkpoint range size for explicit event-type shards; zero disables checkpoint sharding")
	retries := flag.Int("retries", 5, "retry attempts for retryable Sui GraphQL failures")
	retryBase := flag.Duration("retry-base", 750*time.Millisecond, "base retry delay")
	retryJitter := flag.Duration("retry-jitter", 500*time.Millisecond, "maximum retry jitter")
	resetCursors := flag.Bool("reset-cursors", false, "ignore saved Sui cursors for this run")
	onlyIncomplete := flag.Bool("only-incomplete", false, "only run Sui streams with no cursor or a cursor error")
	allowObjectTargetErrors := flag.Bool("allow-object-target-errors", false, "treat per-object-type Sui GraphQL errors as recorded cursor errors instead of failing the whole object backfill")
	noModuleShards := flag.Bool("no-module-shards", false, "scan one package-level event stream per package")
	excludeFixtures := flag.Bool("exclude-fixtures", false, "exclude fixture/proof killmails from report and audit views")
	sampleLimit := flag.Int("sample-limit", 10, "sample rows to include in audit outputs")
	shapeLimit := flag.Int("shape-limit", 1000, "stored Sui object rows to inspect in object-shape audits")
	statusStaleAfter := flag.Duration("status-stale-after", 15*time.Minute, "cursor age after which status mode reports stale")
	migrate := flag.Bool("migrate", true, "apply PostgreSQL migrations before indexing")
	var packageIDs listFlag
	var moduleNames listFlag
	var eventTypes listFlag
	var objectTypeNames listFlag
	var objectTypeReprs listFlag
	flag.Var(&packageIDs, "package", "restrict to one package ID; repeat or comma-separate")
	flag.Var(&moduleNames, "module", "restrict to one module name; repeat or comma-separate")
	flag.Var(&eventTypes, "event-type", "restrict event backfill to one exact Move event type; repeat or comma-separate")
	flag.Var(&objectTypeNames, "object-type-name", "restrict object backfill to one Move type name such as PlayerProfile; repeat or comma-separate")
	flag.Var(&objectTypeReprs, "object-type", "restrict object backfill to one full Move type such as 0x...::character::PlayerProfile; repeat or comma-separate")
	flag.Parse()
	maxPagesExplicit := false
	flag.Visit(func(item *flag.Flag) {
		if item.Name == "max-pages" {
			maxPagesExplicit = true
		}
	})
	cycleScope, err := indexerCycleScope(*cycleValue)
	if err != nil {
		slog.Error("parse cycle scope", "error", err)
		os.Exit(2)
	}

	if *mode == "derive-events" {
		runDeriveEvents(indexOptions{
			environment:     model.Environment(*environment),
			network:         *network,
			databaseURL:     *databaseURL,
			cycles:          cycleScope.Cycles,
			includeUncycled: cycleScope.IncludeUncycled,
			resetCursors:    *resetCursors,
			migrate:         *migrate,
			moduleNames:     moduleNames,
			deriveBatchSize: *deriveBatchSize,
			maxBatches:      *maxBatches,
		})
		return
	}
	if *mode == "derive-objects" {
		runDeriveObjects(indexOptions{
			environment:     model.Environment(*environment),
			network:         *network,
			databaseURL:     *databaseURL,
			cycles:          cycleScope.Cycles,
			includeUncycled: cycleScope.IncludeUncycled,
			resetCursors:    *resetCursors,
			migrate:         *migrate,
			deriveBatchSize: *deriveBatchSize,
			maxBatches:      *maxBatches,
		})
		return
	}
	if *mode == "resolve-evidence" {
		runResolveEvidence(indexOptions{
			environment: model.Environment(*environment),
			network:     *network,
			databaseURL: *databaseURL,
			migrate:     *migrate,
		})
		return
	}
	if *mode == "repair-character-objects" {
		runRepairCharacterObjects(indexOptions{
			environment:     model.Environment(*environment),
			network:         *network,
			endpoint:        *endpoint,
			databaseURL:     *databaseURL,
			cycles:          cycleScope.Cycles,
			includeUncycled: cycleScope.IncludeUncycled,
			concurrency:     *concurrency,
			retries:         *retries,
			retryBase:       *retryBase,
			retryJitter:     *retryJitter,
			deriveBatchSize: *deriveBatchSize,
			migrate:         *migrate,
		})
		return
	}
	if *mode == "report" {
		runReport(indexOptions{
			environment:     model.Environment(*environment),
			network:         *network,
			databaseURL:     *databaseURL,
			excludeFixtures: *excludeFixtures,
			sampleLimit:     *sampleLimit,
			migrate:         *migrate,
		})
		return
	}
	if *mode == "status" {
		runIndexerStatus(indexOptions{
			environment:        model.Environment(*environment),
			network:            *network,
			databaseURL:        *databaseURL,
			migrate:            *migrate,
			statusStaleAfter:   *statusStaleAfter,
			exportManifestPath: *exportManifestPath,
		})
		return
	}
	if *mode == "audit-killmails" {
		runKillmailAudit(indexOptions{
			environment:     model.Environment(*environment),
			network:         *network,
			databaseURL:     *databaseURL,
			excludeFixtures: *excludeFixtures,
			sampleLimit:     *sampleLimit,
			migrate:         *migrate,
		})
		return
	}
	if *mode == "audit-current-state" {
		runCurrentStateAudit(indexOptions{
			environment: model.Environment(*environment),
			network:     *network,
			databaseURL: *databaseURL,
			sampleLimit: *sampleLimit,
			migrate:     *migrate,
		})
		return
	}
	if *mode == "audit-character-profiles" {
		runCharacterProfileAudit(indexOptions{
			environment: model.Environment(*environment),
			network:     *network,
			databaseURL: *databaseURL,
			sampleLimit: *sampleLimit,
			migrate:     *migrate,
		})
		return
	}
	if *mode == "audit-tribe-identity-evidence" {
		runTribeIdentityEvidenceAudit(indexOptions{
			environment:     model.Environment(*environment),
			network:         *network,
			databaseURL:     *databaseURL,
			moduleNames:     moduleNames,
			objectTypeNames: objectTypeNames,
			objectTypeReprs: objectTypeReprs,
			sampleLimit:     *sampleLimit,
			migrate:         *migrate,
		})
		return
	}
	if *mode == "audit-evidence-bridges" {
		runEvidenceBridgeAudit(indexOptions{
			environment: model.Environment(*environment),
			network:     *network,
			databaseURL: *databaseURL,
			sampleLimit: *sampleLimit,
			migrate:     *migrate,
		})
		return
	}
	if *mode == "audit-object-shapes" {
		runObjectShapeAudit(indexOptions{
			environment:     model.Environment(*environment),
			network:         *network,
			databaseURL:     *databaseURL,
			packageIDs:      packageIDs,
			moduleNames:     moduleNames,
			objectTypeNames: objectTypeNames,
			objectTypeReprs: objectTypeReprs,
			sampleLimit:     *sampleLimit,
			shapeLimit:      *shapeLimit,
			migrate:         *migrate,
		})
		return
	}
	if *mode == "audit-systems" {
		runSystemReconciliation(indexOptions{
			environment:        model.Environment(*environment),
			network:            *network,
			databaseURL:        *databaseURL,
			staticUniversePath: *staticUniversePath,
			migrate:            *migrate,
		})
		return
	}

	manifest, err := sui.LoadManifest(*manifestPath)
	if err != nil {
		slog.Error("load Sui manifest", "error", err)
		os.Exit(1)
	}
	switch *mode {
	case "plan":
		runPlan(manifest, *environment, *network, cycleScope, *maxPages, *concurrency, packageIDs, moduleNames, eventTypes, objectTypeNames, objectTypeReprs, *checkpointFrom, *checkpointTo, *checkpointShardSize, *noModuleShards)
	case "audit", "audit-stillness":
		if *mode == "audit-stillness" {
			*environment = string(model.EnvironmentStillness)
		}
		auditMaxPages := 0
		if maxPagesExplicit {
			auditMaxPages = *maxPages
		}
		runAudit(manifest, indexOptions{
			environment:         model.Environment(*environment),
			network:             *network,
			databaseURL:         *databaseURL,
			cycles:              cycleScope.Cycles,
			includeUncycled:     cycleScope.IncludeUncycled,
			maxPages:            auditMaxPages,
			migrate:             *migrate,
			packageIDs:          packageIDs,
			moduleNames:         moduleNames,
			eventTypes:          eventTypes,
			objectTypeNames:     objectTypeNames,
			objectTypeReprs:     objectTypeReprs,
			checkpointFrom:      *checkpointFrom,
			checkpointTo:        *checkpointTo,
			checkpointShardSize: *checkpointShardSize,
			noModuleShards:      *noModuleShards,
		})
	case "events":
		runEvents(manifest, indexOptions{
			environment:             model.Environment(*environment),
			network:                 *network,
			endpoint:                *endpoint,
			databaseURL:             *databaseURL,
			cycles:                  cycleScope.Cycles,
			includeUncycled:         cycleScope.IncludeUncycled,
			first:                   *first,
			maxPages:                *maxPages,
			concurrency:             *concurrency,
			retries:                 *retries,
			retryBase:               *retryBase,
			retryJitter:             *retryJitter,
			resetCursors:            *resetCursors,
			onlyIncomplete:          *onlyIncomplete,
			allowObjectTargetErrors: *allowObjectTargetErrors,
			noModuleShards:          *noModuleShards,
			migrate:                 *migrate,
			packageIDs:              packageIDs,
			moduleNames:             moduleNames,
			eventTypes:              eventTypes,
			objectTypeNames:         objectTypeNames,
			objectTypeReprs:         objectTypeReprs,
			checkpointFrom:          *checkpointFrom,
			checkpointTo:            *checkpointTo,
			checkpointShardSize:     *checkpointShardSize,
			deriveBatchSize:         *deriveBatchSize,
			maxBatches:              *maxBatches,
		})
	case "objects":
		runObjects(manifest, indexOptions{
			environment:             model.Environment(*environment),
			network:                 *network,
			endpoint:                *endpoint,
			databaseURL:             *databaseURL,
			cycles:                  cycleScope.Cycles,
			includeUncycled:         cycleScope.IncludeUncycled,
			first:                   *first,
			maxPages:                *maxPages,
			concurrency:             *concurrency,
			retries:                 *retries,
			retryBase:               *retryBase,
			retryJitter:             *retryJitter,
			resetCursors:            *resetCursors,
			onlyIncomplete:          *onlyIncomplete,
			allowObjectTargetErrors: *allowObjectTargetErrors,
			noModuleShards:          *noModuleShards,
			migrate:                 *migrate,
			packageIDs:              packageIDs,
			moduleNames:             moduleNames,
			eventTypes:              eventTypes,
			objectTypeNames:         objectTypeNames,
			objectTypeReprs:         objectTypeReprs,
			checkpointFrom:          *checkpointFrom,
			checkpointTo:            *checkpointTo,
			checkpointShardSize:     *checkpointShardSize,
			deriveBatchSize:         *deriveBatchSize,
			maxBatches:              *maxBatches,
		})
	case "retry-range-blocked-objects":
		runRetryRangeBlockedObjects(manifest, indexOptions{
			environment:             model.Environment(*environment),
			network:                 *network,
			endpoint:                *endpoint,
			databaseURL:             *databaseURL,
			cycles:                  cycleScope.Cycles,
			includeUncycled:         cycleScope.IncludeUncycled,
			first:                   *first,
			maxPages:                *maxPages,
			concurrency:             *concurrency,
			retries:                 *retries,
			retryBase:               *retryBase,
			retryJitter:             *retryJitter,
			resetCursors:            *resetCursors,
			allowObjectTargetErrors: true,
			migrate:                 *migrate,
			packageIDs:              packageIDs,
			moduleNames:             moduleNames,
			objectTypeNames:         objectTypeNames,
			objectTypeReprs:         objectTypeReprs,
		})
	case "audit-range-blocked-objects":
		runRangeBlockedObjectAudit(manifest, indexOptions{
			environment:     model.Environment(*environment),
			network:         *network,
			databaseURL:     *databaseURL,
			cycles:          cycleScope.Cycles,
			includeUncycled: cycleScope.IncludeUncycled,
			migrate:         *migrate,
			packageIDs:      packageIDs,
			moduleNames:     moduleNames,
			objectTypeNames: objectTypeNames,
			objectTypeReprs: objectTypeReprs,
		})
	case "all":
		runAll(manifest, indexOptions{
			environment:             model.Environment(*environment),
			network:                 *network,
			endpoint:                *endpoint,
			databaseURL:             *databaseURL,
			cycles:                  cycleScope.Cycles,
			includeUncycled:         cycleScope.IncludeUncycled,
			first:                   *first,
			maxPages:                *maxPages,
			concurrency:             *concurrency,
			retries:                 *retries,
			retryBase:               *retryBase,
			retryJitter:             *retryJitter,
			resetCursors:            *resetCursors,
			onlyIncomplete:          *onlyIncomplete,
			allowObjectTargetErrors: *allowObjectTargetErrors,
			noModuleShards:          *noModuleShards,
			migrate:                 *migrate,
			packageIDs:              packageIDs,
			moduleNames:             moduleNames,
			eventTypes:              eventTypes,
			objectTypeNames:         objectTypeNames,
			objectTypeReprs:         objectTypeReprs,
			checkpointFrom:          *checkpointFrom,
			checkpointTo:            *checkpointTo,
			checkpointShardSize:     *checkpointShardSize,
			deriveBatchSize:         *deriveBatchSize,
			maxBatches:              *maxBatches,
		})
	default:
		slog.Error("unknown indexer mode", "mode", *mode)
		os.Exit(1)
	}
}

func runRepairCharacterObjects(options indexOptions) {
	ctx := context.Background()
	pool, err := db.Connect(ctx, options.databaseURL)
	if err != nil {
		slog.Error("connect PostgreSQL", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	if options.migrate {
		if err := db.ApplyMigrations(ctx, pool); err != nil {
			slog.Error("apply migrations", "error", err)
			os.Exit(1)
		}
	}
	if options.environment == "" {
		options.environment = model.EnvironmentStillness
	}
	if options.network == "" {
		options.network = "sui-testnet"
	}
	if options.endpoint == "" {
		options.endpoint = "https://graphql.testnet.sui.io/graphql"
	}
	if options.deriveBatchSize <= 0 {
		options.deriveBatchSize = 1000
	}
	if options.concurrency <= 0 {
		options.concurrency = 8
	}
	store := db.PostgresStore{Pool: pool}
	source := sui.ObjectSourceForNetwork(options.network, options.endpoint, options.environment)
	if err := store.EnsureSource(ctx, source); err != nil {
		slog.Error("ensure Sui object source", "error", err)
		os.Exit(1)
	}
	refs, err := listCharacterObjectRepairRefs(ctx, store, options)
	if err != nil {
		slog.Error("list character object repair refs", "error", err)
		os.Exit(1)
	}
	startedAt := time.Now().UTC()
	summary := characterObjectRepairSummary{
		Environment: options.environment,
		Network:     options.network,
		Endpoint:    options.endpoint,
		Candidates:  len(refs),
		StartedAt:   startedAt,
	}
	if len(refs) == 0 {
		summary.FinishedAt = time.Now().UTC()
		writeJSON(summary)
		return
	}
	client := sui.GraphQLClient{
		Endpoint: options.endpoint,
		Retry: sui.RetryConfig{
			Retries:   options.retries,
			BaseDelay: options.retryBase,
			Jitter:    options.retryJitter,
			OnRetry: func(reason string, attempt int, delay time.Duration) {
				fmt.Fprintf(os.Stderr, "Retrying Sui GraphQL %s attempt %d/%d after %s\n", reason, attempt, options.retries, delay)
			},
		},
	}
	workerCount := options.concurrency
	if workerCount > len(refs) {
		workerCount = len(refs)
	}
	jobs := make(chan characterObjectRef)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error
	addError := func(err error) {
		mu.Lock()
		defer mu.Unlock()
		summary.Errors++
		if firstErr == nil {
			firstErr = err
		}
	}
	addCounts := func(fetched, replayed, missing, entities, relations, killmails int64) {
		mu.Lock()
		defer mu.Unlock()
		summary.ObjectsFetched += fetched
		summary.ObjectsReplayed += replayed
		summary.ObjectsMissing += missing
		summary.EntitiesDerived += entities
		summary.RelationsDerived += relations
		summary.KillmailsDerived += killmails
	}
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ref := range jobs {
				record, ok, fetched, err := repairCharacterObjectRecord(ctx, store, client, source, ref, options)
				if err != nil {
					addError(err)
					continue
				}
				if !ok {
					addCounts(0, 0, 1, 0, 0, 0)
					continue
				}
				if err := store.UpsertSuiObject(ctx, record); err != nil {
					addError(fmt.Errorf("upsert object %s for %s: %w", ref.ObjectID, ref.EntityID, err))
					continue
				}
				graph := sui.DeriveGraphFromObject(record)
				var entities []db.EntityFactSet
				for _, item := range graph.Entities {
					entities = append(entities, db.EntityFactSet{Entity: item.Entity, Facts: item.Facts})
				}
				if err := store.UpsertEventDerivationBatch(ctx, entities, graph.Relations, graph.Killmails); err != nil {
					addError(fmt.Errorf("derive object %s for %s: %w", ref.ObjectID, ref.EntityID, err))
					continue
				}
				if fetched {
					addCounts(1, 0, 0, int64(len(graph.Entities)), int64(len(graph.Relations)), int64(len(graph.Killmails)))
				} else {
					addCounts(0, 1, 0, int64(len(graph.Entities)), int64(len(graph.Relations)), int64(len(graph.Killmails)))
				}
			}
		}()
	}
	for _, ref := range refs {
		jobs <- ref
	}
	close(jobs)
	wg.Wait()
	summary.FinishedAt = time.Now().UTC()
	writeJSON(summary)
	if firstErr != nil {
		slog.Error("repair character objects", "error", firstErr)
		os.Exit(1)
	}
}

func repairCharacterObjectRecord(ctx context.Context, store db.PostgresStore, client sui.GraphQLClient, source model.Source, ref characterObjectRef, options indexOptions) (db.SuiObjectRecord, bool, bool, error) {
	if ref.HasStoredObject {
		record, ok, err := loadSuiObjectRecord(ctx, store, options.environment, ref.ObjectID)
		if err != nil {
			return db.SuiObjectRecord{}, false, false, fmt.Errorf("load stored object %s for %s: %w", ref.ObjectID, ref.EntityID, err)
		}
		if ok {
			return record, true, false, nil
		}
	}
	node, ok, err := client.FetchObject(ctx, sui.ObjectQuery{Address: ref.ObjectID})
	if err != nil {
		return db.SuiObjectRecord{}, false, false, fmt.Errorf("fetch object %s for %s: %w", ref.ObjectID, ref.EntityID, err)
	}
	if !ok {
		return db.SuiObjectRecord{}, false, true, nil
	}
	record, err := sui.NormalizeMoveObject(node, sui.NormalizeOptions{
		Environment: options.environment,
		SourceID:    source.ID,
		FetchedAt:   time.Now().UTC(),
	})
	if err != nil {
		return db.SuiObjectRecord{}, false, true, fmt.Errorf("normalise object %s for %s: %w", ref.ObjectID, ref.EntityID, err)
	}
	return record, true, true, nil
}

func loadSuiObjectRecord(ctx context.Context, store db.PostgresStore, environment model.Environment, objectID string) (db.SuiObjectRecord, bool, error) {
	if store.Pool == nil {
		return db.SuiObjectRecord{}, false, fmt.Errorf("postgres pool is nil")
	}
	var record db.SuiObjectRecord
	var payload []byte
	err := store.Pool.QueryRow(ctx, `
		SELECT id, object_id, environment, type_repr, coalesce(package_id, ''), coalesce(module, ''),
		  coalesce(type_name, ''), coalesce(version, ''), coalesce(digest, ''), coalesce(source_id, ''),
		  payload_json, observed_at
		FROM sui_objects
		WHERE environment = $1
		  AND lower(object_id) = lower($2)
		ORDER BY observed_at DESC, id DESC
		LIMIT 1
	`, environment, objectID).Scan(&record.ID, &record.ObjectID, &record.Environment, &record.TypeRepr, &record.PackageID, &record.Module, &record.TypeName, &record.Version, &record.Digest, &record.SourceID, &payload, &record.ObservedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.SuiObjectRecord{}, false, nil
	}
	if err != nil {
		return db.SuiObjectRecord{}, false, err
	}
	if len(payload) > 0 {
		if err := json.Unmarshal(payload, &record.Payload); err != nil {
			return db.SuiObjectRecord{}, false, err
		}
	}
	return record, true, nil
}

func listCharacterObjectRepairRefs(ctx context.Context, store db.PostgresStore, options indexOptions) ([]characterObjectRef, error) {
	if store.Pool == nil {
		return nil, fmt.Errorf("postgres pool is nil")
	}
	cyclePredicate := cycleSQLPredicate("e.cycle", options.cycles, options.includeUncycled)
	query := `
WITH event_characters AS (
  SELECT
    e.id AS entity_id,
    max(CASE WHEN f.key = 'character_id' THEN f.value_json #>> '{}' END) AS object_id,
    (e.name = 'Character ' || regexp_replace(e.id, '^.*:', '')
      AND (coalesce(e.display_name, '') = '' OR e.display_name = e.name)) AS is_placeholder
  FROM entities e
  JOIN entity_facts f ON f.entity_id = e.id
  WHERE e.environment = $1
    AND e.entity_type = 'character'
` + cyclePredicate + `
  GROUP BY e.id
  HAVING bool_or(f.key IN ('source_event_kind', 'source_event_id', 'transaction_digest'))
     AND max(CASE WHEN f.key = 'character_id' THEN f.value_json #>> '{}' END) ~ '^0x[0-9a-fA-F]{64}$'
)
SELECT entity_id, object_id, EXISTS (
  SELECT 1
  FROM sui_objects o
  WHERE o.environment = $1
    AND lower(o.object_id) = lower(ec.object_id)
) AS has_stored_object
FROM event_characters ec
WHERE is_placeholder
   OR NOT EXISTS (
  SELECT 1
  FROM sui_objects o
  WHERE o.environment = $1
    AND lower(o.object_id) = lower(ec.object_id)
)
ORDER BY entity_id
LIMIT $2`
	rows, err := store.Pool.Query(ctx, query, options.environment, options.deriveBatchSize)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var refs []characterObjectRef
	for rows.Next() {
		var ref characterObjectRef
		if err := rows.Scan(&ref.EntityID, &ref.ObjectID, &ref.HasStoredObject); err != nil {
			return nil, err
		}
		refs = append(refs, ref)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return refs, nil
}

func cycleSQLPredicate(column string, cycles []int, includeUncycled bool) string {
	var values []string
	for _, cycle := range cycles {
		values = append(values, fmt.Sprint(cycle))
	}
	switch {
	case len(values) > 0 && includeUncycled:
		return "    AND (" + column + " IN (" + strings.Join(values, ",") + ") OR " + column + " IS NULL)\n"
	case len(values) > 0:
		return "    AND " + column + " IN (" + strings.Join(values, ",") + ")\n"
	case !includeUncycled:
		return "    AND " + column + " IS NOT NULL\n"
	default:
		return ""
	}
}

func runRetryRangeBlockedObjects(manifest sui.Manifest, options indexOptions) {
	ctx := context.Background()
	pool, err := db.Connect(ctx, options.databaseURL)
	if err != nil {
		slog.Error("connect PostgreSQL", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	if options.migrate {
		if err := db.ApplyMigrations(ctx, pool); err != nil {
			slog.Error("apply migrations", "error", err)
			os.Exit(1)
		}
	}
	store := db.PostgresStore{Pool: pool}
	targets, err := sui.ProviderRangeBlockedObjectTargets(ctx, store, manifest, sui.CoverageAuditOptions{
		Environment:     options.environment,
		Network:         options.network,
		Cycles:          options.cycles,
		IncludeUncycled: options.includeUncycled,
		PackageIDs:      options.packageIDs,
		ModuleNames:     options.moduleNames,
		ObjectTypeNames: options.objectTypeNames,
		ObjectTypeReprs: options.objectTypeReprs,
	})
	if err != nil {
		slog.Error("find provider range-blocked object targets", "error", err)
		os.Exit(1)
	}
	if len(targets) == 0 {
		now := time.Now().UTC()
		writeJSON(sui.BackfillSummary{
			Environment: options.environment,
			Network:     options.network,
			Endpoint:    options.endpoint,
			StartedAt:   now,
			FinishedAt:  now,
		})
		return
	}
	client := sui.GraphQLClient{
		Endpoint: options.endpoint,
		Retry: sui.RetryConfig{
			Retries:   options.retries,
			BaseDelay: options.retryBase,
			Jitter:    options.retryJitter,
			OnRetry: func(reason string, attempt int, delay time.Duration) {
				fmt.Fprintf(os.Stderr, "Retrying Sui GraphQL %s attempt %d/%d after %s\n", reason, attempt, options.retries, delay)
			},
		},
	}
	summary, err := sui.RunObjectBackfill(ctx, store, client, manifest, sui.ObjectBackfillOptions{
		Environment:       options.environment,
		Network:           options.network,
		Endpoint:          options.endpoint,
		First:             options.first,
		MaxPages:          options.maxPages,
		Concurrency:       options.concurrency,
		Cycles:            options.cycles,
		IncludeUncycled:   options.includeUncycled,
		ResetCursors:      options.resetCursors,
		Targets:           targets,
		AllowTargetErrors: true,
	})
	if err != nil {
		writeJSON(summary)
		slog.Error("retry provider range-blocked Sui objects", "error", err)
		os.Exit(1)
	}
	writeJSON(summary)
}

func runRangeBlockedObjectAudit(manifest sui.Manifest, options indexOptions) {
	ctx := context.Background()
	pool, err := db.Connect(ctx, options.databaseURL)
	if err != nil {
		slog.Error("connect PostgreSQL", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	if options.migrate {
		if err := db.ApplyMigrations(ctx, pool); err != nil {
			slog.Error("apply migrations", "error", err)
			os.Exit(1)
		}
	}
	targets, err := sui.ProviderRangeBlockedObjectTargets(ctx, db.PostgresStore{Pool: pool}, manifest, sui.CoverageAuditOptions{
		Environment:     options.environment,
		Network:         options.network,
		Cycles:          options.cycles,
		IncludeUncycled: options.includeUncycled,
		PackageIDs:      options.packageIDs,
		ModuleNames:     options.moduleNames,
		ObjectTypeNames: options.objectTypeNames,
		ObjectTypeReprs: options.objectTypeReprs,
	})
	if err != nil {
		slog.Error("find provider range-blocked object targets", "error", err)
		os.Exit(1)
	}
	writeJSON(rangeBlockedObjectAuditOutput(options.environment, options.network, targets))
}

func rangeBlockedObjectAuditOutput(environment model.Environment, network string, targets []sui.ObjectTypeTarget) rangeBlockedObjectAudit {
	if environment == "" {
		environment = model.EnvironmentStillness
	}
	if network == "" {
		network = "sui-testnet"
	}
	return rangeBlockedObjectAudit{
		SchemaVersion: "registry.range-blocked-objects.v1",
		Environment:   environment,
		Network:       network,
		Count:         len(targets),
		Targets:       append([]sui.ObjectTypeTarget(nil), targets...),
	}
}

func runPlan(manifest sui.Manifest, environment, network string, cycleScope cyclepkg.Scope, maxPages, concurrency int, packageIDs, moduleNames, eventTypes, objectTypeNames, objectTypeReprs []string, checkpointFrom, checkpointTo, checkpointShardSize uint64, noModuleShards bool) {
	eventTargets, err := sui.EventStreamTargets(manifest, sui.TargetOptions{
		Environment:         model.Environment(environment),
		Network:             network,
		Cycles:              cycleScope.Cycles,
		IncludeUncycled:     cycleScope.IncludeUncycled,
		PackageIDs:          packageIDs,
		ModuleNames:         moduleNames,
		EventTypes:          eventTypes,
		CheckpointFrom:      checkpointFrom,
		CheckpointTo:        checkpointTo,
		CheckpointShardSize: checkpointShardSize,
		NoModuleShards:      noModuleShards,
	})
	if err != nil {
		slog.Error("build Sui target plan", "error", err)
		os.Exit(1)
	}
	objectTargets, objectErr := sui.ObjectTypeTargets(manifest, sui.ObjectTargetOptions{
		Environment:     model.Environment(environment),
		Network:         network,
		Cycles:          cycleScope.Cycles,
		IncludeUncycled: cycleScope.IncludeUncycled,
		PackageIDs:      packageIDs,
		ModuleNames:     moduleNames,
		TypeNames:       objectTypeNames,
		TypeReprs:       objectTypeReprs,
	})
	plan := map[string]any{
		"environment":         environment,
		"network":             network,
		"cycles":              cycleScope.Cycles,
		"includeUncycled":     cycleScope.IncludeUncycled,
		"maxPages":            maxPages,
		"concurrency":         concurrency,
		"checkpointFrom":      checkpointFrom,
		"checkpointTo":        checkpointTo,
		"checkpointShardSize": checkpointShardSize,
		"noModuleShards":      noModuleShards,
		"eventTargets":        eventTargets,
	}
	if objectErr == nil {
		plan["objectTargets"] = objectTargets
	} else {
		plan["objectTargetWarning"] = objectErr.Error()
	}
	writeJSON(plan)
}

func runAudit(manifest sui.Manifest, options indexOptions) {
	ctx := context.Background()
	pool, err := db.Connect(ctx, options.databaseURL)
	if err != nil {
		slog.Error("connect PostgreSQL", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	if options.migrate {
		if err := db.ApplyMigrations(ctx, pool); err != nil {
			slog.Error("apply migrations", "error", err)
			os.Exit(1)
		}
	}
	audit, err := sui.AuditCoverage(ctx, db.PostgresStore{Pool: pool}, manifest, sui.CoverageAuditOptions{
		Environment:         options.environment,
		Network:             options.network,
		Cycles:              options.cycles,
		IncludeUncycled:     options.includeUncycled,
		MaxPages:            options.maxPages,
		PackageIDs:          options.packageIDs,
		ModuleNames:         options.moduleNames,
		EventTypes:          options.eventTypes,
		ObjectTypeNames:     options.objectTypeNames,
		ObjectTypeReprs:     options.objectTypeReprs,
		CheckpointFrom:      options.checkpointFrom,
		CheckpointTo:        options.checkpointTo,
		CheckpointShardSize: options.checkpointShardSize,
		NoModuleShards:      options.noModuleShards,
	})
	if err != nil {
		slog.Error("audit Sui coverage", "error", err)
		os.Exit(1)
	}
	writeJSON(audit)
}

func runReport(options indexOptions) {
	ctx := context.Background()
	pool, err := db.Connect(ctx, options.databaseURL)
	if err != nil {
		slog.Error("connect PostgreSQL", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	if options.migrate {
		if err := db.ApplyMigrations(ctx, pool); err != nil {
			slog.Error("apply migrations", "error", err)
			os.Exit(1)
		}
	}
	result, err := report.Build(ctx, db.PostgresStore{Pool: pool}, report.Options{
		Environment:     options.environment,
		ExcludeFixtures: options.excludeFixtures,
	})
	if err != nil {
		slog.Error("build registry report", "error", err)
		os.Exit(1)
	}
	writeJSON(result)
}

func runIndexerStatus(options indexOptions) {
	ctx := context.Background()
	pool, err := db.Connect(ctx, options.databaseURL)
	if err != nil {
		slog.Error("connect PostgreSQL", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	if options.migrate {
		if err := db.ApplyMigrations(ctx, pool); err != nil {
			slog.Error("apply migrations", "error", err)
			os.Exit(1)
		}
	}
	result, err := report.BuildIndexerStatus(ctx, db.PostgresStore{Pool: pool}, report.IndexerStatusOptions{
		Environment:        options.environment,
		StaleAfter:         options.statusStaleAfter,
		ExportManifestPath: options.exportManifestPath,
	})
	if err != nil {
		slog.Error("build indexer status", "error", err)
		os.Exit(1)
	}
	writeJSON(result)
}

func runKillmailAudit(options indexOptions) {
	ctx := context.Background()
	pool, err := db.Connect(ctx, options.databaseURL)
	if err != nil {
		slog.Error("connect PostgreSQL", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	if options.migrate {
		if err := db.ApplyMigrations(ctx, pool); err != nil {
			slog.Error("apply migrations", "error", err)
			os.Exit(1)
		}
	}
	result, err := report.BuildKillmailAudit(ctx, db.PostgresStore{Pool: pool}, report.KillmailAuditOptions{
		Environment:     options.environment,
		ExcludeFixtures: options.excludeFixtures,
		SampleLimit:     options.sampleLimit,
	})
	if err != nil {
		slog.Error("build killmail audit", "error", err)
		os.Exit(1)
	}
	writeJSON(result)
}

func runCurrentStateAudit(options indexOptions) {
	ctx := context.Background()
	pool, err := db.Connect(ctx, options.databaseURL)
	if err != nil {
		slog.Error("connect PostgreSQL", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	if options.migrate {
		if err := db.ApplyMigrations(ctx, pool); err != nil {
			slog.Error("apply migrations", "error", err)
			os.Exit(1)
		}
	}
	result, err := report.BuildCurrentStateAudit(ctx, db.PostgresStore{Pool: pool}, report.CurrentStateAuditOptions{
		Environment: options.environment,
	})
	if err != nil {
		slog.Error("build current-state audit", "error", err)
		os.Exit(1)
	}
	writeJSON(result)
}

func runCharacterProfileAudit(options indexOptions) {
	ctx := context.Background()
	pool, err := db.Connect(ctx, options.databaseURL)
	if err != nil {
		slog.Error("connect PostgreSQL", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	if options.migrate {
		if err := db.ApplyMigrations(ctx, pool); err != nil {
			slog.Error("apply migrations", "error", err)
			os.Exit(1)
		}
	}
	result, err := report.BuildCharacterProfileAudit(ctx, db.PostgresStore{Pool: pool}, report.CharacterProfileAuditOptions{
		Environment: options.environment,
		SampleLimit: options.sampleLimit,
	})
	if err != nil {
		slog.Error("build character profile audit", "error", err)
		os.Exit(1)
	}
	writeJSON(result)
}

func runTribeIdentityEvidenceAudit(options indexOptions) {
	ctx := context.Background()
	pool, err := db.Connect(ctx, options.databaseURL)
	if err != nil {
		slog.Error("connect PostgreSQL", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	if options.migrate {
		if err := db.ApplyMigrations(ctx, pool); err != nil {
			slog.Error("apply migrations", "error", err)
			os.Exit(1)
		}
	}
	result, err := report.BuildTribeIdentityEvidenceAudit(ctx, db.PostgresStore{Pool: pool}, report.TribeIdentityEvidenceAuditOptions{
		Environment:    options.environment,
		Module:         singleFilter("module", options.moduleNames),
		ObjectTypeName: singleFilter("object-type-name", options.objectTypeNames),
		ObjectTypeRepr: singleFilter("object-type", options.objectTypeReprs),
		SampleLimit:    options.sampleLimit,
	})
	if err != nil {
		slog.Error("build tribe identity evidence audit", "error", err)
		os.Exit(1)
	}
	writeJSON(result)
}

func runEvidenceBridgeAudit(options indexOptions) {
	ctx := context.Background()
	pool, err := db.Connect(ctx, options.databaseURL)
	if err != nil {
		slog.Error("connect PostgreSQL", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	if options.migrate {
		if err := db.ApplyMigrations(ctx, pool); err != nil {
			slog.Error("apply migrations", "error", err)
			os.Exit(1)
		}
	}
	result, err := report.BuildEvidenceBridgeAudit(ctx, db.PostgresStore{Pool: pool}, report.EvidenceBridgeAuditOptions{
		Environment: options.environment,
		SampleLimit: options.sampleLimit,
	})
	if err != nil {
		slog.Error("build evidence bridge audit", "error", err)
		os.Exit(1)
	}
	writeJSON(result)
}

func runObjectShapeAudit(options indexOptions) {
	ctx := context.Background()
	pool, err := db.Connect(ctx, options.databaseURL)
	if err != nil {
		slog.Error("connect PostgreSQL", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	if options.migrate {
		if err := db.ApplyMigrations(ctx, pool); err != nil {
			slog.Error("apply migrations", "error", err)
			os.Exit(1)
		}
	}
	result, err := sui.BuildObjectShapeAudit(ctx, db.PostgresStore{Pool: pool}, sui.ObjectShapeAuditOptions{
		Environment: options.environment,
		PackageID:   singleFilter("package", options.packageIDs),
		Module:      singleFilter("module", options.moduleNames),
		TypeName:    singleFilter("object-type-name", options.objectTypeNames),
		TypeRepr:    singleFilter("object-type", options.objectTypeReprs),
		Limit:       options.shapeLimit,
		SampleLimit: options.sampleLimit,
	})
	if err != nil {
		slog.Error("build Sui object-shape audit", "error", err)
		os.Exit(1)
	}
	writeJSON(result)
}

func runSystemReconciliation(options indexOptions) {
	if strings.TrimSpace(options.staticUniversePath) == "" {
		slog.Error("build system reconciliation", "error", "-static-universe-path is required")
		os.Exit(2)
	}
	ctx := context.Background()
	pool, err := db.Connect(ctx, options.databaseURL)
	if err != nil {
		slog.Error("connect PostgreSQL", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	if options.migrate {
		if err := db.ApplyMigrations(ctx, pool); err != nil {
			slog.Error("apply migrations", "error", err)
			os.Exit(1)
		}
	}
	result, err := report.BuildSystemReconciliation(ctx, db.PostgresStore{Pool: pool}, options.staticUniversePath, report.SystemReconciliationOptions{
		Environment: options.environment,
	})
	if err != nil {
		slog.Error("build system reconciliation", "error", err)
		os.Exit(1)
	}
	writeJSON(result)
}

func singleFilter(name string, values []string) string {
	if len(values) == 0 {
		return ""
	}
	if len(values) > 1 {
		slog.Error("object-shape audit accepts at most one filter value", "filter", name, "values", strings.Join(values, ","))
		os.Exit(1)
	}
	return values[0]
}

type indexOptions struct {
	environment             model.Environment
	network                 string
	endpoint                string
	databaseURL             string
	cycles                  []int
	includeUncycled         bool
	first                   int
	maxPages                int
	concurrency             int
	retries                 int
	retryBase               time.Duration
	retryJitter             time.Duration
	resetCursors            bool
	onlyIncomplete          bool
	allowObjectTargetErrors bool
	noModuleShards          bool
	migrate                 bool
	packageIDs              []string
	moduleNames             []string
	eventTypes              []string
	objectTypeNames         []string
	objectTypeReprs         []string
	checkpointFrom          uint64
	checkpointTo            uint64
	checkpointShardSize     uint64
	deriveBatchSize         int
	maxBatches              int
	excludeFixtures         bool
	sampleLimit             int
	shapeLimit              int
	staticUniversePath      string
	statusStaleAfter        time.Duration
	exportManifestPath      string
}

func runEvents(manifest sui.Manifest, options indexOptions) {
	ctx := context.Background()
	pool, err := db.Connect(ctx, options.databaseURL)
	if err != nil {
		slog.Error("connect PostgreSQL", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	if options.migrate {
		if err := db.ApplyMigrations(ctx, pool); err != nil {
			slog.Error("apply migrations", "error", err)
			os.Exit(1)
		}
	}
	client := sui.GraphQLClient{
		Endpoint: options.endpoint,
		Retry: sui.RetryConfig{
			Retries:   options.retries,
			BaseDelay: options.retryBase,
			Jitter:    options.retryJitter,
			OnRetry: func(reason string, attempt int, delay time.Duration) {
				fmt.Fprintf(os.Stderr, "Retrying Sui GraphQL %s attempt %d/%d after %s\n", reason, attempt, options.retries, delay)
			},
		},
	}
	summary, err := sui.RunEventBackfill(ctx, db.PostgresStore{Pool: pool}, client, manifest, sui.BackfillOptions{
		Environment:         options.environment,
		Network:             options.network,
		Endpoint:            options.endpoint,
		Cycles:              options.cycles,
		IncludeUncycled:     options.includeUncycled,
		First:               options.first,
		MaxPages:            options.maxPages,
		Concurrency:         options.concurrency,
		ResetCursors:        options.resetCursors,
		OnlyIncomplete:      options.onlyIncomplete,
		PackageIDs:          options.packageIDs,
		ModuleNames:         options.moduleNames,
		EventTypes:          options.eventTypes,
		CheckpointFrom:      options.checkpointFrom,
		CheckpointTo:        options.checkpointTo,
		CheckpointShardSize: options.checkpointShardSize,
		NoModuleShards:      options.noModuleShards,
	})
	if err != nil {
		writeJSON(summary)
		slog.Error("backfill Sui events", "error", err)
		os.Exit(1)
	}
	writeJSON(summary)
}

func runObjects(manifest sui.Manifest, options indexOptions) {
	ctx := context.Background()
	pool, err := db.Connect(ctx, options.databaseURL)
	if err != nil {
		slog.Error("connect PostgreSQL", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	if options.migrate {
		if err := db.ApplyMigrations(ctx, pool); err != nil {
			slog.Error("apply migrations", "error", err)
			os.Exit(1)
		}
	}
	client := sui.GraphQLClient{
		Endpoint: options.endpoint,
		Retry: sui.RetryConfig{
			Retries:   options.retries,
			BaseDelay: options.retryBase,
			Jitter:    options.retryJitter,
			OnRetry: func(reason string, attempt int, delay time.Duration) {
				fmt.Fprintf(os.Stderr, "Retrying Sui GraphQL %s attempt %d/%d after %s\n", reason, attempt, options.retries, delay)
			},
		},
	}
	summary, err := sui.RunObjectBackfill(ctx, db.PostgresStore{Pool: pool}, client, manifest, sui.ObjectBackfillOptions{
		Environment:       options.environment,
		Network:           options.network,
		Endpoint:          options.endpoint,
		Cycles:            options.cycles,
		IncludeUncycled:   options.includeUncycled,
		First:             options.first,
		MaxPages:          options.maxPages,
		Concurrency:       options.concurrency,
		ResetCursors:      options.resetCursors,
		OnlyIncomplete:    options.onlyIncomplete,
		AllowTargetErrors: options.allowObjectTargetErrors,
		PackageIDs:        options.packageIDs,
		ModuleNames:       options.moduleNames,
		TypeNames:         options.objectTypeNames,
		TypeReprs:         options.objectTypeReprs,
	})
	if err != nil {
		writeJSON(summary)
		slog.Error("backfill Sui objects", "error", err)
		os.Exit(1)
	}
	writeJSON(summary)
}

func runAll(manifest sui.Manifest, options indexOptions) {
	ctx := context.Background()
	pool, err := db.Connect(ctx, options.databaseURL)
	if err != nil {
		slog.Error("connect PostgreSQL", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	if options.migrate {
		if err := db.ApplyMigrations(ctx, pool); err != nil {
			slog.Error("apply migrations", "error", err)
			os.Exit(1)
		}
	}
	client := sui.GraphQLClient{
		Endpoint: options.endpoint,
		Retry: sui.RetryConfig{
			Retries:   options.retries,
			BaseDelay: options.retryBase,
			Jitter:    options.retryJitter,
			OnRetry: func(reason string, attempt int, delay time.Duration) {
				fmt.Fprintf(os.Stderr, "Retrying Sui GraphQL %s attempt %d/%d after %s\n", reason, attempt, options.retries, delay)
			},
		},
	}
	store := db.PostgresStore{Pool: pool}
	events, err := sui.RunEventBackfill(ctx, store, client, manifest, sui.BackfillOptions{
		Environment:         options.environment,
		Network:             options.network,
		Endpoint:            options.endpoint,
		Cycles:              options.cycles,
		IncludeUncycled:     options.includeUncycled,
		First:               options.first,
		MaxPages:            options.maxPages,
		Concurrency:         options.concurrency,
		ResetCursors:        options.resetCursors,
		OnlyIncomplete:      options.onlyIncomplete,
		PackageIDs:          options.packageIDs,
		ModuleNames:         options.moduleNames,
		EventTypes:          options.eventTypes,
		CheckpointFrom:      options.checkpointFrom,
		CheckpointTo:        options.checkpointTo,
		CheckpointShardSize: options.checkpointShardSize,
		NoModuleShards:      options.noModuleShards,
	})
	if err != nil {
		writeJSON(map[string]any{"events": events})
		slog.Error("backfill Sui events", "error", err)
		os.Exit(1)
	}
	objects, err := sui.RunObjectBackfill(ctx, store, client, manifest, sui.ObjectBackfillOptions{
		Environment:       options.environment,
		Network:           options.network,
		Endpoint:          options.endpoint,
		Cycles:            options.cycles,
		IncludeUncycled:   options.includeUncycled,
		First:             options.first,
		MaxPages:          options.maxPages,
		Concurrency:       options.concurrency,
		ResetCursors:      options.resetCursors,
		OnlyIncomplete:    options.onlyIncomplete,
		AllowTargetErrors: options.allowObjectTargetErrors,
		PackageIDs:        options.packageIDs,
		ModuleNames:       options.moduleNames,
		TypeNames:         options.objectTypeNames,
		TypeReprs:         options.objectTypeReprs,
	})
	if err != nil {
		writeJSON(map[string]any{"events": events, "objects": objects})
		slog.Error("backfill Sui objects", "error", err)
		os.Exit(1)
	}
	writeJSON(map[string]any{"events": events, "objects": objects})
}

func runDeriveObjects(options indexOptions) {
	ctx := context.Background()
	pool, err := db.Connect(ctx, options.databaseURL)
	if err != nil {
		slog.Error("connect PostgreSQL", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	if options.migrate {
		if err := db.ApplyMigrations(ctx, pool); err != nil {
			slog.Error("apply migrations", "error", err)
			os.Exit(1)
		}
	}
	if options.deriveBatchSize <= 0 {
		options.deriveBatchSize = 1000
	}
	store := db.PostgresStore{Pool: pool}
	cursorID := deriveObjectsCursorID(options.network, options.environment)
	cursorStatus := db.CursorStatus{
		ID:          cursorID,
		Source:      deriveObjectsCursorSource(options.network),
		Environment: options.environment,
		CursorKind:  "sui_object_derivation",
		UpdatedAt:   time.Now().UTC(),
	}
	after := ""
	if !options.resetCursors {
		saved, ok, err := store.GetSyncCursor(ctx, cursorID)
		if err != nil {
			slog.Error("load derive cursor", "error", err)
			os.Exit(1)
		}
		if ok {
			cursorStatus = saved
			after = saved.CursorValue
		}
	}
	var scanned int64
	var derived int64
	var relations int64
	var killmails int64
	var batches int
	for {
		if options.maxBatches > 0 && batches >= options.maxBatches {
			break
		}
		page, err := store.ListSuiObjects(ctx, db.SuiObjectQuery{
			Environment:     options.environment,
			Cycles:          options.cycles,
			IncludeUncycled: options.includeUncycled,
			Limit:           options.deriveBatchSize,
			Cursor:          after,
		})
		if err != nil {
			cursorStatus.ErrorCount++
			cursorStatus.LastErrorSummary = err.Error()
			_ = store.SaveSyncCursor(ctx, cursorStatus)
			slog.Error("list Sui objects for derivation", "error", err)
			os.Exit(1)
		}
		if len(page.Items) == 0 {
			break
		}
		batches++
		var batchEntities []db.EntityFactSet
		var batchRelations []db.RelationDraft
		var batchKillmails []model.KillmailRaw
		for _, object := range page.Items {
			scanned++
			graph := sui.DeriveGraphFromObject(object)
			if len(graph.Entities) == 0 {
				continue
			}
			for _, draft := range graph.Entities {
				batchEntities = append(batchEntities, db.EntityFactSet{Entity: draft.Entity, Facts: draft.Facts})
				derived++
			}
			batchRelations = append(batchRelations, graph.Relations...)
			relations += int64(len(graph.Relations))
			batchKillmails = append(batchKillmails, graph.Killmails...)
			killmails += int64(len(graph.Killmails))
		}
		if err := store.UpsertEventDerivationBatch(ctx, batchEntities, batchRelations, batchKillmails); err != nil {
			cursorStatus.ErrorCount++
			cursorStatus.LastErrorSummary = err.Error()
			_ = store.SaveSyncCursor(ctx, cursorStatus)
			slog.Error("upsert derived object batch", "error", err)
			os.Exit(1)
		}
		last := page.Items[len(page.Items)-1]
		lastCursor, err := cursor.Encode(cursor.Keyset{Time: last.ObservedAt, ID: last.ID})
		if err != nil {
			slog.Error("encode derive cursor", "error", err)
			os.Exit(1)
		}
		after = lastCursor
		cursorStatus.CursorValue = lastCursor
		cursorStatus.EventsProcessed += int64(len(page.Items))
		now := time.Now().UTC()
		cursorStatus.LastSuccessfulIngest = &now
		cursorStatus.LastErrorSummary = ""
		if err := store.SaveSyncCursor(ctx, cursorStatus); err != nil {
			slog.Error("save derive cursor", "error", err)
			os.Exit(1)
		}
		if page.NextCursor == "" {
			break
		}
	}
	evidenceRelations, err := store.ResolveEvidenceRelations(ctx, options.environment)
	if err != nil {
		slog.Error("resolve evidence relations", "error", err)
		os.Exit(1)
	}
	writeJSON(map[string]any{
		"environment":               options.environment,
		"network":                   options.network,
		"cursor":                    cursorStatus.CursorValue,
		"batches":                   batches,
		"objectsScanned":            scanned,
		"entitiesDerived":           derived,
		"relationsDerived":          relations,
		"killmailsDerived":          killmails,
		"evidenceRelationsResolved": evidenceRelations,
	})
}

func runResolveEvidence(options indexOptions) {
	ctx := context.Background()
	pool, err := db.Connect(ctx, options.databaseURL)
	if err != nil {
		slog.Error("connect PostgreSQL", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	if options.migrate {
		if err := db.ApplyMigrations(ctx, pool); err != nil {
			slog.Error("apply migrations", "error", err)
			os.Exit(1)
		}
	}
	counts, err := db.PostgresStore{Pool: pool}.ResolveEvidenceRelations(ctx, options.environment)
	if err != nil {
		slog.Error("resolve evidence relations", "error", err)
		os.Exit(1)
	}
	writeJSON(resolveEvidenceOutput(options.environment, options.network, counts))
}

func resolveEvidenceOutput(environment model.Environment, network string, counts db.EvidenceRelationResolutionCounts) map[string]any {
	return map[string]any{
		"environment":          environment,
		"network":              network,
		"ownershipRelations":   counts.OwnershipRelations,
		"locationRelations":    counts.LocationRelations,
		"evidenceRelationsRun": true,
	}
}

func runDeriveEvents(options indexOptions) {
	ctx := context.Background()
	pool, err := db.Connect(ctx, options.databaseURL)
	if err != nil {
		slog.Error("connect PostgreSQL", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	if options.migrate {
		if err := db.ApplyMigrations(ctx, pool); err != nil {
			slog.Error("apply migrations", "error", err)
			os.Exit(1)
		}
	}
	summary, err := sui.RunEventDerivation(ctx, db.PostgresStore{Pool: pool}, sui.EventDerivationOptions{
		Environment:     options.environment,
		Network:         options.network,
		Cycles:          options.cycles,
		IncludeUncycled: options.includeUncycled,
		Modules:         options.moduleNames,
		BatchSize:       options.deriveBatchSize,
		MaxBatches:      options.maxBatches,
		ResetCursors:    options.resetCursors,
	})
	if err != nil {
		writeJSON(summary)
		slog.Error("derive Sui events", "error", err)
		os.Exit(1)
	}
	writeJSON(summary)
}

func deriveObjectsCursorID(network string, environment model.Environment) string {
	return "cursor:" + deriveObjectsCursorSource(network) + ":" + string(environment)
}

func deriveObjectsCursorSource(network string) string {
	if network == "" {
		network = "sui-testnet"
	}
	return "registry:derive:sui-objects:" + network
}

func indexerCycleScope(value string) (cyclepkg.Scope, error) {
	return cyclepkg.ParseScope(value, true)
}

func writeJSON(value any) {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		slog.Error("write JSON", "error", err)
		os.Exit(1)
	}
}
