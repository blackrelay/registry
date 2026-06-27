package api

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConsumerExampleResponsesDocumentUsefulContracts(t *testing.T) {
	for _, name := range []string{
		"health.json",
		"current-characters.json",
		"current-systems.json",
		"current-enemies.json",
		"current-materials.json",
		"current-recipes.json",
		"killmail-player.json",
		"killmail-npc.json",
		"killmail-unresolved.json",
		"ops-sui-coverage.json",
		"ops-source-gaps.json",
	} {
		t.Run(name, func(t *testing.T) {
			payload := readExample(t, name)
			assertAPIEnvelope(t, name, payload)
		})
	}

	t.Run("current entity examples expose provenance and derived shape", func(t *testing.T) {
		for _, name := range []string{
			"current-characters.json",
			"current-systems.json",
			"current-enemies.json",
			"current-materials.json",
			"current-recipes.json",
		} {
			payload := readExample(t, name)
			rows := asSlice(t, payload["data"])
			if len(rows) == 0 {
				t.Fatalf("%s has no current entity rows", name)
			}
			row := asMap(t, rows[0])
			entity := asMap(t, row["entity"])
			for _, field := range []string{"id", "slug", "type", "displayName", "environment"} {
				if asString(t, entity[field]) == "" {
					t.Fatalf("%s current entity is missing %s: %#v", name, field, entity)
				}
			}
			if len(asSlice(t, row["sourceIds"])) == 0 {
				t.Fatalf("%s current entity is missing sourceIds: %#v", name, row)
			}
			_ = asSlice(t, row["facts"])
			_ = asSlice(t, row["relations"])
			_ = asSlice(t, row["incomingRelations"])
			_ = asMap(t, row["derived"])
		}
	})

	t.Run("semantic killmail examples expose names before raw ids", func(t *testing.T) {
		player := asMap(t, readExample(t, "killmail-player.json")["data"])
		assertKillmailActor(t, player, "killer", "character", "Killer", false)
		assertKillmailActor(t, player, "victim", "character", "Victim", false)
		assertSummaryAvoidsRawIDs(t, player, "Killer killed Victim in NN0-Y-D5.")

		npc := asMap(t, readExample(t, "killmail-npc.json")["data"])
		assertKillmailActor(t, npc, "killer", "enemy", "Caird [NPC]", true)
		assertKillmailActor(t, npc, "victim", "character", "Fixture Victim", false)
		if asString(t, asMap(t, npc["killer"])["typeId"]) != "92096" {
			t.Fatalf("NPC killmail example does not expose the resolved static type id: %#v", npc["killer"])
		}
		assertSummaryAvoidsRawIDs(t, npc, "Caird [NPC] killed Fixture Victim in NN0-Y-D5.")

		unresolved := asMap(t, readExample(t, "killmail-unresolved.json")["data"])
		killer := asMap(t, unresolved["killer"])
		if asString(t, killer["entityType"]) != "unknown" || asString(t, killer["rawId"]) == "" {
			t.Fatalf("unresolved killmail example should keep raw id only on unresolved actors: %#v", killer)
		}
		assertSummaryAvoidsRawIDs(t, unresolved, "Unknown killed Unknown.")
	})

	t.Run("source gap examples include actionable repair commands", func(t *testing.T) {
		payload := readExample(t, "ops-source-gaps.json")
		gap := asMap(t, asSlice(t, payload["data"])[0])
		if asString(t, gap["kind"]) == "" || asString(t, gap["category"]) == "" || asString(t, gap["severity"]) == "" {
			t.Fatalf("source gap example is missing classification fields: %#v", gap)
		}
		commands := asSlice(t, gap["suggestedCommands"])
		if len(commands) == 0 {
			t.Fatalf("source gap example is missing repair commands: %#v", gap)
		}
		for _, command := range commands {
			if !strings.HasPrefix(asString(t, command), "go run ./cmd/br-indexer ") {
				t.Fatalf("source gap command is not a registry repair command: %#v", command)
			}
		}
	})

	t.Run("sui coverage example records range-blocked targets as provider-limited", func(t *testing.T) {
		payload := readExample(t, "ops-sui-coverage.json")
		data := asMap(t, payload["data"])
		if asString(t, data["coverageBasis"]) != "cursor_table" {
			t.Fatalf("coverage example should identify the cursor-table basis: %#v", data)
		}
		if fullCoverage, ok := data["fullCoverageProven"].(bool); !ok || fullCoverage {
			t.Fatalf("coverage example should not claim full historical proof: %#v", data)
		}
		target := asMap(t, asSlice(t, data["targets"])[0])
		if asString(t, target["status"]) != "range_blocked" || target["providerRangeBlocked"] != true {
			t.Fatalf("coverage example should preserve provider-limited object evidence: %#v", target)
		}
	})

	t.Run("public export examples include auditable file metadata", func(t *testing.T) {
		manifest := readExample(t, "public-export-manifest.json")
		if asString(t, manifest["schemaVersion"]) != "registry.export_manifest.v1" {
			t.Fatalf("unexpected export manifest schema: %#v", manifest["schemaVersion"])
		}
		file := asMap(t, asSlice(t, manifest["files"])[0])
		for _, field := range []string{"path", "contentType", "sha256"} {
			if asString(t, file[field]) == "" {
				t.Fatalf("export manifest file is missing %s: %#v", field, file)
			}
		}
		if asNumber(t, file["sizeBytes"]) <= 0 || asNumber(t, file["rowCount"]) <= 0 {
			t.Fatalf("export manifest file is missing size or row counts: %#v", file)
		}

		published := readExample(t, "public-export-published.json")
		if asString(t, published["schemaVersion"]) != "registry.export_publish.v1" {
			t.Fatalf("unexpected publish schema: %#v", published["schemaVersion"])
		}
		if asString(t, published["bundleId"]) == "" || asString(t, published["manifestKey"]) == "" || asString(t, published["latestPointerKey"]) == "" {
			t.Fatalf("published export example is missing object keys: %#v", published)
		}
	})
}

