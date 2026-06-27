UPDATE events
SET cycle = NULL
WHERE cycle IS NOT NULL
  AND cycle < 5;

UPDATE entities
SET cycle = NULL,
    updated_at = now()
WHERE cycle IS NOT NULL
  AND cycle < 5;

UPDATE entity_facts
SET cycle = NULL
WHERE cycle IS NOT NULL
  AND cycle < 5;
