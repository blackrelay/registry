package terminology_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestBritishEnglishTerminology(t *testing.T) {
	root := repositoryRoot(t)
	forbidden := "arti" + "fact"
	var matches []string

	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", "artefacts", "bin", "coverage", "exports", "exports-all", "local-extract", "node_modules", "published-exports", "tmp":
				return filepath.SkipDir
			}
			return nil
		}
		if !isSourceFile(path) {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if filepath.ToSlash(rel) == ".github/config/cspell.json" {
			return nil
		}
		if strings.Contains(strings.ToLower(filepath.ToSlash(rel)), forbidden) {
			matches = append(matches, filepath.ToSlash(rel))
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		content := strings.ToLower(string(data))
		content = strings.ReplaceAll(content, "actions/upload-artifact", "actions/upload-artefact")
		if strings.Contains(content, forbidden) {
			matches = append(matches, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) > 0 {
		t.Fatalf("use British English terminology; replace American spelling in: %s", strings.Join(matches, ", "))
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not locate test file")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func isSourceFile(path string) bool {
	switch filepath.Base(path) {
	case ".gitignore", "Dockerfile", "go.mod":
		return true
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go", ".json", ".md", ".ps1", ".sh", ".sql", ".toml", ".txt", ".yaml", ".yml":
		return true
	default:
		return false
	}
}
