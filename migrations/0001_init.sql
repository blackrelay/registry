CREATE TABLE IF NOT EXISTS schema_migrations (
  version text PRIMARY KEY,
  applied_at timestamptz NOT NULL DEFAULT now()
);

DO $$
BEGIN
  CREATE TYPE registry_environment AS ENUM ('stillness', 'utopia', 'unknown');
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

DO $$
BEGIN
  CREATE TYPE registry_source_kind AS ENUM (
    'onchain',
    'sui_event',
    'sui_object',
    'world_api',
    'datahub',
    'static_client_data',
    'reverse_engineered',
    'observed_gameplay',
    'community_report',
    'manual_inference'
  );
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

DO $$
BEGIN
  CREATE TYPE registry_confidence AS ENUM ('verified', 'probable', 'reported', 'stale', 'unknown');
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

DO $$
BEGIN
  CREATE TYPE registry_review_status AS ENUM ('candidate', 'reviewed', 'published', 'rejected', 'superseded');
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

DO $$
BEGIN
  CREATE TYPE registry_freshness AS ENUM (
    'live_indexed',
    'cached_snapshot',
    'static_cycle_data',
    'cycle_archive',
    'manual_observation',
    'unknown'
  );
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

DO $$
BEGIN
  CREATE TYPE registry_entity_type AS ENUM (
    'character',
    'player',
    'tribe',
    'alliance',
    'item',
    'material',
    'recipe',
    'blueprint',
    'ship',
    'structure',
    'assembly',
    'gate',
    'storage',
    'market',
    'turret',
    'system',
    'region',
    'constellation',
    'site',
    'resource_object',
    'enemy',
    'killmail',
    'event',
    'route',
    'unknown'
  );
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

