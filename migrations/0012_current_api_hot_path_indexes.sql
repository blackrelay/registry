CREATE INDEX IF NOT EXISTS entity_facts_key_value_entity_idx
  ON entity_facts (key, (md5(value_json #>> '{}')), entity_id)
  WHERE valid_to IS NULL;

CREATE INDEX IF NOT EXISTS entity_facts_owner_cap_value_idx
  ON entity_facts ((md5(value_json #>> '{}')), entity_id)
  WHERE key = 'owner_cap_id' AND valid_to IS NULL;

CREATE INDEX IF NOT EXISTS entity_facts_location_hash_value_idx
  ON entity_facts ((md5(value_json #>> '{}')), entity_id)
  WHERE key = 'location_hash' AND valid_to IS NULL;

CREATE INDEX IF NOT EXISTS entity_relations_current_subject_predicate_object_idx
  ON entity_relations (subject_entity_id, predicate, object_entity_id)
  WHERE valid_to IS NULL;

CREATE INDEX IF NOT EXISTS entity_relations_current_object_predicate_subject_idx
  ON entity_relations (object_entity_id, predicate, subject_entity_id)
  WHERE valid_to IS NULL;

CREATE INDEX IF NOT EXISTS killmails_environment_time_id_idx
  ON killmails (environment, occurred_at DESC, id DESC)
  INCLUDE (system_id, victim_character_id, killer_character_id, killer_type_id, reporter_character_id);

CREATE INDEX IF NOT EXISTS events_environment_module_time_idx
  ON events (environment, module, occurred_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS sui_objects_environment_type_time_idx
  ON sui_objects (environment, type_name, observed_at DESC, id DESC)
  INCLUDE (package_id, module);
