package staticclient

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCompareStaticTypeFilesReportsStableTypeIDDeltas(t *testing.T) {
	dir := t.TempDir()
	resolvedPath := filepath.Join(dir, "resolved.json")
	nativePath := filepath.Join(dir, "native.json")
	writeStaticTypeFixture(t, resolvedPath, `[
  {"typeId":92096,"groupId":5033,"name":"Caird","wreckTypeId":81610},
  {"typeId":49782,"groupId":27,"name":"Winter","wreckTypeId":81610},
  {"typeId":900,"groupId":1,"name":"Old Probe","wreckTypeId":10},
  {"typeId":901,"groupId":1,"name":"Duplicate","wreckTypeId":10},
  {"typeId":902,"groupId":2,"name":"Duplicate","wreckTypeId":10}
]`)
	writeStaticTypeFixture(t, nativePath, `[
  {"typeId":92096,"groupId":5130,"name":"Caird Prime","wreckTypeId":81611},
  {"typeId":94167,"groupId":5130,"name":"Mycena","wreckTypeId":81610},
  {"typeId":901,"groupId":1,"name":"Duplicate","wreckTypeId":10},
  {"typeId":902,"groupId":2,"name":"Duplicate","wreckTypeId":10}
]`)

	result, err := CompareStaticTypeFiles(resolvedPath, nativePath)
	if err != nil {
		t.Fatal(err)
	}
	if result.ResolvedCount != 5 || result.NativeCount != 4 || result.MatchedCount != 3 {
		t.Fatalf("unexpected counts %#v", result)
	}
	if len(result.NativeOnly) != 1 || result.NativeOnly[0].TypeID != 94167 {
		t.Fatalf("native-only rows were not reported by type id: %#v", result.NativeOnly)
	}
	if len(result.ResolvedOnly) != 2 || result.ResolvedOnly[0].TypeID != 900 || result.ResolvedOnly[1].TypeID != 49782 {
		t.Fatalf("resolved-only rows were not reported deterministically: %#v", result.ResolvedOnly)
	}
	if len(result.ChangedName) != 1 || result.ChangedName[0].TypeID != 92096 {
		t.Fatalf("changed names were not reported: %#v", result.ChangedName)
	}
	if len(result.ChangedGroup) != 1 || result.ChangedGroup[0].TypeID != 92096 {
		t.Fatalf("changed groups were not reported: %#v", result.ChangedGroup)
	}
	if len(result.ChangedWreckType) != 1 || result.ChangedWreckType[0].TypeID != 92096 {
		t.Fatalf("changed wreck type ids were not reported: %#v", result.ChangedWreckType)
	}
	if len(result.DuplicateNames) != 1 || result.DuplicateNames[0].Name != "Duplicate" || len(result.DuplicateNames[0].TypeIDs) != 2 {
		t.Fatalf("duplicate names must remain visible without collapsing rows: %#v", result.DuplicateNames)
	}
	if result.SafeToPromote {
		t.Fatalf("meaningful deltas must not be marked safe to promote: %#v", result)
	}
}

func TestCompareStaticTypeFilesAllowsSemanticMatchesDespiteOrder(t *testing.T) {
	dir := t.TempDir()
	resolvedPath := filepath.Join(dir, "resolved.json")
	nativePath := filepath.Join(dir, "native.json")
	writeStaticTypeFixture(t, resolvedPath, `{
  "candidates": [
    {"typeId":92096,"groupId":5033,"name":"Caird","wreckTypeId":81610},
    {"typeId":94167,"groupId":5130,"name":"Mycena","wreckTypeId":81610}
  ]
}`)
	writeStaticTypeFixture(t, nativePath, `[
  {"typeId":94167,"groupId":5130,"name":"Mycena","wreckTypeId":81610},
  {"typeId":92096,"groupId":5033,"name":"Caird","wreckTypeId":81610}
]`)

	result, err := CompareStaticTypeFiles(resolvedPath, nativePath)
	if err != nil {
		t.Fatal(err)
	}
	if !result.SemanticallyEqual || !result.SafeToPromote {
		t.Fatalf("same rows in different order should compare as a semantic match: %#v", result)
	}
	if len(result.NativeOnly)+len(result.ResolvedOnly)+len(result.ChangedName)+len(result.ChangedGroup)+len(result.ChangedWreckType) != 0 {
		t.Fatalf("semantic match reported unexpected deltas: %#v", result)
	}
}

func writeStaticTypeFixture(t *testing.T, path string, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
