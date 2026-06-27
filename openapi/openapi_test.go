package openapi_test

import (
	"os"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

func TestOpenAPIYAMLParses(t *testing.T) {
	data, err := os.ReadFile("registry.v1.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("OpenAPI YAML did not parse: %v", err)
	}
	if doc["openapi"] != "3.1.0" {
		t.Fatalf("unexpected OpenAPI version %#v", doc["openapi"])
	}
}

func TestOpenAPIDocumentsCurrentStateAndKillmailFilters(t *testing.T) {
	data, err := os.ReadFile("registry.v1.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("OpenAPI YAML did not parse: %v", err)
	}
	paths := asMap(t, doc["paths"])
	assertPathParameterRefs(t, paths, "/v1/current/characters", "#/components/parameters/Cycles", "#/components/parameters/Tribe", "#/components/parameters/ProfileState", "#/components/parameters/HasActivity", "#/components/parameters/HasTribe", "#/components/parameters/SourceID")
	assertPathParameterRefs(t, paths, "/v1/current/tribes", "#/components/parameters/Cycles", "#/components/parameters/ProfileState", "#/components/parameters/SourceID")
	assertPathParameterRefs(t, paths, "/v1/current/assemblies", "#/components/parameters/Owner", "#/components/parameters/System", "#/components/parameters/OwnerCap", "#/components/parameters/LocationHash", "#/components/parameters/HasOwnerCap", "#/components/parameters/HasLocationHash", "#/components/parameters/HasResolvedOwner", "#/components/parameters/HasResolvedSystem", "#/components/parameters/SourceID")
	assertPathParameterRefs(t, paths, "/v1/current/regions", "#/components/parameters/Q", "#/components/parameters/Environment")
	assertPathParameterRefs(t, paths, "/v1/current/constellations", "#/components/parameters/Q", "#/components/parameters/Environment")
	assertPathParameterRefs(t, paths, "/v1/current/systems", "#/components/parameters/ConnectedTo", "#/components/parameters/HasActivity")
	assertPathParameterRefs(t, paths, "/v1/current/route-edges", "#/components/parameters/System", "#/components/parameters/SourceID")
	assertPathParameterRefs(t, paths, "/v1/search", "#/components/parameters/Cycles", "#/components/parameters/TypeID", "#/components/parameters/GroupID", "#/components/parameters/CategoryID", "#/components/parameters/MarketGroupID", "#/components/parameters/WreckTypeID", "#/components/parameters/SourceArtefactID", "#/components/parameters/StaticEntityType")
	assertPathParameterRefs(t, paths, "/v1/entities", "#/components/parameters/Cycles", "#/components/parameters/TypeID", "#/components/parameters/GroupID", "#/components/parameters/CategoryID", "#/components/parameters/MarketGroupID", "#/components/parameters/WreckTypeID", "#/components/parameters/SourceArtefactID", "#/components/parameters/StaticEntityType")
	assertPathParameterRefs(t, paths, "/v1/types", "#/components/parameters/Cycles", "#/components/parameters/TypeID", "#/components/parameters/GroupID", "#/components/parameters/CategoryID", "#/components/parameters/MarketGroupID", "#/components/parameters/WreckTypeID", "#/components/parameters/SourceArtefactID", "#/components/parameters/StaticEntityType")
	assertPathParameterRefs(t, paths, "/v1/types/{typeID}", "#/components/parameters/Cycles", "#/components/parameters/TypeIDPath", "#/components/parameters/GroupID", "#/components/parameters/CategoryID", "#/components/parameters/MarketGroupID", "#/components/parameters/WreckTypeID", "#/components/parameters/SourceArtefactID", "#/components/parameters/StaticEntityType")
	assertPathParameterRefs(t, paths, "/v1/events", "#/components/parameters/Cycles")
	assertPathParameterRefs(t, paths, "/v1/killmails", "#/components/parameters/Cycles", "#/components/parameters/System", "#/components/parameters/Killer", "#/components/parameters/KillerTypeID", "#/components/parameters/NPC", "#/components/parameters/FromTime", "#/components/parameters/ToTime", "#/components/parameters/ExcludeFixtures")
	assertPathParameterRefs(t, paths, "/v1/ops/source-gaps", "#/components/parameters/Environment")
	for _, path := range []string{
		"/v1/types",
		"/v1/types/{typeID}",
		"/v1/current/enemies",
		"/v1/current/recipes",
		"/v1/current/blueprints",
		"/v1/regions",
		"/v1/regions/{idOrSlug}",
		"/v1/constellations",
		"/v1/constellations/{idOrSlug}",
		"/v1/items",
		"/v1/items/{idOrSlug}",
		"/v1/materials",
		"/v1/materials/{idOrSlug}",
		"/v1/enemies",
		"/v1/enemies/{idOrSlug}",
		"/v1/recipes",
		"/v1/recipes/{idOrSlug}",
		"/v1/blueprints",
		"/v1/blueprints/{idOrSlug}",
		"/v1/ships",
		"/v1/ships/{idOrSlug}",
		"/v1/structures",
		"/v1/structures/{idOrSlug}",
		"/v1/ops/source-gaps",
	} {
		assertPathDocumented(t, paths, path)
	}

	components := asMap(t, doc["components"])
	schemas := asMap(t, components["schemas"])
	currentEntity := asMap(t, schemas["CurrentEntity"])
	properties := asMap(t, currentEntity["properties"])
	if _, ok := properties["derived"]; !ok {
		t.Fatal("CurrentEntity schema does not document derived current-state summary")
	}
	if _, ok := schemas["CurrentDerived"]; !ok {
		t.Fatal("CurrentDerived schema is missing")
	}
	if _, ok := schemas["SourceGap"]; !ok {
		t.Fatal("SourceGap schema is missing")
	}
	sourceGap := asMap(t, schemas["SourceGap"])
	sourceGapProperties := asMap(t, sourceGap["properties"])
	for _, property := range []string{"category", "suggestedCommands"} {
		if _, ok := sourceGapProperties[property]; !ok {
			t.Fatalf("SourceGap schema does not document %s", property)
		}
	}
	if _, ok := schemas["SourceGapListEnvelope"]; !ok {
		t.Fatal("SourceGapListEnvelope schema is missing")
	}
}

func TestOpenAPIOperationSummariesUseTitleCase(t *testing.T) {
	data, err := os.ReadFile("registry.v1.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("OpenAPI YAML did not parse: %v", err)
	}
	paths := asMap(t, doc["paths"])
	for path, pathValue := range paths {
		pathItem := asMap(t, pathValue)
		for method, operationValue := range pathItem {
			operation, ok := operationValue.(map[string]any)
			if !ok {
				continue
			}
			summary, ok := operation["summary"].(string)
			if !ok {
				continue
			}
			if !isTitleCaseSummary(summary) {
				t.Fatalf("%s %s summary is not title case: %q", strings.ToUpper(method), path, summary)
			}
		}
	}
}

func assertPathDocumented(t *testing.T, paths map[string]any, path string) {
	t.Helper()
	pathItem := asMap(t, paths[path])
	if _, ok := pathItem["get"]; !ok {
		t.Fatalf("%s does not document a GET operation", path)
	}
}

func assertPathParameterRefs(t *testing.T, paths map[string]any, path string, refs ...string) {
	t.Helper()
	pathItem := asMap(t, paths[path])
	get := asMap(t, pathItem["get"])
	parameters, ok := get["parameters"].([]any)
	if !ok {
		t.Fatalf("%s parameters are missing", path)
	}
	seen := make(map[string]struct{})
	for _, parameter := range parameters {
		parameterMap := asMap(t, parameter)
		if ref, ok := parameterMap["$ref"].(string); ok {
			seen[ref] = struct{}{}
		}
	}
	for _, ref := range refs {
		if _, ok := seen[ref]; !ok {
			t.Fatalf("%s does not document parameter %s", path, ref)
		}
	}
}

func asMap(t *testing.T, value any) map[string]any {
	t.Helper()
	out, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("expected map, got %#v", value)
	}
	return out
}

func isTitleCaseSummary(summary string) bool {
	words := strings.Fields(summary)
	for index, word := range words {
		for _, part := range strings.Split(word, "-") {
			trimmed := strings.Trim(part, "(),:/")
			if trimmed == "" {
				continue
			}
			if index > 0 && isMinorTitleWord(trimmed) {
				continue
			}
			first, _ := utf8.DecodeRuneInString(trimmed)
			if !unicode.IsUpper(first) && !unicode.IsDigit(first) {
				return false
			}
		}
	}
	return true
}

func isMinorTitleWord(word string) bool {
	switch strings.ToLower(word) {
	case "a", "an", "and", "as", "at", "by", "for", "from", "in", "of", "on", "or", "the", "to", "with":
		return true
	default:
		return false
	}
}
