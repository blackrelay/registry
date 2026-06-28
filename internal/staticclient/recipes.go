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
)

const recipeImporterName = "br-import-static-client-recipes"

type RecipeImportOptions struct {
	Environment     model.Environment
	AllowedRootDirs []string
	SourceTitle     string
	ClientBuild     string
	PatchLabel      string
	Cycle           *int
	Notes           string
}

type RecipeImportResult struct {
	Source       model.Source         `json:"source"`
	Artefact     model.SourceArtefact `json:"artefact"`
	ImportID     string               `json:"importId"`
	RowsImported int                  `json:"rowsImported"`
}

type RecipeStore interface {
	RecordImport(ctx context.Context, importID string, source model.Source, artefact model.SourceArtefact, summary map[string]any) error
	UpsertEntityFacts(ctx context.Context, entity model.Entity, facts []db.EntityFactDraft) error
	UpsertRelations(ctx context.Context, relations []db.RelationDraft) error
}

type staticRecipePayload struct {
	SchemaVersion string            `json:"schemaVersion,omitempty"`
	Environment   model.Environment `json:"environment,omitempty"`
	Recipes       []staticRecipeRow `json:"recipes"`
	Data          []staticRecipeRow `json:"data"`
	Items         []staticRecipeRow `json:"items"`
	Rows          []staticRecipeRow `json:"rows"`
}

type staticRecipeRow struct {
	RecipeID        string              `json:"recipeId"`
	Name            string              `json:"name"`
	OutputTypeID    int                 `json:"outputTypeId"`
	OutputQuantity  int                 `json:"outputQuantity"`
	BlueprintTypeID int                 `json:"blueprintTypeId,omitempty"`
	FacilityTypeID  int                 `json:"facilityTypeId,omitempty"`
	Inputs          []staticRecipeInput `json:"inputs"`
	SourceContext   string              `json:"sourceContext,omitempty"`
}

type staticRecipeInput struct {
	TypeID   int `json:"typeId"`
	Quantity int `json:"quantity"`
}

func ImportRecipes(ctx context.Context, store RecipeStore, artefactStore artefacts.Store, inputPath string, opts RecipeImportOptions) (RecipeImportResult, error) {
	if store == nil {
		return RecipeImportResult{}, errors.New("store is required")
	}
	if artefactStore == nil {
		return RecipeImportResult{}, errors.New("artefact store is required")
	}
	data, err := os.ReadFile(inputPath)
	if err != nil {
		return RecipeImportResult{}, err
	}
	rows, err := decodeStaticRecipeRows(data)
	if err != nil {
		return RecipeImportResult{}, err
	}
	if err := validateStaticRecipeRows(rows); err != nil {
		return RecipeImportResult{}, err
	}
	rows = normaliseRecipeRows(rows)
	environment := opts.Environment
	if environment == "" {
		environment = model.EnvironmentStillness
	}
	source := model.Source{
		ID:          fmt.Sprintf("source:static-client:recipes:%s", environment),
		Kind:        model.SourceKindStaticClientData,
		Title:       firstNonEmpty(opts.SourceTitle, "Static-client recipe metadata"),
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
		Kind:            "static_client_recipes",
		ArtefactKind:    "static_client_recipes",
		Environment:     environment,
		ContentType:     "application/json",
		RowCount:        int64(len(rows)),
		ImporterName:    recipeImporterName,
		ClientBuild:     opts.ClientBuild,
		PatchLabel:      opts.PatchLabel,
		Cycle:           opts.Cycle,
		ReviewStatus:    model.ReviewStatusReviewed,
		Notes:           opts.Notes,
		AllowedRootDirs: opts.AllowedRootDirs,
	})
	if err != nil {
		return RecipeImportResult{}, err
	}
	importID := fmt.Sprintf("import:static-client-recipes:%s:%s", environment, artefact.SHA256[:12])
	entities, relations := buildRecipeEntities(environment, source.ID, artefact.ID, opts.Cycle, rows)
	if err := store.RecordImport(ctx, importID, source, artefact, map[string]any{
		"rowCount":      len(rows),
		"importedCount": len(rows),
		"sourceKind":    source.Kind,
	}); err != nil {
		return RecipeImportResult{}, err
	}
	if err := upsertTypeEntities(ctx, store, entities); err != nil {
		return RecipeImportResult{}, err
	}
	if err := store.UpsertRelations(ctx, relations); err != nil {
		return RecipeImportResult{}, err
	}
	return RecipeImportResult{Source: source, Artefact: artefact, ImportID: importID, RowsImported: len(rows)}, nil
}

func decodeStaticRecipeRows(data []byte) ([]staticRecipeRow, error) {
	var arrayRows []staticRecipeRow
	if err := json.Unmarshal(data, &arrayRows); err == nil && arrayRows != nil {
		return arrayRows, nil
	}
	var payload staticRecipePayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, err
	}
	for _, rows := range [][]staticRecipeRow{payload.Recipes, payload.Data, payload.Items, payload.Rows} {
		if rows != nil {
			return rows, nil
		}
	}
	return nil, errors.New("static-client recipe input must be an array or contain a recipes/data/items/rows array")
}

