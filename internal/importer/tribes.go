package importer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/blackrelay/registry/internal/artefacts"
	"github.com/blackrelay/registry/internal/db"
	"github.com/blackrelay/registry/internal/model"
)

const (
	SchemaVersionTribeIdentitiesV1 = "registry.tribe-identities.v1"
	TribeIdentityImporter          = "br-import-tribe-identities"
)

type TribeIdentityStore interface {
	RecordImport(ctx context.Context, importID string, source model.Source, artefact model.SourceArtefact, summary map[string]any) error
	UpsertEntityFacts(ctx context.Context, entity model.Entity, facts []db.EntityFactDraft) error
}

type TribeIdentityOptions struct {
	Environment     model.Environment
	ArtefactRoot    string
	AllowedRootDirs []string
	SourceTitle     string
	Cycle           *int
	Notes           string
}

type TribeIdentityResult struct {
	Source       model.Source         `json:"source"`
	Artefact     model.SourceArtefact `json:"artefact"`
	ImportID     string               `json:"importId"`
	RowsImported int                  `json:"rowsImported"`
}

type tribeIdentityEnvelope struct {
	SchemaVersion string                 `json:"schemaVersion"`
	Environment   model.Environment      `json:"environment"`
	Source        tribeIdentitySource    `json:"source"`
	Tribes        []tribeIdentityRecord  `json:"tribes"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
}

type tribeIdentitySource struct {
	Kind         model.SourceKind   `json:"kind"`
	Confidence   model.Confidence   `json:"confidence"`
	Title        string             `json:"title"`
	Locator      string             `json:"locator"`
	URL          string             `json:"url,omitempty"`
	CheckedAt    time.Time          `json:"checkedAt"`
	ReviewStatus model.ReviewStatus `json:"reviewStatus"`
}

type tribeIdentityRecord struct {
	TribeID       string           `json:"tribeId"`
	Name          string           `json:"name"`
	Tag           string           `json:"tag,omitempty"`
	Aliases       []string         `json:"aliases,omitempty"`
	Description   string           `json:"description,omitempty"`
	URL           string           `json:"url,omitempty"`
	Confidence    model.Confidence `json:"confidence,omitempty"`
	SourceContext string           `json:"sourceContext"`
	Cycle         *int             `json:"cycle,omitempty"`
}

func ImportTribeIdentities(ctx context.Context, store TribeIdentityStore, artefactStore artefacts.Store, inputPath string, opts TribeIdentityOptions) (TribeIdentityResult, error) {
	if store == nil {
		return TribeIdentityResult{}, errors.New("store is required")
	}
	if artefactStore == nil {
		return TribeIdentityResult{}, errors.New("artefact store is required")
	}
	data, err := os.ReadFile(inputPath)
	if err != nil {
		return TribeIdentityResult{}, err
	}
	var envelope tribeIdentityEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		return TribeIdentityResult{}, fmt.Errorf("decode tribe identities: %w", err)
	}
	if err := validateTribeIdentityEnvelope(envelope); err != nil {
		return TribeIdentityResult{}, err
	}
	environment := opts.Environment
	if environment == "" {
		environment = envelope.Environment
	}
	if environment == "" {
		environment = model.EnvironmentStillness
	}
	source := model.Source{
		ID:          fmt.Sprintf("source:tribe-identities:%s", environment),
		Kind:        envelope.Source.Kind,
		Title:       nonEmpty(opts.SourceTitle, envelope.Source.Title),
		Locator:     nonEmpty(envelope.Source.Locator, inputPath),
		URL:         envelope.Source.URL,
		Environment: environment,
		Cycle:       opts.Cycle,
		Metadata: map[string]any{
			"rowCount":     len(envelope.Tribes),
			"reviewStatus": envelope.Source.ReviewStatus,
			"checkedAt":    envelope.Source.CheckedAt.Format(time.RFC3339),
			"policy":       "reviewed_public_tribe_identities_only",
		},
		CreatedAt: time.Now().UTC(),
	}
	artefact, err := artefactStore.RegisterFile(ctx, inputPath, artefacts.RegisterMeta{
		SourceID:        source.ID,
		SourceKind:      source.Kind,
		Kind:            "tribe_identities",
		ArtefactKind:    "tribe_identity_map",
		Environment:     environment,
		ContentType:     "application/json",
		RowCount:        int64(len(envelope.Tribes)),
		Cycle:           opts.Cycle,
		ImporterName:    TribeIdentityImporter,
		ReviewStatus:    envelope.Source.ReviewStatus,
		Notes:           opts.Notes,
		AllowedRootDirs: opts.AllowedRootDirs,
	})
	if err != nil {
		return TribeIdentityResult{}, err
	}
	importID := fmt.Sprintf("import:tribe-identities:%s:%s", environment, artefact.SHA256[:12])
	if err := store.RecordImport(ctx, importID, source, artefact, map[string]any{
		"rowCount":     len(envelope.Tribes),
		"sourceKind":   source.Kind,
		"reviewStatus": envelope.Source.ReviewStatus,
	}); err != nil {
		return TribeIdentityResult{}, err
	}
	for _, row := range envelope.Tribes {
		confidence := row.Confidence
		if confidence == "" {
			confidence = envelope.Source.Confidence
		}
		rowCycle := row.Cycle
		if rowCycle == nil {
			rowCycle = opts.Cycle
		}
		entity := model.Entity{
			ID:          tribeEntityID(environment, row.TribeID),
			Slug:        tribeSlug(environment, row.TribeID),
			Type:        model.EntityTypeTribe,
			Name:        strings.TrimSpace(row.Name),
			DisplayName: strings.TrimSpace(row.Name),
			Summary:     "Reviewed public tribe identity.",
			Environment: environment,
			Cycle:       rowCycle,
			UpdatedAt:   time.Now().UTC(),
		}
		facts := []db.EntityFactDraft{
			tribeFact("tribe_id", row.TribeID, source.ID, confidence, environment, rowCycle, envelope.Source.ReviewStatus),
			tribeFact("display_name", entity.DisplayName, source.ID, confidence, environment, rowCycle, envelope.Source.ReviewStatus),
			tribeFact("source_artefact_id", artefact.ID, source.ID, confidence, environment, rowCycle, envelope.Source.ReviewStatus),
			tribeFact("source_context", strings.TrimSpace(row.SourceContext), source.ID, confidence, environment, rowCycle, envelope.Source.ReviewStatus),
		}
		if tag := strings.TrimSpace(row.Tag); tag != "" {
			facts = append(facts, tribeFact("tag", tag, source.ID, confidence, environment, rowCycle, envelope.Source.ReviewStatus))
		}
		if description := strings.TrimSpace(row.Description); description != "" {
			facts = append(facts, tribeFact("description", description, source.ID, confidence, environment, rowCycle, envelope.Source.ReviewStatus))
		}
		if url := strings.TrimSpace(row.URL); url != "" {
			facts = append(facts, tribeFact("url", url, source.ID, confidence, environment, rowCycle, envelope.Source.ReviewStatus))
		}
		aliases := cleanAliases(row.Aliases)
		if len(aliases) > 0 {
			facts = append(facts, tribeFact("aliases", aliases, source.ID, confidence, environment, rowCycle, envelope.Source.ReviewStatus))
		}
		if err := store.UpsertEntityFacts(ctx, entity, facts); err != nil {
			return TribeIdentityResult{}, err
		}
	}
	return TribeIdentityResult{
		Source:       source,
		Artefact:     artefact,
		ImportID:     importID,
		RowsImported: len(envelope.Tribes),
	}, nil
}

func validateTribeIdentityEnvelope(envelope tribeIdentityEnvelope) error {
	if envelope.SchemaVersion != SchemaVersionTribeIdentitiesV1 {
		return fmt.Errorf("unsupported tribe identity schema version %q", envelope.SchemaVersion)
	}
	if envelope.Environment == "" {
		return errors.New("environment is required")
	}
	if !validSourceKind(envelope.Source.Kind) {
		return fmt.Errorf("unsupported source kind %q", envelope.Source.Kind)
	}
	if !validConfidence(envelope.Source.Confidence) {
		return fmt.Errorf("unsupported source confidence %q", envelope.Source.Confidence)
	}
	if envelope.Source.ReviewStatus != model.ReviewStatusReviewed && envelope.Source.ReviewStatus != model.ReviewStatusPublished {
		return errors.New("tribe identity source must be reviewed or published before import")
	}
	if strings.TrimSpace(envelope.Source.Title) == "" {
		return errors.New("source title is required")
	}
	if envelope.Source.CheckedAt.IsZero() {
		return errors.New("source checkedAt is required")
	}
	if len(envelope.Tribes) == 0 {
		return errors.New("at least one tribe identity is required")
	}
	seen := make(map[string]struct{}, len(envelope.Tribes))
	for _, row := range envelope.Tribes {
		id := strings.TrimSpace(row.TribeID)
		if id == "" {
			return errors.New("tribeId is required")
		}
		parsedID, err := strconv.ParseUint(id, 10, 32)
		if err != nil || parsedID == 0 {
			return fmt.Errorf("tribeId %q must be a positive uint32 decimal", row.TribeID)
		}
		if _, ok := seen[id]; ok {
			return fmt.Errorf("duplicate tribeId %q", id)
		}
		seen[id] = struct{}{}
		if strings.TrimSpace(row.Name) == "" {
			return fmt.Errorf("tribe %s name is required", id)
		}
		if strings.TrimSpace(row.SourceContext) == "" {
			return fmt.Errorf("tribe %s sourceContext is required", id)
		}
		if row.Confidence != "" && !validConfidence(row.Confidence) {
			return fmt.Errorf("tribe %s has unsupported confidence %q", id, row.Confidence)
		}
		if row.Cycle != nil && *row.Cycle <= 0 {
			return fmt.Errorf("tribe %s cycle must be positive when set", id)
		}
	}
	return nil
}

func tribeEntityID(environment model.Environment, tribeID string) string {
	return fmt.Sprintf("tribe:%s:%s", environment, strings.TrimSpace(tribeID))
}

func tribeSlug(environment model.Environment, tribeID string) string {
	return fmt.Sprintf("tribe-%s-%s", strings.TrimSpace(tribeID), environment)
}

func tribeFact(key string, value any, sourceID string, confidence model.Confidence, environment model.Environment, cycle *int, reviewStatus model.ReviewStatus) db.EntityFactDraft {
	return db.EntityFactDraft{
		Key:          key,
		Value:        value,
		SourceID:     sourceID,
		Confidence:   confidence,
		Environment:  environment,
		Cycle:        cycle,
		ReviewStatus: reviewStatus,
	}
}

func cleanAliases(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	return out
}

func validSourceKind(value model.SourceKind) bool {
	switch value {
	case model.SourceKindOnChain,
		model.SourceKindSuiEvent,
		model.SourceKindSuiObject,
		model.SourceKindWorldAPI,
		model.SourceKindDatahub,
		model.SourceKindStaticClientData,
		model.SourceKindReverseEngineered,
		model.SourceKindObservedGameplay,
		model.SourceKindCommunityReport,
		model.SourceKindManualInference:
		return true
	default:
		return false
	}
}

func validConfidence(value model.Confidence) bool {
	switch value {
	case model.ConfidenceVerified,
		model.ConfidenceProbable,
		model.ConfidenceReported,
		model.ConfidenceStale,
		model.ConfidenceUnknown:
		return true
	default:
		return false
	}
}
