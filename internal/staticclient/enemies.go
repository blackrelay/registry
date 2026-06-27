package staticclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/blackrelay/registry/internal/artefacts"
	"github.com/blackrelay/registry/internal/model"
	"github.com/blackrelay/registry/internal/staticdata"
)

const (
	enemyCandidateImporterName = "br-import-static-client-enemies"
	defaultSurvivorWreckTypeID = 81610
)

var defaultEnemyGroupIDs = []int{5033, 4963, 4770, 5130}
var defaultEnemyTypeIDs = []int{85702, 88089}

type EnemyCandidateOptions struct {
	Environment     model.Environment
	AllowedRootDirs []string
	SourceTitle     string
	ClientBuild     string
	PatchLabel      string
	Cycle           *int
	Notes           string
	EnemyGroupIDs   []int
	EnemyTypeIDs    []int
	WreckTypeID     int
}

type EnemyCandidateResult struct {
	Source       model.Source                `json:"source"`
	Artefact     model.SourceArtefact        `json:"artefact"`
	ImportID     string                      `json:"importId"`
	RowsImported int                         `json:"rowsImported"`
	Candidates   []staticdata.EnemyCandidate `json:"candidates"`
}

type EnemyCandidateStore interface {
	RecordImport(ctx context.Context, importID string, source model.Source, artefact model.SourceArtefact, summary map[string]any) error
	UpsertStaticEnemy(ctx context.Context, importID string, source model.Source, artefact model.SourceArtefact, candidate staticdata.EnemyCandidate) error
}

type resolverEnemyPayload struct {
	Candidates []resolverEnemyCandidate `json:"candidates"`
}

type resolverEnemyCandidate struct {
	Description string `json:"description,omitempty"`
	GroupID     int    `json:"groupId"`
	Name        string `json:"name"`
	Reason      string `json:"reason,omitempty"`
	TypeID      int    `json:"typeId"`
	TypeNameID  int    `json:"typeNameId,omitempty"`
	WreckTypeID int    `json:"wreckTypeId,omitempty"`
}

func ImportEnemyCandidates(ctx context.Context, store EnemyCandidateStore, artefactStore artefacts.Store, inputPath string, opts EnemyCandidateOptions) (EnemyCandidateResult, error) {
	if store == nil {
		return EnemyCandidateResult{}, errors.New("store is required")
	}
	if artefactStore == nil {
		return EnemyCandidateResult{}, errors.New("artefact store is required")
	}
	data, err := os.ReadFile(inputPath)
	if err != nil {
		return EnemyCandidateResult{}, err
	}
	rawRows, err := decodeResolverEnemyCandidates(data)
	if err != nil {
		return EnemyCandidateResult{}, err
	}
	candidates, err := ParseEnemyCandidates(data, opts)
	if err != nil {
		return EnemyCandidateResult{}, err
	}
	environment := opts.Environment
	if environment == "" {
		environment = model.EnvironmentStillness
	}
	source := model.Source{
		ID:          fmt.Sprintf("source:static-client:enemies:%s", environment),
		Kind:        model.SourceKindStaticClientData,
		Title:       firstNonEmpty(opts.SourceTitle, "Static-client enemy candidate extraction"),
		Locator:     inputPath,
		Environment: environment,
		Cycle:       opts.Cycle,
		Metadata: map[string]any{
			"rawCandidateCount": len(rawRows),
			"importedCount":     len(candidates),
			"enemyGroups":       effectiveIntSet(opts.EnemyGroupIDs, defaultEnemyGroupIDs),
			"enemyTypeIds":      effectiveIntSet(opts.EnemyTypeIDs, defaultEnemyTypeIDs),
			"wreckTypeId":       effectiveWreckTypeID(opts.WreckTypeID),
		},
		CreatedAt: time.Now().UTC(),
	}
	artefact, err := artefactStore.RegisterFile(ctx, inputPath, artefacts.RegisterMeta{
		SourceID:        source.ID,
		SourceKind:      model.SourceKindStaticClientData,
		Kind:            "static_client_enemy_candidates",
		ArtefactKind:    "static_client_enemy_candidates",
		Environment:     environment,
		ContentType:     "application/json",
		RowCount:        int64(len(rawRows)),
		ImporterName:    enemyCandidateImporterName,
		ClientBuild:     opts.ClientBuild,
		PatchLabel:      opts.PatchLabel,
		Cycle:           opts.Cycle,
		ReviewStatus:    model.ReviewStatusReviewed,
		Notes:           opts.Notes,
		AllowedRootDirs: opts.AllowedRootDirs,
	})
	if err != nil {
		return EnemyCandidateResult{}, err
	}
	importID := fmt.Sprintf("import:static-client-enemies:%s:%s", environment, artefact.SHA256[:12])
	if err := store.RecordImport(ctx, importID, source, artefact, map[string]any{
		"rawCandidateCount": len(rawRows),
		"importedCount":     len(candidates),
		"sourceKind":        source.Kind,
	}); err != nil {
		return EnemyCandidateResult{}, err
	}
	for _, candidate := range candidates {
		if err := store.UpsertStaticEnemy(ctx, importID, source, artefact, candidate); err != nil {
			return EnemyCandidateResult{}, err
		}
	}
	return EnemyCandidateResult{
		Source:       source,
		Artefact:     artefact,
		ImportID:     importID,
		RowsImported: len(candidates),
		Candidates:   candidates,
	}, nil
}

