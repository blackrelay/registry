package staticclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/blackrelay/registry/internal/artefacts"
	"github.com/blackrelay/registry/internal/db"
	"github.com/blackrelay/registry/internal/model"
	"github.com/blackrelay/registry/internal/staticdata"
)

const typeImporterName = "br-import-static-client-types"

var staticClientShipGroupIDs = map[int]struct{}{
	25:   {},
	26:   {},
	27:   {},
	31:   {},
	237:  {},
	419:  {},
	420:  {},
	1764: {},
}

var staticClientStructureGroupIDs = map[int]struct{}{
	15:   {},
	365:  {},
	707:  {},
	1404: {},
	1406: {},
	1657: {},
	2017: {},
}

var staticClientEnemyGroupIDSet = intSet(defaultEnemyGroupIDs)
var staticClientEnemyTypeIDSet = intSet(defaultEnemyTypeIDs)

type TypeImportOptions struct {
	Environment     model.Environment
	AllowedRootDirs []string
	SourceTitle     string
	ClientBuild     string
	PatchLabel      string
	Cycle           *int
	Notes           string
}

type TypeImportResult struct {
	Source       model.Source         `json:"source"`
	Artefact     model.SourceArtefact `json:"artefact"`
	ImportID     string               `json:"importId"`
	RowsImported int                  `json:"rowsImported"`
}

type TypeStore interface {
	RecordImport(ctx context.Context, importID string, source model.Source, artefact model.SourceArtefact, summary map[string]any) error
	UpsertEntityFacts(ctx context.Context, entity model.Entity, facts []db.EntityFactDraft) error
}

type bulkTypeStore interface {
	UpsertEventDerivationBatch(ctx context.Context, entities []db.EntityFactSet, relations []db.RelationDraft, killmails []model.KillmailRaw) error
}

type staticTypePayload struct {
	Candidates []staticTypeRow `json:"candidates"`
	Data       []staticTypeRow `json:"data"`
	Items      []staticTypeRow `json:"items"`
	Rows       []staticTypeRow `json:"rows"`
	Types      []staticTypeRow `json:"types"`
}

type staticTypeRow struct {
	CategoryID    int    `json:"categoryId,omitempty"`
	CategoryName  string `json:"categoryName,omitempty"`
	Description   string `json:"description,omitempty"`
	GroupID       int    `json:"groupId"`
	GroupName     string `json:"groupName,omitempty"`
	MarketGroupID int    `json:"marketGroupId,omitempty"`
	Name          string `json:"name"`
	Reason        string `json:"reason,omitempty"`
	TypeID        int    `json:"typeId"`
	TypeNameID    int    `json:"typeNameId,omitempty"`
	WreckTypeID   int    `json:"wreckTypeId,omitempty"`
}

func ImportTypes(ctx context.Context, store TypeStore, artefactStore artefacts.Store, inputPath string, opts TypeImportOptions) (TypeImportResult, error) {
	if store == nil {
		return TypeImportResult{}, errors.New("store is required")
	}
	if artefactStore == nil {
		return TypeImportResult{}, errors.New("artefact store is required")
	}
	data, err := os.ReadFile(inputPath)
	if err != nil {
		return TypeImportResult{}, err
	}
	rows, err := decodeStaticTypeRows(data)
	if err != nil {
		return TypeImportResult{}, err
	}
	rows = normaliseStaticTypeRows(rows)
	environment := opts.Environment
	if environment == "" {
		environment = model.EnvironmentStillness
	}
	source := model.Source{
		ID:          fmt.Sprintf("source:static-client:types:%s", environment),
		Kind:        model.SourceKindStaticClientData,
		Title:       firstNonEmpty(opts.SourceTitle, "Static-client type metadata"),
		Locator:     inputPath,
		Environment: environment,
		Cycle:       opts.Cycle,
		Metadata: map[string]any{
			"rowCount": len(rows),
		},
		CreatedAt: time.Now().UTC(),
	}
	artefact, err := artefactStore.RegisterFile(ctx, inputPath, artefacts.RegisterMeta{
		SourceID:        source.ID,
		SourceKind:      model.SourceKindStaticClientData,
		Kind:            "static_client_types",
		ArtefactKind:    "static_client_types",
		Environment:     environment,
		ContentType:     "application/json",
		RowCount:        int64(len(rows)),
		ImporterName:    typeImporterName,
		ClientBuild:     opts.ClientBuild,
		PatchLabel:      opts.PatchLabel,
		Cycle:           opts.Cycle,
		ReviewStatus:    model.ReviewStatusReviewed,
		Notes:           opts.Notes,
		AllowedRootDirs: opts.AllowedRootDirs,
	})
	if err != nil {
		return TypeImportResult{}, err
	}
	importID := fmt.Sprintf("import:static-client-types:%s:%s", environment, artefact.SHA256[:12])
	entities := buildTypeEntities(environment, source.ID, artefact.ID, opts.Cycle, rows)
	if err := store.RecordImport(ctx, importID, source, artefact, map[string]any{
		"rowCount":      len(rows),
		"importedCount": len(entities),
		"sourceKind":    source.Kind,
	}); err != nil {
		return TypeImportResult{}, err
	}
	if err := upsertTypeEntities(ctx, store, entities); err != nil {
		return TypeImportResult{}, err
	}
	return TypeImportResult{Source: source, Artefact: artefact, ImportID: importID, RowsImported: len(entities)}, nil
}