func validateStaticRecipeRows(rows []staticRecipeRow) error {
	if len(rows) == 0 {
		return errors.New("static-client recipe input contains no rows")
	}
	for index, row := range rows {
		label := row.RecipeID
		if strings.TrimSpace(label) == "" {
			label = fmt.Sprintf("row %d", index)
		}
		if strings.TrimSpace(row.Name) == "" {
			return fmt.Errorf("static-client recipe %s name is required", label)
		}
		if row.OutputTypeID <= 0 {
			return fmt.Errorf("static-client recipe %s outputTypeId must be positive", label)
		}
		if row.OutputQuantity <= 0 {
			return fmt.Errorf("static-client recipe %s outputQuantity must be positive", label)
		}
		if row.BlueprintTypeID < 0 {
			return fmt.Errorf("static-client recipe %s blueprintTypeId must not be negative", label)
		}
		if row.FacilityTypeID < 0 {
			return fmt.Errorf("static-client recipe %s facilityTypeId must not be negative", label)
		}
		if len(row.Inputs) == 0 {
			return fmt.Errorf("static-client recipe %s must include at least one input", label)
		}
		for inputIndex, input := range row.Inputs {
			if input.TypeID <= 0 {
				return fmt.Errorf("static-client recipe %s input %d typeId must be positive", label, inputIndex)
			}
			if input.Quantity <= 0 {
				return fmt.Errorf("static-client recipe %s input %d quantity must be positive", label, inputIndex)
			}
		}
	}
	return nil
}

func normaliseRecipeRows(rows []staticRecipeRow) []staticRecipeRow {
	out := make([]staticRecipeRow, 0, len(rows))
	seen := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		if row.OutputTypeID <= 0 || strings.TrimSpace(row.Name) == "" {
			continue
		}
		row.Name = repairStaticClientName(strings.TrimSpace(row.Name))
		row.RecipeID = strings.TrimSpace(row.RecipeID)
		if row.RecipeID == "" {
			row.RecipeID = fmt.Sprintf("type:%d", row.OutputTypeID)
		}
		if _, ok := seen[row.RecipeID]; ok {
			continue
		}
		seen[row.RecipeID] = struct{}{}
		inputs := make([]staticRecipeInput, 0, len(row.Inputs))
		for _, input := range row.Inputs {
			if input.TypeID <= 0 {
				continue
			}
			if input.Quantity <= 0 {
				input.Quantity = 1
			}
			inputs = append(inputs, input)
		}
		sort.Slice(inputs, func(i, j int) bool { return inputs[i].TypeID < inputs[j].TypeID })
		row.Inputs = inputs
		if row.OutputQuantity <= 0 {
			row.OutputQuantity = 1
		}
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].RecipeID < out[j].RecipeID
	})
	return out
}

