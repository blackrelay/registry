package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunStaticClientInspectTypesPrintsProbeSummary(t *testing.T) {
	root := t.TempDir()
	typePath := filepath.Join(filepath.Dir(root), "ResFiles", "3c", "types-file")
	if err := os.MkdirAll(filepath.Dir(typePath), 0o755); err != nil {
		t.Fatal(err)
	}
	typeBytes := make([]byte, 96)
	binary.LittleEndian.PutUint32(typeBytes[0:4], 5033)
	binary.LittleEndian.PutUint32(typeBytes[32:36], 92096)
	binary.LittleEndian.PutUint32(typeBytes[36:40], 1035497)
	binary.LittleEndian.PutUint32(typeBytes[44:48], 81610)
	if err := os.WriteFile(typePath, typeBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "resfileindex.txt"), []byte("res:/staticdata/types.fsdbinary,3c/types-file,hash,96,48\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := runStaticClientInspectTypes([]string{"-client-path", root, "-probe-type-ids", "92096"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected zero exit code, got %d stderr=%s", code, stderr.String())
	}
	output := stdout.String()
	for _, expected := range []string{
		"parser status: native_probe_row_decoder_available",
		"types.fsdbinary sha256:",
		"probe type 92096 offsets: 32",
		"decoded type row: type=92096 group=5033 typeName=1035497 name=\"(unresolved)\" wreck=81610 offset=32",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("output missing %q:\n%s", expected, output)
		}
	}
}