func buildTypeEntities(environment model.Environment, sourceID, artefactID string, cycle *int, rows []staticTypeRow) []db.EntityFactSet {
	now := time.Now().UTC()
	seen := make(map[int]struct{}, len(rows))
	out := make([]db.EntityFactSet, 0, len(rows))
	for _, row := range rows {
		if row.TypeID <= 0 || strings.TrimSpace(row.Name) == "" {
			continue
		}
		if _, ok := seen[row.TypeID]; ok {
			continue
		}
		seen[row.TypeID] = struct{}{}
		entityType := classifyStaticType(row)
		entity := model.Entity{
			ID:          staticTypeEntityID(environment, entityType, row.TypeID),
			Slug:        slugify(fmt.Sprintf("%s-%s-%d-%s", entityType, row.Name, row.TypeID, environment)),
			Type:        entityType,
			Name:        row.Name,
			DisplayName: staticTypeDisplayName(row, entityType),
			Summary:     fmt.Sprintf("Static-client type metadata, type %d in group %d.", row.TypeID, row.GroupID),
			Environment: environment,
			Cycle:       cycle,
			UpdatedAt:   now,
		}
		out = append(out, db.EntityFactSet{
			Entity: entity,
			Facts:  facts(sourceID, environment, cycle, typeFactValues(row, entityType, artefactID)),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Entity.ID < out[j].Entity.ID
	})
	return out
}

func typeFactValues(row staticTypeRow, entityType model.EntityType, artefactID string) map[string]any {
	values := map[string]any{
		"type_id":            row.TypeID,
		"group_id":           row.GroupID,
		"description":        row.Description,
		"resolver_reason":    row.Reason,
		"static_entity_type": string(entityType),
		"source_artefact_id": artefactID,
	}
	if row.CategoryID > 0 {
		values["category_id"] = row.CategoryID
	}
	if row.CategoryName != "" {
		values["category_name"] = row.CategoryName
	}
	if row.GroupName != "" {
		values["group_name"] = row.GroupName
	}
	if row.MarketGroupID > 0 {
		values["market_group_id"] = row.MarketGroupID
	}
	if row.TypeNameID > 0 {
		values["type_name_id"] = row.TypeNameID
	}
	if row.WreckTypeID > 0 {
		values["wreck_type_id"] = row.WreckTypeID
	}
	return values
}

func upsertTypeEntities(ctx context.Context, store TypeStore, entities []db.EntityFactSet) error {
	if bulkStore, ok := store.(bulkTypeStore); ok {
		for start := 0; start < len(entities); start += bulkChunkSize {
			end := min(start+bulkChunkSize, len(entities))
			if err := bulkStore.UpsertEventDerivationBatch(ctx, entities[start:end], nil, nil); err != nil {
				return err
			}
		}
		return nil
	}
	for _, item := range entities {
		if err := store.UpsertEntityFacts(ctx, item.Entity, item.Facts); err != nil {
			return err
		}
	}
	return nil
}

func decodeStaticTypeRows(data []byte) ([]staticTypeRow, error) {
	var arrayRows []staticTypeRow
	if err := json.Unmarshal(data, &arrayRows); err == nil && arrayRows != nil {
		return arrayRows, nil
	}
	var payload staticTypePayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, err
	}
	for _, rows := range [][]staticTypeRow{payload.Candidates, payload.Data, payload.Items, payload.Rows, payload.Types} {
		if rows != nil {
			return rows, nil
		}
	}
	return nil, errors.New("static-client type input must be an array or contain a candidates/data/items/rows/types array")
}

func classifyStaticType(row staticTypeRow) model.EntityType {
	if _, ok := staticClientEnemyTypeIDSet[row.TypeID]; ok {
		return model.EntityTypeEnemy
	}
	if _, ok := staticClientEnemyGroupIDSet[row.GroupID]; ok {
		return model.EntityTypeEnemy
	}
	if _, ok := staticClientShipGroupIDs[row.GroupID]; ok {
		return model.EntityTypeShip
	}
	if _, ok := staticClientStructureGroupIDs[row.GroupID]; ok {
		return model.EntityTypeStructure
	}
	metadata := strings.ToLower(strings.Join([]string{row.CategoryName, row.GroupName}, " "))
	switch {
	case containsAny(metadata, "ship", "hull"):
		return model.EntityTypeShip
	case containsAny(metadata, "structure", "assembly", "assemblies", "deployable", "stargate", "starbase", "station", "turret", "storage", "market", "refinery"):
		return model.EntityTypeStructure
	case containsAny(metadata, "material", "materials", "resource", "resources", "commodity", "commodities", "mineral", "minerals", "gas", "ore"):
		return model.EntityTypeMaterial
	}
	name := strings.ToLower(row.Name)
	switch {
	case strings.Contains(name, "gate"), strings.Contains(name, "turret"), strings.Contains(name, "storage"),
		strings.Contains(name, "assembly"), strings.Contains(name, "refinery"), strings.Contains(name, "market"),
		strings.Contains(name, "structure"):
		return model.EntityTypeStructure
	case strings.Contains(name, "material"), strings.Contains(name, "matrix"), strings.Contains(name, "crystal"),
		strings.Contains(name, "sulfide"), strings.Contains(name, "feldspar"), strings.Contains(name, "platinum"),
		strings.Contains(name, "palladium"), strings.Contains(name, "exotronics"):
		return model.EntityTypeMaterial
	default:
		return model.EntityTypeItem
	}
}

func containsAny(value string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

func staticTypeEntityID(environment model.Environment, entityType model.EntityType, typeID int) string {
	if entityType == model.EntityTypeEnemy {
		return staticdata.EntityID(environment, typeID)
	}
	return fmt.Sprintf("%s:%s:type:%d", entityType, environment, typeID)
}

func staticTypeDisplayName(row staticTypeRow, entityType model.EntityType) string {
	if entityType == model.EntityTypeEnemy {
		return staticdata.DisplayName(row.Name)
	}
	return row.Name
}
