package staticclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/blackrelay/registry/internal/artefacts"
	"github.com/blackrelay/registry/internal/db"
	"github.com/blackrelay/registry/internal/model"
)

const (
	universeImporterName = "br-import-static-universe"
	universeSourceTitle  = "Stillness static-client universe data"
	bulkChunkSize        = 1000
)

type UniverseOptions struct {
	Environment     model.Environment
	AllowedRootDirs []string
	SourceTitle     string
	ClientBuild     string
	PatchLabel      string
	Cycle           *int
	Notes           string
}

type UniverseResult struct {
	Source                 model.Source           `json:"source"`
	Artefacts              []model.SourceArtefact `json:"artefacts"`
	ImportID               string                 `json:"importId"`
	RegionsImported        int                    `json:"regionsImported"`
	ConstellationsObserved int                    `json:"constellationsObserved"`
	SystemsImported        int                    `json:"systemsImported"`
	RoutesImported         int                    `json:"routesImported"`
}

type UniverseStore interface {
	RecordImport(ctx context.Context, importID string, source model.Source, artefact model.SourceArtefact, summary map[string]any) error
	UpsertEntityFacts(ctx context.Context, entity model.Entity, facts []db.EntityFactDraft) error
	UpsertRelations(ctx context.Context, relations []db.RelationDraft) error
}

type bulkUniverseStore interface {
	UpsertEventDerivationBatch(ctx context.Context, entities []db.EntityFactSet, relations []db.RelationDraft, killmails []model.KillmailRaw) error
}

type regionRow struct {
	ID   string
	Name string
	Raw  map[string]any
}

type constellationRow struct {
	ID       string
	Name     string
	RegionID string
	Raw      map[string]any
}

type systemRow struct {
	ID              string
	Name            string
	RegionID        string
	ConstellationID string
	Raw             map[string]any
}

type jumpRow struct {
	ID           string
	StargateID   string
	FromSystemID string
	ToSystemID   string
	JumpType     string
	Raw          map[string]any
}

