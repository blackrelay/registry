package snapshots

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/blackrelay/registry/internal/artefacts"
	"github.com/blackrelay/registry/internal/model"
)

const (
	StaticEnemySnapshotKind  = "static_enemy_candidates"
	StaticEnemyArtefactKind  = "static_enemy_candidates_jsonl"
	StaticEnemyJSONLImporter = "br-import-static-enemies-jsonl"
)

type EnemyJSONLRow struct {
	GroupID              int    `json:"group_id"`
	TypeID               int    `json:"type_id"`
	Name                 string `json:"name"`
	IsEnemyGroup         bool   `json:"is_enemy_group"`
	IsReviewedIndividual bool   `json:"is_reviewed_individual"`
	SourceContext        string `json:"source_context"`
}

type NormalizedEnemyRow struct {
	GroupID              int    `json:"group_id"`
	TypeID               int    `json:"type_id"`
	Name                 string `json:"name"`
	IsEnemyGroup         bool   `json:"is_enemy_group"`
	IsReviewedIndividual bool   `json:"is_reviewed_individual"`
	SourceContext        string `json:"source_context"`
}

func (r NormalizedEnemyRow) Key() string {
	return fmt.Sprintf("type:%d", r.TypeID)
}

type NormalizedJSONL struct {
	RawSHA256    string
	RowCount     int64
	Rows         []NormalizedEnemyRow
	SemanticHash string
}

func NormalizeEnemyJSONLFile(path string) (NormalizedJSONL, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return NormalizedJSONL{}, err
	}
	return NormalizeEnemyJSONL(data)
}

func NormalizeEnemyJSONL(data []byte) (NormalizedJSONL, error) {
	rawSum := sha256.Sum256(data)
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 64*1024), 8*1024*1024)
	var rows []NormalizedEnemyRow
	for lineNumber := 1; scanner.Scan(); lineNumber++ {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var raw EnemyJSONLRow
		if err := json.Unmarshal(line, &raw); err != nil {
			return NormalizedJSONL{}, fmt.Errorf("decode JSONL line %d: %w", lineNumber, err)
		}
		row, err := normalizeEnemyRow(raw)
		if err != nil {
			return NormalizedJSONL{}, fmt.Errorf("normalise JSONL line %d: %w", lineNumber, err)
		}
		rows = append(rows, row)
	}
	if err := scanner.Err(); err != nil {
		return NormalizedJSONL{}, err
	}
	sortNormalizedRows(rows)
	semantic, err := CanonicalRowsJSONL(rows)
	if err != nil {
		return NormalizedJSONL{}, err
	}
	semanticSum := sha256.Sum256(semantic)
	return NormalizedJSONL{
		RawSHA256:    hex.EncodeToString(rawSum[:]),
		RowCount:     int64(len(rows)),
		Rows:         rows,
		SemanticHash: hex.EncodeToString(semanticSum[:]),
	}, nil
}

func CanonicalRowsJSONL(rows []NormalizedEnemyRow) ([]byte, error) {
	sorted := append([]NormalizedEnemyRow(nil), rows...)
	sortNormalizedRows(sorted)
	var out bytes.Buffer
	encoder := json.NewEncoder(&out)
	encoder.SetEscapeHTML(false)
	for _, row := range sorted {
		if err := encoder.Encode(row); err != nil {
			return nil, err
		}
	}
	return out.Bytes(), nil
}

func normalizeEnemyRow(raw EnemyJSONLRow) (NormalizedEnemyRow, error) {
	if raw.GroupID <= 0 {
		return NormalizedEnemyRow{}, errors.New("group_id must be positive")
	}
	if raw.TypeID <= 0 {
		return NormalizedEnemyRow{}, errors.New("type_id must be positive")
	}
	name := strings.TrimSpace(raw.Name)
	if name == "" {
		return NormalizedEnemyRow{}, errors.New("name is required")
	}
	return NormalizedEnemyRow{
		GroupID:              raw.GroupID,
		TypeID:               raw.TypeID,
		Name:                 name,
		IsEnemyGroup:         raw.IsEnemyGroup,
		IsReviewedIndividual: raw.IsReviewedIndividual,
		SourceContext:        strings.TrimSpace(raw.SourceContext),
	}, nil
}

