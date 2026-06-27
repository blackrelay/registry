package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/blackrelay/registry/internal/model"
	"github.com/blackrelay/registry/internal/snapshots"
	"github.com/jackc/pgx/v5"
)

func (s PostgresStore) FindArtefactByHash(ctx context.Context, sha256 string) (model.SourceArtefact, bool, error) {
	row := s.Pool.QueryRow(ctx, `
		SELECT id, source_id, source_kind, kind, artefact_kind, environment, path_or_uri, sha256, size_bytes, row_count,
		  content_type, extracted_at, importer_name, importer_version, coalesce(client_build, ''), coalesce(patch_label, ''),
		  cycle, review_status, coalesce(superseded_by_artefact_id, ''), coalesce(notes, ''), created_at
		FROM source_artefacts
		WHERE sha256 = $1
		LIMIT 1
	`, sha256)
	artefact, err := scanArtefact(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.SourceArtefact{}, false, nil
		}
		return model.SourceArtefact{}, false, err
	}
	return artefact, true, nil
}

func (s PostgresStore) CurrentSnapshot(ctx context.Context, environment model.Environment, kind string) (snapshots.SnapshotSet, bool, error) {
	row := s.Pool.QueryRow(ctx, `
		SELECT ss.id, ss.environment, ss.kind, ss.label, ss.source_summary, ss.created_at, coalesce(ss.superseded_by_snapshot_set_id, ''), coalesce(ss.notes, ''),
		  sa.id, sa.source_id, sa.source_kind, sa.kind, sa.artefact_kind, sa.environment, sa.path_or_uri, sa.sha256, sa.size_bytes, sa.row_count,
		  sa.content_type, sa.extracted_at, sa.importer_name, sa.importer_version, coalesce(sa.client_build, ''), coalesce(sa.patch_label, ''),
		  sa.cycle, sa.review_status, coalesce(sa.superseded_by_artefact_id, ''), coalesce(sa.notes, ''), sa.created_at
		FROM snapshot_sets ss
		JOIN snapshot_set_artefacts ssa ON ssa.snapshot_set_id = ss.id
		JOIN source_artefacts sa ON sa.id = ssa.source_artefact_id
		WHERE ss.environment = $1
		  AND ss.kind = $2
		  AND ss.superseded_by_snapshot_set_id IS NULL
		ORDER BY ss.created_at DESC, ss.id DESC
		LIMIT 1
	`, environment, kind)
	var snapshot snapshots.SnapshotSet
	if err := row.Scan(
		&snapshot.ID, &snapshot.Environment, &snapshot.Kind, &snapshot.Label, &snapshot.SourceSummary, &snapshot.CreatedAt, &snapshot.SupersededBySnapshotSetID, &snapshot.Notes,
		&snapshot.Artefact.ID, &snapshot.Artefact.SourceID, &snapshot.Artefact.SourceKind, &snapshot.Artefact.Kind, &snapshot.Artefact.ArtefactKind, &snapshot.Artefact.Environment,
		&snapshot.Artefact.PathOrURI, &snapshot.Artefact.SHA256, &snapshot.Artefact.SizeBytes, &snapshot.Artefact.RowCount, &snapshot.Artefact.ContentType,
		&snapshot.Artefact.ExtractedAt, &snapshot.Artefact.ImporterName, &snapshot.Artefact.ImporterVersion, &snapshot.Artefact.ClientBuild, &snapshot.Artefact.PatchLabel,
		&snapshot.Artefact.Cycle, &snapshot.Artefact.ReviewStatus, &snapshot.Artefact.SupersededByArtefactID, &snapshot.Artefact.Notes, &snapshot.Artefact.CreatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return snapshots.SnapshotSet{}, false, nil
		}
		return snapshots.SnapshotSet{}, false, err
	}
	rows, err := s.snapshotRows(ctx, snapshot.Artefact.ID)
	if err != nil {
		return snapshots.SnapshotSet{}, false, err
	}
	snapshot.Rows = rows
	return snapshot, true, nil
}

func (s PostgresStore) RecordSnapshotNoop(ctx context.Context, record snapshots.NoopRecord) error {
	if _, err := s.Pool.Exec(ctx, `
		INSERT INTO sources(id, kind, title, locator, url, environment, cycle, metadata, created_at)
		VALUES ($1, $2, $3, $4, '', $5, $6, $7::jsonb, $8)
		ON CONFLICT (id) DO UPDATE SET metadata = EXCLUDED.metadata
	`, record.Source.ID, record.Source.Kind, record.Source.Title, record.Source.Locator, record.Source.Environment, record.Source.Cycle, mustJSON(record.Source.Metadata), record.CreatedAt); err != nil {
		return err
	}
	metadata := map[string]any{
		"artefactHash":          record.ArtefactHash,
		"artefactId":            record.ArtefactID,
		"reason":                record.Reason,
		"rowCount":              record.RowCount,
		"byteIdentical":         record.ByteIdentical,
		"semanticallyUnchanged": record.SemanticallyUnchanged,
	}
	_, err := s.Pool.Exec(ctx, `
		INSERT INTO ingest_runs(id, source_id, environment, status, started_at, finished_at, metadata)
		VALUES ($1, $2, $3, 'no_op', $4, $4, $5::jsonb)
		ON CONFLICT (id) DO UPDATE SET metadata = EXCLUDED.metadata
	`, record.ID, record.Source.ID, record.Environment, record.CreatedAt, mustJSON(metadata))
	return err
}