func ImportUniverse(ctx context.Context, store UniverseStore, artefactStore artefacts.Store, inputDir string, opts UniverseOptions) (UniverseResult, error) {
	if store == nil {
		return UniverseResult{}, errors.New("store is required")
	}
	if artefactStore == nil {
		return UniverseResult{}, errors.New("artefact store is required")
	}
	environment := opts.Environment
	if environment == "" {
		environment = model.EnvironmentStillness
	}
	schemaDir := filepath.Join(inputDir, "fsd_binary_schema")
	regionsPath := filepath.Join(schemaDir, "regions.json")
	constellationsPath := filepath.Join(schemaDir, "constellations.json")
	systemsPath := filepath.Join(schemaDir, "systems.json")
	jumpsPath := filepath.Join(schemaDir, "jumps.json")

	regions, err := decodeRegions(regionsPath)
	if err != nil {
		return UniverseResult{}, err
	}
	constellations, err := decodeConstellations(constellationsPath)
	if err != nil {
		return UniverseResult{}, err
	}
	systems, err := decodeSystems(systemsPath)
	if err != nil {
		return UniverseResult{}, err
	}
	jumps, err := decodeJumps(jumpsPath)
	if err != nil {
		return UniverseResult{}, err
	}

	source := model.Source{
		ID:          fmt.Sprintf("source:static-client:universe:%s", environment),
		Kind:        model.SourceKindStaticClientData,
		Title:       firstNonEmpty(opts.SourceTitle, universeSourceTitle),
		Locator:     inputDir,
		Environment: environment,
		Cycle:       opts.Cycle,
		Metadata: map[string]any{
			"regionCount":        len(regions),
			"constellationCount": len(constellations),
			"systemCount":        len(systems),
			"jumpCount":          len(jumps),
			"sourceDirectory":    inputDir,
		},
		CreatedAt: time.Now().UTC(),
	}

	artefactSpecs := []struct {
		path string
		kind string
		rows int
	}{
		{path: regionsPath, kind: "static_client_regions", rows: len(regions)},
		{path: constellationsPath, kind: "static_client_constellations", rows: len(constellations)},
		{path: systemsPath, kind: "static_client_systems", rows: len(systems)},
		{path: jumpsPath, kind: "static_client_jumps", rows: len(jumps)},
	}
	artefactsOut := make([]model.SourceArtefact, 0, len(artefactSpecs))
	primaryImportID := ""
	for _, spec := range artefactSpecs {
		artefact, err := artefactStore.RegisterFile(ctx, spec.path, artefacts.RegisterMeta{
			SourceID:        source.ID,
			SourceKind:      model.SourceKindStaticClientData,
			Kind:            spec.kind,
			ArtefactKind:    spec.kind,
			Environment:     environment,
			ContentType:     "application/json",
			RowCount:        int64(spec.rows),
			ImporterName:    universeImporterName,
			ClientBuild:     opts.ClientBuild,
			PatchLabel:      opts.PatchLabel,
			Cycle:           opts.Cycle,
			ReviewStatus:    model.ReviewStatusReviewed,
			Notes:           opts.Notes,
			AllowedRootDirs: opts.AllowedRootDirs,
		})
		if err != nil {
			return UniverseResult{}, err
		}
		importID := fmt.Sprintf("import:static-universe:%s:%s:%s", environment, spec.kind, artefact.SHA256[:12])
		if spec.kind == "static_client_systems" {
			primaryImportID = importID
		}
		if err := store.RecordImport(ctx, importID, source, artefact, map[string]any{
			"artefactKind":       spec.kind,
			"rowCount":           spec.rows,
			"regionCount":        len(regions),
			"constellationCount": len(constellations),
			"systemCount":        len(systems),
			"jumpCount":          len(jumps),
			"sourceKind":         source.Kind,
		}); err != nil {
			return UniverseResult{}, err
		}
		artefactsOut = append(artefactsOut, artefact)
	}

	entities, relations := buildUniverseEntities(environment, source.ID, opts.Cycle, regions, constellations, systems, jumps)
	if err := upsertUniverse(ctx, store, entities, relations); err != nil {
		return UniverseResult{}, err
	}
	return UniverseResult{
		Source:                 source,
		Artefacts:              artefactsOut,
		ImportID:               primaryImportID,
		RegionsImported:        len(regions),
		ConstellationsObserved: len(constellations),
		SystemsImported:        len(systems),
		RoutesImported:         len(jumps),
	}, nil
}

