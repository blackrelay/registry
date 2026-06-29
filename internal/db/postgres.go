package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/blackrelay/registry/internal/cursor"
	"github.com/blackrelay/registry/internal/cycles"
	"github.com/blackrelay/registry/internal/model"
	"github.com/blackrelay/registry/internal/staticdata"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresStore struct {
	Pool *pgxpool.Pool
}

func Connect(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, err
	}
	return pgxpool.NewWithConfig(ctx, cfg)
}

func (s PostgresStore) Ping(ctx context.Context) error {
	return s.Pool.Ping(ctx)
}

func (s PostgresStore) CountRegistryRows(ctx context.Context, environment model.Environment) (RegistryCountSnapshot, error) {
	snapshot := RegistryCountSnapshot{
		EntitiesByType:       make(map[model.EntityType]int64),
		EventsByModule:       make(map[string]int64),
		SuiObjectsByType:     make(map[string]int64),
		RelationsByPredicate: make(map[string]int64),
	}
	if err := s.Pool.QueryRow(ctx, `
		SELECT
		  (SELECT count(*) FROM sources WHERE ($1::text = '' OR environment = nullif($1::text, '')::registry_environment)),
		  (SELECT count(*) FROM source_artefacts WHERE ($1::text = '' OR environment = nullif($1::text, '')::registry_environment)),
		  (SELECT count(*) FROM imports WHERE ($1::text = '' OR environment = nullif($1::text, '')::registry_environment)),
		  (SELECT count(*) FROM reviews),
		  (SELECT count(*) FROM events WHERE ($1::text = '' OR environment = nullif($1::text, '')::registry_environment)),
		  (SELECT count(*) FROM sui_objects WHERE ($1::text = '' OR environment = nullif($1::text, '')::registry_environment)),
		  (SELECT count(*) FROM entities WHERE ($1::text = '' OR environment = nullif($1::text, '')::registry_environment)),
		  (SELECT count(*) FROM entity_facts WHERE ($1::text = '' OR environment = nullif($1::text, '')::registry_environment)),
		  (SELECT count(*) FROM entity_relations WHERE ($1::text = '' OR environment = nullif($1::text, '')::registry_environment)),
		  (SELECT count(*) FROM killmails WHERE ($1::text = '' OR environment = nullif($1::text, '')::registry_environment)),
		  (SELECT count(*) FROM search_terms st JOIN entities e ON e.id = st.entity_id WHERE ($1::text = '' OR e.environment = nullif($1::text, '')::registry_environment)),
		  (SELECT count(*) FROM sync_cursors WHERE ($1::text = '' OR environment = nullif($1::text, '')::registry_environment)),
		  (SELECT count(*) FROM sui_objects WHERE module = 'character' AND type_name = 'PlayerProfile' AND ($1::text = '' OR environment = nullif($1::text, '')::registry_environment))
	`, string(environment)).Scan(
		&snapshot.Counts.Sources,
		&snapshot.Counts.SourceArtefacts,
		&snapshot.Counts.Imports,
		&snapshot.Counts.Reviews,
		&snapshot.Counts.RawSuiEvents,
		&snapshot.Counts.RawSuiObjects,
		&snapshot.Counts.Entities,
		&snapshot.Counts.Facts,
		&snapshot.Counts.Relations,
		&snapshot.Counts.Killmails,
		&snapshot.Counts.SearchTerms,
		&snapshot.Counts.SyncCursors,
		&snapshot.Counts.PlayerProfiles,
	); err != nil {
		return RegistryCountSnapshot{}, err
	}
	if err := countGroup(ctx, s.Pool, snapshot.EntitiesByType, "SELECT entity_type::text, count(*) FROM entities", "environment", environment); err != nil {
		return RegistryCountSnapshot{}, err
	}
	if err := countGroup(ctx, s.Pool, snapshot.EventsByModule, "SELECT coalesce(nullif(module, ''), '(none)'), count(*) FROM events", "environment", environment); err != nil {
		return RegistryCountSnapshot{}, err
	}
	if err := countGroup(ctx, s.Pool, snapshot.SuiObjectsByType, "SELECT coalesce(nullif(type_name, ''), nullif(type_repr, ''), '(unknown)'), count(*) FROM sui_objects", "environment", environment); err != nil {
		return RegistryCountSnapshot{}, err
	}
	if err := countGroup(ctx, s.Pool, snapshot.RelationsByPredicate, "SELECT predicate, count(*) FROM entity_relations", "environment", environment); err != nil {
		return RegistryCountSnapshot{}, err
	}
	return snapshot, nil
}

func countGroup[T ~string](ctx context.Context, pool *pgxpool.Pool, out map[T]int64, query, environmentColumn string, environment model.Environment) error {
	var args []any
	if environment != "" {
		query += " WHERE " + environmentColumn + " = $1"
		args = append(args, environment)
	}
	query += " GROUP BY 1 ORDER BY 1"
	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		var count int64
		if err := rows.Scan(&key, &count); err != nil {
			return err
		}
		out[T(key)] = count
	}
	return rows.Err()
}

