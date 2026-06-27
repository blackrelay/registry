package db

import (
	"strings"
	"testing"
)

func TestCurrentStateMigrationEmbedsRelationDisplayNames(t *testing.T) {
	migrations, err := MigrationsFromDir("../../migrations")
	if err != nil {
		t.Fatal(err)
	}
	var currentStateSQL string
	for _, migration := range migrations {
		if strings.Contains(migration.SQL, "CREATE VIEW entity_current_state") {
			currentStateSQL = migration.SQL
		}
	}
	if currentStateSQL == "" {
		t.Fatal("current-state view migration was not found")
	}
	for _, expected := range []string{
		"'subjectDisplayName'",
		"'subjectEntityType'",
		"'objectDisplayName'",
		"'objectEntityType'",
	} {
		if !strings.Contains(currentStateSQL, expected) {
			t.Fatalf("current-state relation JSON is missing %s", expected)
		}
	}
}

func TestFilterIndexMigrationCoversCurrentAPIHotPaths(t *testing.T) {
	migrations, err := MigrationsFromDir("../../migrations")
	if err != nil {
		t.Fatal(err)
	}
	var sql string
	for _, migration := range migrations {
		if migration.Version == "0012_current_api_hot_path_indexes" {
			sql = migration.SQL
			break
		}
	}
	if sql == "" {
		t.Fatal("current API hot-path index migration was not found")
	}
	for _, expected := range []string{
		"entity_facts_key_value_entity_idx",
		"md5(value_json #>> '{}')",
		"entity_facts_owner_cap_value_idx",
		"entity_facts_location_hash_value_idx",
		"entity_relations_current_subject_predicate_object_idx",
		"entity_relations_current_object_predicate_subject_idx",
		"killmails_environment_time_id_idx",
		"events_environment_module_time_idx",
		"sui_objects_environment_type_time_idx",
	} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("hot-path index migration is missing %s", expected)
		}
	}
}

func TestEntityFactFilterUsesHashableValuePredicate(t *testing.T) {
	var args []any
	where := addEntityFactFilter(&args, "WHERE 1=1", "description", strings.Repeat("x", 4096))
	for _, expected := range []string{
		"md5(f.value_json #>> '{}') = md5($2)",
		"f.value_json #>> '{}' = $2",
	} {
		if !strings.Contains(where, expected) {
			t.Fatalf("entity fact filter is missing %s in %s", expected, where)
		}
	}
	if len(args) != 2 || args[0] != "description" || args[1] != strings.Repeat("x", 4096) {
		t.Fatalf("unexpected filter args: %#v", args)
	}
}
