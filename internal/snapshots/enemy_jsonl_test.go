package snapshots

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/blackrelay/registry/internal/artefacts"
	"github.com/blackrelay/registry/internal/model"
)

func TestByteIdenticalJSONLArtefactIsNoop(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := writeJSONL(t, dir, "static.jsonl", baseRows())
	store := NewMemoryStore()
	artefactStore := artefacts.LocalStore{Root: filepath.Join(dir, "artefacts"), Now: fixedNow}
	first, err := ProcessStaticEnemyJSONL(ctx, store, artefactStore, path, testOptions(dir))
	if err != nil {
		t.Fatalf("first process returned error: %v", err)
	}
	if !first.Promoted {
		t.Fatal("first snapshot should be promoted")
	}
	beforeJobs := len(store.Jobs)
	second, err := ProcessStaticEnemyJSONL(ctx, store, artefactStore, path, testOptions(dir))
	if err != nil {
		t.Fatalf("second process returned error: %v", err)
	}
	if !second.ByteIdentical {
		t.Fatal("expected byte-identical no-op")
	}
	if second.Promoted {
		t.Fatal("byte-identical run must not promote a duplicate")
	}
	if len(store.Jobs) != beforeJobs {
		t.Fatal("byte-identical no-op appended outbox jobs")
	}
	if got := len(store.ArtefactIDs()); got != 1 {
		t.Fatalf("expected one canonical artefact, got %d", got)
	}
}

func TestDifferentByteOrderSameSemanticRowsIsUnchanged(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	firstPath := writeJSONL(t, dir, "before.jsonl", baseRows())
	secondPath := filepath.Join(dir, "reordered.jsonl")
	if err := os.WriteFile(secondPath, []byte(
		`{"ignored":"metadata","group_id":27,"type_id":85702,"name":"Feral Mooneater","is_enemy_group":false,"is_reviewed_individual":true,"source_context":"reviewed individual enemy row"}`+"\n"+
			`{"group_id":5033,"type_id":92098,"name":"Ostler","is_enemy_group":true,"is_reviewed_individual":false,"source_context":"reviewed enemy group 5033"}`+"\n"+
			`{"group_id":5033,"type_id":92096,"name":"Caird","is_enemy_group":true,"is_reviewed_individual":false,"source_context":"reviewed enemy group 5033"}`+"\n",
	), 0o644); err != nil {
		t.Fatal(err)
	}
	store := NewMemoryStore()
	artefactStore := artefacts.LocalStore{Root: filepath.Join(dir, "artefacts"), Now: fixedNow}
	if _, err := ProcessStaticEnemyJSONL(ctx, store, artefactStore, firstPath, testOptions(dir)); err != nil {
		t.Fatalf("first process returned error: %v", err)
	}
	beforeJobs := len(store.Jobs)
	result, err := ProcessStaticEnemyJSONL(ctx, store, artefactStore, secondPath, testOptions(dir))
	if err != nil {
		t.Fatalf("second process returned error: %v", err)
	}
	if !result.SemanticallyUnchanged {
		t.Fatal("expected semantic no-op")
	}
	if result.Promoted {
		t.Fatal("semantic no-op must not be promoted")
	}
	if !result.CandidateArtefactUnpromoted {
		t.Fatal("semantic no-op artefact should remain unpromoted")
	}
	if len(store.Jobs) != beforeJobs {
		t.Fatal("semantic no-op appended import/export jobs")
	}
	if got := len(store.ArtefactIDs()); got != 2 {
		t.Fatalf("expected canonical and unpromoted candidate artefacts, got %d", got)
	}
}

func TestMeaningfulChangeAppendsOutboxJobsAndSupersedesPreviousSnapshot(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	firstPath := writeJSONL(t, dir, "before.jsonl", baseRows())
	nextRows := append(baseRows(), `{"group_id":5033,"type_id":92101,"name":"Dowser","is_enemy_group":true,"is_reviewed_individual":false,"source_context":"reviewed enemy group 5033"}`)
	nextPath := writeJSONL(t, dir, "after.jsonl", nextRows)
	store := NewMemoryStore()
	artefactStore := artefacts.LocalStore{Root: filepath.Join(dir, "artefacts"), Now: fixedNow}
	first, err := ProcessStaticEnemyJSONL(ctx, store, artefactStore, firstPath, testOptions(dir))
	if err != nil {
		t.Fatalf("first process returned error: %v", err)
	}
	second, err := ProcessStaticEnemyJSONL(ctx, store, artefactStore, nextPath, testOptions(dir))
	if err != nil {
		t.Fatalf("second process returned error: %v", err)
	}
	if !second.Promoted {
		t.Fatal("meaningful change should be promoted")
	}
	if len(second.DiffSummary.NewTypeIDs) != 1 || second.DiffSummary.NewTypeIDs[0] != 92101 {
		t.Fatalf("expected new type ID diff, got %#v", second.DiffSummary.NewTypeIDs)
	}
	if len(second.OutboxJobsAppended) != 5 {
		t.Fatalf("expected five outbox jobs, got %#v", second.OutboxJobsAppended)
	}
	firstArtefact := store.Artefacts[first.ArtefactID]
	if firstArtefact.ID == "" {
		t.Fatal("previous canonical artefact disappeared")
	}
	if firstArtefact.SupersededByArtefactID != second.ArtefactID {
		t.Fatalf("previous artefact was not superseded by new artefact: %#v", firstArtefact)
	}
}