func (s PostgresStore) UpsertStaticEnemy(ctx context.Context, importID string, source model.Source, artefact model.SourceArtefact, candidate staticdata.EnemyCandidate) error {
	if s.Pool == nil {
		return errors.New("postgres pool is nil")
	}
	tx, err := s.Pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := upsertSource(ctx, tx, source); err != nil {
		return err
	}
	if err := upsertArtefact(ctx, tx, artefact); err != nil {
		return err
	}
	entity := model.Entity{
		ID:          staticdata.EntityID(source.Environment, candidate.TypeID),
		Slug:        staticdata.Slug(candidate.Name, candidate.TypeID, source.Environment),
		Type:        model.EntityTypeEnemy,
		Name:        candidate.Name,
		DisplayName: staticdata.DisplayName(candidate.Name),
		Summary:     fmt.Sprintf("Reviewed static-client enemy candidate, type %d in group %d.", candidate.TypeID, candidate.GroupID),
		Environment: source.Environment,
		Cycle:       artefact.Cycle,
		UpdatedAt:   time.Now().UTC(),
	}
	if err := upsertEntity(ctx, tx, entity); err != nil {
		return err
	}
	facts := map[string]any{
		"type_id":            candidate.TypeID,
		"group_id":           candidate.GroupID,
		"basis":              candidate.Basis,
		"display_name":       entity.DisplayName,
		"source_artefact_id": artefact.ID,
	}
	for key, value := range facts {
		if err := upsertFact(ctx, tx, entity.ID, key, value, importID, source.ID, model.Confidence(candidate.Confidence), source.Environment, artefact.Cycle, model.ReviewStatusReviewed); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO search_terms(entity_id, entity_type, name, aliases, body, updated_at)
		VALUES ($1, $2, $3, $4, $5, now())
		ON CONFLICT (entity_id) DO UPDATE SET
		  entity_type = EXCLUDED.entity_type,
		  name = EXCLUDED.name,
		  aliases = EXCLUDED.aliases,
		  body = EXCLUDED.body,
		  updated_at = now()
	`, entity.ID, entity.Type, entity.Name, entity.DisplayName, entity.Summary+" "+candidate.Basis); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s PostgresStore) RecordImport(ctx context.Context, importID string, source model.Source, artefact model.SourceArtefact, summary map[string]any) error {
	if s.Pool == nil {
		return errors.New("postgres pool is nil")
	}
	tx, err := s.Pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := upsertSource(ctx, tx, source); err != nil {
		return err
	}
	if err := upsertArtefact(ctx, tx, artefact); err != nil {
		return err
	}
	summaryJSON, err := json.Marshal(summary)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO imports(id, source_id, artefact_id, environment, importer_name, importer_version, summary)
		VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb)
		ON CONFLICT (id) DO UPDATE SET summary = EXCLUDED.summary
	`, importID, source.ID, artefact.ID, source.Environment, artefact.ImporterName, artefact.ImporterVersion, string(summaryJSON)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s PostgresStore) UpsertKillmail(ctx context.Context, raw model.KillmailRaw) error {
	if s.Pool == nil {
		return errors.New("postgres pool is nil")
	}
	payload, err := json.Marshal(raw.Raw)
	if err != nil {
		return err
	}
	_, err = s.Pool.Exec(ctx, `
		INSERT INTO killmails(
		  id, environment, occurred_at, system_id, system_name, victim_character_id, victim_name,
		  killer_character_id, killer_name, killer_type_id, reporter_character_id, reporter_name,
		  loss_type, source_ids, raw_json, updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15::jsonb, now())
		ON CONFLICT (id) DO UPDATE SET
		  environment = EXCLUDED.environment,
		  occurred_at = EXCLUDED.occurred_at,
		  system_id = EXCLUDED.system_id,
		  system_name = EXCLUDED.system_name,
		  victim_character_id = EXCLUDED.victim_character_id,
		  victim_name = EXCLUDED.victim_name,
		  killer_character_id = EXCLUDED.killer_character_id,
		  killer_name = EXCLUDED.killer_name,
		  killer_type_id = EXCLUDED.killer_type_id,
		  reporter_character_id = EXCLUDED.reporter_character_id,
		  reporter_name = EXCLUDED.reporter_name,
		  loss_type = EXCLUDED.loss_type,
		  source_ids = EXCLUDED.source_ids,
		  raw_json = EXCLUDED.raw_json,
		  updated_at = now()
	`, raw.ID, raw.Environment, raw.OccurredAt, raw.SystemID, raw.SystemName, raw.VictimCharacterID, raw.VictimName, raw.KillerCharacterID, raw.KillerName, raw.KillerTypeID, raw.ReporterCharacterID, raw.ReporterName, raw.LossType, raw.SourceIDs, string(payload))
	return err
}

func (s PostgresStore) UpsertEventDerivationBatch(ctx context.Context, entities []EntityFactSet, relations []RelationDraft, killmails []model.KillmailRaw) error {
	if s.Pool == nil {
		return errors.New("postgres pool is nil")
	}
	if len(entities) == 0 && len(relations) == 0 && len(killmails) == 0 {
		return nil
	}
	entities = dedupeEntityFactSets(entities)
	relations = dedupeRelations(relations)
	killmails = dedupeKillmails(killmails)
	tx, err := s.Pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	batch := &pgx.Batch{}
	queued := 0
	for _, item := range entities {
		entity := item.Entity
		preserveExistingDisplay := shouldPreserveExistingEntityOnPlaceholder(entity)
		batch.Queue(`
			INSERT INTO entities(id, slug, entity_type, name, display_name, summary, environment, cycle, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, now())
			ON CONFLICT (id) DO UPDATE SET
			  slug = CASE WHEN $9 THEN entities.slug ELSE EXCLUDED.slug END,
			  entity_type = EXCLUDED.entity_type,
			  name = CASE WHEN $9 THEN entities.name ELSE EXCLUDED.name END,
			  display_name = CASE WHEN $9 THEN entities.display_name ELSE EXCLUDED.display_name END,
			  summary = CASE WHEN $9 THEN entities.summary ELSE EXCLUDED.summary END,
			  environment = EXCLUDED.environment,
			  cycle = EXCLUDED.cycle,
			  updated_at = now()
		`, entity.ID, entity.Slug, entity.Type, entity.Name, entity.DisplayName, entity.Summary, entity.Environment, entity.Cycle, preserveExistingDisplay)
		queued++
		searchBody := entity.Summary
		for _, fact := range item.Facts {
			if fact.Confidence == "" {
				fact.Confidence = model.ConfidenceUnknown
			}
			if fact.Environment == "" {
				fact.Environment = entity.Environment
			}
			if fact.Cycle == nil {
				fact.Cycle = entity.Cycle
			}
			if fact.ReviewStatus == "" {
				fact.ReviewStatus = model.ReviewStatusCandidate
			}
			valueJSON, err := json.Marshal(fact.Value)
			if err != nil {
				return err
			}
			factID := fmt.Sprintf("fact:%s:%s", entity.ID, fact.Key)
			batch.Queue(`
				INSERT INTO entity_facts(id, entity_id, key, value_json, source_id, confidence, environment, cycle, review_status, import_id, published_at)
				VALUES ($1, $2, $3, $4::jsonb, $5, $6, $7, $8, $9, null, now())
				ON CONFLICT (id) DO UPDATE SET
				  value_json = EXCLUDED.value_json,
				  source_id = EXCLUDED.source_id,
				  confidence = EXCLUDED.confidence,
				  environment = EXCLUDED.environment,
				  cycle = EXCLUDED.cycle,
				  review_status = EXCLUDED.review_status,
				  import_id = EXCLUDED.import_id,
				  published_at = EXCLUDED.published_at
			`, factID, entity.ID, fact.Key, string(valueJSON), fact.SourceID, fact.Confidence, fact.Environment, fact.Cycle, fact.ReviewStatus)
			queued++
			searchBody += " " + fact.Key + " " + fmt.Sprint(fact.Value)
		}
		batch.Queue(`
			INSERT INTO search_terms(entity_id, entity_type, name, aliases, body, updated_at)
			VALUES ($1, $2, $3, $4, $5, now())
			ON CONFLICT (entity_id) DO UPDATE SET
			  entity_type = EXCLUDED.entity_type,
			  name = CASE WHEN $6 THEN search_terms.name ELSE EXCLUDED.name END,
			  aliases = CASE WHEN $6 THEN search_terms.aliases ELSE EXCLUDED.aliases END,
			  body = CASE WHEN $6 THEN search_terms.body ELSE EXCLUDED.body END,
			  updated_at = now()
		`, entity.ID, entity.Type, entity.Name, entity.DisplayName, searchBody, preserveExistingDisplay)
		queued++
	}
	for _, raw := range killmails {
		payload, err := json.Marshal(raw.Raw)
		if err != nil {
			return err
		}
		batch.Queue(`
			INSERT INTO killmails(
			  id, environment, occurred_at, system_id, system_name, victim_character_id, victim_name,
			  killer_character_id, killer_name, killer_type_id, reporter_character_id, reporter_name,
			  loss_type, source_ids, raw_json, updated_at
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15::jsonb, now())
			ON CONFLICT (id) DO UPDATE SET
			  environment = EXCLUDED.environment,
			  occurred_at = EXCLUDED.occurred_at,
			  system_id = EXCLUDED.system_id,
			  system_name = EXCLUDED.system_name,
			  victim_character_id = EXCLUDED.victim_character_id,
			  victim_name = EXCLUDED.victim_name,
			  killer_character_id = EXCLUDED.killer_character_id,
			  killer_name = EXCLUDED.killer_name,
			  killer_type_id = EXCLUDED.killer_type_id,
			  reporter_character_id = EXCLUDED.reporter_character_id,
			  reporter_name = EXCLUDED.reporter_name,
			  loss_type = EXCLUDED.loss_type,
			  source_ids = EXCLUDED.source_ids,
			  raw_json = EXCLUDED.raw_json,
			  updated_at = now()
		`, raw.ID, raw.Environment, raw.OccurredAt, raw.SystemID, raw.SystemName, raw.VictimCharacterID, raw.VictimName, raw.KillerCharacterID, raw.KillerName, raw.KillerTypeID, raw.ReporterCharacterID, raw.ReporterName, raw.LossType, raw.SourceIDs, string(payload))
		queued++
	}
	for _, relation := range relations {
		if relation.SubjectEntityID == "" || relation.Predicate == "" || relation.ObjectEntityID == "" {
			return errors.New("relation subject, predicate and object are required")
		}
		if relation.SourceID == "" {
			return errors.New("relation source id is required")
		}
		if relation.Confidence == "" {
			relation.Confidence = model.ConfidenceUnknown
		}
		if relation.Environment == "" {
			relation.Environment = model.EnvironmentUnknown
		}
		batch.Queue(`
			INSERT INTO entity_relations(
			  id, subject_entity_id, predicate, object_entity_id, source_id, confidence, environment, valid_from, valid_to
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			ON CONFLICT (id) DO UPDATE SET
			  source_id = EXCLUDED.source_id,
			  confidence = EXCLUDED.confidence,
			  environment = EXCLUDED.environment,
			  valid_from = EXCLUDED.valid_from,
			  valid_to = EXCLUDED.valid_to
		`, RelationID(relation), relation.SubjectEntityID, relation.Predicate, relation.ObjectEntityID, relation.SourceID, relation.Confidence, relation.Environment, relation.ValidFrom, relation.ValidTo)
		queued++
	}
	results := tx.SendBatch(ctx, batch)
	for i := 0; i < queued; i++ {
		if _, err := results.Exec(); err != nil {
			_ = results.Close()
			return err
		}
	}
	if err := results.Close(); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s PostgresStore) ResolveEvidenceRelations(ctx context.Context, environment model.Environment) (EvidenceRelationResolutionCounts, error) {
	if s.Pool == nil {
		return EvidenceRelationResolutionCounts{}, errors.New("postgres pool is nil")
	}
	ownership, err := s.resolveOwnershipEvidenceRelations(ctx, environment)
	if err != nil {
		return EvidenceRelationResolutionCounts{}, err
	}
	locations, err := s.resolveLocationEvidenceRelations(ctx, environment)
	if err != nil {
		return EvidenceRelationResolutionCounts{}, err
	}
	return EvidenceRelationResolutionCounts{
		OwnershipRelations: ownership,
		LocationRelations:  locations,
	}, nil
}

func (s PostgresStore) resolveOwnershipEvidenceRelations(ctx context.Context, environment model.Environment) (int64, error) {
	var count int64
	err := s.Pool.QueryRow(ctx, `
		WITH character_caps AS (
			SELECT
			  f.environment,
			  f.value_json #>> '{}' AS owner_cap_id,
			  min(f.entity_id) AS character_entity_id,
			  bool_and(f.confidence = 'verified'::registry_confidence) AS all_verified
			FROM entity_facts f
			JOIN entities e ON e.id = f.entity_id AND e.entity_type = 'character'
			WHERE f.key = 'owner_cap_id'
			  AND nullif(f.value_json #>> '{}', '') IS NOT NULL
			  AND ($1::text = '' OR f.environment = nullif($1::text, '')::registry_environment)
			GROUP BY f.environment, f.value_json #>> '{}'
			HAVING count(DISTINCT f.entity_id) = 1
		),
		matches AS (
			SELECT DISTINCT
			  infra.entity_id AS subject_entity_id,
			  'owned_by' AS predicate,
			  character_caps.character_entity_id AS object_entity_id,
			  infra.source_id,
			  CASE
			    WHEN infra.confidence = 'verified'::registry_confidence AND character_caps.all_verified
			      THEN 'verified'::registry_confidence
			    ELSE 'probable'::registry_confidence
			  END AS confidence,
			  infra.environment
			FROM entity_facts infra
			JOIN entities subject ON subject.id = infra.entity_id
			  AND subject.entity_type IN ('assembly', 'gate', 'storage', 'turret')
			JOIN character_caps ON character_caps.environment = infra.environment
			  AND character_caps.owner_cap_id = infra.value_json #>> '{}'
			WHERE infra.key = 'owner_cap_id'
			  AND nullif(infra.value_json #>> '{}', '') IS NOT NULL
			  AND ($1::text = '' OR infra.environment = nullif($1::text, '')::registry_environment)
		),
		upserted AS (
			INSERT INTO entity_relations(id, subject_entity_id, predicate, object_entity_id, source_id, confidence, environment)
			SELECT
			  'relation:' || subject_entity_id || ':' || predicate || ':' || object_entity_id || ':' || source_id,
			  subject_entity_id,
			  predicate,
			  object_entity_id,
			  source_id,
			  confidence,
			  environment
			FROM matches
			ON CONFLICT (id) DO UPDATE SET
			  source_id = EXCLUDED.source_id,
			  confidence = EXCLUDED.confidence,
			  environment = EXCLUDED.environment,
			  valid_to = NULL
			RETURNING 1
		)
		SELECT count(*) FROM upserted
	`, string(environment)).Scan(&count)
	return count, err
}

func (s PostgresStore) resolveLocationEvidenceRelations(ctx context.Context, environment model.Environment) (int64, error) {
	var count int64
	err := s.Pool.QueryRow(ctx, `
		WITH system_locations AS (
			SELECT
			  f.environment,
			  f.value_json #>> '{}' AS location_hash,
			  min(f.entity_id) AS system_entity_id,
			  bool_and(f.confidence = 'verified'::registry_confidence) AS all_verified
			FROM entity_facts f
			JOIN entities e ON e.id = f.entity_id AND e.entity_type = 'system'
			WHERE f.key = 'location_hash'
			  AND nullif(f.value_json #>> '{}', '') IS NOT NULL
			  AND ($1::text = '' OR f.environment = nullif($1::text, '')::registry_environment)
			GROUP BY f.environment, f.value_json #>> '{}'
			HAVING count(DISTINCT f.entity_id) = 1
		),
		matches AS (
			SELECT DISTINCT
			  infra.entity_id AS subject_entity_id,
			  'located_in' AS predicate,
			  system_locations.system_entity_id AS object_entity_id,
			  infra.source_id,
			  CASE
			    WHEN infra.confidence = 'verified'::registry_confidence AND system_locations.all_verified
			      THEN 'verified'::registry_confidence
			    ELSE 'probable'::registry_confidence
			  END AS confidence,
			  infra.environment
			FROM entity_facts infra
			JOIN entities subject ON subject.id = infra.entity_id
			  AND subject.entity_type IN ('assembly', 'gate', 'storage', 'turret')
			JOIN system_locations ON system_locations.environment = infra.environment
			  AND system_locations.location_hash = infra.value_json #>> '{}'
			WHERE infra.key = 'location_hash'
			  AND nullif(infra.value_json #>> '{}', '') IS NOT NULL
			  AND ($1::text = '' OR infra.environment = nullif($1::text, '')::registry_environment)
		),
		upserted AS (
			INSERT INTO entity_relations(id, subject_entity_id, predicate, object_entity_id, source_id, confidence, environment)
			SELECT
			  'relation:' || subject_entity_id || ':' || predicate || ':' || object_entity_id || ':' || source_id,
			  subject_entity_id,
			  predicate,
			  object_entity_id,
			  source_id,
			  confidence,
			  environment
			FROM matches
			ON CONFLICT (id) DO UPDATE SET
			  source_id = EXCLUDED.source_id,
			  confidence = EXCLUDED.confidence,
			  environment = EXCLUDED.environment,
			  valid_to = NULL
			RETURNING 1
		)
		SELECT count(*) FROM upserted
	`, string(environment)).Scan(&count)
	return count, err
}

func (s PostgresStore) ListEntities(ctx context.Context, query EntityQuery) (EntityPage, error) {
	limit := saneLimit(query.Limit, 50, 200)
	var args []any
	where := "WHERE 1=1"
	if query.Type != "" {
		args = append(args, query.Type)
		where += fmt.Sprintf(" AND e.entity_type = $%d", len(args))
	}
	if query.Environment != "" {
		args = append(args, query.Environment)
		where += fmt.Sprintf(" AND e.environment = $%d", len(args))
	}
	where = addCycleColumnFilter(&args, where, "e.cycle", query.Cycles, query.IncludeUncycled)
	if query.PublicOnly {
		where += " AND " + publicListedCharacterSQL()
		where += " AND " + publicListedTribeSQL()
	}
	if query.Q != "" {
		args = append(args, query.Q)
		qArg := len(args)
		where += fmt.Sprintf(" AND (e.name ILIKE '%%' || $%d || '%%' OR e.slug ILIKE '%%' || $%d || '%%' OR EXISTS (SELECT 1 FROM search_terms st WHERE st.entity_id = e.id AND st.document @@ plainto_tsquery('simple', $%d)))", qArg, qArg, qArg)
	}
	where = addEntityFactFilter(&args, where, "type_id", query.TypeID)
	where = addEntityFactFilter(&args, where, "group_id", query.GroupID)
	where = addEntityFactFilter(&args, where, "category_id", query.CategoryID)
	where = addEntityFactFilter(&args, where, "market_group_id", query.MarketGroupID)
	where = addEntityFactFilter(&args, where, "wreck_type_id", query.WreckTypeID)
	where = addEntityFactFilter(&args, where, "source_artefact_id", query.SourceArtefactID)
	where = addEntityFactFilter(&args, where, "static_entity_type", query.StaticEntityType)
	if query.Cursor != "" {
		decoded, err := cursor.Decode(query.Cursor)
		if err != nil {
			return EntityPage{}, err
		}
		args = append(args, decoded.Time, decoded.ID)
		where += fmt.Sprintf(" AND (e.updated_at, e.id) < ($%d, $%d)", len(args)-1, len(args))
	}
	args = append(args, limit+1)
	rows, err := s.Pool.Query(ctx, `
		SELECT e.id, e.slug, e.entity_type, e.name, coalesce(e.display_name, ''), coalesce(e.summary, ''), e.environment, e.cycle, e.updated_at
		FROM entities e `+where+`
		ORDER BY e.updated_at DESC, e.id DESC
		LIMIT $`+strconv.Itoa(len(args)), args...)
	if err != nil {
		return EntityPage{}, err
	}
	defer rows.Close()
	var items []model.Entity
	for rows.Next() {
		entity, err := scanEntity(rows)
		if err != nil {
			return EntityPage{}, err
		}
		items = append(items, entity)
	}
	if err := rows.Err(); err != nil {
		return EntityPage{}, err
	}
	next := ""
	if len(items) > limit {
		items = items[:limit]
		last := items[len(items)-1]
		encoded, err := cursor.Encode(cursor.Keyset{Time: last.UpdatedAt, ID: last.ID})
		if err != nil {
			return EntityPage{}, err
		}
		next = encoded
	}
	return EntityPage{Items: items, NextCursor: next}, nil
}

func addEntityFactFilter(args *[]any, where, key, value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return where
	}
	*args = append(*args, key, value)
	return where + fmt.Sprintf(` AND EXISTS (
		SELECT 1 FROM entity_facts f
		WHERE f.entity_id = e.id
		  AND f.key = $%d
		  AND md5(f.value_json #>> '{}') = md5($%d)
		  AND f.value_json #>> '{}' = $%d
	)`, len(*args)-1, len(*args), len(*args))
}

func addCycleColumnFilter(args *[]any, where, column string, values []int, includeUncycled bool) string {
	if len(values) == 0 {
		return where
	}
	placeholders := make([]string, 0, len(values))
	for _, value := range values {
		*args = append(*args, value)
		placeholders = append(placeholders, "$"+strconv.Itoa(len(*args)))
	}
	condition := column + " IN (" + strings.Join(placeholders, ", ") + ")"
	if includeUncycled {
		condition = "(" + condition + " OR " + column + " IS NULL)"
	}
	return where + " AND " + condition
}

func addRelationEndpointCycleFilter(args *[]any, where string, values []int, includeUncycled bool) string {
	if len(values) == 0 {
		return where
	}
	subjectCondition, objectCondition := cycleEndpointConditions(args, values, includeUncycled)
	return where + " AND " + subjectCondition + " AND " + objectCondition
}

func cycleEndpointConditions(args *[]any, values []int, includeUncycled bool) (string, string) {
	subjectPlaceholders := make([]string, 0, len(values))
	objectPlaceholders := make([]string, 0, len(values))
	for _, value := range values {
		*args = append(*args, value)
		subjectPlaceholders = append(subjectPlaceholders, "$"+strconv.Itoa(len(*args)))
	}
	for _, value := range values {
		*args = append(*args, value)
		objectPlaceholders = append(objectPlaceholders, "$"+strconv.Itoa(len(*args)))
	}
	subject := "se.cycle IN (" + strings.Join(subjectPlaceholders, ", ") + ")"
	object := "oe.cycle IN (" + strings.Join(objectPlaceholders, ", ") + ")"
	if includeUncycled {
		subject = "(" + subject + " OR se.cycle IS NULL)"
		object = "(" + object + " OR oe.cycle IS NULL)"
	}
	return subject, object
}

func addTimeCycleWindowFilter(args *[]any, where, column string, values []int, includeUncycled bool) string {
	if len(values) == 0 {
		return where
	}
	var conditions []string
	for _, value := range values {
		window, ok := cycles.Window(value)
		if !ok {
			continue
		}
		*args = append(*args, window.StartsAt)
		startPlaceholder := "$" + strconv.Itoa(len(*args))
		if window.EndsBefore == nil {
			conditions = append(conditions, column+" >= "+startPlaceholder)
			continue
		}
		*args = append(*args, *window.EndsBefore)
		conditions = append(conditions, "("+column+" >= "+startPlaceholder+" AND "+column+" < $"+strconv.Itoa(len(*args))+")")
	}
	if len(conditions) == 0 {
		return where + " AND false"
	}
	return where + " AND (" + strings.Join(conditions, " OR ") + ")"
}

func addKillmailCycleWindowFilter(args *[]any, where string, values []int, includeUncycled bool) string {
	return addTimeCycleWindowFilter(args, where, "k.occurred_at", values, includeUncycled)
}

