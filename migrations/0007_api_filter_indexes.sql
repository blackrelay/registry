CREATE INDEX IF NOT EXISTS entities_environment_type_updated_idx
  ON entities (environment, entity_type, updated_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS entity_relations_environment_predicate_subject_idx
  ON entity_relations (environment, predicate, subject_entity_id, created_at DESC, id DESC)
  WHERE valid_to IS NULL;

CREATE INDEX IF NOT EXISTS entity_relations_environment_predicate_object_idx
  ON entity_relations (environment, predicate, object_entity_id, created_at DESC, id DESC)
  WHERE valid_to IS NULL;

CREATE INDEX IF NOT EXISTS killmails_environment_system_time_idx
  ON killmails (environment, system_id, occurred_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS killmails_environment_victim_time_idx
  ON killmails (environment, victim_character_id, occurred_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS killmails_environment_killer_character_time_idx
  ON killmails (environment, killer_character_id, occurred_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS killmails_environment_killer_type_time_idx
  ON killmails (environment, killer_type_id, occurred_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS killmails_environment_reporter_time_idx
  ON killmails (environment, reporter_character_id, occurred_at DESC, id DESC);