func TestDiffCases(t *testing.T) {
	oldRows := mustNormalizeRows(t, []string{
		`{"group_id":5033,"type_id":92096,"name":"Caird","is_enemy_group":true,"is_reviewed_individual":false,"source_context":"a"}`,
		`{"group_id":27,"type_id":85702,"name":"Feral Mooneater","is_enemy_group":false,"is_reviewed_individual":true,"source_context":"b"}`,
		`{"group_id":4000,"type_id":1,"name":"Old Group","is_enemy_group":false,"is_reviewed_individual":false,"source_context":"c"}`,
	})
	newRows := mustNormalizeRows(t, []string{
		`{"group_id":5033,"type_id":92096,"name":"Grave Caird","is_enemy_group":true,"is_reviewed_individual":false,"source_context":"a"}`,
		`{"group_id":4770,"type_id":85702,"name":"Feral Mooneater","is_enemy_group":true,"is_reviewed_individual":false,"source_context":"b"}`,
		`{"group_id":5000,"type_id":2,"name":"New Group","is_enemy_group":true,"is_reviewed_individual":false,"source_context":"c"}`,
		`{"group_id":27,"type_id":3,"name":"Individual","is_enemy_group":false,"is_reviewed_individual":true,"source_context":"d"}`,
		`{"group_id":5033,"type_id":4,"name":"Grave Caird","is_enemy_group":true,"is_reviewed_individual":false,"source_context":"e"}`,
	})
	diff := DiffEnemyRows(oldRows, newRows)
	if !containsInt(diff.NewTypeIDs, 2) || !containsInt(diff.NewTypeIDs, 3) || !containsInt(diff.NewTypeIDs, 4) {
		t.Fatalf("missing new type ID diff: %#v", diff.NewTypeIDs)
	}
	if !containsInt(diff.RemovedTypeIDs, 1) {
		t.Fatalf("missing removed type ID diff: %#v", diff.RemovedTypeIDs)
	}
	if len(diff.ChangedNames) != 1 || diff.ChangedNames[0].TypeID != 92096 {
		t.Fatalf("missing changed name diff: %#v", diff.ChangedNames)
	}
	if len(diff.ChangedGroups) != 1 || diff.ChangedGroups[0].TypeID != 85702 {
		t.Fatalf("missing changed group diff: %#v", diff.ChangedGroups)
	}
	if !containsInt(diff.GroupsNewlyClassifiedAsEnemy, 4770) || !containsInt(diff.GroupsNewlyClassifiedAsEnemy, 5000) {
		t.Fatalf("missing newly classified enemy group diff: %#v", diff.GroupsNewlyClassifiedAsEnemy)
	}
	if !containsInt(diff.IndividualRowsAddedOutsideEnemyGroup, 3) {
		t.Fatalf("missing individual added diff: %#v", diff.IndividualRowsAddedOutsideEnemyGroup)
	}
	if !containsInt(diff.DuplicateNamesWithDifferentTypeIDs[0].TypeIDs, 4) {
		t.Fatalf("duplicate name with distinct type IDs was not preserved: %#v", diff.DuplicateNamesWithDifferentTypeIDs)
	}
}

func TestIndividualRowRemovedOutsideEnemyGroup(t *testing.T) {
	oldRows := mustNormalizeRows(t, []string{
		`{"group_id":27,"type_id":85702,"name":"Feral Mooneater","is_enemy_group":false,"is_reviewed_individual":true,"source_context":"b"}`,
	})
	diff := DiffEnemyRows(oldRows, nil)
	if !containsInt(diff.IndividualRowsRemovedOutsideEnemyGroup, 85702) {
		t.Fatalf("missing individual removed diff: %#v", diff.IndividualRowsRemovedOutsideEnemyGroup)
	}
}

func baseRows() []string {
	return []string{
		`{"group_id":5033,"type_id":92096,"name":"Caird","is_enemy_group":true,"is_reviewed_individual":false,"source_context":"reviewed enemy group 5033"}`,
		`{"group_id":5033,"type_id":92098,"name":"Ostler","is_enemy_group":true,"is_reviewed_individual":false,"source_context":"reviewed enemy group 5033"}`,
		`{"group_id":27,"type_id":85702,"name":"Feral Mooneater","is_enemy_group":false,"is_reviewed_individual":true,"source_context":"reviewed individual enemy row"}`,
	}
}

func writeJSONL(t *testing.T, dir, name string, rows []string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	data := ""
	for _, row := range rows {
		data += row + "\n"
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func mustNormalizeRows(t *testing.T, rows []string) []NormalizedEnemyRow {
	t.Helper()
	data := ""
	for _, row := range rows {
		data += row + "\n"
	}
	normalised, err := NormalizeEnemyJSONL([]byte(data))
	if err != nil {
		t.Fatal(err)
	}
	return normalised.Rows
}

func testOptions(dir string) PipelineOptions {
	return PipelineOptions{
		Environment:     model.EnvironmentStillness,
		AllowedRootDirs: []string{dir},
		PatchLabel:      "test-patch",
		Now:             fixedNow,
	}
}

func fixedNow() time.Time {
	return time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)
}

func containsInt(values []int, want int) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
