DROP VIEW IF EXISTS killmails_resolved_current CASCADE;
DROP VIEW IF EXISTS routes_current CASCADE;
DROP VIEW IF EXISTS systems_current CASCADE;
DROP VIEW IF EXISTS turrets_current CASCADE;
DROP VIEW IF EXISTS storage_current CASCADE;
DROP VIEW IF EXISTS gates_current CASCADE;
DROP VIEW IF EXISTS assemblies_current CASCADE;
DROP VIEW IF EXISTS tribes_current CASCADE;
DROP VIEW IF EXISTS characters_current CASCADE;
DROP VIEW IF EXISTS entity_current_state CASCADE;

CREATE VIEW entity_current_state AS
SELECT
  e.id,
  e.slug,
  e.entity_type,
  e.name,
  e.display_name,
  e.summary,
  e.environment,
  e.cycle,
  e.created_at,
  e.updated_at,
  COALESCE(
    (
      SELECT jsonb_object_agg(latest.key, latest.value_json)
      FROM (
        SELECT DISTINCT ON (f.key) f.key, f.value_json
        FROM entity_facts f
        WHERE f.entity_id = e.id
          AND f.valid_to IS NULL
        ORDER BY f.key, f.created_at DESC, f.id DESC
      ) latest
    ),
    '{}'::jsonb
  ) AS facts_json,
  COALESCE(
    (
      SELECT jsonb_agg(
        jsonb_build_object(
          'predicate', r.predicate,
          'objectEntityId', r.object_entity_id,
          'sourceId', r.source_id,
          'confidence', r.confidence,
          'environment', r.environment
        )
        ORDER BY r.predicate, r.object_entity_id
      )
      FROM entity_relations r
      WHERE r.subject_entity_id = e.id
        AND r.valid_to IS NULL
    ),
    '[]'::jsonb
  ) AS outgoing_relations_json,
  COALESCE(
    (
      SELECT jsonb_agg(
        jsonb_build_object(
          'subjectEntityId', r.subject_entity_id,
          'predicate', r.predicate,
          'sourceId', r.source_id,
          'confidence', r.confidence,
          'environment', r.environment
        )
        ORDER BY r.predicate, r.subject_entity_id
      )
      FROM entity_relations r
      WHERE r.object_entity_id = e.id
        AND r.valid_to IS NULL
    ),
    '[]'::jsonb
  ) AS incoming_relations_json,
  COALESCE(
    (
      SELECT array_agg(DISTINCT source_id ORDER BY source_id)
      FROM (
        SELECT f.source_id
        FROM entity_facts f
        WHERE f.entity_id = e.id
        UNION
        SELECT r.source_id
        FROM entity_relations r
        WHERE r.subject_entity_id = e.id OR r.object_entity_id = e.id
      ) sources
    ),
    ARRAY[]::text[]
  ) AS source_ids
FROM entities e;

CREATE VIEW characters_current AS
  SELECT * FROM entity_current_state WHERE entity_type = 'character';

CREATE VIEW tribes_current AS
  SELECT * FROM entity_current_state WHERE entity_type = 'tribe';

CREATE VIEW assemblies_current AS
  SELECT * FROM entity_current_state WHERE entity_type = 'assembly';

CREATE VIEW gates_current AS
  SELECT * FROM entity_current_state WHERE entity_type = 'gate';

CREATE VIEW storage_current AS
  SELECT * FROM entity_current_state WHERE entity_type = 'storage';

CREATE VIEW turrets_current AS
  SELECT * FROM entity_current_state WHERE entity_type = 'turret';

CREATE VIEW systems_current AS
  SELECT * FROM entity_current_state WHERE entity_type = 'system';

CREATE VIEW routes_current AS
  SELECT * FROM entity_current_state WHERE entity_type = 'route';

CREATE VIEW killmails_resolved_current AS
  SELECT
    k.*,
    ecs.facts_json,
    ecs.outgoing_relations_json,
    ecs.source_ids AS entity_source_ids
  FROM killmails k
  LEFT JOIN entity_current_state ecs ON ecs.id = k.id;
