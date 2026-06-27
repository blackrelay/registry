ALTER TABLE events
  ADD COLUMN IF NOT EXISTS cycle integer CHECK (cycle IS NULL OR cycle > 0);

CREATE INDEX IF NOT EXISTS events_environment_cycle_time_idx
  ON events (environment, cycle, occurred_at DESC, id DESC);
