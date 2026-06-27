DROP INDEX IF EXISTS events_environment_cycle_time_idx;

UPDATE events
SET cycle = CASE
  WHEN occurred_at >= timestamptz '2026-06-25T09:00:00Z' THEN 6
  WHEN occurred_at >= timestamptz '2026-03-11T09:00:00Z' THEN 5
  ELSE NULL
END
WHERE cycle IS DISTINCT FROM CASE
  WHEN occurred_at >= timestamptz '2026-06-25T09:00:00Z' THEN 6
  WHEN occurred_at >= timestamptz '2026-03-11T09:00:00Z' THEN 5
  ELSE NULL
END;

CREATE INDEX IF NOT EXISTS events_environment_cycle_time_idx
  ON events (environment, cycle, occurred_at DESC, id DESC);