func addOptionalSQLCondition(where string, value *bool, condition string) string {
	if value == nil {
		return where
	}
	if *value {
		return where + " AND (" + condition + ")"
	}
	return where + " AND NOT (" + condition + ")"
}

func currentProfileKnownSQL() string {
	return `(
		nullif(btrim(coalesce(e.facts_json #>> '{metadata_name}', '')), '') IS NOT NULL
		OR nullif(btrim(coalesce(e.facts_json #>> '{metadata_description}', '')), '') IS NOT NULL
		OR nullif(btrim(coalesce(e.facts_json #>> '{metadata_url}', '')), '') IS NOT NULL
		OR nullif(btrim(coalesce(e.facts_json #>> '{tag}', '')), '') IS NOT NULL
		OR nullif(btrim(coalesce(e.facts_json #>> '{description}', '')), '') IS NOT NULL
		OR nullif(btrim(coalesce(e.facts_json #>> '{url}', '')), '') IS NOT NULL
		OR (
			jsonb_typeof(e.facts_json -> 'aliases') = 'array'
			AND EXISTS (
				SELECT 1
				FROM jsonb_array_elements_text(e.facts_json -> 'aliases') AS alias(value)
				WHERE nullif(btrim(alias.value), '') IS NOT NULL
			)
		)
		OR (
			jsonb_typeof(e.facts_json -> 'aliases') = 'string'
			AND nullif(btrim(e.facts_json ->> 'aliases'), '') IS NOT NULL
		)
	)`
}

func currentPlaceholderSQL() string {
	return `(
		(e.entity_type = 'character' AND e.name = 'Character ' || regexp_replace(e.id, '^.*:', '') AND (coalesce(e.display_name, '') = '' OR e.display_name = e.name))
		OR (e.entity_type = 'tribe' AND e.name = 'Tribe ' || regexp_replace(e.id, '^.*:', '') AND (coalesce(e.display_name, '') = '' OR e.display_name = e.name))
		OR (e.entity_type = 'system' AND e.name = 'System ' || regexp_replace(e.id, '^.*:', '') AND (coalesce(e.display_name, '') = '' OR e.display_name = e.name))
	)`
}

func currentPublicTribeSQL() string {
	displayName := `coalesce(nullif(btrim(coalesce(e.display_name, '')), ''), nullif(btrim(e.name), ''), '')`
	return `(
		e.entity_type <> 'tribe'
		OR (
			nullif(` + displayName + `, '') IS NOT NULL
			AND lower(` + displayName + `) <> lower('Tribe ' || regexp_replace(e.id, '^.*:', ''))
			AND lower(` + displayName + `) NOT LIKE 'npc corp %'
			AND CASE
				WHEN regexp_replace(e.id, '^.*:', '') ~ '^[0-9]+$'
				THEN regexp_replace(e.id, '^.*:', '') = '1000167'
					OR regexp_replace(e.id, '^.*:', '')::bigint >= 98000535
				ELSE false
			END
			AND NOT (
				e.name = 'Tribe ' || regexp_replace(e.id, '^.*:', '')
				AND (coalesce(e.display_name, '') = '' OR e.display_name = e.name)
			)
		)
	)`
}

func publicListedTribeSQL() string {
	displayName := `coalesce(nullif(btrim(coalesce(e.display_name, '')), ''), nullif(btrim(e.name), ''), '')`
	tribeID := `regexp_replace(e.id, '^.*:', '')`
	return `(
		e.entity_type <> 'tribe'
		OR (
			nullif(` + displayName + `, '') IS NOT NULL
			AND lower(` + displayName + `) <> lower('Tribe ' || ` + tribeID + `)
			AND lower(` + displayName + `) NOT LIKE 'npc corp %'
			AND CASE
				WHEN ` + tribeID + ` ~ '^[0-9]+$'
				THEN ` + tribeID + ` = '1000167'
					OR ` + tribeID + `::bigint >= 98000535
				ELSE true
			END
			AND NOT (
				e.name = 'Tribe ' || ` + tribeID + `
				AND (coalesce(e.display_name, '') = '' OR e.display_name = e.name)
			)
		)
	)`
}

func publicListedCharacterSQL() string {
	return `(
		e.entity_type <> 'character'
		OR EXISTS (
			SELECT 1
			FROM entity_facts f
			WHERE f.entity_id = e.id
			  AND f.key IN ('source_event_kind', 'source_event_id', 'transaction_digest')
			  AND nullif(btrim(f.value_json #>> '{}'), '') IS NOT NULL
		)
	)`
}

func (s PostgresStore) ListCurrentEntities(ctx context.Context, query CurrentEntityQuery) (CurrentEntityPage, error) {
	limit := saneLimit(query.Limit, 50, 200)
	var args []any
	where := "WHERE 1=1"
	if query.Type != "" {
		args = append(args, query.Type)
		where += fmt.Sprintf(" AND e.entity_type = $%d", len(args))
	}
	if query.Environment != "" {
		args = append(args, query.Environment)
		where += fmt.Sprintf(" AND e.environment = $%d", len(args))
	}
	where = addCycleColumnFilter(&args, where, "e.cycle", query.Cycles, query.IncludeUncycled)
	if currentScopedCharacterEvidenceRequired(query) {
		where += ` AND (
			e.entity_type <> 'character'
			OR e.facts_json ? 'source_event_kind'
			OR e.facts_json ? 'source_event_id'
			OR e.facts_json ? 'transaction_digest'
		)`
	}
	if currentScopedTribeProfileRequired(query) {
		where += " AND " + currentPublicTribeSQL()
	}
	if strings.TrimSpace(query.Q) != "" {
		args = append(args, "%"+strings.ToLower(strings.TrimSpace(query.Q))+"%")
		where += fmt.Sprintf(" AND lower(e.id || ' ' || e.slug || ' ' || e.name || ' ' || coalesce(e.display_name, '') || ' ' || coalesce(e.summary, '')) LIKE $%d", len(args))
	}
	switch strings.ToLower(strings.TrimSpace(query.ProfileState)) {
	case "":
	case "known":
		where += " AND " + currentProfileKnownSQL()
	case "placeholder":
		where += " AND " + currentPlaceholderSQL()
	default:
		where += " AND false"
	}
	if query.TribeID != "" {
		args = append(args, query.TribeID, tribeIdentityToken(query.TribeID))
		where += fmt.Sprintf(` AND EXISTS (
			SELECT 1 FROM entity_relations r
			WHERE r.subject_entity_id = e.id
			  AND r.predicate = 'belongs_to'
			  AND r.valid_to IS NULL
			  AND (
			    r.object_entity_id = $%d
			    OR (
			      nullif(btrim($%d), '') IS NOT NULL
			      AND regexp_replace(r.object_entity_id, '^.*:', '') = $%d
			    )
			  )
		)`, len(args)-1, len(args), len(args))
	}
	if query.OwnerID != "" {
		args = append(args, query.OwnerID)
		where += fmt.Sprintf(" AND EXISTS (SELECT 1 FROM entity_relations r WHERE r.subject_entity_id = e.id AND r.predicate = 'owned_by' AND r.object_entity_id = $%d AND r.valid_to IS NULL)", len(args))
	}
	if query.SystemID != "" {
		args = append(args, query.SystemID)
		where += fmt.Sprintf(` AND (
			e.id = $%d OR EXISTS (
				SELECT 1 FROM entity_relations r
				WHERE r.subject_entity_id = e.id
				  AND r.predicate IN ('located_in', 'located_at', 'deployed_in', 'observed_in', 'occurred_in', 'observed_between', 'member_of_region')
				  AND r.object_entity_id = $%d
				  AND r.valid_to IS NULL
			) OR EXISTS (
				SELECT 1 FROM entity_relations r
				WHERE r.object_entity_id = e.id
				  AND r.predicate IN ('located_in', 'located_at', 'deployed_in', 'observed_in', 'occurred_in', 'observed_between', 'member_of_region')
				  AND r.subject_entity_id = $%d
				  AND r.valid_to IS NULL
			)
		)`, len(args), len(args), len(args))
	}
	if query.OwnerCapID != "" {
		args = append(args, query.OwnerCapID)
		where += fmt.Sprintf(` AND EXISTS (
			SELECT 1 FROM entity_facts f
			WHERE f.entity_id = e.id
			  AND f.key = 'owner_cap_id'
			  AND md5(f.value_json #>> '{}') = md5($%d)
			  AND f.value_json #>> '{}' = $%d
		)`, len(args), len(args))
	}
	if query.LocationHash != "" {
		args = append(args, query.LocationHash)
		where += fmt.Sprintf(` AND EXISTS (
			SELECT 1 FROM entity_facts f
			WHERE f.entity_id = e.id
			  AND f.key = 'location_hash'
			  AND md5(f.value_json #>> '{}') = md5($%d)
			  AND f.value_json #>> '{}' = $%d
		)`, len(args), len(args))
	}
	if query.ConnectedTo != "" {
		args = append(args, query.ConnectedTo)
		where += fmt.Sprintf(` AND EXISTS (
			SELECT 1 FROM entity_relations r
			WHERE r.predicate IN ('links_to', 'observed_between')
			  AND r.valid_to IS NULL
			  AND ((r.subject_entity_id = e.id AND r.object_entity_id = $%d) OR (r.object_entity_id = e.id AND r.subject_entity_id = $%d))
		)`, len(args), len(args))
	}
	if query.HasActivity != nil {
		prefix := "EXISTS"
		if !*query.HasActivity {
			prefix = "NOT EXISTS"
		}
		where += ` AND ` + prefix + ` (
			SELECT 1 FROM entity_relations r
			WHERE r.predicate IN ('victim', 'killer', 'reported_by', 'occurred_in')
			  AND r.valid_to IS NULL
		  AND (r.subject_entity_id = e.id OR r.object_entity_id = e.id)
	)`
	}
	where = addOptionalSQLCondition(where, query.HasTribe, `EXISTS (
		SELECT 1 FROM entity_relations r
		WHERE r.subject_entity_id = e.id
		  AND r.predicate = 'belongs_to'
		  AND r.valid_to IS NULL
	)`)
	where = addOptionalSQLCondition(where, query.HasOwnerCap, `(
		EXISTS (
			SELECT 1 FROM entity_facts f
			WHERE f.entity_id = e.id
			  AND f.key = 'owner_cap_id'
			  AND nullif(btrim(f.value_json #>> '{}'), '') IS NOT NULL
		)
		OR EXISTS (
			SELECT 1 FROM entity_relations r
			WHERE r.subject_entity_id = e.id
			  AND r.predicate = 'has_owner_cap'
			  AND r.valid_to IS NULL
		)
	)`)
	where = addOptionalSQLCondition(where, query.HasLocationHash, `(
		EXISTS (
			SELECT 1 FROM entity_facts f
			WHERE f.entity_id = e.id
			  AND f.key = 'location_hash'
			  AND nullif(btrim(f.value_json #>> '{}'), '') IS NOT NULL
		)
		OR EXISTS (
			SELECT 1 FROM entity_relations r
			WHERE r.subject_entity_id = e.id
			  AND r.predicate = 'has_location_hash'
			  AND r.valid_to IS NULL
		)
	)`)
	where = addOptionalSQLCondition(where, query.HasResolvedOwner, `EXISTS (
		SELECT 1 FROM entity_relations r
		WHERE r.subject_entity_id = e.id
		  AND r.predicate = 'owned_by'
		  AND r.valid_to IS NULL
	)`)
	where = addOptionalSQLCondition(where, query.HasResolvedSystem, `EXISTS (
		SELECT 1 FROM entity_relations r
		WHERE r.subject_entity_id = e.id
		  AND r.predicate IN ('located_in', 'located_at', 'deployed_in', 'observed_in', 'occurred_in', 'observed_between')
		  AND r.valid_to IS NULL
	)`)
	if query.SourceID != "" {
		args = append(args, query.SourceID)
		where += fmt.Sprintf(" AND e.source_ids @> ARRAY[$%d::text]", len(args))
	}
	if query.Cursor != "" {
		decoded, err := cursor.Decode(query.Cursor)
		if err != nil {
			return CurrentEntityPage{}, err
		}
		args = append(args, decoded.Time, decoded.ID)
		where += fmt.Sprintf(" AND (e.updated_at, e.id) < ($%d, $%d)", len(args)-1, len(args))
	}
	orderBy := "ORDER BY e.updated_at DESC, e.id DESC"
	if query.Type == model.EntityTypeTribe && query.Cursor == "" {
		orderBy = `ORDER BY CASE
		  WHEN e.entity_type = 'tribe'
		    AND e.name = 'Tribe ' || regexp_replace(e.id, '^.*:', '')
		    AND (coalesce(e.display_name, '') = '' OR e.display_name = e.name)
		  THEN 2
		  WHEN e.entity_type = 'tribe'
		    AND (
		      nullif(btrim(coalesce(e.facts_json #>> '{tag}', '')), '') IS NOT NULL
		      OR nullif(btrim(coalesce(e.facts_json #>> '{description}', '')), '') IS NOT NULL
		      OR nullif(btrim(coalesce(e.facts_json #>> '{url}', '')), '') IS NOT NULL
		      OR (
		        jsonb_typeof(e.facts_json -> 'aliases') = 'array'
		        AND EXISTS (
		          SELECT 1
		          FROM jsonb_array_elements_text(e.facts_json -> 'aliases') AS alias(value)
		          WHERE nullif(btrim(alias.value), '') IS NOT NULL
		        )
		      )
		    )
		  THEN 0 ELSE 1 END ASC, e.updated_at DESC, e.id DESC`
	}
	fetchLimit := limit + 1
	if query.Type == model.EntityTypeCharacter || query.Type == model.EntityTypeTribe {
		fetchLimit = limit*4 + 1
	}
	args = append(args, fetchLimit)
	rows, err := s.Pool.Query(ctx, `
		SELECT e.id, e.slug, e.entity_type, e.name, coalesce(e.display_name, ''), coalesce(e.summary, ''), e.environment, e.cycle, e.updated_at,
		  facts_json, outgoing_relations_json, incoming_relations_json, source_ids
		FROM entity_current_state e `+where+`
		`+orderBy+`
		LIMIT $`+strconv.Itoa(len(args)), args...)
	if err != nil {
		return CurrentEntityPage{}, err
	}
	defer rows.Close()
	var items []model.CurrentEntity
	for rows.Next() {
		item, err := scanCurrentEntity(rows)
		if err != nil {
			return CurrentEntityPage{}, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return CurrentEntityPage{}, err
	}
	rawItems := items
	items = dedupeCurrentEntities(items, query)
	next := ""
	if len(items) > limit {
		items = items[:limit]
		last := items[len(items)-1].Entity
		encoded, err := cursor.Encode(cursor.Keyset{Time: last.UpdatedAt, ID: last.ID})
		if err != nil {
			return CurrentEntityPage{}, err
		}
		next = encoded
	} else if len(rawItems) == fetchLimit {
		last := rawItems[len(rawItems)-1].Entity
		encoded, err := cursor.Encode(cursor.Keyset{Time: last.UpdatedAt, ID: last.ID})
		if err != nil {
			return CurrentEntityPage{}, err
		}
		next = encoded
	}
	return CurrentEntityPage{Items: items, NextCursor: next}, nil
}

func (s PostgresStore) ListCurrentRelations(ctx context.Context, query CurrentRelationQuery) (CurrentRelationPage, error) {
	limit := saneLimit(query.Limit, 50, 200)
	var args []any
	where := "WHERE r.valid_to IS NULL"
	if query.Environment != "" {
		args = append(args, query.Environment)
		where += fmt.Sprintf(" AND r.environment = $%d", len(args))
	}
	if len(query.Predicates) > 0 {
		placeholders := make([]string, 0, len(query.Predicates))
		for _, predicate := range query.Predicates {
			if strings.TrimSpace(predicate) == "" {
				continue
			}
			args = append(args, predicate)
			placeholders = append(placeholders, "$"+strconv.Itoa(len(args)))
		}
		if len(placeholders) > 0 {
			where += " AND r.predicate IN (" + strings.Join(placeholders, ", ") + ")"
		}
	}
	if query.SystemID != "" {
		args = append(args, query.SystemID)
		where += fmt.Sprintf(" AND (r.subject_entity_id = $%d OR r.object_entity_id = $%d)", len(args), len(args))
	}
	if query.SourceID != "" {
		args = append(args, query.SourceID)
		where += fmt.Sprintf(" AND r.source_id = $%d", len(args))
	}
	where = addRelationEndpointCycleFilter(&args, where, query.Cycles, query.IncludeUncycled)
	if query.Cursor != "" {
		decoded, err := cursor.Decode(query.Cursor)
		if err != nil {
			return CurrentRelationPage{}, err
		}
		args = append(args, decoded.Time, decoded.ID)
		where += fmt.Sprintf(" AND (r.created_at, r.id) < ($%d, $%d)", len(args)-1, len(args))
	}
	args = append(args, limit+1)
	rows, err := s.Pool.Query(ctx, `
		SELECT r.id, r.subject_entity_id, se.entity_type, coalesce(nullif(se.display_name, ''), se.name, ''), r.predicate,
		  r.object_entity_id, oe.entity_type, coalesce(nullif(oe.display_name, ''), oe.name, ''), r.source_id, r.confidence, r.environment, r.created_at
		FROM entity_relations r
		JOIN entities se ON se.id = r.subject_entity_id
		JOIN entities oe ON oe.id = r.object_entity_id
		`+where+`
		ORDER BY r.created_at DESC, r.id DESC
		LIMIT $`+strconv.Itoa(len(args)), args...)
	if err != nil {
		return CurrentRelationPage{}, err
	}
	defer rows.Close()
	var items []model.CurrentRelation
	for rows.Next() {
		item, err := scanCurrentRelation(rows)
		if err != nil {
			return CurrentRelationPage{}, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return CurrentRelationPage{}, err
	}
	next := ""
	if len(items) > limit {
		items = items[:limit]
		last := items[len(items)-1]
		encoded, err := cursor.Encode(cursor.Keyset{Time: last.CreatedAt, ID: last.ID})
		if err != nil {
			return CurrentRelationPage{}, err
		}
		next = encoded
	}
	return CurrentRelationPage{Items: items, NextCursor: next}, nil
}

func (s PostgresStore) GetEntity(ctx context.Context, idOrSlug string) (model.Entity, bool, error) {
	row := s.Pool.QueryRow(ctx, `
		SELECT id, slug, entity_type, name, coalesce(display_name, ''), coalesce(summary, ''), environment, cycle, updated_at
		FROM entities WHERE id = $1 OR slug = $1
	`, idOrSlug)
	entity, err := scanEntity(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.Entity{}, false, nil
		}
		return model.Entity{}, false, err
	}
	return entity, true, nil
}

func (s PostgresStore) GetSource(ctx context.Context, id string) (model.Source, bool, error) {
	row := s.Pool.QueryRow(ctx, `
		SELECT id, kind, title, locator, coalesce(url, ''), environment, cycle, metadata, created_at
		FROM sources WHERE id = $1
	`, id)
	source, err := scanSource(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.Source{}, false, nil
		}
		return model.Source{}, false, err
	}
	return source, true, nil
}

