CREATE INDEX IF NOT EXISTS events_package_module_time_idx
  ON events (environment, package_id, module, occurred_at DESC, id);

CREATE INDEX IF NOT EXISTS events_transaction_digest_idx
  ON events (transaction_digest);

CREATE INDEX IF NOT EXISTS events_source_time_idx
  ON events (source_id, occurred_at DESC, id);
