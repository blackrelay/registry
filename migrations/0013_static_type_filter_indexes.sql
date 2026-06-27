CREATE INDEX IF NOT EXISTS entity_facts_static_type_id_idx
  ON entity_facts ((md5(value_json #>> '{}')), entity_id)
  WHERE key = 'type_id' AND valid_to IS NULL;

CREATE INDEX IF NOT EXISTS entity_facts_static_group_id_idx
  ON entity_facts ((md5(value_json #>> '{}')), entity_id)
  WHERE key = 'group_id' AND valid_to IS NULL;

CREATE INDEX IF NOT EXISTS entity_facts_static_category_id_idx
  ON entity_facts ((md5(value_json #>> '{}')), entity_id)
  WHERE key = 'category_id' AND valid_to IS NULL;

CREATE INDEX IF NOT EXISTS entity_facts_static_market_group_id_idx
  ON entity_facts ((md5(value_json #>> '{}')), entity_id)
  WHERE key = 'market_group_id' AND valid_to IS NULL;

CREATE INDEX IF NOT EXISTS entity_facts_static_wreck_type_id_idx
  ON entity_facts ((md5(value_json #>> '{}')), entity_id)
  WHERE key = 'wreck_type_id' AND valid_to IS NULL;

CREATE INDEX IF NOT EXISTS entity_facts_static_entity_type_idx
  ON entity_facts ((md5(value_json #>> '{}')), entity_id)
  WHERE key = 'static_entity_type' AND valid_to IS NULL;