func (s PostgresStore) RegisterSnapshotCandidate(ctx context.Context, source model.Source, artefact model.SourceArtefact, rows []snapshots.NormalizedEnemyRow) error {
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
	if err := upsertSnapshotRows(ctx, tx, artefact.ID, rows); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s PostgresStore) PromoteSnapshot(ctx context.Context, promotion snapshots.Promotion) error {
	tx, err := s.Pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := upsertSource(ctx, tx, promotion.Source); err != nil {
		return err
	}
	if err := upsertArtefact(ctx, tx, promotion.Artefact); err != nil {
		return err
	}
	if err := upsertSnapshotRows(ctx, tx, promotion.Artefact.ID, promotion.Rows); err != nil {
		return err
	}
	if promotion.PreviousSnapshot != nil {
		if _, err := tx.Exec(ctx, `UPDATE snapshot_sets SET superseded_by_snapshot_set_id = $1 WHERE id = $2`, promotion.NewSnapshot.ID, promotion.PreviousSnapshot.ID); err != nil {
			return err
		}
		if promotion.PreviousSnapshot.Artefact.ID != "" {
			if _, err := tx.Exec(ctx, `UPDATE source_artefacts SET superseded_by_artefact_id = $1 WHERE id = $2`, promotion.Artefact.ID, promotion.PreviousSnapshot.Artefact.ID); err != nil {
				return err
			}
		}
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO snapshot_sets(id, environment, kind, label, source_summary, created_at, notes)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (id) DO UPDATE SET
		  label = EXCLUDED.label,
		  source_summary = EXCLUDED.source_summary,
		  notes = EXCLUDED.notes
	`, promotion.NewSnapshot.ID, promotion.NewSnapshot.Environment, promotion.NewSnapshot.Kind, promotion.NewSnapshot.Label, promotion.NewSnapshot.SourceSummary, promotion.NewSnapshot.CreatedAt, promotion.NewSnapshot.Notes); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO snapshot_set_artefacts(snapshot_set_id, source_artefact_id)
		VALUES ($1, $2)
		ON CONFLICT DO NOTHING
	`, promotion.NewSnapshot.ID, promotion.Artefact.ID); err != nil {
		return err
	}
	previousID := ""
	if promotion.PreviousSnapshot != nil {
		previousID = promotion.PreviousSnapshot.ID
	}
	diffJSON := mustJSON(promotion.Diff)
	if _, err := tx.Exec(ctx, `
		INSERT INTO snapshot_diffs(id, source_id, previous_artefact_id, current_artefact_id, environment, diff_kind, diff_json, from_snapshot_set_id, to_snapshot_set_id, kind, summary_json)
		VALUES ($1, $2, nullif($3, ''), $4, $5, 'static_enemy_jsonl', $6::jsonb, nullif($7, ''), $8, $9, $6::jsonb)
		ON CONFLICT (id) DO UPDATE SET summary_json = EXCLUDED.summary_json
	`, fmt.Sprintf("snapshot-diff:%s", promotion.NewSnapshot.ID), promotion.Source.ID, previousArtefactID(promotion.PreviousSnapshot), promotion.Artefact.ID, promotion.Artefact.Environment, diffJSON, previousID, promotion.NewSnapshot.ID, promotion.NewSnapshot.Kind); err != nil {
		return err
	}
	for _, job := range promotion.OutboxJobs {
		if _, err := tx.Exec(ctx, `
			INSERT INTO outbox_jobs(id, job_kind, status, payload_json)
			VALUES ($1, $2, 'queued', $3::jsonb)
			ON CONFLICT DO NOTHING
		`, job.ID, job.Kind, mustJSON(job.Payload)); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s PostgresStore) snapshotRows(ctx context.Context, artefactID string) ([]snapshots.NormalizedEnemyRow, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT row_json
		FROM snapshot_artefact_rows
		WHERE source_artefact_id = $1
		ORDER BY row_key
	`, artefactID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []snapshots.NormalizedEnemyRow
	for rows.Next() {
		var data []byte
		var row snapshots.NormalizedEnemyRow
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(data, &row); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func upsertSnapshotRows(ctx context.Context, tx pgx.Tx, artefactID string, rows []snapshots.NormalizedEnemyRow) error {
	for _, row := range rows {
		rowJSON, err := json.Marshal(row)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO snapshot_artefact_rows(source_artefact_id, row_key, row_json)
			VALUES ($1, $2, $3::jsonb)
			ON CONFLICT (source_artefact_id, row_key) DO UPDATE SET row_json = EXCLUDED.row_json
		`, artefactID, row.Key(), string(rowJSON)); err != nil {
			return err
		}
	}
	return nil
}

func scanArtefact(row entityScanner) (model.SourceArtefact, error) {
	var artefact model.SourceArtefact
	if err := row.Scan(&artefact.ID, &artefact.SourceID, &artefact.SourceKind, &artefact.Kind, &artefact.ArtefactKind, &artefact.Environment, &artefact.PathOrURI, &artefact.SHA256, &artefact.SizeBytes, &artefact.RowCount, &artefact.ContentType, &artefact.ExtractedAt, &artefact.ImporterName, &artefact.ImporterVersion, &artefact.ClientBuild, &artefact.PatchLabel, &artefact.Cycle, &artefact.ReviewStatus, &artefact.SupersededByArtefactID, &artefact.Notes, &artefact.CreatedAt); err != nil {
		return model.SourceArtefact{}, err
	}
	return artefact, nil
}

func previousArtefactID(snapshot *snapshots.SnapshotSet) string {
	if snapshot == nil {
		return ""
	}
	return snapshot.Artefact.ID
}

func mustJSON(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(data)
}