func TestRunStaticClientExtractTypesAcceptsNativeScanFlag(t *testing.T) {
	root := t.TempDir()
	typePath := filepath.Join(filepath.Dir(root), "ResFiles", "3c", "types-file")
	locPath := filepath.Join(filepath.Dir(root), "ResFiles", "2c", "localisation-file")
	if err := os.MkdirAll(filepath.Dir(typePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(locPath), 0o755); err != nil {
		t.Fatal(err)
	}
	typeBytes := make([]byte, 96)
	binary.LittleEndian.PutUint32(typeBytes[0:4], 5033)
	binary.LittleEndian.PutUint32(typeBytes[32:36], 92096)
	binary.LittleEndian.PutUint32(typeBytes[36:40], 1035497)
	binary.LittleEndian.PutUint32(typeBytes[44:48], 81610)
	if err := os.WriteFile(typePath, typeBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	localisation := []byte{
		0x4a, 0xe9, 0xcc, 0x0f, 0x00,
		0x8c, 0x05, 'C', 'a', 'i', 'r', 'd',
	}
	if err := os.WriteFile(locPath, localisation, 0o644); err != nil {
		t.Fatal(err)
	}
	index := strings.Join([]string{
		"res:/staticdata/types.fsdbinary,3c/types-file,hash,96,48",
		"res:/localizationfsd/localization_fsd_en-us.pickle,2c/localisation-file,hash,12,12",
	}, "\n")
	if err := os.WriteFile(filepath.Join(root, "resfileindex.txt"), []byte(index), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(root, "static-client-types.native-scan.json")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := runStaticClientExtractTypes([]string{"-client-path", root, "-native-scan", "-out", out}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected zero exit code, got %d stderr=%s", code, stderr.String())
	}
	var result struct {
		RowCount int `json:"rowCount"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode stdout: %v\n%s", err, stdout.String())
	}
	if result.RowCount != 1 {
		t.Fatalf("expected one native scan row, got %#v", result)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"name": "Caird"`) {
		t.Fatalf("native scan output did not contain Caird row:\n%s", string(data))
	}
}

func TestRunStaticClientDecodeTypesWritesNativeDecoderArtefact(t *testing.T) {
	root := t.TempDir()
	typePath := filepath.Join(filepath.Dir(root), "ResFiles", "3c", "types-file")
	locPath := filepath.Join(filepath.Dir(root), "ResFiles", "2c", "localisation-file")
	if err := os.MkdirAll(filepath.Dir(typePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(locPath), 0o755); err != nil {
		t.Fatal(err)
	}
	typeBytes := make([]byte, 96)
	binary.LittleEndian.PutUint32(typeBytes[0:4], 5033)
	binary.LittleEndian.PutUint32(typeBytes[32:36], 92096)
	binary.LittleEndian.PutUint32(typeBytes[36:40], 1035497)
	binary.LittleEndian.PutUint32(typeBytes[44:48], 81610)
	if err := os.WriteFile(typePath, typeBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	localisation := []byte{
		0x4a, 0xe9, 0xcc, 0x0f, 0x00,
		0x8c, 0x05, 'C', 'a', 'i', 'r', 'd',
	}
	if err := os.WriteFile(locPath, localisation, 0o644); err != nil {
		t.Fatal(err)
	}
	index := strings.Join([]string{
		"res:/staticdata/types.fsdbinary,3c/types-file,hash,96,48",
		"res:/localizationfsd/localization_fsd_en-us.pickle,2c/localisation-file,hash,12,12",
	}, "\n")
	if err := os.WriteFile(filepath.Join(root, "resfileindex.txt"), []byte(index), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(root, "decoded-types.json")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := runStaticClientDecodeTypes([]string{"-client-path", root, "-out", out}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected zero exit code, got %d stderr=%s", code, stderr.String())
	}
	var result struct {
		SchemaVersion string `json:"schemaVersion"`
		DecoderStatus string `json:"decoderStatus"`
		OutputPath    string `json:"outputPath"`
		RowCount      int    `json:"rowCount"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode stdout: %v\n%s", err, stdout.String())
	}
	if result.SchemaVersion != "registry.static-client-type-decode.v1" || result.RowCount != 1 || result.OutputPath != out {
		t.Fatalf("unexpected decode result: %#v", result)
	}
	if result.DecoderStatus != "native_localisation_backed_type_rows" {
		t.Fatalf("unexpected decoder status: %#v", result)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"name": "Caird"`) || !strings.Contains(string(data), `"offsetBytes": 32`) {
		t.Fatalf("decoded type artefact did not contain the native row:\n%s", string(data))
	}
}

func TestRunStaticClientCompareTypesPrintsStableDiffSummary(t *testing.T) {
	dir := t.TempDir()
	resolvedPath := filepath.Join(dir, "resolved.json")
	nativePath := filepath.Join(dir, "native.json")
	if err := os.WriteFile(resolvedPath, []byte(`[
  {"typeId":92096,"groupId":5033,"name":"Caird","wreckTypeId":81610}
]`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(nativePath, []byte(`[
  {"typeId":92096,"groupId":5130,"name":"Caird Prime","wreckTypeId":81611},
  {"typeId":94167,"groupId":5130,"name":"Mycena","wreckTypeId":81610}
]`), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := runStaticClientCompareTypes([]string{"-resolved", resolvedPath, "-native", nativePath}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected zero exit code, got %d stderr=%s", code, stderr.String())
	}
	var result struct {
		SchemaVersion     string `json:"schemaVersion"`
		SemanticallyEqual bool   `json:"semanticallyEqual"`
		NativeOnly        []struct {
			TypeID int `json:"typeId"`
		} `json:"nativeOnly"`
		ChangedGroup []struct {
			TypeID int `json:"typeId"`
		} `json:"changedGroup"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode stdout: %v\n%s", err, stdout.String())
	}
	if result.SchemaVersion != "registry.static-client-type-compare.v1" {
		t.Fatalf("unexpected schema version %#v", result)
	}
	if result.SemanticallyEqual {
		t.Fatalf("meaningful deltas should not compare equal: %#v", result)
	}
	if len(result.NativeOnly) != 1 || result.NativeOnly[0].TypeID != 94167 {
		t.Fatalf("native-only Mycena row was not reported: %#v", result.NativeOnly)
	}
	if len(result.ChangedGroup) != 1 || result.ChangedGroup[0].TypeID != 92096 {
		t.Fatalf("changed group was not reported by type id: %#v", result.ChangedGroup)
	}
}

func TestRunStaticClientCompareProductionPrintsResourceDiffSummary(t *testing.T) {
	dir := t.TempDir()
	beforePath := filepath.Join(dir, "before.json")
	afterPath := filepath.Join(dir, "after.json")
	if err := os.WriteFile(beforePath, []byte(`{
  "resources": [
    {"resourcePath":"res:/staticdata/industry_blueprints.fsdbinary","kind":"blueprint_metadata","evidence":{"resourcePath":"res:/staticdata/industry_blueprints.fsdbinary","sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","sizeBytes":10}}
  ]
}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(afterPath, []byte(`{
  "resources": [
    {"resourcePath":"res:/staticdata/industry_blueprints.fsdbinary","kind":"blueprint_metadata","evidence":{"resourcePath":"res:/staticdata/industry_blueprints.fsdbinary","sha256":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","sizeBytes":12}}
  ]
}`), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := runStaticClientCompareProduction([]string{"-before", beforePath, "-after", afterPath}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected zero exit code, got %d stderr=%s", code, stderr.String())
	}
	var result struct {
		SchemaVersion string `json:"schemaVersion"`
		Changed       []struct {
			ResourcePath string `json:"resourcePath"`
		} `json:"changed"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode stdout: %v\n%s", err, stdout.String())
	}
	if result.SchemaVersion != "registry.static-client-production-resource-compare.v1" {
		t.Fatalf("unexpected schema version %#v", result)
	}
	if len(result.Changed) != 1 || result.Changed[0].ResourcePath != "res:/staticdata/industry_blueprints.fsdbinary" {
		t.Fatalf("changed production resource was not reported: %#v", result.Changed)
	}
}

func TestRunStaticClientSummariseProductionPrintsKindCounts(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "production.json")
	if err := os.WriteFile(path, []byte(`{
  "resources": [
    {"resourcePath":"res:/staticdata/industry_blueprints.fsdbinary","kind":"blueprint_metadata","evidence":{"resourcePath":"res:/staticdata/industry_blueprints.fsdbinary","sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","sizeBytes":10}},
    {"resourcePath":"res:/staticdata/typematerials.fsdbinary","kind":"material_requirement_metadata","evidence":{"resourcePath":"res:/staticdata/typematerials.fsdbinary","sha256":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","sizeBytes":20}}
  ]
}`), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := runStaticClientSummariseProduction([]string{"-path", path}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected zero exit code, got %d stderr=%s", code, stderr.String())
	}
	var result struct {
		SchemaVersion string         `json:"schemaVersion"`
		ResourceCount int            `json:"resourceCount"`
		Kinds         map[string]int `json:"kinds"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode stdout: %v\n%s", err, stdout.String())
	}
	if result.SchemaVersion != "registry.static-client-production-resource-summary.v1" || result.ResourceCount != 2 {
		t.Fatalf("unexpected production summary: %#v", result)
	}
	if result.Kinds["blueprint_metadata"] != 1 || result.Kinds["material_requirement_metadata"] != 1 {
		t.Fatalf("production kinds were not reported: %#v", result.Kinds)
	}
}

func TestRunStaticClientDecodeProductionWritesCandidateArtefact(t *testing.T) {
	root := t.TempDir()
	for _, file := range []string{"types-file", "localisation-file", "blueprints-file", "materials-file"} {
		path := filepath.Join(filepath.Dir(root), "ResFiles", "aa", file)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	typeBytes := make([]byte, 512)
	writeNativeTypeRow(typeBytes, 104, 18, 77801, 1001)
	writeNativeTypeRow(typeBytes, 184, 18, 77802, 1002)
	writeNativeTypeRow(typeBytes, 264, 4738, 78516, 1003)
	writeNativeTypeRow(typeBytes, 344, 4747, 82654, 1004)
	writeNativeTypeRow(typeBytes, 424, 4832, 87434, 1005)
	if err := os.WriteFile(filepath.Join(filepath.Dir(root), "ResFiles", "aa", "types-file"), typeBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	localisation := []byte{
		0x4a, 0xe9, 0x03, 0x00, 0x00,
		0x8c, 0x12, 'N', 'i', 'c', 'k', 'e', 'l', '-', 'I', 'r', 'o', 'n', ' ', 'V', 'e', 'i', 'n', 's',
		0x4a, 0xea, 0x03, 0x00, 0x00,
		0x8c, 0x0c, 'L', 'i', 'g', 'h', 't', ' ', 'M', 'e', 't', 'a', 'l', 's',
		0x4a, 0xeb, 0x03, 0x00, 0x00,
		0x8c, 0x0a, 'E', 'U', '-', '4', '0', ' ', 'F', 'u', 'e', 'l',
		0x4a, 0xec, 0x03, 0x00, 0x00,
		0x8c, 0x1e, 'R', 'e', 'i', 'n', 'f', 'o', 'r', 'c', 'e', 'd', ' ', 'S', 'h', 'i', 'e', 'l', 'd', ' ', 'G', 'e', 'n', 'e', 'r', 'a', 't', 'o', 'r', ' ', 'I', 'I',
		0x4a, 0xed, 0x03, 0x00, 0x00,
		0x8c, 0x10, 'D', 'e', 's', 'i', 'c', 'c', 'a', 't', 'e', 'd', ' ', 'F', 'l', 'e', 's', 'h',
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(root), "ResFiles", "aa", "localisation-file"), localisation, 0o644); err != nil {
		t.Fatal(err)
	}
	blueprintBytes := make([]byte, 192)
	writeFSDDictHeader(blueprintBytes, 1, []uint64{48})
	binary.LittleEndian.PutUint64(blueprintBytes[72:80], 1)
	binary.LittleEndian.PutUint64(blueprintBytes[80:88], 1056)
	binary.LittleEndian.PutUint64(blueprintBytes[88:96], 80)
	writeBlueprintCandidate(blueprintBytes, 104, 82654, 207, []uint32{78516, 60, 82654, 1})
	if err := os.WriteFile(filepath.Join(filepath.Dir(root), "ResFiles", "aa", "blueprints-file"), blueprintBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	materialBytes := make([]byte, 160)
	writeFSDDictHeader(materialBytes, 1, []uint64{48})
	binary.LittleEndian.PutUint64(materialBytes[72:80], 1)
	binary.LittleEndian.PutUint64(materialBytes[80:88], 87434)
	binary.LittleEndian.PutUint64(materialBytes[88:96], 72)
	binary.LittleEndian.PutUint64(materialBytes[96:104], 2)
	binary.LittleEndian.PutUint32(materialBytes[104:108], 77801)
	binary.LittleEndian.PutUint32(materialBytes[108:112], 82)
	binary.LittleEndian.PutUint32(materialBytes[112:116], 77802)
	binary.LittleEndian.PutUint32(materialBytes[116:120], 7)
	if err := os.WriteFile(filepath.Join(filepath.Dir(root), "ResFiles", "aa", "materials-file"), materialBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	index := strings.Join([]string{
		"res:/staticdata/types.fsdbinary,aa/types-file,hash,512,256",
		"res:/localizationfsd/localization_fsd_en-us.pickle,aa/localisation-file,hash,64,64",
		"res:/staticdata/industry_blueprints.fsdbinary,aa/blueprints-file,hash,192,96",
		"res:/staticdata/typematerials.fsdbinary,aa/materials-file,hash,160,80",
	}, "\n")
	if err := os.WriteFile(filepath.Join(root, "resfileindex.txt"), []byte(index), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(root, "production-decode.json")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := runStaticClientDecodeProduction([]string{"-client-path", root, "-out", out}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected zero exit code, got %d stderr=%s", code, stderr.String())
	}
	var result struct {
		SchemaVersion        string `json:"schemaVersion"`
		DecoderStatus        string `json:"decoderStatus"`
		RowCount             int    `json:"rowCount"`
		Blueprints           []any  `json:"blueprints"`
		Recipes              []any  `json:"recipes"`
		MaterialRequirements []any  `json:"materialRequirements"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode stdout: %v\n%s", err, stdout.String())
	}
	if result.SchemaVersion != "registry.static-client-production-decode.v1" || result.DecoderStatus != "native_candidate_production_rows" {
		t.Fatalf("unexpected production decode metadata: %#v", result)
	}
	if result.RowCount != 3 || len(result.Blueprints) != 1 || len(result.Recipes) != 1 || len(result.MaterialRequirements) != 1 {
		t.Fatalf("expected blueprint, recipe and material rows, got %#v", result)
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("decode artefact was not written: %v", err)
	}
}

func writeNativeTypeRow(data []byte, offset int, groupID, typeID, typeNameID uint32) {
	binary.LittleEndian.PutUint32(data[offset-32:offset-28], groupID)
	binary.LittleEndian.PutUint32(data[offset:offset+4], typeID)
	binary.LittleEndian.PutUint32(data[offset+4:offset+8], typeNameID)
}

func writeFSDDictHeader(data []byte, rowCount uint64, bucketOffsets []uint64) {
	binary.LittleEndian.PutUint64(data[24:32], uint64(len(data)-32))
	binary.LittleEndian.PutUint64(data[32:40], 24)
	binary.LittleEndian.PutUint64(data[40:48], rowCount)
	binary.LittleEndian.PutUint64(data[48:56], uint64(len(bucketOffsets)))
	for index, offset := range bucketOffsets {
		binary.LittleEndian.PutUint64(data[56+index*8:64+index*8], offset)
	}
}

func writeBlueprintCandidate(data []byte, offset int, primaryTypeID, runTime uint32, pairs []uint32) {
	binary.LittleEndian.PutUint32(data[offset:offset+4], 0)
	binary.LittleEndian.PutUint32(data[offset+4:offset+8], primaryTypeID)
	binary.LittleEndian.PutUint32(data[offset+8:offset+12], runTime)
	binary.LittleEndian.PutUint32(data[offset+12:offset+16], 8)
	for index, value := range pairs {
		start := offset + 16 + index*4
		binary.LittleEndian.PutUint32(data[start:start+4], value)
	}
}