CREATE TABLE IF NOT EXISTS sources (
  id text PRIMARY KEY,
  kind registry_source_kind NOT NULL,
  title text NOT NULL,
  locator text NOT NULL,
  url text,
  environment registry_environment NOT NULL DEFAULT 'unknown',
  cycle integer CHECK (cycle IS NULL OR cycle > 0),
  metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS source_artefacts (
  id text PRIMARY KEY,
  source_id text NOT NULL REFERENCES sources(id) ON DELETE RESTRICT,
  kind text NOT NULL,
  environment registry_environment NOT NULL,
  path_or_uri text NOT NULL,
  sha256 text NOT NULL CHECK (sha256 ~ '^[a-f0-9]{64}$'),
  size_bytes bigint NOT NULL CHECK (size_bytes >= 0),
  content_type text NOT NULL,
  extracted_at timestamptz NOT NULL,
  importer_name text NOT NULL,
  importer_version text NOT NULL,
  review_status registry_review_status NOT NULL,
  notes text,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS source_artefacts_sha256_idx ON source_artefacts (sha256);

CREATE TABLE IF NOT EXISTS imports (
  id text PRIMARY KEY,
  source_id text NOT NULL REFERENCES sources(id) ON DELETE RESTRICT,
  artefact_id text NOT NULL REFERENCES source_artefacts(id) ON DELETE RESTRICT,
  environment registry_environment NOT NULL,
  importer_name text NOT NULL,
  importer_version text NOT NULL,
  summary jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS reviews (
  id text PRIMARY KEY,
  target_kind text NOT NULL,
  target_id text NOT NULL,
  review_status registry_review_status NOT NULL,
  reviewer text,
  notes text,
  reviewed_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS ingest_runs (
  id text PRIMARY KEY,
  source_id text REFERENCES sources(id) ON DELETE SET NULL,
  environment registry_environment NOT NULL,
  status text NOT NULL,
  started_at timestamptz NOT NULL,
  finished_at timestamptz,
  events_processed bigint NOT NULL DEFAULT 0,
  objects_processed bigint NOT NULL DEFAULT 0,
  error_count bigint NOT NULL DEFAULT 0,
  last_error_summary text,
  metadata jsonb NOT NULL DEFAULT '{}'::jsonb
);

CREATE TABLE IF NOT EXISTS sync_cursors (
  id text PRIMARY KEY,
  source text NOT NULL,
  environment registry_environment NOT NULL,
  cursor_value text NOT NULL,
  cursor_kind text NOT NULL,
  last_successful_ingest timestamptz,
  last_checkpoint text,
  events_processed bigint NOT NULL DEFAULT 0,
  error_count bigint NOT NULL DEFAULT 0,
  last_error_summary text,
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS sync_cursors_source_environment_idx ON sync_cursors (source, environment, cursor_kind);

CREATE TABLE IF NOT EXISTS entities (
  id text PRIMARY KEY,
  slug text NOT NULL UNIQUE,
  entity_type registry_entity_type NOT NULL,
  name text NOT NULL,
  display_name text,
  summary text,
  environment registry_environment NOT NULL DEFAULT 'unknown',
  cycle integer CHECK (cycle IS NULL OR cycle > 0),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS entities_type_environment_idx ON entities (entity_type, environment, updated_at DESC, id);

CREATE TABLE IF NOT EXISTS entity_aliases (
  id text PRIMARY KEY,
  entity_id text NOT NULL REFERENCES entities(id) ON DELETE CASCADE,
  alias text NOT NULL,
  source_id text REFERENCES sources(id) ON DELETE SET NULL,
  confidence registry_confidence NOT NULL DEFAULT 'unknown',
  environment registry_environment NOT NULL DEFAULT 'unknown',
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS entity_aliases_entity_idx ON entity_aliases (entity_id);
CREATE INDEX IF NOT EXISTS entity_aliases_alias_idx ON entity_aliases (lower(alias));

CREATE TABLE IF NOT EXISTS entity_facts (
  id text PRIMARY KEY,
  entity_id text NOT NULL REFERENCES entities(id) ON DELETE CASCADE,
  key text NOT NULL,
  value_json jsonb NOT NULL,
  source_id text NOT NULL REFERENCES sources(id) ON DELETE RESTRICT,
  confidence registry_confidence NOT NULL,
  environment registry_environment NOT NULL,
  cycle integer CHECK (cycle IS NULL OR cycle > 0),
  review_status registry_review_status NOT NULL,
  valid_from timestamptz,
  valid_to timestamptz,
  import_id text REFERENCES imports(id) ON DELETE SET NULL,
  published_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS entity_facts_entity_key_idx ON entity_facts (entity_id, key, created_at DESC);
CREATE INDEX IF NOT EXISTS entity_facts_source_idx ON entity_facts (source_id);

CREATE TABLE IF NOT EXISTS entity_relations (
  id text PRIMARY KEY,
  subject_entity_id text NOT NULL REFERENCES entities(id) ON DELETE CASCADE,
  predicate text NOT NULL,
  object_entity_id text NOT NULL REFERENCES entities(id) ON DELETE CASCADE,
  source_id text NOT NULL REFERENCES sources(id) ON DELETE RESTRICT,
  confidence registry_confidence NOT NULL,
  environment registry_environment NOT NULL,
  valid_from timestamptz,
  valid_to timestamptz,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS entity_relations_subject_idx ON entity_relations (subject_entity_id, predicate);
CREATE INDEX IF NOT EXISTS entity_relations_object_idx ON entity_relations (object_entity_id, predicate);

CREATE TABLE IF NOT EXISTS events (
  id text PRIMARY KEY,
  event_kind text NOT NULL,
  environment registry_environment NOT NULL,
  occurred_at timestamptz NOT NULL,
  package_id text,
  module text,
  transaction_digest text,
  checkpoint text,
  source_id text REFERENCES sources(id) ON DELETE SET NULL,
  payload_json jsonb NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS events_time_idx ON events (occurred_at DESC, id);
CREATE INDEX IF NOT EXISTS events_kind_environment_idx ON events (event_kind, environment, occurred_at DESC, id);

CREATE TABLE IF NOT EXISTS killmails (
  id text PRIMARY KEY,
  environment registry_environment NOT NULL,
  occurred_at timestamptz NOT NULL,
  system_id text,
  system_name text,
  victim_character_id text,
  victim_name text,
  killer_character_id text,
  killer_name text,
  killer_type_id text,
  reporter_character_id text,
  reporter_name text,
  loss_type text,
  source_ids text[] NOT NULL DEFAULT '{}',
  raw_json jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS killmails_time_idx ON killmails (occurred_at DESC, id);
CREATE INDEX IF NOT EXISTS killmails_environment_time_idx ON killmails (environment, occurred_at DESC, id);
CREATE INDEX IF NOT EXISTS killmails_killer_type_idx ON killmails (killer_type_id);

CREATE TABLE IF NOT EXISTS snapshot_diffs (
  id text PRIMARY KEY,
  source_id text NOT NULL REFERENCES sources(id) ON DELETE RESTRICT,
  previous_artefact_id text REFERENCES source_artefacts(id) ON DELETE SET NULL,
  current_artefact_id text NOT NULL REFERENCES source_artefacts(id) ON DELETE RESTRICT,
  environment registry_environment NOT NULL,
  diff_kind text NOT NULL,
  diff_json jsonb NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS search_terms (
  entity_id text PRIMARY KEY REFERENCES entities(id) ON DELETE CASCADE,
  entity_type registry_entity_type NOT NULL,
  name text NOT NULL,
  aliases text NOT NULL DEFAULT '',
  body text NOT NULL DEFAULT '',
  document tsvector GENERATED ALWAYS AS (
    setweight(to_tsvector('simple', coalesce(name, '')), 'A') ||
    setweight(to_tsvector('simple', coalesce(aliases, '')), 'B') ||
    setweight(to_tsvector('simple', coalesce(body, '')), 'C')
  ) STORED,
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS search_terms_document_idx ON search_terms USING gin(document);
CREATE INDEX IF NOT EXISTS search_terms_type_updated_idx ON search_terms (entity_type, updated_at DESC, entity_id);

CREATE OR REPLACE VIEW characters_current AS
  SELECT * FROM entities WHERE entity_type = 'character';

CREATE OR REPLACE VIEW tribes_current AS
  SELECT * FROM entities WHERE entity_type = 'tribe';

CREATE OR REPLACE VIEW assemblies_current AS
  SELECT * FROM entities WHERE entity_type = 'assembly';

CREATE OR REPLACE VIEW gates_current AS
  SELECT * FROM entities WHERE entity_type = 'gate';

CREATE OR REPLACE VIEW storage_current AS
  SELECT * FROM entities WHERE entity_type = 'storage';

CREATE OR REPLACE VIEW turrets_current AS
  SELECT * FROM entities WHERE entity_type = 'turret';

CREATE OR REPLACE VIEW systems_current AS
  SELECT * FROM entities WHERE entity_type = 'system';

CREATE OR REPLACE VIEW routes_current AS
  SELECT * FROM entities WHERE entity_type = 'route';

CREATE OR REPLACE VIEW killmails_resolved_current AS
  SELECT * FROM killmails;