func buildRecipeEntities(environment model.Environment, sourceID, artefactID string, cycle *int, rows []staticRecipeRow) ([]db.EntityFactSet, []db.RelationDraft) {
	now := time.Now().UTC()
	entityMap := make(map[string]db.EntityFactSet)
	var order []string
	var relations []db.RelationDraft
	add := func(entity model.Entity, values map[string]any) {
		if _, ok := entityMap[entity.ID]; !ok {
			order = append(order, entity.ID)
		}
		entityMap[entity.ID] = db.EntityFactSet{
			Entity: entity,
			Facts:  facts(sourceID, environment, cycle, values),
		}
	}
	addRelation := func(subject, predicate, object string) {
		if subject == "" || object == "" {
			return
		}
		relations = append(relations, db.RelationDraft{
			SubjectEntityID: subject,
			Predicate:       predicate,
			ObjectEntityID:  object,
			SourceID:        sourceID,
			Confidence:      model.ConfidenceProbable,
			Environment:     environment,
		})
	}
	for _, row := range rows {
		recipeID := recipeEntityID(environment, row.RecipeID)
		inputFacts := recipeInputFacts(row.Inputs)
		add(model.Entity{
			ID:          recipeID,
			Slug:        slugify(fmt.Sprintf("recipe-%s-%s", row.RecipeID, environment)),
			Type:        model.EntityTypeRecipe,
			Name:        row.Name + " recipe",
			DisplayName: row.Name + " recipe",
			Summary:     fmt.Sprintf("Static-client recipe metadata for output type %d.", row.OutputTypeID),
			Environment: environment,
			Cycle:       cycle,
			UpdatedAt:   now,
		}, map[string]any{
			"recipe_id":          row.RecipeID,
			"output":             map[string]any{"typeId": row.OutputTypeID, "quantity": row.OutputQuantity},
			"output_type_id":     row.OutputTypeID,
			"output_quantity":    row.OutputQuantity,
			"inputs":             inputFacts,
			"blueprint_type_id":  row.BlueprintTypeID,
			"facility_type_id":   row.FacilityTypeID,
			"source_artefact_id": artefactID,
			"source_context":     repairStaticClientText(strings.TrimSpace(row.SourceContext)),
			"static_entity_type": string(model.EntityTypeRecipe),
		})

		outputID := staticTypeEntityID(environment, model.EntityTypeItem, row.OutputTypeID)
		add(itemPlaceholder(environment, row.Name, row.OutputTypeID, now, cycle), map[string]any{
			"type_id":            row.OutputTypeID,
			"source_artefact_id": artefactID,
			"static_entity_type": string(model.EntityTypeItem),
		})
		addRelation(recipeID, "produces", outputID)

		if row.BlueprintTypeID > 0 {
			blueprintID := blueprintEntityID(environment, row.BlueprintTypeID)
			add(model.Entity{
				ID:          blueprintID,
				Slug:        slugify(fmt.Sprintf("blueprint-%s-%d-%s", row.Name, row.BlueprintTypeID, environment)),
				Type:        model.EntityTypeBlueprint,
				Name:        row.Name + " blueprint",
				DisplayName: row.Name + " blueprint",
				Summary:     fmt.Sprintf("Static-client blueprint metadata, type %d.", row.BlueprintTypeID),
				Environment: environment,
				Cycle:       cycle,
				UpdatedAt:   now,
			}, map[string]any{
				"type_id":            row.BlueprintTypeID,
				"source_artefact_id": artefactID,
				"static_entity_type": string(model.EntityTypeBlueprint),
			})
			addRelation(recipeID, "uses_blueprint", blueprintID)
		}
		if row.FacilityTypeID > 0 {
			facilityID := staticTypeEntityID(environment, model.EntityTypeStructure, row.FacilityTypeID)
			add(model.Entity{
				ID:          facilityID,
				Slug:        slugify(fmt.Sprintf("structure-facility-%d-%s", row.FacilityTypeID, environment)),
				Type:        model.EntityTypeStructure,
				Name:        fmt.Sprintf("Facility type %d", row.FacilityTypeID),
				DisplayName: fmt.Sprintf("Facility type %d", row.FacilityTypeID),
				Summary:     fmt.Sprintf("Static-client recipe facility placeholder, type %d.", row.FacilityTypeID),
				Environment: environment,
				Cycle:       cycle,
				UpdatedAt:   now,
			}, map[string]any{
				"type_id":            row.FacilityTypeID,
				"source_artefact_id": artefactID,
				"static_entity_type": string(model.EntityTypeStructure),
			})
			addRelation(recipeID, "uses_facility", facilityID)
		}
		for _, input := range row.Inputs {
			inputID := staticTypeEntityID(environment, model.EntityTypeItem, input.TypeID)
			add(itemPlaceholder(environment, fmt.Sprintf("Input type %d", input.TypeID), input.TypeID, now, cycle), map[string]any{
				"type_id":            input.TypeID,
				"source_artefact_id": artefactID,
				"static_entity_type": string(model.EntityTypeItem),
				"recipe_quantity":    input.Quantity,
			})
			addRelation(recipeID, "requires_input", inputID)
		}
	}
	sort.Strings(order)
	out := make([]db.EntityFactSet, 0, len(order))
	for _, id := range order {
		out = append(out, entityMap[id])
	}
	return out, dedupeRecipeRelations(relations)
}

func recipeInputFacts(inputs []staticRecipeInput) []map[string]any {
	out := make([]map[string]any, 0, len(inputs))
	for _, input := range inputs {
		out = append(out, map[string]any{
			"typeId":   input.TypeID,
			"quantity": input.Quantity,
		})
	}
	return out
}

func recipeEntityID(environment model.Environment, recipeID string) string {
	return fmt.Sprintf("recipe:%s:%s", environment, slugify(recipeID))
}

func blueprintEntityID(environment model.Environment, typeID int) string {
	return fmt.Sprintf("blueprint:%s:type:%d", environment, typeID)
}

func itemPlaceholder(environment model.Environment, name string, typeID int, now time.Time, cycle *int) model.Entity {
	return model.Entity{
		ID:          staticTypeEntityID(environment, model.EntityTypeItem, typeID),
		Slug:        slugify(fmt.Sprintf("item-%s-%d-%s", name, typeID, environment)),
		Type:        model.EntityTypeItem,
		Name:        name,
		DisplayName: name,
		Summary:     fmt.Sprintf("Static-client type placeholder from recipe metadata, type %d.", typeID),
		Environment: environment,
		Cycle:       cycle,
		UpdatedAt:   now,
	}
}

func dedupeRecipeRelations(items []db.RelationDraft) []db.RelationDraft {
	out := make([]db.RelationDraft, 0, len(items))
	seen := make(map[string]int, len(items))
	for _, item := range items {
		id := db.RelationID(item)
		if index, ok := seen[id]; ok {
			out[index] = item
			continue
		}
		seen[id] = len(out)
		out = append(out, item)
	}
	return out
}