func sortNormalizedRows(rows []NormalizedEnemyRow) {
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].GroupID != rows[j].GroupID {
			return rows[i].GroupID < rows[j].GroupID
		}
		if rows[i].TypeID != rows[j].TypeID {
			return rows[i].TypeID < rows[j].TypeID
		}
		return rows[i].Name < rows[j].Name
	})
}

type DiffSummary struct {
	NewTypeIDs                             []int           `json:"new_type_ids"`
	RemovedTypeIDs                         []int           `json:"removed_type_ids"`
	ChangedNames                           []ChangedName   `json:"changed_names"`
	ChangedGroups                          []ChangedGroup  `json:"changed_groups"`
	DuplicateNamesWithDifferentTypeIDs     []DuplicateName `json:"duplicate_names_with_different_type_ids"`
	GroupsNewlyClassifiedAsEnemy           []int           `json:"groups_newly_classified_as_enemy"`
	IndividualRowsAddedOutsideEnemyGroup   []int           `json:"individual_rows_added_outside_enemy_group"`
	IndividualRowsRemovedOutsideEnemyGroup []int           `json:"individual_rows_removed_outside_enemy_group"`
}

type ChangedName struct {
	TypeID int    `json:"type_id"`
	From   string `json:"from"`
	To     string `json:"to"`
}

type ChangedGroup struct {
	TypeID int `json:"type_id"`
	From   int `json:"from"`
	To     int `json:"to"`
}

type DuplicateName struct {
	Name    string `json:"name"`
	TypeIDs []int  `json:"type_ids"`
}

func (d DiffSummary) HasMeaningfulChanges() bool {
	return len(d.NewTypeIDs) > 0 ||
		len(d.RemovedTypeIDs) > 0 ||
		len(d.ChangedNames) > 0 ||
		len(d.ChangedGroups) > 0 ||
		len(d.DuplicateNamesWithDifferentTypeIDs) > 0 ||
		len(d.GroupsNewlyClassifiedAsEnemy) > 0 ||
		len(d.IndividualRowsAddedOutsideEnemyGroup) > 0 ||
		len(d.IndividualRowsRemovedOutsideEnemyGroup) > 0
}

func DiffEnemyRows(previous, current []NormalizedEnemyRow) DiffSummary {
	oldByType := rowsByType(previous)
	newByType := rowsByType(current)
	var summary DiffSummary
	for typeID, row := range newByType {
		old, ok := oldByType[typeID]
		if !ok {
			summary.NewTypeIDs = append(summary.NewTypeIDs, typeID)
			if !row.IsEnemyGroup && row.IsReviewedIndividual {
				summary.IndividualRowsAddedOutsideEnemyGroup = append(summary.IndividualRowsAddedOutsideEnemyGroup, typeID)
			}
			continue
		}
		if old.Name != row.Name {
			summary.ChangedNames = append(summary.ChangedNames, ChangedName{TypeID: typeID, From: old.Name, To: row.Name})
		}
		if old.GroupID != row.GroupID {
			summary.ChangedGroups = append(summary.ChangedGroups, ChangedGroup{TypeID: typeID, From: old.GroupID, To: row.GroupID})
		}
	}
	for typeID, row := range oldByType {
		if _, ok := newByType[typeID]; ok {
			continue
		}
		summary.RemovedTypeIDs = append(summary.RemovedTypeIDs, typeID)
		if !row.IsEnemyGroup && row.IsReviewedIndividual {
			summary.IndividualRowsRemovedOutsideEnemyGroup = append(summary.IndividualRowsRemovedOutsideEnemyGroup, typeID)
		}
	}
	summary.GroupsNewlyClassifiedAsEnemy = groupsNewlyClassified(previous, current)
	summary.DuplicateNamesWithDifferentTypeIDs = newDuplicateNames(previous, current)
	sortDiffSummary(&summary)
	return summary
}

func rowsByType(rows []NormalizedEnemyRow) map[int]NormalizedEnemyRow {
	out := make(map[int]NormalizedEnemyRow, len(rows))
	for _, row := range rows {
		out[row.TypeID] = row
	}
	return out
}

func groupsNewlyClassified(previous, current []NormalizedEnemyRow) []int {
	oldGroups := groupEnemyState(previous)
	newGroups := groupEnemyState(current)
	var out []int
	for groupID, newEnemy := range newGroups {
		if newEnemy && !oldGroups[groupID] {
			out = append(out, groupID)
		}
	}
	sort.Ints(out)
	return out
}

