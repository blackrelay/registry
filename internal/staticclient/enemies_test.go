package staticclient

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/blackrelay/registry/internal/artefacts"
	"github.com/blackrelay/registry/internal/db"
	"github.com/blackrelay/registry/internal/model"
)

func TestImportEnemyCandidatesImportsReviewedStaticClientGroups(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "static-client-types.json")
	writeFixture(t, path, `{
		"candidates": [
			{"groupId":5130,"name":"Mycena","typeId":94167,"typeNameId":1044223,"wreckTypeId":81610},
			{"groupId":5130,"name":"Mycena","typeId":95504,"typeNameId":1047620,"wreckTypeId":81610},
			{"groupId":5130,"name":"Dermestid","typeId":95283,"typeNameId":1047619,"wreckTypeId":81610},
			{"groupId":5130,"name":"Chrysalis","typeId":95291,"typeNameId":1047621,"wreckTypeId":81610},
			{"groupId":25,"name":"Cliff","typeId":91160,"typeNameId":1041,"wreckTypeId":81610},
			{"groupId":5130,"name":"Harmless Copy","typeId":99999,"typeNameId":1042,"wreckTypeId":12345}
		]
	}`)
	store := db.NewMemoryStore()
	result, err := ImportEnemyCandidates(context.Background(), store, artefacts.LocalStore{Root: filepath.Join(dir, "artefacts")}, path, EnemyCandidateOptions{
		Environment:     model.EnvironmentStillness,
		AllowedRootDirs: []string{dir},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.RowsImported != 4 {
		t.Fatalf("expected four imported enemy rows, got %#v", result)
	}
	for _, id := range []string{"enemy:stillness:type:94167", "enemy:stillness:type:95504", "enemy:stillness:type:95283", "enemy:stillness:type:95291"} {
		entity := store.Entities[id]
		if entity.Type != model.EntityTypeEnemy || entity.DisplayName == "" {
			t.Fatalf("expected enemy entity %s, got %#v", id, entity)
		}
	}
	if _, ok := store.Entities["enemy:stillness:type:91160"]; ok {
		t.Fatal("player ship group 25 row was imported as an enemy")
	}
	if _, ok := store.Entities["enemy:stillness:type:99999"]; ok {
		t.Fatal("row without survivor wreck type was imported as an enemy")
	}
	if _, ok := store.Artefacts[result.Artefact.ID]; !ok {
		t.Fatalf("source artefact was not recorded: %#v", store.Artefacts)
	}
}

func TestParseEnemyCandidatesReportsNewStillnessNPCs(t *testing.T) {
	payload := []byte(`{
		"candidates": [
			{"groupId":5130,"name":"Chrysalis","typeId":95291,"typeNameId":1047621,"wreckTypeId":81610},
			{"groupId":5130,"name":"Dermestid","typeId":95283,"typeNameId":1047619,"wreckTypeId":81610},
			{"groupId":5130,"name":"Mycena","typeId":94167,"typeNameId":1044223,"wreckTypeId":81610},
			{"groupId":5130,"name":"Mycena","typeId":95504,"typeNameId":1047620,"wreckTypeId":81610}
		]
	}`)
	candidates, err := ParseEnemyCandidates(payload, EnemyCandidateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 4 {
		t.Fatalf("expected four group 5130 candidates, got %#v", candidates)
	}
}
