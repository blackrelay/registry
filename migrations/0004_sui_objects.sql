CREATE TABLE IF NOT EXISTS sui_objects (
  id text PRIMARY KEY,
  object_id text NOT NULL,
  environment registry_environment NOT NULL,
  type_repr text NOT NULL,
  package_id text,
  module text,
  type_name text,
  version text,
  digest text,
  source_id text REFERENCES sources(id) ON DELETE SET NULL,
  payload_json jsonb NOT NULL,
  observed_at timestamptz NOT NULL DEFAULT now(),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS sui_objects_object_version_idx
  ON sui_objects (environment, object_id, version);

CREATE INDEX IF NOT EXISTS sui_objects_type_observed_idx
  ON sui_objects (environment, package_id, module, type_name, observed_at DESC, id);

CREATE INDEX IF NOT EXISTS sui_objects_source_observed_idx
  ON sui_objects (source_id, observed_at DESC, id);