func (s PostgresStore) ExportDatabaseIdentity(ctx context.Context) (DatabaseIdentity, error) {
	var identity DatabaseIdentity
	identity.Engine = "postgresql"
	if err := s.Pool.QueryRow(ctx, "SELECT current_database(), version()").Scan(&identity.Database, &identity.ServerVersion); err != nil {
		return DatabaseIdentity{}, err
	}
	rows, err := s.Pool.Query(ctx, "SELECT version FROM schema_migrations ORDER BY version")
	if err != nil {
		return DatabaseIdentity{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var version string
		if err := rows.Scan(&version); err != nil {
			return DatabaseIdentity{}, err
		}
		identity.SchemaVersions = append(identity.SchemaVersions, version)
	}
	return identity, rows.Err()
}

func (s PostgresStore) ListSources(ctx context.Context, limit int) ([]model.Source, error) {
	limit = saneLimit(limit, 200, 5000)
	page, err := s.ListSourcesPage(ctx, SourceQuery{Limit: limit})
	if err != nil {
		return nil, err
	}
	return page.Items, nil
}

func (s PostgresStore) ListSourcesPage(ctx context.Context, query SourceQuery) (SourcePage, error) {
	limit := saneLimit(query.Limit, 200, 5000)
	var args []any
	where := "WHERE 1=1"
	if query.Environment != "" {
		args = append(args, query.Environment)
		where += fmt.Sprintf(" AND environment = $%d", len(args))
	}
	where = addCycleColumnFilter(&args, where, "cycle", query.Cycles, query.IncludeUncycled)
	if query.Cursor != "" {
		decoded, err := cursor.Decode(query.Cursor)
		if err != nil {
			return SourcePage{}, err
		}
		args = append(args, decoded.Time, decoded.ID)
		where += fmt.Sprintf(" AND (created_at, id) < ($%d, $%d)", len(args)-1, len(args))
	}
	args = append(args, limit+1)
	rows, err := s.Pool.Query(ctx, `
		SELECT id, kind, title, locator, coalesce(url, ''), environment, cycle, metadata, created_at
		FROM sources `+where+`
		ORDER BY created_at DESC, id DESC
		LIMIT $`+strconv.Itoa(len(args)), args...)
	if err != nil {
		return SourcePage{}, err
	}
	defer rows.Close()
	var out []model.Source
	for rows.Next() {
		source, err := scanSource(rows)
		if err != nil {
			return SourcePage{}, err
		}
		out = append(out, source)
	}
	next := ""
	if len(out) > limit {
		out = out[:limit]
		last := out[len(out)-1]
		encoded, err := cursor.Encode(cursor.Keyset{Time: last.CreatedAt, ID: last.ID})
		if err != nil {
			return SourcePage{}, err
		}
		next = encoded
	}
	return SourcePage{Items: out, NextCursor: next}, rows.Err()
}

func (s PostgresStore) GetArtefact(ctx context.Context, id string) (model.SourceArtefact, bool, error) {
	var artefact model.SourceArtefact
	var cycle pgtype.Int4
	row := s.Pool.QueryRow(ctx, `
		SELECT id, source_id, source_kind, kind, artefact_kind, environment, path_or_uri, sha256, size_bytes, row_count,
		  content_type, extracted_at, importer_name, importer_version, coalesce(client_build, ''), coalesce(patch_label, ''),
		  cycle, review_status, coalesce(superseded_by_artefact_id, ''), coalesce(notes, ''), created_at
		FROM source_artefacts WHERE id = $1
	`, id)
	if err := row.Scan(&artefact.ID, &artefact.SourceID, &artefact.SourceKind, &artefact.Kind, &artefact.ArtefactKind, &artefact.Environment, &artefact.PathOrURI, &artefact.SHA256, &artefact.SizeBytes, &artefact.RowCount, &artefact.ContentType, &artefact.ExtractedAt, &artefact.ImporterName, &artefact.ImporterVersion, &artefact.ClientBuild, &artefact.PatchLabel, &cycle, &artefact.ReviewStatus, &artefact.SupersededByArtefactID, &artefact.Notes, &artefact.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.SourceArtefact{}, false, nil
		}
		return model.SourceArtefact{}, false, err
	}
	artefact.Cycle = cycleFromPG(cycle)
	return artefact, true, nil
}

func (s PostgresStore) ListSourceArtefactsPage(ctx context.Context, query SourceArtefactQuery) (SourceArtefactPage, error) {
	limit := saneLimit(query.Limit, 200, 5000)
	var args []any
	where := "WHERE 1=1"
	if query.Environment != "" {
		args = append(args, query.Environment)
		where += fmt.Sprintf(" AND environment = $%d", len(args))
	}
	where = addCycleColumnFilter(&args, where, "cycle", query.Cycles, query.IncludeUncycled)
	if query.Cursor != "" {
		decoded, err := cursor.Decode(query.Cursor)
		if err != nil {
			return SourceArtefactPage{}, err
		}
		args = append(args, decoded.Time, decoded.ID)
		where += fmt.Sprintf(" AND (created_at, id) < ($%d, $%d)", len(args)-1, len(args))
	}
	args = append(args, limit+1)
	rows, err := s.Pool.Query(ctx, `
		SELECT id, source_id, source_kind, kind, artefact_kind, environment, path_or_uri, sha256, size_bytes, row_count,
		  content_type, extracted_at, importer_name, importer_version, coalesce(client_build, ''), coalesce(patch_label, ''),
		  cycle, review_status, coalesce(superseded_by_artefact_id, ''), coalesce(notes, ''), created_at
		FROM source_artefacts `+where+`
		ORDER BY created_at DESC, id DESC
		LIMIT $`+strconv.Itoa(len(args)), args...)
	if err != nil {
		return SourceArtefactPage{}, err
	}
	defer rows.Close()
	var out []model.SourceArtefact
	for rows.Next() {
		artefact, err := scanArtefact(rows)
		if err != nil {
			return SourceArtefactPage{}, err
		}
		out = append(out, artefact)
	}
	next := ""
	if len(out) > limit {
		out = out[:limit]
		last := out[len(out)-1]
		encoded, err := cursor.Encode(cursor.Keyset{Time: last.CreatedAt, ID: last.ID})
		if err != nil {
			return SourceArtefactPage{}, err
		}
		next = encoded
	}
	return SourceArtefactPage{Items: out, NextCursor: next}, rows.Err()
}