func buildUniverseEntities(environment model.Environment, sourceID string, cycle *int, regions map[string]regionRow, constellations map[string]constellationRow, systems map[string]systemRow, jumps []jumpRow) ([]db.EntityFactSet, []db.RelationDraft) {
	now := time.Now().UTC()
	entities := make([]db.EntityFactSet, 0, len(regions)+len(constellations)+len(systems)+len(jumps))
	relations := make([]db.RelationDraft, 0, len(constellations)+len(systems)*2+len(jumps)*3)
	for _, region := range sortedRegions(regions) {
		entity := model.Entity{
			ID:          regionEntityID(environment, region.ID),
			Slug:        slugify(fmt.Sprintf("region-%s-%s", region.ID, environment)),
			Type:        model.EntityTypeRegion,
			Name:        region.Name,
			DisplayName: region.Name,
			Summary:     "Static-client region metadata.",
			Environment: environment,
			Cycle:       cycle,
			UpdatedAt:   now,
		}
		entities = append(entities, db.EntityFactSet{Entity: entity, Facts: facts(sourceID, environment, cycle, map[string]any{
			"region_id": region.ID,
			"x":         vectorCoordinate(region.Raw["center"], 0),
			"y":         vectorCoordinate(region.Raw["center"], 1),
			"z":         vectorCoordinate(region.Raw["center"], 2),
		})})
	}
	for _, constellation := range sortedConstellations(constellations) {
		region := regions[constellation.RegionID]
		entity := model.Entity{
			ID:          constellationEntityID(environment, constellation.ID),
			Slug:        slugify(fmt.Sprintf("constellation-%s-%s", constellation.ID, environment)),
			Type:        model.EntityTypeConstellation,
			Name:        constellation.Name,
			DisplayName: constellation.Name,
			Summary:     "Static-client constellation metadata.",
			Environment: environment,
			Cycle:       cycle,
			UpdatedAt:   now,
		}
		entities = append(entities, db.EntityFactSet{Entity: entity, Facts: facts(sourceID, environment, cycle, map[string]any{
			"constellation_id": constellation.ID,
			"region_id":        constellation.RegionID,
			"region_name":      region.Name,
			"x":                vectorCoordinate(constellation.Raw["center"], 0),
			"y":                vectorCoordinate(constellation.Raw["center"], 1),
			"z":                vectorCoordinate(constellation.Raw["center"], 2),
		})})
		if constellation.RegionID != "" {
			relations = append(relations, relation(entity.ID, "located_in", regionEntityID(environment, constellation.RegionID), sourceID, environment))
		}
	}
	for _, system := range sortedSystems(systems) {
		region := regions[system.RegionID]
		constellation := constellations[system.ConstellationID]
		entity := model.Entity{
			ID:          systemEntityID(environment, system.ID),
			Slug:        slugify(fmt.Sprintf("system-%s-%s", system.ID, environment)),
			Type:        model.EntityTypeSystem,
			Name:        system.Name,
			DisplayName: system.Name,
			Summary:     "Static-client solar system metadata.",
			Environment: environment,
			Cycle:       cycle,
			UpdatedAt:   now,
		}
		entities = append(entities, db.EntityFactSet{Entity: entity, Facts: facts(sourceID, environment, cycle, map[string]any{
			"system_id":            system.ID,
			"solar_system_id":      system.ID,
			"region_id":            system.RegionID,
			"region_name":          region.Name,
			"constellation_id":     system.ConstellationID,
			"constellation_name":   constellation.Name,
			"x":                    vectorCoordinate(system.Raw["center"], 0),
			"y":                    vectorCoordinate(system.Raw["center"], 1),
			"z":                    vectorCoordinate(system.Raw["center"], 2),
			"security_class":       stringField(system.Raw, "securityClass"),
			"security_status":      stringField(system.Raw, "securityStatus"),
			"sun_type_id":          stringField(system.Raw, "sunTypeID"),
			"wormhole_class_id":    stringField(system.Raw, "wormholeClassID"),
			"planet_count_by_type": nonEmptyAny(system.Raw["planetCountByType"]),
			"planet_item_ids":      stringField(system.Raw, "planetItemIDs"),
		})})
		if system.ConstellationID != "" {
			relations = append(relations, relation(entity.ID, "located_in", constellationEntityID(environment, system.ConstellationID), sourceID, environment))
		}
		if system.RegionID != "" {
			relations = append(relations, relation(entity.ID, "member_of_region", regionEntityID(environment, system.RegionID), sourceID, environment))
		}
	}
	for _, jump := range jumps {
		from := systems[jump.FromSystemID]
		to := systems[jump.ToSystemID]
		routeName := fmt.Sprintf("%s to %s", firstNonEmpty(from.Name, jump.FromSystemID), firstNonEmpty(to.Name, jump.ToSystemID))
		routeID := routeEntityID(environment, jump.FromSystemID, jump.ToSystemID)
		entity := model.Entity{
			ID:          routeID,
			Slug:        slugify(fmt.Sprintf("route-%s-to-%s-%s", jump.FromSystemID, jump.ToSystemID, environment)),
			Type:        model.EntityTypeRoute,
			Name:        routeName,
			DisplayName: routeName,
			Summary:     "Static-client jump connection.",
			Environment: environment,
			Cycle:       cycle,
			UpdatedAt:   now,
		}
		entities = append(entities, db.EntityFactSet{Entity: entity, Facts: facts(sourceID, environment, cycle, map[string]any{
			"jump_id":        jump.ID,
			"stargate_id":    jump.StargateID,
			"from_system_id": jump.FromSystemID,
			"to_system_id":   jump.ToSystemID,
			"jump_type":      jump.JumpType,
		})})
		fromID := systemEntityID(environment, jump.FromSystemID)
		toID := systemEntityID(environment, jump.ToSystemID)
		relations = append(relations,
			relation(fromID, "links_to", toID, sourceID, environment),
			relation(routeID, "observed_between", fromID, sourceID, environment),
			relation(routeID, "observed_between", toID, sourceID, environment),
		)
	}
	return entities, relations
}