func ParseEnemyCandidates(data []byte, opts EnemyCandidateOptions) ([]staticdata.EnemyCandidate, error) {
	rows, err := decodeResolverEnemyCandidates(data)
	if err != nil {
		return nil, err
	}
	groupIDs := intSet(effectiveIntSet(opts.EnemyGroupIDs, defaultEnemyGroupIDs))
	typeIDs := intSet(effectiveIntSet(opts.EnemyTypeIDs, defaultEnemyTypeIDs))
	wreckTypeID := effectiveWreckTypeID(opts.WreckTypeID)
	seen := make(map[int]struct{})
	out := make([]staticdata.EnemyCandidate, 0, len(rows))
	for _, row := range rows {
		if row.Name == "" || row.TypeID <= 0 {
			continue
		}
		if wreckTypeID > 0 && row.WreckTypeID != wreckTypeID {
			continue
		}
		_, groupOK := groupIDs[row.GroupID]
		_, typeOK := typeIDs[row.TypeID]
		if !groupOK && !typeOK {
			continue
		}
		if _, exists := seen[row.TypeID]; exists {
			continue
		}
		seen[row.TypeID] = struct{}{}
		out = append(out, staticdata.EnemyCandidate{
			Name:       row.Name,
			GroupID:    row.GroupID,
			TypeID:     row.TypeID,
			Confidence: string(model.ConfidenceProbable),
			Basis:      enemyBasis(row, groupOK, wreckTypeID),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].GroupID == out[j].GroupID {
			if out[i].Name == out[j].Name {
				return out[i].TypeID < out[j].TypeID
			}
			return out[i].Name < out[j].Name
		}
		return out[i].GroupID < out[j].GroupID
	})
	return out, nil
}

func decodeResolverEnemyCandidates(data []byte) ([]resolverEnemyCandidate, error) {
	var payload resolverEnemyPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, err
	}
	if payload.Candidates == nil {
		return nil, errors.New("static-client enemy input must contain a candidates array")
	}
	return payload.Candidates, nil
}

func enemyBasis(row resolverEnemyCandidate, groupOK bool, wreckTypeID int) string {
	if groupOK {
		return fmt.Sprintf("static-client group %d with wreck type %d", row.GroupID, wreckTypeID)
	}
	return fmt.Sprintf("reviewed static-client individual type %d with wreck type %d", row.TypeID, wreckTypeID)
}

func effectiveWreckTypeID(value int) int {
	if value > 0 {
		return value
	}
	return defaultSurvivorWreckTypeID
}

func effectiveIntSet(values []int, defaults []int) []int {
	if len(values) > 0 {
		out := append([]int(nil), values...)
		sort.Ints(out)
		return out
	}
	out := append([]int(nil), defaults...)
	sort.Ints(out)
	return out
}

func intSet(values []int) map[int]struct{} {
	out := make(map[int]struct{}, len(values))
	for _, value := range values {
		out[value] = struct{}{}
	}
	return out
}
