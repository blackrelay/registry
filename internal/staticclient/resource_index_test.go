package staticclient

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blackrelay/registry/internal/model"
)

func TestParseResourceIndexAndResolveStaticDataResource(t *testing.T) {
	root := t.TempDir()
	resPath := filepath.Join(filepath.Dir(root), "ResFiles", "3c", "types-file")
	if err := os.MkdirAll(filepath.Dir(resPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(resPath, []byte("raw types evidence"), 0o644); err != nil {
		t.Fatal(err)
	}

	entries, err := ParseResourceIndex(strings.NewReader("res:/staticdata/types.fsdbinary,3c/types-file,abc123,5083656,674010\n"))
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := entries.Find("res:/staticdata/types.fsdbinary")
	if !ok {
		t.Fatalf("types.fsdbinary was not parsed: %#v", entries)
	}
	if entry.HashPath != filepath.ToSlash("3c/types-file") || entry.ContentHash != "abc123" || entry.IndexSize != 5083656 || entry.PackedSize != 674010 {
		t.Fatalf("resource fields were not parsed correctly: %#v", entry)
	}

	resolved, err := ResolveResourcePath(root, entry)
	if err != nil {
		t.Fatal(err)
	}
	if resolved != resPath {
		t.Fatalf("expected %s, got %s", resPath, resolved)
	}
}

func TestExtractStaticClientTypesWritesDeterministicEvidenceManifest(t *testing.T) {
	root := t.TempDir()
	resFile := filepath.Join(filepath.Dir(root), "ResFiles", "3c", "types-file")
	if err := os.MkdirAll(filepath.Dir(resFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(resFile, []byte("raw fsdbinary evidence"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "resfileindex.txt"), []byte("res:/staticdata/types.fsdbinary,3c/types-file,hash,21,7\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	resolvedJSON := filepath.Join(root, "resolved-types.json")
	if err := os.WriteFile(resolvedJSON, []byte(`{
		"candidates": [
			{"groupId":5130,"name":"Host�s Mycena","typeId":95504,"typeNameId":1047620,"description":"Host�s copy","wreckTypeId":81610},
			{"groupId":5033,"name":"Caird","typeId":92096,"typeNameId":1035497,"wreckTypeId":81610}
		]
	}`), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(root, "static-client-types.normalized.json")

	result, err := ExtractStaticClientTypes(context.Background(), StaticTypeExtractionOptions{
		ClientRoot:       root,
		ResolvedJSONPath: resolvedJSON,
		OutputPath:       out,
		Environment:      model.EnvironmentStillness,
		ClientBuild:      "test-build",
		PatchLabel:       "test-patch",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.RowCount != 2 || len(result.Resources) != 1 {
		t.Fatalf("unexpected extraction result: %#v", result)
	}
	if result.Resources[0].ResourcePath != "res:/staticdata/types.fsdbinary" || result.Resources[0].SHA256 == "" {
		t.Fatalf("types resource evidence was not captured: %#v", result.Resources)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		SchemaVersion string          `json:"schemaVersion"`
		Environment   string          `json:"environment"`
		Candidates    []staticTypeRow `json:"candidates"`
		Resources     []StaticResourceEvidence
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.SchemaVersion != "registry.static-client-types.v1" || payload.Environment != "stillness" {
		t.Fatalf("manifest metadata was not written: %#v", payload)
	}
	if len(payload.Candidates) != 2 || payload.Candidates[0].TypeID != 92096 || payload.Candidates[1].Name != "Host's Mycena" || payload.Candidates[1].Description != "Host's copy" {
		t.Fatalf("rows were not sorted and normalised: %#v", payload.Candidates)
	}
}

func TestExtractStaticClientTypesUsesNativeProbeRowsWhenResolvedJSONMissing(t *testing.T) {
	root := t.TempDir()
	typeFile := filepath.Join(filepath.Dir(root), "ResFiles", "3c", "types-file")
	locFile := filepath.Join(filepath.Dir(root), "ResFiles", "2c", "localisation-file")
	if err := os.MkdirAll(filepath.Dir(typeFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(locFile), 0o755); err != nil {
		t.Fatal(err)
	}
	typeBytes := make([]byte, 128)
	writeNativeTypeProbeRow(typeBytes, 72, 5033, 92096, 1035497, 81610)
	if err := os.WriteFile(typeFile, typeBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	localisation := []byte{
		0x4a, 0xe9, 0xcc, 0x0f, 0x00,
		0x8c, 0x05, 'C', 'a', 'i', 'r', 'd',
	}
	if err := os.WriteFile(locFile, localisation, 0o644); err != nil {
		t.Fatal(err)
	}
	index := strings.Join([]string{
		"res:/staticdata/types.fsdbinary,3c/types-file,hash,128,64",
		"res:/localizationfsd/localization_fsd_en-us.pickle,2c/localisation-file,hash,7,7",
	}, "\n")
	if err := os.WriteFile(filepath.Join(root, "resfileindex.txt"), []byte(index), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(root, "static-client-types.native.json")

	result, err := ExtractStaticClientTypes(context.Background(), StaticTypeExtractionOptions{
		ClientRoot:   root,
		OutputPath:   out,
		Environment:  model.EnvironmentStillness,
		ProbeTypeIDs: []int{92096},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.RowCount != 1 {
		t.Fatalf("expected one native probe row, got %#v", result)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Candidates []staticTypeRow `json:"candidates"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Candidates) != 1 || payload.Candidates[0].Name != "Caird" || payload.Candidates[0].TypeID != 92096 || payload.Candidates[0].GroupID != 5033 {
		t.Fatalf("native probe row was not serialised as a type row: %#v", payload.Candidates)
	}
}

func TestExtractStaticClientTypesCanUseNativeFullScanWhenRequested(t *testing.T) {
	root := t.TempDir()
	typeFile := filepath.Join(filepath.Dir(root), "ResFiles", "3c", "types-file")
	locFile := filepath.Join(filepath.Dir(root), "ResFiles", "2c", "localisation-file")
	if err := os.MkdirAll(filepath.Dir(typeFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(locFile), 0o755); err != nil {
		t.Fatal(err)
	}
	typeBytes := make([]byte, 160)
	writeNativeTypeProbeRow(typeBytes, 72, 5033, 92096, 1035497, 81610)
	writeNativeTypeProbeRow(typeBytes, 128, 5130, 94167, 1044223, 81610)
	if err := os.WriteFile(typeFile, typeBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	localisation := []byte{
		0x4a, 0xe9, 0xcc, 0x0f, 0x00,
		0x8c, 0x05, 'C', 'a', 'i', 'r', 'd',
		0x4a, 0xff, 0xee, 0x0f, 0x00,
		0x8c, 0x06, 'M', 'y', 'c', 'e', 'n', 'a',
	}
	if err := os.WriteFile(locFile, localisation, 0o644); err != nil {
		t.Fatal(err)
	}
	index := strings.Join([]string{
		"res:/staticdata/types.fsdbinary,3c/types-file,hash,160,80",
		"res:/localizationfsd/localization_fsd_en-us.pickle,2c/localisation-file,hash,25,25",
	}, "\n")
	if err := os.WriteFile(filepath.Join(root, "resfileindex.txt"), []byte(index), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(root, "static-client-types.native-scan.json")

	result, err := ExtractStaticClientTypes(context.Background(), StaticTypeExtractionOptions{
		ClientRoot:     root,
		OutputPath:     out,
		Environment:    model.EnvironmentStillness,
		NativeFullScan: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.RowCount != 2 {
		t.Fatalf("expected two native scan rows, got %#v", result)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Candidates []staticTypeRow `json:"candidates"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Candidates) != 2 || payload.Candidates[0].Name != "Caird" || payload.Candidates[1].Name != "Mycena" {
		t.Fatalf("native scan rows were not serialised deterministically: %#v", payload.Candidates)
	}
}

func TestDiscoverStaticClientResourcesClassifiesRecipeBlueprintAndTypeEvidence(t *testing.T) {
	root := t.TempDir()
	for _, file := range []string{"types-file", "blueprints-file", "recipes-file", "ui-blueprint-file", "noise-file"} {
		path := filepath.Join(filepath.Dir(root), "ResFiles", "aa", file)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(file), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	index := strings.Join([]string{
		"res:/staticdata/types.fsdbinary,aa/types-file,hash,10,5",
		"res:/staticdata/blueprints.fsdbinary,aa/blueprints-file,hash,20,6",
		"res:/staticdata/industry/recipes.fsdbinary,aa/recipes-file,hash,30,7",
		"res:/ui/texture/icons/blueprint.png,aa/ui-blueprint-file,hash,35,7",
		"res:/textures/unrelated.dds,aa/noise-file,hash,40,8",
	}, "\n")
	if err := os.WriteFile(filepath.Join(root, "resfileindex.txt"), []byte(index), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := DiscoverStaticClientResources(root)
	if err != nil {
		t.Fatal(err)
	}
	seen := make(map[string]StaticResourceDiscovery)
	for _, item := range result {
		seen[item.ResourcePath] = item
	}
	for resource, kind := range map[string]string{
		"res:/staticdata/types.fsdbinary":            "type_metadata",
		"res:/staticdata/blueprints.fsdbinary":       "blueprint_metadata",
		"res:/staticdata/industry/recipes.fsdbinary": "recipe_metadata",
	} {
		item, ok := seen[resource]
		if !ok {
			t.Fatalf("expected %s to be discovered in %#v", resource, result)
		}
		if item.Kind != kind || item.Evidence.SHA256 == "" {
			t.Fatalf("wrong discovery classification for %s: %#v", resource, item)
		}
	}
	if _, ok := seen["res:/textures/unrelated.dds"]; ok {
		t.Fatalf("unrelated resources should not be reported: %#v", result)
	}
	if _, ok := seen["res:/ui/texture/icons/blueprint.png"]; ok {
		t.Fatalf("UI resources should not be reported as static data: %#v", result)
	}
}

func TestExtractStaticClientProductionResourcesWritesDeterministicManifest(t *testing.T) {
	root := t.TempDir()
	for _, file := range []string{"types-file", "blueprints-file", "materials-file", "facilities-file", "noise-file"} {
		path := filepath.Join(filepath.Dir(root), "ResFiles", "aa", file)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(file), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	index := strings.Join([]string{
		"res:/staticdata/types.fsdbinary,aa/types-file,hash,10,5",
		"res:/staticdata/industry_blueprints.fsdbinary,aa/blueprints-file,hash,20,6",
		"res:/staticdata/typematerials.fsdbinary,aa/materials-file,hash,30,7",
		"res:/staticdata/industry_facilities.fsdbinary,aa/facilities-file,hash,40,8",
		"res:/textures/unrelated.dds,aa/noise-file,hash,50,9",
	}, "\n")
	if err := os.WriteFile(filepath.Join(root, "resfileindex.txt"), []byte(index), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(root, "static-client-production-resources.json")

	result, err := ExtractStaticClientProductionResources(context.Background(), StaticProductionExtractionOptions{
		ClientRoot:  root,
		OutputPath:  out,
		Environment: model.EnvironmentStillness,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ResourceCount != 3 {
		t.Fatalf("expected three production resources, got %#v", result)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		SchemaVersion string                    `json:"schemaVersion"`
		Resources     []StaticResourceDiscovery `json:"resources"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.SchemaVersion != "registry.static-client-production-resources.v1" {
		t.Fatalf("unexpected schema version %#v", payload)
	}
	kinds := map[string]bool{}
	for _, item := range payload.Resources {
		kinds[item.Kind] = true
	}
	for _, kind := range []string{"blueprint_metadata", "material_requirement_metadata", "recipe_metadata"} {
		if !kinds[kind] {
			t.Fatalf("production manifest missing %s in %#v", kind, payload.Resources)
		}
	}
}

func TestCompareStaticProductionResourceFilesReportsResourceDeltas(t *testing.T) {
	dir := t.TempDir()
	beforePath := filepath.Join(dir, "production-before.json")
	afterPath := filepath.Join(dir, "production-after.json")
	if err := os.WriteFile(beforePath, []byte(`{
  "schemaVersion": "registry.static-client-production-resources.v1",
  "resources": [
    {
      "resourcePath": "res:/staticdata/industry_blueprints.fsdbinary",
      "kind": "blueprint_metadata",
      "evidence": {"resourcePath":"res:/staticdata/industry_blueprints.fsdbinary","path":"before-blueprints","sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","sizeBytes":10}
    },
    {
      "resourcePath": "res:/staticdata/typematerials.fsdbinary",
      "kind": "material_requirement_metadata",
      "evidence": {"resourcePath":"res:/staticdata/typematerials.fsdbinary","path":"before-materials","sha256":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","sizeBytes":20}
    }
  ]
}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(afterPath, []byte(`{
  "schemaVersion": "registry.static-client-production-resources.v1",
  "resources": [
    {
      "resourcePath": "res:/staticdata/industry_blueprints.fsdbinary",
      "kind": "blueprint_metadata",
      "evidence": {"resourcePath":"res:/staticdata/industry_blueprints.fsdbinary","path":"after-blueprints","sha256":"cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc","sizeBytes":12}
    },
    {
      "resourcePath": "res:/staticdata/industry/recipes.fsdbinary",
      "kind": "recipe_metadata",
      "evidence": {"resourcePath":"res:/staticdata/industry/recipes.fsdbinary","path":"after-recipes","sha256":"dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd","sizeBytes":30}
    }
  ]
}`), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := CompareStaticProductionResourceFiles(beforePath, afterPath)
	if err != nil {
		t.Fatal(err)
	}
	if result.BeforeCount != 2 || result.AfterCount != 2 || result.SemanticallyEqual {
		t.Fatalf("unexpected compare result counts: %#v", result)
	}
	if len(result.Changed) != 1 || result.Changed[0].ResourcePath != "res:/staticdata/industry_blueprints.fsdbinary" {
		t.Fatalf("changed blueprint resource was not reported: %#v", result.Changed)
	}
	if len(result.Removed) != 1 || result.Removed[0].ResourcePath != "res:/staticdata/typematerials.fsdbinary" {
		t.Fatalf("removed material resource was not reported: %#v", result.Removed)
	}
	if len(result.Added) != 1 || result.Added[0].ResourcePath != "res:/staticdata/industry/recipes.fsdbinary" {
		t.Fatalf("new recipe resource was not reported: %#v", result.Added)
	}
}

func TestCompareStaticProductionResourceFilesIgnoresManifestOrder(t *testing.T) {
	dir := t.TempDir()
	beforePath := filepath.Join(dir, "production-before.json")
	afterPath := filepath.Join(dir, "production-after.json")
	before := `{
  "resources": [
    {"resourcePath":"res:/staticdata/typematerials.fsdbinary","kind":"material_requirement_metadata","evidence":{"resourcePath":"res:/staticdata/typematerials.fsdbinary","sha256":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","sizeBytes":20}},
    {"resourcePath":"res:/staticdata/industry_blueprints.fsdbinary","kind":"blueprint_metadata","evidence":{"resourcePath":"res:/staticdata/industry_blueprints.fsdbinary","sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","sizeBytes":10}}
  ]
}`
	after := `{
  "resources": [
    {"resourcePath":"res:/staticdata/industry_blueprints.fsdbinary","kind":"blueprint_metadata","evidence":{"resourcePath":"res:/staticdata/industry_blueprints.fsdbinary","sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","sizeBytes":10}},
    {"resourcePath":"res:/staticdata/typematerials.fsdbinary","kind":"material_requirement_metadata","evidence":{"resourcePath":"res:/staticdata/typematerials.fsdbinary","sha256":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","sizeBytes":20}}
  ]
}`
	if err := os.WriteFile(beforePath, []byte(before), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(afterPath, []byte(after), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := CompareStaticProductionResourceFiles(beforePath, afterPath)
	if err != nil {
		t.Fatal(err)
	}
	if !result.SemanticallyEqual || len(result.Added)+len(result.Removed)+len(result.Changed) != 0 {
		t.Fatalf("same resources in different order should compare equal: %#v", result)
	}
}

func TestSummariseStaticProductionResourceFileCountsKinds(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "production.json")
	if err := os.WriteFile(path, []byte(`{
  "resources": [
    {"resourcePath":"res:/staticdata/industry_blueprints.fsdbinary","kind":"blueprint_metadata","evidence":{"resourcePath":"res:/staticdata/industry_blueprints.fsdbinary","sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","sizeBytes":10}},
    {"resourcePath":"res:/staticdata/industry/recipes.fsdbinary","kind":"recipe_metadata","evidence":{"resourcePath":"res:/staticdata/industry/recipes.fsdbinary","sha256":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","sizeBytes":20}},
    {"resourcePath":"res:/staticdata/typematerials.fsdbinary","kind":"material_requirement_metadata","evidence":{"resourcePath":"res:/staticdata/typematerials.fsdbinary","sha256":"cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc","sizeBytes":30}},
    {"resourcePath":"res:/staticdata/industry_materials_extra.fsdbinary","kind":"material_requirement_metadata","evidence":{"resourcePath":"res:/staticdata/industry_materials_extra.fsdbinary","sha256":"dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd","sizeBytes":40}}
  ]
}`), 0o644); err != nil {
		t.Fatal(err)
	}

	summary, err := SummariseStaticProductionResourceFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if summary.SchemaVersion != "registry.static-client-production-resource-summary.v1" || summary.Path != path {
		t.Fatalf("unexpected summary metadata: %#v", summary)
	}
	if summary.ResourceCount != 4 || summary.TotalSizeBytes != 100 {
		t.Fatalf("unexpected summary counts: %#v", summary)
	}
	if summary.Kinds["blueprint_metadata"] != 1 || summary.Kinds["recipe_metadata"] != 1 || summary.Kinds["material_requirement_metadata"] != 2 {
		t.Fatalf("resource kinds were not counted: %#v", summary.Kinds)
	}
}

func writeNativeTypeProbeRow(data []byte, offset int, groupID, typeID, typeNameID, wreckTypeID uint32) {
	binary.LittleEndian.PutUint32(data[offset-32:offset-28], groupID)
	binary.LittleEndian.PutUint32(data[offset:offset+4], typeID)
	binary.LittleEndian.PutUint32(data[offset+4:offset+8], typeNameID)
	binary.LittleEndian.PutUint32(data[offset+12:offset+16], wreckTypeID)
}
