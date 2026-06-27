package exporter

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

func TestExportJSONLSchemasMatchVerifierRequiredFields(t *testing.T) {
	for _, tc := range []struct {
		exportName string
		schemaPath string
	}{
		{exportName: "entities.jsonl", schemaPath: "entities.export.v1.schema.json"},
		{exportName: "killmails.jsonl", schemaPath: "killmails.export.v1.schema.json"},
		{exportName: "sources.jsonl", schemaPath: "sources.export.v1.schema.json"},
		{exportName: "events.jsonl", schemaPath: "events.export.v1.schema.json"},
		{exportName: "sui_objects.jsonl", schemaPath: "sui-objects.export.v1.schema.json"},
	} {
		t.Run(tc.exportName, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("..", "..", "contracts", tc.schemaPath))
			if err != nil {
				t.Fatal(err)
			}
			var schema struct {
				Schema   string   `json:"$schema"`
				ID       string   `json:"$id"`
				Type     string   `json:"type"`
				Required []string `json:"required"`
			}
			if err := json.Unmarshal(data, &schema); err != nil {
				t.Fatal(err)
			}
			if schema.Schema != "https://json-schema.org/draft/2020-12/schema" {
				t.Fatalf("schema does not use draft 2020-12: %#v", schema)
			}
			if schema.ID != tc.schemaPath {
				t.Fatalf("schema id should match file name: %#v", schema)
			}
			if schema.Type != "object" {
				t.Fatalf("export row schema should describe JSONL objects: %#v", schema)
			}
			got := append([]string(nil), schema.Required...)
			want := append([]string(nil), requiredExportFields(tc.exportName)...)
			sort.Strings(got)
			sort.Strings(want)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("schema required fields do not match verifier for %s: got %#v want %#v", tc.exportName, got, want)
			}
		})
	}
}
