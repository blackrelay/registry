ALTER TABLE source_artefacts
  ADD COLUMN IF NOT EXISTS source_kind registry_source_kind NOT NULL DEFAULT 'static_client_data',
  ADD COLUMN IF NOT EXISTS artefact_kind text,
  ADD COLUMN IF NOT EXISTS row_count bigint NOT NULL DEFAULT 0 CHECK (row_count >= 0),
  ADD COLUMN IF NOT EXISTS client_build text,
  ADD COLUMN IF NOT EXISTS patch_label text,
  ADD COLUMN IF NOT EXISTS cycle integer CHECK (cycle IS NULL OR cycle > 0),
  ADD COLUMN IF NOT EXISTS superseded_by_artefact_id text REFERENCES source_artefacts(id) ON DELETE SET NULL;

UPDATE source_artefacts
SET artefact_kind = kind
WHERE artefact_kind IS NULL;

ALTER TABLE source_artefacts
  ALTER COLUMN artefact_kind SET NOT NULL;

CREATE INDEX IF NOT EXISTS source_artefacts_kind_environment_created_idx
  ON source_artefacts (artefact_kind, environment, created_at DESC, id);

CREATE INDEX IF NOT EXISTS source_artefacts_superseded_idx
  ON source_artefacts (superseded_by_artefact_id)
  WHERE superseded_by_artefact_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS snapshot_sets (
  id text PRIMARY KEY,
  environment registry_environment NOT NULL,
  kind text NOT NULL,
  label text NOT NULL,
  source_summary text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  superseded_by_snapshot_set_id text REFERENCES snapshot_sets(id) ON DELETE SET NULL,
  notes text
);

CREATE INDEX IF NOT EXISTS snapshot_sets_current_idx
  ON snapshot_sets (environment, kind, created_at DESC, id)
  WHERE superseded_by_snapshot_set_id IS NULL;

CREATE TABLE IF NOT EXISTS snapshot_set_artefacts (
  snapshot_set_id text NOT NULL REFERENCES snapshot_sets(id) ON DELETE CASCADE,
  source_artefact_id text NOT NULL REFERENCES source_artefacts(id) ON DELETE RESTRICT,
  PRIMARY KEY (snapshot_set_id, source_artefact_id)
);

ALTER TABLE snapshot_diffs
  ADD COLUMN IF NOT EXISTS from_snapshot_set_id text REFERENCES snapshot_sets(id) ON DELETE SET NULL,
  ADD COLUMN IF NOT EXISTS to_snapshot_set_id text REFERENCES snapshot_sets(id) ON DELETE SET NULL,
  ADD COLUMN IF NOT EXISTS kind text,
  ADD COLUMN IF NOT EXISTS summary_json jsonb NOT NULL DEFAULT '{}'::jsonb;

CREATE INDEX IF NOT EXISTS snapshot_diffs_sets_idx
  ON snapshot_diffs (from_snapshot_set_id, to_snapshot_set_id);

CREATE TABLE IF NOT EXISTS snapshot_artefact_rows (
  source_artefact_id text NOT NULL REFERENCES source_artefacts(id) ON DELETE CASCADE,
  row_key text NOT NULL,
  row_json jsonb NOT NULL,
  PRIMARY KEY (source_artefact_id, row_key)
);

CREATE TABLE IF NOT EXISTS outbox_jobs (
  id text PRIMARY KEY,
  job_kind text NOT NULL,
  status text NOT NULL DEFAULT 'queued',
  payload_json jsonb NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  started_at timestamptz,
  finished_at timestamptz,
  error text
);

CREATE INDEX IF NOT EXISTS outbox_jobs_status_created_idx
  ON outbox_jobs (status, created_at, id);