func (s PostgresStore) ListEvents(ctx context.Context, query EventQuery) (EventPage, error) {
	maxLimit := query.MaxLimit
	if maxLimit <= 0 {
		maxLimit = 200
	}
	limit := saneLimit(query.Limit, 50, maxLimit)
	var args []any
	where := "WHERE 1=1"
	if query.Kind != "" {
		args = append(args, query.Kind)
		where += fmt.Sprintf(" AND event_kind = $%d", len(args))
	}
	if query.Environment != "" {
		args = append(args, query.Environment)
		where += fmt.Sprintf(" AND environment = $%d", len(args))
	}
	where = addCycleColumnFilter(&args, where, "cycle", effectiveEventCycles(query), query.IncludeUncycled)
	if query.PackageID != "" {
		args = append(args, query.PackageID)
		where += fmt.Sprintf(" AND package_id = $%d", len(args))
	}
	if query.Module != "" {
		args = append(args, query.Module)
		where += fmt.Sprintf(" AND module = $%d", len(args))
	}
	if query.TransactionDigest != "" {
		args = append(args, query.TransactionDigest)
		where += fmt.Sprintf(" AND transaction_digest = $%d", len(args))
	}
	if query.SourceID != "" {
		args = append(args, query.SourceID)
		where += fmt.Sprintf(" AND source_id = $%d", len(args))
	}
	if query.Cursor != "" {
		decoded, err := cursor.Decode(query.Cursor)
		if err != nil {
			return EventPage{}, err
		}
		args = append(args, decoded.Time, decoded.ID)
		operator := "<"
		if query.Ascending {
			operator = ">"
		}
		where += fmt.Sprintf(" AND (occurred_at, id) %s ($%d, $%d)", operator, len(args)-1, len(args))
	}
	args = append(args, limit+1)
	order := "ORDER BY occurred_at DESC, id DESC"
	if query.Ascending {
		order = "ORDER BY occurred_at ASC, id ASC"
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT id, event_kind, environment, occurred_at, cycle, coalesce(package_id, ''), coalesce(module, ''),
		  coalesce(transaction_digest, ''), coalesce(checkpoint, ''), coalesce(source_id, ''), payload_json
		FROM events `+where+`
		`+order+`
		LIMIT $`+strconv.Itoa(len(args)), args...)
	if err != nil {
		return EventPage{}, err
	}
	defer rows.Close()
	var items []EventRecord
	for rows.Next() {
		item, err := scanEvent(rows)
		if err != nil {
			return EventPage{}, err
		}
		items = append(items, item)
	}
	next := ""
	if len(items) > limit {
		items = items[:limit]
		last := items[len(items)-1]
		encoded, err := cursor.Encode(cursor.Keyset{Time: last.OccurredAt, ID: last.ID})
		if err != nil {
			return EventPage{}, err
		}
		next = encoded
	}
	return EventPage{Items: items, NextCursor: next}, rows.Err()
}

func (s PostgresStore) ListSuiObjects(ctx context.Context, query SuiObjectQuery) (SuiObjectPage, error) {
	limit := saneLimit(query.Limit, 1000, 5000)
	var args []any
	where := "WHERE 1=1"
	if query.Environment != "" {
		args = append(args, query.Environment)
		where += fmt.Sprintf(" AND environment = $%d", len(args))
	}
	where = addTimeCycleWindowFilter(&args, where, "observed_at", query.Cycles, query.IncludeUncycled)
	if query.PackageID != "" {
		args = append(args, query.PackageID)
		where += fmt.Sprintf(" AND package_id = $%d", len(args))
	}
	if query.Module != "" {
		args = append(args, query.Module)
		where += fmt.Sprintf(" AND module = $%d", len(args))
	}
	if query.TypeName != "" {
		args = append(args, query.TypeName)
		where += fmt.Sprintf(" AND type_name = $%d", len(args))
	}
	if query.TypeRepr != "" {
		args = append(args, query.TypeRepr)
		where += fmt.Sprintf(" AND type_repr = $%d", len(args))
	}
	if query.Cursor != "" {
		decoded, err := cursor.Decode(query.Cursor)
		if err != nil {
			return SuiObjectPage{}, err
		}
		args = append(args, decoded.Time, decoded.ID)
		where += fmt.Sprintf(" AND (observed_at, id) > ($%d, $%d)", len(args)-1, len(args))
	}
	args = append(args, limit+1)
	rows, err := s.Pool.Query(ctx, `
		SELECT id, object_id, environment, type_repr, coalesce(package_id, ''), coalesce(module, ''),
		  coalesce(type_name, ''), coalesce(version, ''), coalesce(digest, ''), coalesce(source_id, ''),
		  payload_json, observed_at
		FROM sui_objects `+where+`
		ORDER BY observed_at ASC, id ASC
		LIMIT $`+strconv.Itoa(len(args)), args...)
	if err != nil {
		return SuiObjectPage{}, err
	}
	defer rows.Close()
	var items []SuiObjectRecord
	for rows.Next() {
		item, err := scanSuiObject(rows)
		if err != nil {
			return SuiObjectPage{}, err
		}
		items = append(items, item)
	}
	next := ""
	if len(items) > limit {
		items = items[:limit]
		last := items[len(items)-1]
		encoded, err := cursor.Encode(cursor.Keyset{Time: last.ObservedAt, ID: last.ID})
		if err != nil {
			return SuiObjectPage{}, err
		}
		next = encoded
	}
	return SuiObjectPage{Items: items, NextCursor: next}, rows.Err()
}

func (s PostgresStore) GetEvent(ctx context.Context, id string) (EventRecord, bool, error) {
	row := s.Pool.QueryRow(ctx, `
		SELECT id, event_kind, environment, occurred_at, cycle, coalesce(package_id, ''), coalesce(module, ''),
		  coalesce(transaction_digest, ''), coalesce(checkpoint, ''), coalesce(source_id, ''), payload_json
		FROM events WHERE id = $1
	`, id)
	item, err := scanEvent(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return EventRecord{}, false, nil
		}
		return EventRecord{}, false, err
	}
	return item, true, nil
}

func (s PostgresStore) ListKillmailRaw(ctx context.Context, query KillmailQuery) ([]model.KillmailRaw, string, error) {
	limit := saneLimit(query.Limit, 50, 200)
	var args []any
	where := "WHERE 1=1"
	if query.ExcludeFixtures {
		where += " AND NOT " + killmailFixtureSQL("k")
	}
	if query.Environment != "" {
		args = append(args, query.Environment)
		where += fmt.Sprintf(" AND k.environment = $%d", len(args))
	}
	where = addKillmailCycleWindowFilter(&args, where, query.Cycles, query.IncludeUncycled)
	if query.SystemID != "" {
		args = append(args, query.SystemID)
		where += fmt.Sprintf(` AND (
			k.system_lookup = $%d OR EXISTS (
				SELECT 1 FROM entity_relations r
				WHERE r.subject_entity_id = k.id
				  AND r.predicate = 'occurred_in'
				  AND r.object_entity_id = $%d
				  AND r.valid_to IS NULL
			)
		)`, len(args), len(args))
	}
	if query.VictimID != "" {
		args = append(args, query.VictimID)
		where += fmt.Sprintf(` AND (
			k.victim_lookup = $%d OR EXISTS (
				SELECT 1 FROM entity_relations r
				WHERE r.subject_entity_id = k.id
				  AND r.predicate = 'victim'
				  AND r.object_entity_id = $%d
				  AND r.valid_to IS NULL
			)
		)`, len(args), len(args))
	}
	if query.KillerID != "" {
		args = append(args, query.KillerID)
		where += fmt.Sprintf(` AND (
			k.killer_character_lookup = $%d OR k.killer_name_lookup = $%d OR EXISTS (
				SELECT 1 FROM entity_relations r
				WHERE r.subject_entity_id = k.id
				  AND r.predicate = 'killer'
				  AND r.object_entity_id = $%d
				  AND r.valid_to IS NULL
			)
		)`, len(args), len(args), len(args))
	}
	if query.KillerTypeID != "" {
		args = append(args, query.KillerTypeID)
		where += fmt.Sprintf(" AND k.killer_type_lookup = $%d", len(args))
	}
	if query.ReporterID != "" {
		args = append(args, query.ReporterID)
		where += fmt.Sprintf(` AND (
			k.reporter_lookup = $%d OR EXISTS (
				SELECT 1 FROM entity_relations r
				WHERE r.subject_entity_id = k.id
				  AND r.predicate = 'reported_by'
				  AND r.object_entity_id = $%d
				  AND r.valid_to IS NULL
			)
		)`, len(args), len(args))
	}
	if query.NPCOnly != nil {
		prefix := ""
		if !*query.NPCOnly {
			prefix = "NOT "
		}
		where += ` AND ` + prefix + `(
			k.killer_type_lookup IS NOT NULL OR EXISTS (
				SELECT 1 FROM entity_relations r
				JOIN entities e ON e.id = r.object_entity_id
				WHERE r.subject_entity_id = k.id
				  AND r.predicate = 'killer'
				  AND r.valid_to IS NULL
				  AND e.entity_type = 'enemy'
			)
		)`
	}
	if query.From != nil {
		args = append(args, *query.From)
		where += fmt.Sprintf(" AND k.occurred_at >= $%d", len(args))
	}
	if query.To != nil {
		args = append(args, *query.To)
		where += fmt.Sprintf(" AND k.occurred_at <= $%d", len(args))
	}
	if query.Cursor != "" {
		decoded, err := cursor.Decode(query.Cursor)
		if err != nil {
			return nil, "", err
		}
		args = append(args, decoded.Time, decoded.ID)
		where += fmt.Sprintf(" AND (k.occurred_at, k.id) < ($%d, $%d)", len(args)-1, len(args))
	}
	args = append(args, limit+1)
	rows, err := s.Pool.Query(ctx, `
		WITH normalised AS (
		  SELECT
		    k.*,
		    nullif(coalesce(
		      nullif(k.system_id, ''),
		      nullif(k.system_name, ''),
		      CASE
		        WHEN nullif(k.raw_json #>> '{event,json,solar_system_id,item_id}', '') IS NOT NULL
		        THEN 'system:' || coalesce(nullif(k.raw_json #>> '{event,json,solar_system_id,tenant}', ''), k.environment::text) || ':' || (k.raw_json #>> '{event,json,solar_system_id,item_id}')
		        ELSE NULL
		      END
		    ), '') AS system_lookup,
		    nullif(coalesce(
		      nullif(k.victim_character_id, ''),
		      nullif(k.victim_name, ''),
		      CASE
		        WHEN nullif(k.raw_json #>> '{event,json,victim_id,item_id}', '') IS NOT NULL
		        THEN 'character:' || coalesce(nullif(k.raw_json #>> '{event,json,victim_id,tenant}', ''), k.environment::text) || ':' || (k.raw_json #>> '{event,json,victim_id,item_id}')
		        ELSE NULL
		      END
		    ), '') AS victim_lookup,
		    nullif(coalesce(
		      nullif(k.reporter_character_id, ''),
		      nullif(k.reporter_name, ''),
		      CASE
		        WHEN nullif(k.raw_json #>> '{event,json,reported_by_character_id,item_id}', '') IS NOT NULL
		        THEN 'character:' || coalesce(nullif(k.raw_json #>> '{event,json,reported_by_character_id,tenant}', ''), k.environment::text) || ':' || (k.raw_json #>> '{event,json,reported_by_character_id,item_id}')
		        ELSE NULL
		      END
		    ), '') AS reporter_lookup,
		    nullif(coalesce(
		      nullif(k.killer_character_id, ''),
		      CASE
		        WHEN nullif(k.raw_json #>> '{event,json,killer_id,item_id}', '') IS NOT NULL
		        THEN 'character:' || coalesce(nullif(k.raw_json #>> '{event,json,killer_id,tenant}', ''), k.environment::text) || ':' || (k.raw_json #>> '{event,json,killer_id,item_id}')
		        ELSE NULL
		      END
		    ), '') AS killer_character_lookup,
		    nullif(coalesce(nullif(k.killer_type_id, ''), k.raw_json #>> '{event,json,killer_type_id}'), '') AS killer_type_lookup,
		    nullif(k.killer_name, '') AS killer_name_lookup
		  FROM killmails k
		)
		SELECT k.id, k.environment, k.occurred_at, coalesce(k.system_id, ''), coalesce(k.system_name, ''),
		  coalesce(k.victim_character_id, ''), coalesce(k.victim_name, ''),
		  coalesce(k.killer_character_id, ''), coalesce(k.killer_name, ''), coalesce(k.killer_type_id, ''),
		  coalesce(k.reporter_character_id, ''), coalesce(k.reporter_name, ''), coalesce(k.loss_type, ''),
		  k.source_ids, k.raw_json
		FROM normalised k `+where+`
		ORDER BY k.occurred_at DESC, k.id DESC
		LIMIT $`+strconv.Itoa(len(args)), args...)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	var out []model.KillmailRaw
	for rows.Next() {
		item, err := scanKillmailRaw(rows)
		if err != nil {
			return nil, "", err
		}
		out = append(out, item)
	}
	next := ""
	if len(out) > limit {
		out = out[:limit]
		last := out[len(out)-1]
		encoded, err := cursor.Encode(cursor.Keyset{Time: last.OccurredAt, ID: last.ID})
		if err != nil {
			return nil, "", err
		}
		next = encoded
	}
	return out, next, rows.Err()
}

func (s PostgresStore) CountKillmailResolution(ctx context.Context, environment model.Environment) (KillmailResolutionCounts, error) {
	return s.CountKillmailResolutionFiltered(ctx, KillmailQuery{Environment: environment})
}

func (s PostgresStore) CountKillmailResolutionFiltered(ctx context.Context, query KillmailQuery) (KillmailResolutionCounts, error) {
	scopedWhere := "WHERE ($1::text = '' OR $1::text = 'unknown' OR environment = nullif($1::text, '')::registry_environment)"
	if query.ExcludeFixtures {
		scopedWhere += " AND NOT " + killmailFixtureSQL("killmails")
	}
	row := s.Pool.QueryRow(ctx, `
		WITH scoped AS (
		  SELECT *
		  FROM killmails
		  `+scopedWhere+`
		),
		resolved_entities AS (
		  SELECT DISTINCT e.id, e.entity_type
		  FROM entities e
		  JOIN entity_facts f ON f.entity_id = e.id
		  WHERE ($1::text = '' OR $1::text = 'unknown' OR e.environment = nullif($1::text, '')::registry_environment)
		    AND f.confidence <> 'unknown'
		    AND e.entity_type IN ('character', 'system')
		),
		resolved_enemy_types AS (
		  SELECT DISTINCT f.value_json #>> '{}' AS type_id
		  FROM entities e
		  JOIN entity_facts f ON f.entity_id = e.id
		  WHERE ($1::text = '' OR $1::text = 'unknown' OR e.environment = nullif($1::text, '')::registry_environment)
		    AND e.entity_type = 'enemy'
		    AND f.key = 'type_id'
		    AND f.confidence <> 'unknown'
		    AND nullif(f.value_json #>> '{}', '') IS NOT NULL
		),
		normalised AS (
		  SELECT
		    k.*,
		    nullif(coalesce(
		      nullif(k.system_id, ''),
		      nullif(k.system_name, ''),
		      CASE
		        WHEN nullif(k.raw_json #>> '{event,json,solar_system_id,item_id}', '') IS NOT NULL
		        THEN 'system:' || coalesce(nullif(k.raw_json #>> '{event,json,solar_system_id,tenant}', ''), k.environment::text) || ':' || (k.raw_json #>> '{event,json,solar_system_id,item_id}')
		        ELSE NULL
		      END
		    ), '') AS system_lookup,
		    nullif(coalesce(
		      nullif(k.victim_character_id, ''),
		      nullif(k.victim_name, ''),
		      CASE
		        WHEN nullif(k.raw_json #>> '{event,json,victim_id,item_id}', '') IS NOT NULL
		        THEN 'character:' || coalesce(nullif(k.raw_json #>> '{event,json,victim_id,tenant}', ''), k.environment::text) || ':' || (k.raw_json #>> '{event,json,victim_id,item_id}')
		        ELSE NULL
		      END
		    ), '') AS victim_lookup,
		    nullif(coalesce(
		      nullif(k.reporter_character_id, ''),
		      nullif(k.reporter_name, ''),
		      CASE
		        WHEN nullif(k.raw_json #>> '{event,json,reported_by_character_id,item_id}', '') IS NOT NULL
		        THEN 'character:' || coalesce(nullif(k.raw_json #>> '{event,json,reported_by_character_id,tenant}', ''), k.environment::text) || ':' || (k.raw_json #>> '{event,json,reported_by_character_id,item_id}')
		        ELSE NULL
		      END
		    ), '') AS reporter_lookup,
		    nullif(coalesce(
		      nullif(k.killer_character_id, ''),
		      CASE
		        WHEN nullif(k.raw_json #>> '{event,json,killer_id,item_id}', '') IS NOT NULL
		        THEN 'character:' || coalesce(nullif(k.raw_json #>> '{event,json,killer_id,tenant}', ''), k.environment::text) || ':' || (k.raw_json #>> '{event,json,killer_id,item_id}')
		        ELSE NULL
		      END
		    ), '') AS killer_character_lookup,
		    nullif(coalesce(nullif(k.killer_type_id, ''), k.raw_json #>> '{event,json,killer_type_id}'), '') AS killer_type_lookup,
		    nullif(k.killer_name, '') AS killer_name_lookup
		  FROM scoped k
		),
		resolved AS (
		  SELECT
		    n.id,
		    n.system_lookup,
		    n.victim_lookup,
		    n.reporter_lookup,
		    n.killer_character_lookup,
		    n.killer_type_lookup,
		    n.killer_name_lookup,
		    system_entity.id IS NOT NULL AS raw_system_resolved,
		    victim_entity.id IS NOT NULL AS raw_victim_resolved,
		    reporter_entity.id IS NOT NULL AS raw_reporter_resolved,
		    killer_character.id IS NOT NULL AS raw_killer_character_resolved,
		    killer_enemy.type_id IS NOT NULL AS raw_killer_enemy_resolved
		  FROM normalised n
		  LEFT JOIN resolved_entities system_entity
		    ON system_entity.entity_type = 'system' AND system_entity.id = n.system_lookup
		  LEFT JOIN resolved_entities victim_entity
		    ON victim_entity.entity_type = 'character' AND victim_entity.id = n.victim_lookup
		  LEFT JOIN resolved_entities reporter_entity
		    ON reporter_entity.entity_type = 'character' AND reporter_entity.id = n.reporter_lookup
		  LEFT JOIN resolved_entities killer_character
		    ON killer_character.entity_type = 'character' AND killer_character.id = n.killer_character_lookup
		  LEFT JOIN resolved_enemy_types killer_enemy
		    ON killer_enemy.type_id = n.killer_type_lookup
		),
		graph AS (
		  SELECT
		    r.subject_entity_id AS killmail_id,
		    bool_or(r.predicate = 'occurred_in' AND e.entity_type = 'system' AND r.confidence <> 'unknown') AS system_resolved,
		    bool_or(r.predicate = 'victim' AND e.entity_type = 'character' AND r.confidence <> 'unknown') AS victim_resolved,
		    bool_or(r.predicate = 'reported_by' AND e.entity_type = 'character' AND r.confidence <> 'unknown') AS reporter_resolved,
		    bool_or(r.predicate = 'killer' AND e.entity_type = 'character' AND r.confidence <> 'unknown') AS killer_character_resolved,
		    bool_or(r.predicate = 'killer' AND e.entity_type = 'enemy' AND r.confidence <> 'unknown') AS killer_enemy_resolved
		  FROM entity_relations r
		  JOIN entities e ON e.id = r.object_entity_id
		  JOIN scoped k ON k.id = r.subject_entity_id
		  WHERE r.environment = k.environment
		    AND r.predicate IN ('occurred_in', 'victim', 'reported_by', 'killer')
		  GROUP BY r.subject_entity_id
		),
		final AS (
		  SELECT
		    r.*,
		    CASE
		      WHEN r.system_lookup IS NOT NULL THEN r.raw_system_resolved
		      ELSE coalesce(g.system_resolved, false)
		    END AS system_resolved,
		    CASE
		      WHEN r.victim_lookup IS NOT NULL THEN r.raw_victim_resolved
		      ELSE coalesce(g.victim_resolved, false)
		    END AS victim_resolved,
		    CASE
		      WHEN r.reporter_lookup IS NOT NULL THEN r.raw_reporter_resolved
		      ELSE coalesce(g.reporter_resolved, false)
		    END AS reporter_resolved,
		    CASE
		      WHEN r.killer_type_lookup IS NOT NULL AND r.raw_killer_enemy_resolved THEN true
		      WHEN r.killer_character_lookup IS NOT NULL THEN r.raw_killer_character_resolved
		      WHEN r.killer_type_lookup IS NOT NULL THEN false
		      WHEN r.killer_name_lookup IS NOT NULL THEN false
		      ELSE coalesce(g.killer_character_resolved, false) OR coalesce(g.killer_enemy_resolved, false)
		    END AS killer_resolved,
		    CASE
		      WHEN r.killer_type_lookup IS NOT NULL AND r.raw_killer_enemy_resolved THEN false
		      WHEN r.killer_character_lookup IS NOT NULL THEN r.raw_killer_character_resolved
		      WHEN r.killer_type_lookup IS NOT NULL THEN false
		      WHEN r.killer_name_lookup IS NOT NULL THEN false
		      ELSE coalesce(g.killer_character_resolved, false)
		    END AS character_killer,
		    CASE
		      WHEN r.killer_type_lookup IS NOT NULL AND r.raw_killer_enemy_resolved THEN true
		      WHEN r.killer_character_lookup IS NOT NULL THEN false
		      WHEN r.killer_type_lookup IS NOT NULL THEN false
		      WHEN r.killer_name_lookup IS NOT NULL THEN false
		      ELSE coalesce(g.killer_enemy_resolved, false)
		    END AS npc_killer
		  FROM resolved r
		  LEFT JOIN graph g ON g.killmail_id = r.id
		)
		SELECT
		  count(*)::bigint,
		  count(*) FILTER (WHERE system_resolved)::bigint,
		  count(*) FILTER (WHERE NOT system_resolved)::bigint,
		  count(*) FILTER (WHERE victim_resolved)::bigint,
		  count(*) FILTER (WHERE NOT victim_resolved)::bigint,
		  count(*) FILTER (WHERE killer_resolved)::bigint,
		  count(*) FILTER (WHERE NOT killer_resolved)::bigint,
		  count(*) FILTER (WHERE reporter_resolved)::bigint,
		  count(*) FILTER (WHERE NOT reporter_resolved)::bigint,
		  count(*) FILTER (WHERE character_killer)::bigint,
		  count(*) FILTER (WHERE npc_killer)::bigint
		FROM final
	`, string(query.Environment))
	var counts KillmailResolutionCounts
	if err := row.Scan(
		&counts.Total,
		&counts.ResolvedSystems,
		&counts.UnresolvedSystems,
		&counts.ResolvedVictims,
		&counts.UnresolvedVictims,
		&counts.ResolvedKillers,
		&counts.UnresolvedKillers,
		&counts.ResolvedReporters,
		&counts.UnresolvedReporters,
		&counts.CharacterKillers,
		&counts.NPCKillers,
	); err != nil {
		return KillmailResolutionCounts{}, err
	}
	return counts, nil
}

func killmailFixtureSQL(alias string) string {
	return fmt.Sprintf(`(
		%s.id LIKE '%%:fixture:%%'
		OR EXISTS (
			SELECT 1
			FROM unnest(%s.source_ids) AS fixture_source_id
			WHERE fixture_source_id = 'source:fixture'
			   OR fixture_source_id LIKE 'source:fixture:%%'
			   OR fixture_source_id LIKE '%%:fixture:%%'
		)
	)`, alias, alias)
}

func (s PostgresStore) GetKillmailRaw(ctx context.Context, id string) (model.KillmailRaw, bool, error) {
	row := s.Pool.QueryRow(ctx, `
		SELECT id, environment, occurred_at, coalesce(system_id, ''), coalesce(system_name, ''),
		  coalesce(victim_character_id, ''), coalesce(victim_name, ''),
		  coalesce(killer_character_id, ''), coalesce(killer_name, ''), coalesce(killer_type_id, ''),
		  coalesce(reporter_character_id, ''), coalesce(reporter_name, ''), coalesce(loss_type, ''),
		  source_ids, raw_json
		FROM killmails WHERE id = $1
	`, id)
	item, err := scanKillmailRaw(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.KillmailRaw{}, false, nil
		}
		return model.KillmailRaw{}, false, err
	}
	return item, true, nil
}

func (s PostgresStore) ResolveCharacter(ctx context.Context, idOrName string, environment model.Environment) (model.ResolvedValue, bool, error) {
	return s.resolveEntity(ctx, idOrName, environment, model.EntityTypeCharacter)
}

func (s PostgresStore) ResolveSystem(ctx context.Context, idOrName string, environment model.Environment) (model.ResolvedValue, bool, error) {
	return s.resolveEntity(ctx, idOrName, environment, model.EntityTypeSystem)
}

func (s PostgresStore) ResolveEnemyType(ctx context.Context, typeID string, environment model.Environment) (model.ResolvedValue, bool, error) {
	id := fmt.Sprintf("enemy:%s:type:%s", environment, typeID)
	entity, ok, err := s.GetEntity(ctx, id)
	if err != nil || !ok {
		return model.ResolvedValue{}, ok, err
	}
	sources, confidence := s.sourcesForEntity(ctx, entity.ID)
	return model.ResolvedValue{
		EntityID:    entity.ID,
		EntityType:  entity.Type,
		TypeID:      typeID,
		DisplayName: displayName(entity),
		Confidence:  confidence,
		SourceIDs:   sources,
	}, true, nil
}

func (s PostgresStore) ListCursors(ctx context.Context) ([]CursorStatus, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT id, source, environment, cursor_value, cursor_kind, last_successful_ingest, coalesce(last_checkpoint, ''),
		  events_processed, error_count, coalesce(last_error_summary, ''), updated_at
		FROM sync_cursors
		ORDER BY updated_at DESC, id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CursorStatus
	for rows.Next() {
		var item CursorStatus
		if err := rows.Scan(&item.ID, &item.Source, &item.Environment, &item.CursorValue, &item.CursorKind, &item.LastSuccessfulIngest, &item.LastCheckpoint, &item.EventsProcessed, &item.ErrorCount, &item.LastErrorSummary, &item.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s PostgresStore) EnsureSource(ctx context.Context, source model.Source) error {
	if s.Pool == nil {
		return errors.New("postgres pool is nil")
	}
	tx, err := s.Pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := upsertSource(ctx, tx, source); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s PostgresStore) UpsertSuiEvent(ctx context.Context, event EventRecord) error {
	payload, err := json.Marshal(event.Payload)
	if err != nil {
		return err
	}
	_, err = s.Pool.Exec(ctx, `
		INSERT INTO events(id, event_kind, environment, occurred_at, cycle, package_id, module, transaction_digest, checkpoint, source_id, payload_json)
		VALUES ($1, $2, $3, $4, $5, nullif($6, ''), nullif($7, ''), nullif($8, ''), nullif($9, ''), nullif($10, ''), $11::jsonb)
		ON CONFLICT (id) DO UPDATE SET
		  event_kind = EXCLUDED.event_kind,
		  environment = EXCLUDED.environment,
		  occurred_at = EXCLUDED.occurred_at,
		  cycle = EXCLUDED.cycle,
		  package_id = EXCLUDED.package_id,
		  module = EXCLUDED.module,
		  transaction_digest = EXCLUDED.transaction_digest,
		  checkpoint = EXCLUDED.checkpoint,
		  source_id = EXCLUDED.source_id,
		  payload_json = EXCLUDED.payload_json
	`, event.ID, event.Kind, event.Environment, event.OccurredAt, event.Cycle, event.PackageID, event.Module, event.TransactionDigest, event.Checkpoint, event.SourceID, string(payload))
	return err
}

func (s PostgresStore) UpsertSuiObject(ctx context.Context, object SuiObjectRecord) error {
	payload, err := json.Marshal(object.Payload)
	if err != nil {
		return err
	}
	_, err = s.Pool.Exec(ctx, `
		INSERT INTO sui_objects(
		  id, object_id, environment, type_repr, package_id, module, type_name,
		  version, digest, source_id, payload_json, observed_at, updated_at
		)
		VALUES ($1, $2, $3, $4, nullif($5, ''), nullif($6, ''), nullif($7, ''),
		  nullif($8, ''), nullif($9, ''), nullif($10, ''), $11::jsonb, $12, now())
		ON CONFLICT (id) DO UPDATE SET
		  object_id = EXCLUDED.object_id,
		  environment = EXCLUDED.environment,
		  type_repr = EXCLUDED.type_repr,
		  package_id = EXCLUDED.package_id,
		  module = EXCLUDED.module,
		  type_name = EXCLUDED.type_name,
		  version = EXCLUDED.version,
		  digest = EXCLUDED.digest,
		  source_id = EXCLUDED.source_id,
		  payload_json = EXCLUDED.payload_json,
		  observed_at = EXCLUDED.observed_at,
		  updated_at = now()
	`, object.ID, object.ObjectID, object.Environment, object.TypeRepr, object.PackageID, object.Module, object.TypeName, object.Version, object.Digest, object.SourceID, string(payload), object.ObservedAt)
	return err
}

func (s PostgresStore) UpsertEntityFacts(ctx context.Context, entity model.Entity, facts []EntityFactDraft) error {
	if s.Pool == nil {
		return errors.New("postgres pool is nil")
	}
	tx, err := s.Pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := upsertEntity(ctx, tx, entity); err != nil {
		return err
	}
	preserveExistingDisplay := shouldPreserveExistingEntityOnPlaceholder(entity)
	searchBody := entity.Summary
	for _, fact := range facts {
		if fact.Confidence == "" {
			fact.Confidence = model.ConfidenceUnknown
		}
		if fact.Environment == "" {
			fact.Environment = entity.Environment
		}
		if fact.Cycle == nil {
			fact.Cycle = entity.Cycle
		}
		if fact.ReviewStatus == "" {
			fact.ReviewStatus = model.ReviewStatusCandidate
		}
		if err := upsertFact(ctx, tx, entity.ID, fact.Key, fact.Value, "", fact.SourceID, fact.Confidence, fact.Environment, fact.Cycle, fact.ReviewStatus); err != nil {
			return err
		}
		searchBody += " " + fact.Key + " " + fmt.Sprint(fact.Value)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO search_terms(entity_id, entity_type, name, aliases, body, updated_at)
		VALUES ($1, $2, $3, $4, $5, now())
		ON CONFLICT (entity_id) DO UPDATE SET
		  entity_type = EXCLUDED.entity_type,
		  name = CASE WHEN $6 THEN search_terms.name ELSE EXCLUDED.name END,
		  aliases = CASE WHEN $6 THEN search_terms.aliases ELSE EXCLUDED.aliases END,
		  body = CASE WHEN $6 THEN search_terms.body ELSE EXCLUDED.body END,
		  updated_at = now()
	`, entity.ID, entity.Type, entity.Name, entity.DisplayName, searchBody, preserveExistingDisplay); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s PostgresStore) UpsertRelations(ctx context.Context, relations []RelationDraft) error {
	if s.Pool == nil {
		return errors.New("postgres pool is nil")
	}
	if len(relations) == 0 {
		return nil
	}
	tx, err := s.Pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	for _, relation := range relations {
		if relation.SubjectEntityID == "" || relation.Predicate == "" || relation.ObjectEntityID == "" {
			return errors.New("relation subject, predicate and object are required")
		}
		if relation.SourceID == "" {
			return errors.New("relation source id is required")
		}
		if relation.Confidence == "" {
			relation.Confidence = model.ConfidenceUnknown
		}
		if relation.Environment == "" {
			relation.Environment = model.EnvironmentUnknown
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO entity_relations(
			  id, subject_entity_id, predicate, object_entity_id, source_id, confidence, environment, valid_from, valid_to
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			ON CONFLICT (id) DO UPDATE SET
			  source_id = EXCLUDED.source_id,
			  confidence = EXCLUDED.confidence,
			  environment = EXCLUDED.environment,
			  valid_from = EXCLUDED.valid_from,
			  valid_to = EXCLUDED.valid_to
		`, RelationID(relation), relation.SubjectEntityID, relation.Predicate, relation.ObjectEntityID, relation.SourceID, relation.Confidence, relation.Environment, relation.ValidFrom, relation.ValidTo); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s PostgresStore) ListEntityFacts(ctx context.Context, entityID string) ([]model.Fact, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT entity_id, key, value_json, source_id, confidence, environment, cycle, review_status, coalesce(import_id, ''), published_at
		FROM entity_facts
		WHERE entity_id = $1
		ORDER BY key ASC, created_at DESC, id ASC
	`, entityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Fact
	for rows.Next() {
		var fact model.Fact
		var cycle pgtype.Int4
		var valueJSON []byte
		if err := rows.Scan(&fact.EntityID, &fact.Key, &valueJSON, &fact.SourceID, &fact.Confidence, &fact.Environment, &cycle, &fact.ReviewStatus, &fact.ImportID, &fact.PublishedAt); err != nil {
			return nil, err
		}
		fact.Cycle = cycleFromPG(cycle)
		if len(valueJSON) > 0 {
			if err := json.Unmarshal(valueJSON, &fact.Value); err != nil {
				return nil, err
			}
		}
		out = append(out, fact)
	}
	return out, rows.Err()
}

func (s PostgresStore) ListEntityRelations(ctx context.Context, entityID string) ([]model.Relation, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT subject_entity_id, predicate, object_entity_id, source_id, confidence, environment
		FROM entity_relations
		WHERE subject_entity_id = $1 OR object_entity_id = $1
		ORDER BY predicate ASC, object_entity_id ASC, subject_entity_id ASC
	`, entityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Relation
	for rows.Next() {
		var relation model.Relation
		if err := rows.Scan(&relation.SubjectEntityID, &relation.Predicate, &relation.ObjectEntityID, &relation.SourceID, &relation.Confidence, &relation.Environment); err != nil {
			return nil, err
		}
		out = append(out, relation)
	}
	return out, rows.Err()
}

func (s PostgresStore) ListEntitySources(ctx context.Context, entityID string) ([]model.Source, error) {
	rows, err := s.Pool.Query(ctx, `
		WITH source_ids AS (
		  SELECT source_id FROM entity_facts WHERE entity_id = $1
		  UNION
		  SELECT source_id FROM entity_relations WHERE subject_entity_id = $1 OR object_entity_id = $1
		)
		SELECT s.id, s.kind, s.title, s.locator, coalesce(s.url, ''), s.environment, s.cycle, s.metadata, s.created_at
		FROM sources s
		JOIN source_ids ids ON ids.source_id = s.id
		ORDER BY s.id ASC
	`, entityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Source
	for rows.Next() {
		source, err := scanSource(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, source)
	}
	return out, rows.Err()
}

func (s PostgresStore) GetSyncCursor(ctx context.Context, id string) (CursorStatus, bool, error) {
	row := s.Pool.QueryRow(ctx, `
		SELECT id, source, environment, cursor_value, cursor_kind, last_successful_ingest, coalesce(last_checkpoint, ''),
		  events_processed, error_count, coalesce(last_error_summary, ''), updated_at
		FROM sync_cursors
		WHERE id = $1
	`, id)
	var item CursorStatus
	if err := row.Scan(&item.ID, &item.Source, &item.Environment, &item.CursorValue, &item.CursorKind, &item.LastSuccessfulIngest, &item.LastCheckpoint, &item.EventsProcessed, &item.ErrorCount, &item.LastErrorSummary, &item.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return CursorStatus{}, false, nil
		}
		return CursorStatus{}, false, err
	}
	return item, true, nil
}

func (s PostgresStore) SaveSyncCursor(ctx context.Context, item CursorStatus) error {
	_, err := s.Pool.Exec(ctx, `
		INSERT INTO sync_cursors(
		  id, source, environment, cursor_value, cursor_kind, last_successful_ingest, last_checkpoint,
		  events_processed, error_count, last_error_summary, updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, nullif($7, ''), $8, $9, nullif($10, ''), now())
		ON CONFLICT (id) DO UPDATE SET
		  source = EXCLUDED.source,
		  environment = EXCLUDED.environment,
		  cursor_value = EXCLUDED.cursor_value,
		  cursor_kind = EXCLUDED.cursor_kind,
		  last_successful_ingest = EXCLUDED.last_successful_ingest,
		  last_checkpoint = EXCLUDED.last_checkpoint,
		  events_processed = EXCLUDED.events_processed,
		  error_count = EXCLUDED.error_count,
		  last_error_summary = EXCLUDED.last_error_summary,
		  updated_at = now()
	`, item.ID, item.Source, item.Environment, item.CursorValue, item.CursorKind, item.LastSuccessfulIngest, item.LastCheckpoint, item.EventsProcessed, item.ErrorCount, item.LastErrorSummary)
	return err
}

func (s PostgresStore) ListFreshness(ctx context.Context) ([]FreshnessStatus, error) {
	cursors, err := s.ListCursors(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]FreshnessStatus, 0, len(cursors))
	for _, cursor := range cursors {
		status := model.FreshnessUnknown
		if cursor.LastSuccessfulIngest != nil {
			if time.Since(*cursor.LastSuccessfulIngest) <= 15*time.Minute {
				status = model.FreshnessLiveIndexed
			} else {
				status = model.FreshnessCachedSnapshot
			}
		}
		out = append(out, FreshnessStatus{
			Source:               cursor.Source,
			Environment:          cursor.Environment,
			LastSuccessfulIngest: cursor.LastSuccessfulIngest,
			LastCheckpoint:       cursor.LastCheckpoint,
			EventsProcessed:      cursor.EventsProcessed,
			ErrorCount:           cursor.ErrorCount,
			LastErrorSummary:     cursor.LastErrorSummary,
			StalenessStatus:      status,
			UpdatedAt:            cursor.UpdatedAt,
		})
	}
	return out, nil
}

func (s PostgresStore) ListSourceGaps(ctx context.Context, environment model.Environment) ([]model.SourceGap, error) {
	var ownershipEvidenceOnly int64
	var locationEvidenceOnly int64
	var unresolvedKillmails int64
	var recipeCount int64
	var suiObjectRangeBlocked int64
	var placeholderTribeNames int64
	var tribeProfileGaps int64
	err := s.Pool.QueryRow(ctx, `
		SELECT
		  (
		    SELECT count(DISTINCT e.id)
		    FROM entities e
		    JOIN entity_facts f ON f.entity_id = e.id AND f.key = 'owner_cap_id' AND nullif(f.value_json #>> '{}', '') IS NOT NULL
		    WHERE e.entity_type IN ('assembly', 'gate', 'storage', 'turret')
		      AND ($1::text = '' OR e.environment = nullif($1::text, '')::registry_environment)
		      AND NOT EXISTS (
		        SELECT 1 FROM entity_relations r
		        WHERE r.subject_entity_id = e.id AND r.predicate = 'owned_by' AND r.valid_to IS NULL
		      )
		  ),
		  (
		    SELECT count(DISTINCT e.id)
		    FROM entities e
		    JOIN entity_facts f ON f.entity_id = e.id AND f.key = 'location_hash' AND nullif(f.value_json #>> '{}', '') IS NOT NULL
		    WHERE e.entity_type IN ('assembly', 'gate', 'storage', 'turret')
		      AND ($1::text = '' OR e.environment = nullif($1::text, '')::registry_environment)
		      AND NOT EXISTS (
		        SELECT 1 FROM entity_relations r
		        WHERE r.subject_entity_id = e.id AND r.predicate = 'located_in' AND r.valid_to IS NULL
		      )
		  ),
		  (
		    SELECT count(*)
		    FROM killmails k
		    WHERE ($1::text = '' OR k.environment = nullif($1::text, '')::registry_environment)
		      AND (
		        nullif(k.system_id, '') IS NULL
		        OR nullif(k.victim_character_id, '') IS NULL
		        OR (nullif(k.killer_character_id, '') IS NULL AND nullif(k.killer_type_id, '') IS NULL)
		        OR nullif(k.reporter_character_id, '') IS NULL
		      )
		  ),
		  (
		    SELECT count(*)
		    FROM entities e
		    WHERE e.entity_type = 'recipe'
		      AND ($1::text = '' OR e.environment = nullif($1::text, '')::registry_environment)
		  ),
		  (
		    SELECT count(*)
		    FROM sync_cursors c
		    WHERE c.cursor_kind = 'sui_object'
		      AND c.last_error_summary ILIKE '%outside consistent range%'
		      AND ($1::text = '' OR c.environment = nullif($1::text, '')::registry_environment)
		  ),
		  (
		    SELECT count(*)
		    FROM entities e
		    WHERE e.entity_type = 'tribe'
		      AND e.name = 'Tribe ' || regexp_replace(e.id, '^.*:', '')
		      AND (coalesce(e.display_name, '') = '' OR e.display_name = e.name)
		      AND ($1::text = '' OR e.environment = nullif($1::text, '')::registry_environment)
		  ),
		  (
		    SELECT count(*)
		    FROM entities e
		    WHERE e.entity_type = 'tribe'
		      AND ($1::text = '' OR e.environment = nullif($1::text, '')::registry_environment)
		      AND NOT EXISTS (
		        SELECT 1
		        FROM entity_facts f
		        WHERE f.entity_id = e.id
		          AND f.key IN ('tag', 'aliases', 'description', 'url')
		          AND (
		            (f.key = 'aliases'
		              AND jsonb_typeof(f.value_json) = 'array'
		              AND EXISTS (
		                SELECT 1
		                FROM jsonb_array_elements_text(f.value_json) AS alias(value)
		                WHERE nullif(btrim(alias.value), '') IS NOT NULL
		              ))
		            OR (f.key <> 'aliases' AND nullif(btrim(f.value_json #>> '{}'), '') IS NOT NULL)
		          )
		      )
		  )
	`, string(environment)).Scan(&ownershipEvidenceOnly, &locationEvidenceOnly, &unresolvedKillmails, &recipeCount, &suiObjectRangeBlocked, &placeholderTribeNames, &tribeProfileGaps)
	if err != nil {
		return nil, err
	}
	return sourceGapRows(environment, ownershipEvidenceOnly, locationEvidenceOnly, unresolvedKillmails, recipeCount, suiObjectRangeBlocked, placeholderTribeNames, tribeProfileGaps), nil
}

func (s PostgresStore) CreateReview(ctx context.Context, draft ReviewDraft) (model.Review, error) {
	if strings.TrimSpace(draft.TargetKind) == "" || strings.TrimSpace(draft.TargetID) == "" {
		return model.Review{}, errors.New("review target kind and id are required")
	}
	id := ReviewID(draft.TargetKind, draft.TargetID)
	row := s.Pool.QueryRow(ctx, `
		INSERT INTO reviews(id, target_kind, target_id, review_status, notes)
		VALUES ($1, $2, $3, $4, nullif($5, ''))
		ON CONFLICT (id) DO UPDATE SET
		  notes = EXCLUDED.notes
		RETURNING id, target_kind, target_id, review_status, coalesce(reviewer, ''), coalesce(notes, ''), reviewed_at, created_at
	`, id, draft.TargetKind, draft.TargetID, model.ReviewStatusCandidate, draft.Notes)
	return scanReview(row)
}

func (s PostgresStore) ListReviews(ctx context.Context, status model.ReviewStatus) ([]model.Review, error) {
	var args []any
	where := "WHERE 1=1"
	if status != "" {
		args = append(args, status)
		where += fmt.Sprintf(" AND review_status = $%d", len(args))
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT id, target_kind, target_id, review_status, coalesce(reviewer, ''), coalesce(notes, ''), reviewed_at, created_at
		FROM reviews `+where+`
		ORDER BY created_at DESC, id DESC
	`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Review
	for rows.Next() {
		review, err := scanReview(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, review)
	}
	return out, rows.Err()
}

func (s PostgresStore) UpdateReviewStatus(ctx context.Context, id string, status model.ReviewStatus, update ReviewUpdate) (model.Review, bool, error) {
	row := s.Pool.QueryRow(ctx, `
		UPDATE reviews
		SET review_status = $2,
		    reviewer = nullif($3, ''),
		    notes = nullif($4, ''),
		    reviewed_at = now()
		WHERE id = $1
		RETURNING id, target_kind, target_id, review_status, coalesce(reviewer, ''), coalesce(notes, ''), reviewed_at, created_at
	`, id, status, update.Reviewer, update.Notes)
	review, err := scanReview(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.Review{}, false, nil
		}
		return model.Review{}, false, err
	}
	return review, true, nil
}

func (s PostgresStore) resolveEntity(ctx context.Context, idOrName string, environment model.Environment, entityType model.EntityType) (model.ResolvedValue, bool, error) {
	row := s.Pool.QueryRow(ctx, `
		SELECT id, slug, entity_type, name, coalesce(display_name, ''), coalesce(summary, ''), environment, cycle, updated_at
		FROM entities
		WHERE entity_type = $1
		  AND ($2::registry_environment = 'unknown' OR environment = $2)
		  AND (id = $3 OR slug = $3 OR lower(name) = lower($3))
		ORDER BY updated_at DESC
		LIMIT 1
	`, entityType, environment, idOrName)
	entity, err := scanEntity(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.ResolvedValue{}, false, nil
		}
		return model.ResolvedValue{}, false, err
	}
	sources, confidence := s.sourcesForEntity(ctx, entity.ID)
	resolvedDisplayName := displayName(entity)
	if preferred, preferredSource, preferredConfidence, ok := s.preferredDisplayFact(ctx, entity.ID); ok {
		resolvedDisplayName = preferred
		if preferredConfidence != "" {
			confidence = preferredConfidence
		}
		if preferredSource != "" && !containsString(sources, preferredSource) {
			sources = append(sources, preferredSource)
		}
	}
	return model.ResolvedValue{
		EntityID:    entity.ID,
		EntityType:  entity.Type,
		RawID:       idOrName,
		DisplayName: resolvedDisplayName,
		Confidence:  confidence,
		SourceIDs:   sources,
	}, true, nil
}

func (s PostgresStore) preferredDisplayFact(ctx context.Context, entityID string) (string, string, model.Confidence, bool) {
	row := s.Pool.QueryRow(ctx, `
		SELECT value_json #>> '{}', source_id, confidence
		FROM entity_facts
		WHERE entity_id = $1
		  AND key IN ('metadata_name', 'display_name')
		  AND nullif(value_json #>> '{}', '') IS NOT NULL
		ORDER BY CASE key WHEN 'metadata_name' THEN 0 ELSE 1 END, created_at DESC, id DESC
		LIMIT 1
	`, entityID)
	var value string
	var sourceID string
	var confidence model.Confidence
	if err := row.Scan(&value, &sourceID, &confidence); err != nil {
		return "", "", "", false
	}
	return value, sourceID, confidence, true
}

func (s PostgresStore) sourcesForEntity(ctx context.Context, entityID string) ([]string, model.Confidence) {
	rows, err := s.Pool.Query(ctx, `
		SELECT DISTINCT source_id, confidence
		FROM entity_facts
		WHERE entity_id = $1
		ORDER BY source_id
	`, entityID)
	if err != nil {
		return nil, model.ConfidenceUnknown
	}
	defer rows.Close()
	var sources []string
	confidence := model.ConfidenceUnknown
	for rows.Next() {
		var source string
		var found model.Confidence
		if err := rows.Scan(&source, &found); err != nil {
			return sources, confidence
		}
		sources = append(sources, source)
		if confidence == model.ConfidenceUnknown {
			confidence = found
		}
	}
	return sources, confidence
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func upsertSource(ctx context.Context, tx pgx.Tx, source model.Source) error {
	metadata, err := json.Marshal(source.Metadata)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO sources(id, kind, title, locator, url, environment, cycle, metadata, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8::jsonb, coalesce(nullif($9::timestamptz, '0001-01-01T00:00:00Z'::timestamptz), now()))
		ON CONFLICT (id) DO UPDATE SET
		  kind = EXCLUDED.kind,
		  title = EXCLUDED.title,
		  locator = EXCLUDED.locator,
		  url = EXCLUDED.url,
		  environment = EXCLUDED.environment,
		  cycle = EXCLUDED.cycle,
		  metadata = EXCLUDED.metadata
	`, source.ID, source.Kind, source.Title, source.Locator, source.URL, source.Environment, source.Cycle, string(metadata), source.CreatedAt)
	return err
}

func upsertArtefact(ctx context.Context, tx pgx.Tx, artefact model.SourceArtefact) error {
	sourceKind := artefact.SourceKind
	if sourceKind == "" {
		sourceKind = model.SourceKindStaticClientData
	}
	artefactKind := artefact.ArtefactKind
	if artefactKind == "" {
		artefactKind = artefact.Kind
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO source_artefacts(
		  id, source_id, source_kind, kind, artefact_kind, environment, path_or_uri, sha256, size_bytes, row_count, content_type,
		  extracted_at, importer_name, importer_version, client_build, patch_label, cycle, review_status, superseded_by_artefact_id, notes, created_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, nullif($19, ''), $20, $21)
		ON CONFLICT (id) DO UPDATE SET
		  source_kind = EXCLUDED.source_kind,
		  kind = EXCLUDED.kind,
		  artefact_kind = EXCLUDED.artefact_kind,
		  environment = EXCLUDED.environment,
		  path_or_uri = EXCLUDED.path_or_uri,
		  sha256 = EXCLUDED.sha256,
		  size_bytes = EXCLUDED.size_bytes,
		  row_count = EXCLUDED.row_count,
		  content_type = EXCLUDED.content_type,
		  extracted_at = EXCLUDED.extracted_at,
		  importer_name = EXCLUDED.importer_name,
		  importer_version = EXCLUDED.importer_version,
		  client_build = EXCLUDED.client_build,
		  patch_label = EXCLUDED.patch_label,
		  cycle = EXCLUDED.cycle,
		  review_status = EXCLUDED.review_status,
		  superseded_by_artefact_id = EXCLUDED.superseded_by_artefact_id,
		  notes = EXCLUDED.notes
	`, artefact.ID, artefact.SourceID, sourceKind, artefact.Kind, artefactKind, artefact.Environment, artefact.PathOrURI, artefact.SHA256, artefact.SizeBytes, artefact.RowCount, artefact.ContentType, artefact.ExtractedAt, artefact.ImporterName, artefact.ImporterVersion, artefact.ClientBuild, artefact.PatchLabel, artefact.Cycle, artefact.ReviewStatus, artefact.SupersededByArtefactID, artefact.Notes, artefact.CreatedAt)
	return err
}

func upsertEntity(ctx context.Context, tx pgx.Tx, entity model.Entity) error {
	preserveExistingDisplay := shouldPreserveExistingEntityOnPlaceholder(entity)
	_, err := tx.Exec(ctx, `
		INSERT INTO entities(id, slug, entity_type, name, display_name, summary, environment, cycle, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, now())
		ON CONFLICT (id) DO UPDATE SET
		  slug = CASE WHEN $9 THEN entities.slug ELSE EXCLUDED.slug END,
		  entity_type = EXCLUDED.entity_type,
		  name = CASE WHEN $9 THEN entities.name ELSE EXCLUDED.name END,
		  display_name = CASE WHEN $9 THEN entities.display_name ELSE EXCLUDED.display_name END,
		  summary = CASE WHEN $9 THEN entities.summary ELSE EXCLUDED.summary END,
		  environment = EXCLUDED.environment,
		  cycle = EXCLUDED.cycle,
		  updated_at = now()
	`, entity.ID, entity.Slug, entity.Type, entity.Name, entity.DisplayName, entity.Summary, entity.Environment, entity.Cycle, preserveExistingDisplay)
	return err
}

func upsertFact(ctx context.Context, tx pgx.Tx, entityID, key string, value any, importID, sourceID string, confidence model.Confidence, environment model.Environment, cycle *int, reviewStatus model.ReviewStatus) error {
	valueJSON, err := json.Marshal(value)
	if err != nil {
		return err
	}
	factID := fmt.Sprintf("fact:%s:%s", entityID, key)
	_, err = tx.Exec(ctx, `
		INSERT INTO entity_facts(id, entity_id, key, value_json, source_id, confidence, environment, cycle, review_status, import_id, published_at)
		VALUES ($1, $2, $3, $4::jsonb, $5, $6, $7, $8, $9, nullif($10, ''), now())
		ON CONFLICT (id) DO UPDATE SET
		  value_json = EXCLUDED.value_json,
		  source_id = EXCLUDED.source_id,
		  confidence = EXCLUDED.confidence,
		  environment = EXCLUDED.environment,
		  cycle = EXCLUDED.cycle,
		  review_status = EXCLUDED.review_status,
		  import_id = EXCLUDED.import_id,
		  published_at = EXCLUDED.published_at
	`, factID, entityID, key, string(valueJSON), sourceID, confidence, environment, cycle, reviewStatus, importID)
	return err
}

func dedupeEntityFactSets(items []EntityFactSet) []EntityFactSet {
	type mergedEntity struct {
		entity    model.Entity
		facts     map[string]EntityFactDraft
		factOrder []string
	}
	entities := make(map[string]*mergedEntity, len(items))
	order := make([]string, 0, len(items))
	for _, item := range items {
		if item.Entity.ID == "" {
			continue
		}
		merged, ok := entities[item.Entity.ID]
		if !ok {
			merged = &mergedEntity{facts: make(map[string]EntityFactDraft, len(item.Facts))}
			entities[item.Entity.ID] = merged
			order = append(order, item.Entity.ID)
		}
		merged.entity = item.Entity
		for _, fact := range item.Facts {
			if fact.Key == "" {
				continue
			}
			if _, ok := merged.facts[fact.Key]; !ok {
				merged.factOrder = append(merged.factOrder, fact.Key)
			}
			merged.facts[fact.Key] = fact
		}
	}
	out := make([]EntityFactSet, 0, len(order))
	for _, id := range order {
		merged := entities[id]
		facts := make([]EntityFactDraft, 0, len(merged.factOrder))
		for _, key := range merged.factOrder {
			facts = append(facts, merged.facts[key])
		}
		out = append(out, EntityFactSet{Entity: merged.entity, Facts: facts})
	}
	return out
}

func dedupeRelations(items []RelationDraft) []RelationDraft {
	out := make([]RelationDraft, 0, len(items))
	seen := make(map[string]int, len(items))
	for _, item := range items {
		id := RelationID(item)
		if index, ok := seen[id]; ok {
			out[index] = item
			continue
		}
		seen[id] = len(out)
		out = append(out, item)
	}
	return out
}

func dedupeKillmails(items []model.KillmailRaw) []model.KillmailRaw {
	out := make([]model.KillmailRaw, 0, len(items))
	seen := make(map[string]int, len(items))
	for _, item := range items {
		if item.ID == "" {
			continue
		}
		if index, ok := seen[item.ID]; ok {
			out[index] = item
			continue
		}
		seen[item.ID] = len(out)
		out = append(out, item)
	}
	return out
}

type entityScanner interface {
	Scan(dest ...any) error
}

func scanEntity(row entityScanner) (model.Entity, error) {
	var entity model.Entity
	var cycle pgtype.Int4
	if err := row.Scan(&entity.ID, &entity.Slug, &entity.Type, &entity.Name, &entity.DisplayName, &entity.Summary, &entity.Environment, &cycle, &entity.UpdatedAt); err != nil {
		return model.Entity{}, err
	}
	entity.Cycle = cycleFromPG(cycle)
	return entity, nil
}

func scanCurrentEntity(row entityScanner) (model.CurrentEntity, error) {
	var item model.CurrentEntity
	var cycle pgtype.Int4
	var factsJSON []byte
	var outgoingJSON []byte
	var incomingJSON []byte
	if err := row.Scan(
		&item.Entity.ID, &item.Entity.Slug, &item.Entity.Type, &item.Entity.Name, &item.Entity.DisplayName,
		&item.Entity.Summary, &item.Entity.Environment, &cycle, &item.Entity.UpdatedAt,
		&factsJSON, &outgoingJSON, &incomingJSON, &item.SourceIDs,
	); err != nil {
		return model.CurrentEntity{}, err
	}
	item.Entity.Cycle = cycleFromPG(cycle)
	if len(factsJSON) > 0 {
		if err := json.Unmarshal(factsJSON, &item.Facts); err != nil {
			return model.CurrentEntity{}, err
		}
	}
	if item.Facts == nil {
		item.Facts = map[string]any{}
	}
	if len(outgoingJSON) > 0 {
		if err := json.Unmarshal(outgoingJSON, &item.OutgoingRelations); err != nil {
			return model.CurrentEntity{}, err
		}
	}
	for i := range item.OutgoingRelations {
		item.OutgoingRelations[i].SubjectEntityID = item.Entity.ID
	}
	if len(incomingJSON) > 0 {
		if err := json.Unmarshal(incomingJSON, &item.IncomingRelations); err != nil {
			return model.CurrentEntity{}, err
		}
	}
	for i := range item.IncomingRelations {
		item.IncomingRelations[i].ObjectEntityID = item.Entity.ID
	}
	deriveCurrentEntity(&item)
	return item, nil
}

func scanCurrentRelation(row entityScanner) (model.CurrentRelation, error) {
	var item model.CurrentRelation
	if err := row.Scan(
		&item.ID, &item.SubjectEntityID, &item.SubjectEntityType, &item.SubjectDisplayName,
		&item.Predicate, &item.ObjectEntityID, &item.ObjectEntityType, &item.ObjectDisplayName,
		&item.SourceID, &item.Confidence, &item.Environment, &item.CreatedAt,
	); err != nil {
		return model.CurrentRelation{}, err
	}
	return item, nil
}

func scanSource(row entityScanner) (model.Source, error) {
	var source model.Source
	var cycle pgtype.Int4
	var metadata []byte
	if err := row.Scan(&source.ID, &source.Kind, &source.Title, &source.Locator, &source.URL, &source.Environment, &cycle, &metadata, &source.CreatedAt); err != nil {
		return model.Source{}, err
	}
	source.Cycle = cycleFromPG(cycle)
	if len(metadata) > 0 {
		if err := json.Unmarshal(metadata, &source.Metadata); err != nil {
			return model.Source{}, err
		}
	}
	return source, nil
}

func scanReview(row entityScanner) (model.Review, error) {
	var review model.Review
	if err := row.Scan(&review.ID, &review.TargetKind, &review.TargetID, &review.ReviewStatus, &review.Reviewer, &review.Notes, &review.ReviewedAt, &review.CreatedAt); err != nil {
		return model.Review{}, err
	}
	return review, nil
}

func scanEvent(row entityScanner) (EventRecord, error) {
	var item EventRecord
	var cycle pgtype.Int4
	var payload []byte
	if err := row.Scan(&item.ID, &item.Kind, &item.Environment, &item.OccurredAt, &cycle, &item.PackageID, &item.Module, &item.TransactionDigest, &item.Checkpoint, &item.SourceID, &payload); err != nil {
		return EventRecord{}, err
	}
	item.Cycle = cycleFromPG(cycle)
	if len(payload) > 0 {
		if err := json.Unmarshal(payload, &item.Payload); err != nil {
			return EventRecord{}, err
		}
	}
	return item, nil
}

func cycleFromPG(value pgtype.Int4) *int {
	if !value.Valid {
		return nil
	}
	cycle := int(value.Int32)
	return &cycle
}

func scanSuiObject(row entityScanner) (SuiObjectRecord, error) {
	var item SuiObjectRecord
	var payload []byte
	if err := row.Scan(&item.ID, &item.ObjectID, &item.Environment, &item.TypeRepr, &item.PackageID, &item.Module, &item.TypeName, &item.Version, &item.Digest, &item.SourceID, &payload, &item.ObservedAt); err != nil {
		return SuiObjectRecord{}, err
	}
	if len(payload) > 0 {
		if err := json.Unmarshal(payload, &item.Payload); err != nil {
			return SuiObjectRecord{}, err
		}
	}
	return item, nil
}

func scanKillmailRaw(row entityScanner) (model.KillmailRaw, error) {
	var item model.KillmailRaw
	var raw []byte
	if err := row.Scan(
		&item.ID, &item.Environment, &item.OccurredAt, &item.SystemID, &item.SystemName,
		&item.VictimCharacterID, &item.VictimName,
		&item.KillerCharacterID, &item.KillerName, &item.KillerTypeID,
		&item.ReporterCharacterID, &item.ReporterName, &item.LossType,
		&item.SourceIDs, &raw,
	); err != nil {
		return model.KillmailRaw{}, err
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &item.Raw); err != nil {
			return model.KillmailRaw{}, err
		}
	}
	return item, nil
}

func displayName(entity model.Entity) string {
	if entity.DisplayName != "" {
		return entity.DisplayName
	}
	return entity.Name
}

func saneLimit(value, fallback, max int) int {
	if value <= 0 {
		return fallback
	}
	if value > max {
		return max
	}
	return value
}

func IsUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
