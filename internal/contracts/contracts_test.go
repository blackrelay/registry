package contracts

import (
	"os"
	"path/filepath"
	"testing"
)

func TestImportEnvelopeFixtureValidates(t *testing.T) {
	validator := NewValidator(filepath.Join("..", "..", "contracts"))
	if err := validator.ValidateFile("import-envelope.v1.schema.json", filepath.Join("..", "..", "testdata", "fixtures", "static-enemies.reviewed.json")); err != nil {
		t.Fatalf("fixture did not validate: %v", err)
	}
}

func TestImportEnvelopeRejectsUnknownSourceKind(t *testing.T) {
	dir := t.TempDir()
	data := []byte(`{
	  "schemaVersion": "registry.import.v1",
	  "environment": "stillness",
	  "source": {
	    "kind": "private_scrape",
	    "confidence": "probable",
	    "artefactId": "artefact:test",
	    "checkedAt": "2026-06-25T12:00:00Z"
	  },
	  "entities": [],
	  "facts": [],
	  "relations": [],
	  "events": []
	}`)
	path := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	validator := NewValidator(filepath.Join("..", "..", "contracts"))
	if err := validator.ValidateFile("import-envelope.v1.schema.json", path); err == nil {
		t.Fatal("validator accepted an unknown source kind")
	}
}

func TestExportManifestValidates(t *testing.T) {
	data := []byte(`{
	  "schemaVersion": "registry.export_manifest.v1",
	  "registry": "black-relay-registry",
	  "apiVersion": "v1",
	  "generatedAt": "2026-06-25T12:00:00Z",
	  "cycleScope": "current",
	  "cycles": [6],
	  "includeUncycled": true,
	  "database": {
	    "engine": "postgresql",
	    "database": "blackrelay_registry",
	    "serverVersion": "PostgreSQL 18.4",
	    "schemaVersions": ["0001_init"]
	  },
	  "files": [
	    {
	      "path": "entities.jsonl",
	      "contentType": "application/x-ndjson",
	      "sha256": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	      "sizeBytes": 123,
	      "rowCount": 1
	    }
	  ],
	  "highWaterMarks": {
	    "entities": {
	      "rowCount": 1,
	      "complete": true,
	      "cursorOrder": "updated_at DESC, id DESC",
	      "firstId": "entity:1",
	      "firstTime": "2026-06-25T12:00:00Z",
	      "lastId": "entity:1",
	      "lastTime": "2026-06-25T12:00:00Z"
	    }
	  }
	}`)
	validator := NewValidator(filepath.Join("..", "..", "contracts"))
	if err := validator.ValidateBytes("export-manifest.v1.schema.json", data); err != nil {
		t.Fatalf("manifest did not validate: %v", err)
	}
}

func TestStaticClientRecipeFixtureValidates(t *testing.T) {
	validator := NewValidator(filepath.Join("..", "..", "contracts"))
	if err := validator.ValidateFile("static-client-recipes.v1.schema.json", filepath.Join("..", "..", "testdata", "fixtures", "static-client-recipes.reviewed.json")); err != nil {
		t.Fatalf("recipe fixture did not validate: %v", err)
	}
}

func TestStaticClientRecipeContractRejectsMalformedRows(t *testing.T) {
	data := []byte(`{
	  "schemaVersion": "registry.static-client-recipes.v1",
	  "environment": "stillness",
	  "recipes": [
	    {
	      "recipeId": "broken",
	      "name": "Broken",
	      "outputTypeId": 0,
	      "outputQuantity": 1,
	      "inputs": [{"typeId": 3001, "quantity": 1}]
	    }
	  ]
	}`)
	validator := NewValidator(filepath.Join("..", "..", "contracts"))
	if err := validator.ValidateBytes("static-client-recipes.v1.schema.json", data); err == nil {
		t.Fatal("validator accepted malformed static-client recipe rows")
	}
}

func TestTribeIdentityContractValidatesReviewedRows(t *testing.T) {
	data := []byte(`{
	  "schemaVersion": "registry.tribe-identities.v1",
	  "environment": "stillness",
	  "source": {
	    "kind": "community_report",
	    "confidence": "reported",
	    "title": "Reviewed public tribe identity list",
	    "locator": "operator-reviewed-public-list",
	    "checkedAt": "2026-06-26T00:00:00Z",
	    "reviewStatus": "reviewed"
	  },
	  "tribes": [
	    {
	      "tribeId": "42",
	      "name": "Example Relay",
	      "tag": "ER",
	      "aliases": ["Relay Example"],
	      "description": "Example public tribe profile",
	      "url": "https://example.invalid/tribes/example-relay",
	      "confidence": "reported",
	      "sourceContext": "reviewed public profile"
	    }
	  ]
	}`)
	validator := NewValidator(filepath.Join("..", "..", "contracts"))
	if err := validator.ValidateBytes("tribe-identities.v1.schema.json", data); err != nil {
		t.Fatalf("tribe identity fixture did not validate: %v", err)
	}
}

func TestTribeIdentityFixtureValidates(t *testing.T) {
	validator := NewValidator(filepath.Join("..", "..", "contracts"))
	if err := validator.ValidateFile("tribe-identities.v1.schema.json", filepath.Join("..", "..", "testdata", "fixtures", "tribe-identities.reviewed.json")); err != nil {
		t.Fatalf("tribe identity fixture did not validate: %v", err)
	}
}

func TestTribeIdentityContractRejectsUnreviewedRows(t *testing.T) {
	data := []byte(`{
	  "schemaVersion": "registry.tribe-identities.v1",
	  "environment": "stillness",
	  "source": {
	    "kind": "community_report",
	    "confidence": "reported",
	    "title": "Unreviewed tribe identity list",
	    "locator": "operator-draft",
	    "checkedAt": "2026-06-26T00:00:00Z",
	    "reviewStatus": "candidate"
	  },
	  "tribes": [
	    {
	      "tribeId": "42",
	      "name": "Example Relay",
	      "sourceContext": "draft note"
	    }
	  ]
	}`)
	validator := NewValidator(filepath.Join("..", "..", "contracts"))
	if err := validator.ValidateBytes("tribe-identities.v1.schema.json", data); err == nil {
		t.Fatal("validator accepted unreviewed tribe identity rows")
	}
}
