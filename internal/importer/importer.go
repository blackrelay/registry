package importer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/blackrelay/registry/internal/artefacts"
	"github.com/blackrelay/registry/internal/contracts"
	"github.com/blackrelay/registry/internal/model"
	"github.com/blackrelay/registry/internal/staticdata"
)

const (
	SchemaVersionImportV1 = "registry.import.v1"
	StaticEnemySourceID   = "source:static-client:stillness:reviewed-enemies"
	StaticEnemyImporter   = "br-import-static-enemies"
)

type Envelope struct {
	SchemaVersion string            `json:"schemaVersion"`
	Environment   model.Environment `json:"environment"`
	Source        EnvelopeSource    `json:"source"`
	Entities      []model.Entity    `json:"entities"`
	Facts         []model.Fact      `json:"facts"`
	Relations     []model.Relation  `json:"relations"`
	Events        []EventRecord     `json:"events"`
}

type EnvelopeSource struct {
	Kind       model.SourceKind `json:"kind"`
	Confidence model.Confidence `json:"confidence"`
	ArtefactID string           `json:"artefactId"`
	CheckedAt  time.Time        `json:"checkedAt"`
}

type EventRecord struct {
	ID          string            `json:"id"`
	Kind        string            `json:"kind"`
	Environment model.Environment `json:"environment"`
	OccurredAt  time.Time         `json:"occurredAt"`
	Payload     map[string]any    `json:"payload"`
	SourceIDs   []string          `json:"sourceIds"`
}

type StaticEnemyStore interface {
	UpsertStaticEnemy(ctx context.Context, importID string, source model.Source, artefact model.SourceArtefact, candidate staticdata.EnemyCandidate) error
	RecordImport(ctx context.Context, importID string, source model.Source, artefact model.SourceArtefact, summary map[string]any) error
}

type StaticEnemyOptions struct {
	Environment     model.Environment
	ArtefactRoot    string
	AllowedRootDirs []string
	SourceTitle     string
	Cycle           *int
	Notes           string
}

type StaticEnemyResult struct {
	Source     model.Source                `json:"source"`
	Artefact   model.SourceArtefact        `json:"artefact"`
	ImportID   string                      `json:"importId"`
	Candidates []staticdata.EnemyCandidate `json:"candidates"`
}

func BuildStaticEnemyFixture(environment model.Environment) ([]byte, error) {
	checkedAt := time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)
	envelope := map[string]any{
		"schemaVersion": SchemaVersionImportV1,
		"environment":   environment,
		"source": map[string]any{
			"kind":       model.SourceKindStaticClientData,
			"confidence": model.ConfidenceProbable,
			"artefactId": "artefact:filled-after-registration",
			"checkedAt":  checkedAt.Format(time.RFC3339),
		},
		"entities":   []any{},
		"facts":      []any{},
		"relations":  []any{},
		"events":     []any{},
		"candidates": staticdata.ReviewedEnemies(),
	}
	return json.MarshalIndent(envelope, "", "  ")
}

func ImportStaticEnemies(ctx context.Context, store StaticEnemyStore, artefactStore artefacts.Store, candidatePath string, opts StaticEnemyOptions) (StaticEnemyResult, error) {
	if store == nil {
		return StaticEnemyResult{}, errors.New("store is required")
	}
	if artefactStore == nil {
		return StaticEnemyResult{}, errors.New("artefact store is required")
	}
	data, input, err := artefacts.ReadLocalInput(candidatePath, opts.AllowedRootDirs)
	if err != nil {
		return StaticEnemyResult{}, err
	}
	candidates, err := staticdata.ParseCandidatesJSON(data)
	if err != nil {
		return StaticEnemyResult{}, err
	}
	if err := staticdata.ValidateReviewedEnemies(candidates); err != nil {
		return StaticEnemyResult{}, err
	}
	environment := opts.Environment
	if environment == "" {
		environment = model.EnvironmentStillness
	}
	source := model.Source{
		ID:          StaticEnemySourceID,
		Kind:        model.SourceKindStaticClientData,
		Title:       nonEmpty(opts.SourceTitle, "Reviewed Stillness static-client enemy candidates"),
		Locator:     input.Path,
		Environment: environment,
		Cycle:       opts.Cycle,
		Metadata: map[string]any{
			"candidateCount": len(candidates),
			"policy":         "reviewed_static_client_enemy_candidates_only",
		},
		CreatedAt: time.Now().UTC(),
	}
	artefact, err := artefactStore.RegisterFile(ctx, candidatePath, artefacts.RegisterMeta{
		SourceID:        source.ID,
		Kind:            "static_enemy_candidates",
		Environment:     environment,
		ContentType:     "application/json",
		RowCount:        int64(len(candidates)),
		ImporterName:    StaticEnemyImporter,
		Cycle:           opts.Cycle,
		ReviewStatus:    model.ReviewStatusReviewed,
		Notes:           opts.Notes,
		AllowedRootDirs: opts.AllowedRootDirs,
	})
	if err != nil {
		return StaticEnemyResult{}, err
	}
	importID := fmt.Sprintf("import:static-enemies:%s:%s", environment, artefact.SHA256[:12])
	if err := store.RecordImport(ctx, importID, source, artefact, map[string]any{
		"candidateCount": len(candidates),
		"sourceKind":     source.Kind,
	}); err != nil {
		return StaticEnemyResult{}, err
	}
	for _, candidate := range candidates {
		if err := store.UpsertStaticEnemy(ctx, importID, source, artefact, candidate); err != nil {
			return StaticEnemyResult{}, err
		}
	}
	return StaticEnemyResult{
		Source:     source,
		Artefact:   artefact,
		ImportID:   importID,
		Candidates: candidates,
	}, nil
}

func ValidateEnvelope(schemaDir string, data []byte) error {
	var envelope Envelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		return err
	}
	if envelope.SchemaVersion != SchemaVersionImportV1 {
		return fmt.Errorf("unsupported import schema version %q", envelope.SchemaVersion)
	}
	return contracts.NewValidator(schemaDir).ValidateBytes("import-envelope.v1.schema.json", data)
}

func nonEmpty(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