func upsertUniverse(ctx context.Context, store UniverseStore, entities []db.EntityFactSet, relations []db.RelationDraft) error {
	if bulkStore, ok := store.(bulkUniverseStore); ok {
		for start := 0; start < len(entities); start += bulkChunkSize {
			endEntities := min(start+bulkChunkSize, len(entities))
			if err := bulkStore.UpsertEventDerivationBatch(ctx, entities[start:endEntities], nil, nil); err != nil {
				return err
			}
		}
		for start := 0; start < len(relations); start += bulkChunkSize {
			endRelations := min(start+bulkChunkSize, len(relations))
			if err := bulkStore.UpsertEventDerivationBatch(ctx, nil, relations[start:endRelations], nil); err != nil {
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
	return store.UpsertRelations(ctx, relations)
}

func decodeRegions(path string) (map[string]regionRow, error) {
	rows, err := decodeObjectMap(path)
	if err != nil {
		return nil, err
	}
	out := make(map[string]regionRow, len(rows))
	for key, raw := range rows {
		id := firstNonEmpty(stringField(raw, "regionID", "regionId", "id"), key)
		name := firstNonEmpty(stringField(raw, "name", "regionName"), "Region "+id)
		out[id] = regionRow{ID: id, Name: name, Raw: raw}
	}
	return out, nil
}

func decodeConstellations(path string) (map[string]constellationRow, error) {
	rows, err := decodeObjectMap(path)
	if err != nil {
		return nil, err
	}
	out := make(map[string]constellationRow, len(rows))
	for key, raw := range rows {
		id := firstNonEmpty(stringField(raw, "constellationID", "constellationId", "id"), key)
		name := firstNonEmpty(stringField(raw, "name", "constellationName"), "Constellation "+id)
		out[id] = constellationRow{
			ID:       id,
			Name:     name,
			RegionID: stringField(raw, "regionID", "regionId"),
			Raw:      raw,
		}
	}
	return out, nil
}

func decodeSystems(path string) (map[string]systemRow, error) {
	rows, err := decodeObjectMap(path)
	if err != nil {
		return nil, err
	}
	out := make(map[string]systemRow, len(rows))
	for key, raw := range rows {
		id := firstNonEmpty(stringField(raw, "solarSystemID", "solarSystemId", "systemID", "systemId", "id"), key)
		name := firstNonEmpty(stringField(raw, "name", "systemName"), "System "+id)
		out[id] = systemRow{
			ID:              id,
			Name:            name,
			RegionID:        stringField(raw, "regionID", "regionId"),
			ConstellationID: stringField(raw, "constellationID", "constellationId"),
			Raw:             raw,
		}
	}
	return out, nil
}

func ReadUniverseSystemIDs(inputDir string) ([]string, error) {
	systems, err := decodeSystems(filepath.Join(inputDir, "fsd_binary_schema", "systems.json"))
	if err != nil {
		return nil, err
	}
	rows := sortedSystems(systems)
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		if row.ID != "" {
			out = append(out, row.ID)
		}
	}
	return out, nil
}

func decodeObjectMap(path string) (map[string]map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	out := make(map[string]map[string]any, len(raw))
	for key, value := range raw {
		row, ok := value.(map[string]any)
		if !ok {
			continue
		}
		out[key] = row
	}
	return out, nil
}

func decodeJumps(path string) ([]jumpRow, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	list, ok := raw["Type: FSD List"].([]any)
	if !ok {
		return nil, errors.New("jumps.json must contain Type: FSD List")
	}
	out := make([]jumpRow, 0, len(list))
	for _, item := range list {
		row := unwrapFSDObject(item)
		fromID := stringField(row, "fromSystemID", "fromSystemId", "from")
		toID := stringField(row, "toSystemID", "toSystemId", "to")
		if fromID == "" || toID == "" {
			continue
		}
		out = append(out, jumpRow{
			ID:           firstNonEmpty(stringField(row, "jumpID", "jumpId", "id"), fromID+":"+toID),
			StargateID:   stringField(row, "stargateID", "stargateId"),
			FromSystemID: fromID,
			ToSystemID:   toID,
			JumpType:     stringField(row, "jumpType"),
			Raw:          row,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].FromSystemID == out[j].FromSystemID {
			return out[i].ToSystemID < out[j].ToSystemID
		}
		return out[i].FromSystemID < out[j].FromSystemID
	})
	return out, nil
}

func unwrapFSDObject(value any) map[string]any {
	row, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	if _, ok := row["fromSystemID"]; ok {
		return row
	}
	if _, ok := row["fromSystemId"]; ok {
		return row
	}
	for _, value := range row {
		if nested, ok := value.(map[string]any); ok {
			return nested
		}
	}
	return row
}

func facts(sourceID string, environment model.Environment, cycle *int, values map[string]any) []db.EntityFactDraft {
	out := make([]db.EntityFactDraft, 0, len(values))
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value := values[key]
		if value == nil || value == "" {
			continue
		}
		out = append(out, db.EntityFactDraft{
			Key:          key,
			Value:        value,
			SourceID:     sourceID,
			Confidence:   model.ConfidenceVerified,
			Environment:  environment,
			Cycle:        cycle,
			ReviewStatus: model.ReviewStatusReviewed,
		})
	}
	return out
}

func relation(subject, predicate, object, sourceID string, environment model.Environment) db.RelationDraft {
	return db.RelationDraft{
		SubjectEntityID: subject,
		Predicate:       predicate,
		ObjectEntityID:  object,
		SourceID:        sourceID,
		Confidence:      model.ConfidenceVerified,
		Environment:     environment,
	}
}

func sortedRegions(rows map[string]regionRow) []regionRow {
	out := make([]regionRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func sortedSystems(rows map[string]systemRow) []systemRow {
	out := make([]systemRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func sortedConstellations(rows map[string]constellationRow) []constellationRow {
	out := make([]constellationRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func stringField(row map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := row[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case string:
			if strings.TrimSpace(typed) != "" {
				return typed
			}
		case float64:
			return fmt.Sprintf("%.0f", typed)
		case int:
			return fmt.Sprint(typed)
		case int64:
			return fmt.Sprint(typed)
		}
	}
	return ""
}

func vectorCoordinate(value any, index int) string {
	list, ok := value.([]any)
	if !ok {
		return ""
	}
	position := index + 1
	if position >= len(list) {
		return ""
	}
	switch typed := list[position].(type) {
	case string:
		return typed
	case float64:
		return fmt.Sprint(typed)
	default:
		return ""
	}
}

func nonEmptyAny(value any) any {
	switch typed := value.(type) {
	case nil:
		return nil
	case string:
		if typed == "" {
			return nil
		}
		return typed
	case map[string]any:
		if len(typed) == 0 {
			return nil
		}
		return typed
	default:
		return value
	}
}

func systemEntityID(environment model.Environment, id string) string {
	return fmt.Sprintf("system:%s:%s", environment, id)
}

func regionEntityID(environment model.Environment, id string) string {
	return fmt.Sprintf("region:%s:%s", environment, id)
}

func constellationEntityID(environment model.Environment, id string) string {
	return fmt.Sprintf("constellation:%s:%s", environment, id)
}

func routeEntityID(environment model.Environment, fromID, toID string) string {
	return fmt.Sprintf("route:%s:%s:%s", environment, fromID, toID)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

var slugPattern = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(value string) string {
	slug := strings.ToLower(value)
	slug = slugPattern.ReplaceAllString(slug, "-")
	return strings.Trim(slug, "-")
}