func groupEnemyState(rows []NormalizedEnemyRow) map[int]bool {
	out := make(map[int]bool)
	for _, row := range rows {
		if row.IsEnemyGroup {
			out[row.GroupID] = true
			continue
		}
		if _, ok := out[row.GroupID]; !ok {
			out[row.GroupID] = false
		}
	}
	return out
}

func newDuplicateNames(previous, current []NormalizedEnemyRow) []DuplicateName {
	old := duplicateNameSets(previous)
	next := duplicateNameSets(current)
	var out []DuplicateName
	for name, ids := range next {
		oldIDs, existed := old[name]
		if existed && equalInts(oldIDs, ids) {
			continue
		}
		out = append(out, DuplicateName{Name: name, TypeIDs: ids})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out
}

func duplicateNameSets(rows []NormalizedEnemyRow) map[string][]int {
	nameTypes := make(map[string]map[int]struct{})
	displayNames := make(map[string]string)
	for _, row := range rows {
		key := strings.ToLower(row.Name)
		displayNames[key] = row.Name
		if nameTypes[key] == nil {
			nameTypes[key] = make(map[int]struct{})
		}
		nameTypes[key][row.TypeID] = struct{}{}
	}
	out := make(map[string][]int)
	for key, typeSet := range nameTypes {
		if len(typeSet) < 2 {
			continue
		}
		for typeID := range typeSet {
			out[displayNames[key]] = append(out[displayNames[key]], typeID)
		}
		sort.Ints(out[displayNames[key]])
	}
	return out
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func sortDiffSummary(summary *DiffSummary) {
	sort.Ints(summary.NewTypeIDs)
	sort.Ints(summary.RemovedTypeIDs)
	sort.Slice(summary.ChangedNames, func(i, j int) bool { return summary.ChangedNames[i].TypeID < summary.ChangedNames[j].TypeID })
	sort.Slice(summary.ChangedGroups, func(i, j int) bool { return summary.ChangedGroups[i].TypeID < summary.ChangedGroups[j].TypeID })
	sort.Ints(summary.GroupsNewlyClassifiedAsEnemy)
	sort.Ints(summary.IndividualRowsAddedOutsideEnemyGroup)
	sort.Ints(summary.IndividualRowsRemovedOutsideEnemyGroup)
}

type SnapshotSet struct {
	ID                        string
	Environment               model.Environment
	Kind                      string
	Label                     string
	SourceSummary             string
	CreatedAt                 time.Time
	SupersededBySnapshotSetID string
	Notes                     string
	Artefact                  model.SourceArtefact
	Rows                      []NormalizedEnemyRow
}

type OutboxJob struct {
	ID      string         `json:"id"`
	Kind    string         `json:"kind"`
	Payload map[string]any `json:"payload"`
}

type Store interface {
	FindArtefactByHash(ctx context.Context, sha256 string) (model.SourceArtefact, bool, error)
	CurrentSnapshot(ctx context.Context, environment model.Environment, kind string) (SnapshotSet, bool, error)
	RecordSnapshotNoop(ctx context.Context, record NoopRecord) error
	RegisterSnapshotCandidate(ctx context.Context, source model.Source, artefact model.SourceArtefact, rows []NormalizedEnemyRow) error
	PromoteSnapshot(ctx context.Context, promotion Promotion) error
}

type NoopRecord struct {
	ID                    string
	Source                model.Source
	ArtefactHash          string
	ArtefactID            string
	Environment           model.Environment
	Kind                  string
	Reason                string
	RowCount              int64
	ByteIdentical         bool
	SemanticallyUnchanged bool
	CreatedAt             time.Time
}

type Promotion struct {
	Source           model.Source
	Artefact         model.SourceArtefact
	Rows             []NormalizedEnemyRow
	PreviousSnapshot *SnapshotSet
	NewSnapshot      SnapshotSet
	Diff             DiffSummary
	OutboxJobs       []OutboxJob
}

type PipelineOptions struct {
	Environment     model.Environment
	ArtefactRoot    string
	AllowedRootDirs []string
	SourceID        string
	SourceTitle     string
	ClientBuild     string
	PatchLabel      string
	Cycle           *int
	Notes           string
	Now             func() time.Time
}

type PipelineResult struct {
	ArtefactHash                string      `json:"artefactHash"`
	ArtefactID                  string      `json:"artefactId,omitempty"`
	RowCount                    int64       `json:"rowCount"`
	ByteIdentical               bool        `json:"byteIdentical"`
	SemanticallyUnchanged       bool        `json:"semanticallyUnchanged"`
	DiffSummary                 DiffSummary `json:"diffSummary"`
	Promoted                    bool        `json:"promoted"`
	OutboxJobsAppended          []string    `json:"outboxJobsAppended"`
	CanonicalSnapshotSetID      string      `json:"canonicalSnapshotSetId,omitempty"`
	PreviousSnapshotSetID       string      `json:"previousSnapshotSetId,omitempty"`
	CandidateArtefactUnpromoted bool        `json:"candidateArtefactUnpromoted"`
}

func ProcessStaticEnemyJSONL(ctx context.Context, store Store, artefactStore artefacts.Store, path string, opts PipelineOptions) (PipelineResult, error) {
	if store == nil {
		return PipelineResult{}, errors.New("snapshot store is required")
	}
	if artefactStore == nil {
		return PipelineResult{}, errors.New("artefact store is required")
	}
	normalised, err := NormalizeEnemyJSONLFile(path)
	if err != nil {
		return PipelineResult{}, err
	}
	now := time.Now().UTC()
	if opts.Now != nil {
		now = opts.Now().UTC()
	}
	environment := opts.Environment
	if environment == "" {
		environment = model.EnvironmentStillness
	}
	source := model.Source{
		ID:          nonEmpty(opts.SourceID, fmt.Sprintf("source:static-client:%s:static-enemies-jsonl", environment)),
		Kind:        model.SourceKindStaticClientData,
		Title:       nonEmpty(opts.SourceTitle, "Static-client enemy JSONL extraction"),
		Locator:     path,
		Environment: environment,
		Cycle:       opts.Cycle,
		Metadata: map[string]any{
			"clientBuild": opts.ClientBuild,
			"patchLabel":  opts.PatchLabel,
		},
		CreatedAt: now,
	}
	if existing, ok, err := store.FindArtefactByHash(ctx, normalised.RawSHA256); err != nil {
		return PipelineResult{}, err
	} else if ok {
		if err := store.RecordSnapshotNoop(ctx, NoopRecord{
			ID:                    fmt.Sprintf("ingest:no-op:%s:%s", environment, normalised.RawSHA256[:12]),
			Source:                source,
			ArtefactHash:          normalised.RawSHA256,
			ArtefactID:            existing.ID,
			Environment:           environment,
			Kind:                  StaticEnemySnapshotKind,
			Reason:                "byte-identical",
			RowCount:              normalised.RowCount,
			ByteIdentical:         true,
			SemanticallyUnchanged: true,
			CreatedAt:             now,
		}); err != nil {
			return PipelineResult{}, err
		}
		return PipelineResult{
			ArtefactHash:          normalised.RawSHA256,
			ArtefactID:            existing.ID,
			RowCount:              normalised.RowCount,
			ByteIdentical:         true,
			SemanticallyUnchanged: true,
			DiffSummary:           DiffSummary{},
			Promoted:              false,
		}, nil
	}
	current, hasCurrent, err := store.CurrentSnapshot(ctx, environment, StaticEnemySnapshotKind)
	if err != nil {
		return PipelineResult{}, err
	}
	var diff DiffSummary
	semanticallyUnchanged := false
	if hasCurrent {
		previousCanonical, err := CanonicalRowsJSONL(current.Rows)
		if err != nil {
			return PipelineResult{}, err
		}
		nextCanonical, err := CanonicalRowsJSONL(normalised.Rows)
		if err != nil {
			return PipelineResult{}, err
		}
		semanticallyUnchanged = bytes.Equal(previousCanonical, nextCanonical)
		diff = DiffEnemyRows(current.Rows, normalised.Rows)
	} else {
		diff = DiffEnemyRows(nil, normalised.Rows)
	}
	reviewStatus := model.ReviewStatusReviewed
	notes := opts.Notes
	if semanticallyUnchanged {
		reviewStatus = model.ReviewStatusCandidate
		notes = strings.TrimSpace(notes + " unpromoted semantic no-op")
	}
	artefact, err := artefactStore.RegisterFile(ctx, path, artefacts.RegisterMeta{
		SourceID:        source.ID,
		SourceKind:      model.SourceKindStaticClientData,
		Kind:            StaticEnemyArtefactKind,
		ArtefactKind:    StaticEnemyArtefactKind,
		Environment:     environment,
		ContentType:     "application/x-ndjson",
		RowCount:        normalised.RowCount,
		ImporterName:    StaticEnemyJSONLImporter,
		ClientBuild:     opts.ClientBuild,
		PatchLabel:      opts.PatchLabel,
		Cycle:           opts.Cycle,
		ReviewStatus:    reviewStatus,
		Notes:           notes,
		AllowedRootDirs: opts.AllowedRootDirs,
	})
	if err != nil {
		return PipelineResult{}, err
	}
	result := PipelineResult{
		ArtefactHash:          normalised.RawSHA256,
		ArtefactID:            artefact.ID,
		RowCount:              normalised.RowCount,
		ByteIdentical:         false,
		SemanticallyUnchanged: semanticallyUnchanged,
		DiffSummary:           diff,
		Promoted:              false,
	}
	if semanticallyUnchanged {
		if err := store.RegisterSnapshotCandidate(ctx, source, artefact, normalised.Rows); err != nil {
			return PipelineResult{}, err
		}
		if err := store.RecordSnapshotNoop(ctx, NoopRecord{
			ID:                    fmt.Sprintf("ingest:no-op:%s:%s", environment, normalised.RawSHA256[:12]),
			Source:                source,
			ArtefactHash:          normalised.RawSHA256,
			ArtefactID:            artefact.ID,
			Environment:           environment,
			Kind:                  StaticEnemySnapshotKind,
			Reason:                "semantically unchanged",
			RowCount:              normalised.RowCount,
			SemanticallyUnchanged: true,
			CreatedAt:             now,
		}); err != nil {
			return PipelineResult{}, err
		}
		result.CandidateArtefactUnpromoted = true
		return result, nil
	}
	if !diff.HasMeaningfulChanges() && hasCurrent {
		return PipelineResult{}, errors.New("snapshot differs by bytes but produced no semantic diff")
	}
	newSnapshotID := fmt.Sprintf("snapshot:%s:%s:%s", environment, StaticEnemySnapshotKind, normalised.RawSHA256[:12])
	newSnapshot := SnapshotSet{
		ID:            newSnapshotID,
		Environment:   environment,
		Kind:          StaticEnemySnapshotKind,
		Label:         nonEmpty(opts.PatchLabel, normalised.RawSHA256[:12]),
		SourceSummary: fmt.Sprintf("%d static enemy JSONL row(s)", normalised.RowCount),
		CreatedAt:     now,
		Artefact:      artefact,
		Rows:          normalised.Rows,
		Notes:         opts.Notes,
	}
	jobs := buildOutboxJobs(environment, artefact.ID, current.ID, newSnapshotID, diff, now)
	var previous *SnapshotSet
	if hasCurrent {
		previous = &current
		result.PreviousSnapshotSetID = current.ID
	}
	if err := store.PromoteSnapshot(ctx, Promotion{
		Source:           source,
		Artefact:         artefact,
		Rows:             normalised.Rows,
		PreviousSnapshot: previous,
		NewSnapshot:      newSnapshot,
		Diff:             diff,
		OutboxJobs:       jobs,
	}); err != nil {
		return PipelineResult{}, err
	}
	result.Promoted = true
	result.CanonicalSnapshotSetID = newSnapshotID
	for _, job := range jobs {
		result.OutboxJobsAppended = append(result.OutboxJobsAppended, job.Kind)
	}
	return result, nil
}

func buildOutboxJobs(environment model.Environment, artefactID, previousSnapshotID, nextSnapshotID string, diff DiffSummary, now time.Time) []OutboxJob {
	payload := map[string]any{
		"environment":         environment,
		"artefactId":          artefactID,
		"previousSnapshotSet": previousSnapshotID,
		"nextSnapshotSet":     nextSnapshotID,
		"diff":                diff,
	}
	kinds := []string{
		"static_enemy_import",
		"snapshot_diff_generated",
		"public_export_required",
		"search_reindex_required",
		"resolver_refresh_required",
	}
	jobs := make([]OutboxJob, 0, len(kinds))
	for _, kind := range kinds {
		jobs = append(jobs, OutboxJob{
			ID:      fmt.Sprintf("outbox:%s:%d", kind, now.UnixNano()+int64(len(jobs))),
			Kind:    kind,
			Payload: payload,
		})
	}
	return jobs
}

func nonEmpty(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