func readExample(t *testing.T, name string) map[string]any {
	t.Helper()
	path := filepath.Join("..", "..", "testdata", "examples", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("%s did not contain valid JSON: %v", name, err)
	}
	return payload
}

func assertAPIEnvelope(t *testing.T, name string, payload map[string]any) {
	t.Helper()
	if _, ok := payload["data"]; !ok {
		t.Fatalf("%s is missing data: %#v", name, payload)
	}
	meta := asMap(t, payload["meta"])
	if asString(t, meta["registry"]) == "" || asString(t, meta["apiVersion"]) == "" {
		t.Fatalf("%s has incomplete meta: %#v", name, meta)
	}
}

func assertKillmailActor(t *testing.T, killmail map[string]any, role, entityType, displayName string, isNPC bool) {
	t.Helper()
	actor := asMap(t, killmail[role])
	if asString(t, actor["entityType"]) != entityType || asString(t, actor["displayName"]) != displayName {
		t.Fatalf("%s actor did not resolve as expected: %#v", role, actor)
	}
	if rawID := asString(t, actor["rawId"]); rawID != "" {
		t.Fatalf("%s actor leaked a raw id despite being resolved: %#v", role, actor)
	}
	if role == "killer" {
		if got, ok := actor["isNpc"].(bool); ok && got != isNPC {
			t.Fatalf("killer NPC flag mismatch: got %v want %v", got, isNPC)
		}
	}
}

func assertSummaryAvoidsRawIDs(t *testing.T, killmail map[string]any, want string) {
	t.Helper()
	summary := asString(t, killmail["summaryText"])
	if summary != want {
		t.Fatalf("unexpected killmail summary: got %q want %q", summary, want)
	}
	for _, marker := range []string{"0x", "character:", "system:", "enemy:"} {
		if strings.Contains(summary, marker) {
			t.Fatalf("killmail summary leaked raw identifier marker %q: %q", marker, summary)
		}
	}
}

func asMap(t *testing.T, value any) map[string]any {
	t.Helper()
	out, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("expected object, got %#v", value)
	}
	return out
}

func asSlice(t *testing.T, value any) []any {
	t.Helper()
	out, ok := value.([]any)
	if !ok {
		t.Fatalf("expected array, got %#v", value)
	}
	return out
}

func asString(t *testing.T, value any) string {
	t.Helper()
	if value == nil {
		return ""
	}
	out, ok := value.(string)
	if !ok {
		t.Fatalf("expected string, got %#v", value)
	}
	return out
}

func asNumber(t *testing.T, value any) float64 {
	t.Helper()
	out, ok := value.(float64)
	if !ok {
		t.Fatalf("expected number, got %#v", value)
	}
	return out
}
