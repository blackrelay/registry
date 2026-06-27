package staticclient

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/blackrelay/registry/internal/model"
)

const staticProductionDecodeSchema = "registry.static-client-production-decode.v1"

type StaticProductionDecodeOptions struct {
	ClientRoot  string
	OutputPath  string
	Environment model.Environment
	ClientBuild string
	PatchLabel  string
	Now         func() time.Time
}

type StaticProductionDecodeResult struct {
	SchemaVersion        string                                `json:"schemaVersion"`
	Environment          model.Environment                     `json:"environment"`
	GeneratedAt          time.Time                             `json:"generatedAt"`
	ClientBuild          string                                `json:"clientBuild,omitempty"`
	PatchLabel           string                                `json:"patchLabel,omitempty"`
	DecoderStatus        string                                `json:"decoderStatus"`
	OutputPath           string                                `json:"outputPath,omitempty"`
	RowCount             int                                   `json:"rowCount"`
	Resources            []StaticResourceEvidence              `json:"resources"`
	Blueprints           []StaticBlueprintDecodedRow           `json:"blueprints,omitempty"`
	Recipes              []StaticProductionRecipeDecodedRow    `json:"recipes,omitempty"`
	MaterialRequirements []StaticMaterialRequirementDecodedRow `json:"materialRequirements,omitempty"`
	Warnings             []string                              `json:"warnings,omitempty"`
}

type StaticQuantityDecodedRow struct {
	TypeID   int    `json:"typeId"`
	Name     string `json:"name,omitempty"`
	Quantity int    `json:"quantity"`
}

type StaticMaterialRequirementDecodedRow struct {
	TypeID      int                        `json:"typeId"`
	Name        string                     `json:"name,omitempty"`
	Materials   []StaticQuantityDecodedRow `json:"materials"`
	OffsetBytes int64                      `json:"offsetBytes"`
	Basis       string                     `json:"basis"`
}

type StaticBlueprintDecodedRow struct {
	BlueprintID    int                        `json:"blueprintId,omitempty"`
	RowKey         string                     `json:"rowKey"`
	PrimaryTypeID  int                        `json:"primaryTypeId"`
	PrimaryName    string                     `json:"primaryName,omitempty"`
	RunTimeSeconds int                        `json:"runTimeSeconds"`
	Inputs         []StaticQuantityDecodedRow `json:"inputs,omitempty"`
	Outputs        []StaticQuantityDecodedRow `json:"outputs,omitempty"`
	OffsetBytes    int64                      `json:"offsetBytes"`
	Basis          string                     `json:"basis"`
}

type StaticProductionRecipeDecodedRow struct {
	RecipeID       string                     `json:"recipeId"`
	BlueprintID    int                        `json:"blueprintId,omitempty"`
	Output         StaticQuantityDecodedRow   `json:"output"`
	Inputs         []StaticQuantityDecodedRow `json:"inputs"`
	RunTimeSeconds int                        `json:"runTimeSeconds"`
	OffsetBytes    int64                      `json:"offsetBytes"`
	Basis          string                     `json:"basis"`
}

type fsdDictionaryEntry struct {
	Key         int
	ValueRel    int64
	ValueAbs    int64
	BucketIndex int
	EntryOffset int64
}

func DecodeStaticClientProductionFiles(ctx context.Context, opts StaticProductionDecodeOptions) (StaticProductionDecodeResult, error) {
	if strings.TrimSpace(opts.ClientRoot) == "" {
		return StaticProductionDecodeResult{}, errors.New("client root is required")
	}
	environment := opts.Environment
	if environment == "" {
		environment = model.EnvironmentStillness
	}
	now := time.Now().UTC()
	if opts.Now != nil {
		now = opts.Now().UTC()
	}
	typeNames, typeResources, err := staticClientTypeNameMap(ctx, opts.ClientRoot)
	if err != nil {
		return StaticProductionDecodeResult{}, err
	}
	discoveries, err := DiscoverStaticClientResources(opts.ClientRoot)
	if err != nil {
		return StaticProductionDecodeResult{}, err
	}
	resources := append([]StaticResourceEvidence{}, typeResources...)
	var blueprints []StaticBlueprintDecodedRow
	var materials []StaticMaterialRequirementDecodedRow
	var warnings []string
	for _, item := range discoveries {
		if !isProductionResourceKind(item.Kind) {
			continue
		}
		resources = append(resources, item.Evidence)
		data, err := os.ReadFile(item.Evidence.Path)
		if err != nil {
			return StaticProductionDecodeResult{}, err
		}
		resourcePath := strings.ToLower(item.ResourcePath)
		switch {
		case strings.Contains(resourcePath, "typematerial") || strings.Contains(resourcePath, "materialrequirements"):
			materials = append(materials, DecodeFSDBinaryMaterialRequirementRows(data, typeNames)...)
		case strings.Contains(resourcePath, "blueprint"):
			blueprints = append(blueprints, DecodeFSDBinaryBlueprintRows(ctx, data, typeNames)...)
		default:
			warnings = append(warnings, fmt.Sprintf("production resource %s is hashed but has no reviewed native row decoder", item.ResourcePath))
		}
	}
	sortBlueprintRows(blueprints)
	sortMaterialRequirementRows(materials)
	recipes := buildRecipeCandidatesFromBlueprints(blueprints)
	rowCount := len(blueprints) + len(recipes) + len(materials)
	if rowCount == 0 {
		return StaticProductionDecodeResult{}, errors.New("native production decoder found no validated rows")
	}
	result := StaticProductionDecodeResult{
		SchemaVersion:        staticProductionDecodeSchema,
		Environment:          environment,
		GeneratedAt:          now,
		ClientBuild:          opts.ClientBuild,
		PatchLabel:           opts.PatchLabel,
		DecoderStatus:        "native_candidate_production_rows",
		OutputPath:           opts.OutputPath,
		RowCount:             rowCount,
		Resources:            resources,
		Blueprints:           blueprints,
		Recipes:              recipes,
		MaterialRequirements: materials,
		Warnings:             warnings,
	}
	if strings.TrimSpace(opts.OutputPath) == "" {
		return result, nil
	}
	if err := os.MkdirAll(filepath.Dir(opts.OutputPath), 0o755); err != nil {
		return StaticProductionDecodeResult{}, err
	}
	file, err := os.Create(opts.OutputPath)
	if err != nil {
		return StaticProductionDecodeResult{}, err
	}
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	encodeErr := encoder.Encode(result)
	closeErr := file.Close()
	if encodeErr != nil {
		return StaticProductionDecodeResult{}, encodeErr
	}
	if closeErr != nil {
		return StaticProductionDecodeResult{}, closeErr
	}
	return result, nil
}

func DecodeFSDBinaryMaterialRequirementRows(data []byte, typeNames map[int]string) []StaticMaterialRequirementDecodedRow {
	if len(typeNames) == 0 {
		return nil
	}
	entries := decodeFSDBinaryDictionaryEntries(data)
	rows := make([]StaticMaterialRequirementDecodedRow, 0, len(entries))
	for _, entry := range entries {
		name := strings.TrimSpace(typeNames[entry.Key])
		if entry.Key <= 0 || name == "" {
			continue
		}
		materials, ok := decodeStaticQuantityList(data, entry.ValueAbs, typeNames)
		if !ok || len(materials) == 0 {
			continue
		}
		rows = append(rows, StaticMaterialRequirementDecodedRow{
			TypeID:      entry.Key,
			Name:        name,
			Materials:   materials,
			OffsetBytes: entry.ValueAbs,
			Basis:       "fsdbinary dictionary row: key type id with materialTypeID/quantity list",
		})
	}
	sortMaterialRequirementRows(rows)
	return rows
}

func DecodeFSDBinaryBlueprintRows(ctx context.Context, data []byte, typeNames map[int]string) []StaticBlueprintDecodedRow {
	if len(typeNames) == 0 || len(data) < 64 {
		return nil
	}
	entries := decodeFSDBinaryDictionaryEntries(data)
	blueprintIDByOffset := make(map[int64]int, len(entries))
	for _, entry := range entries {
		if entry.Key > 0 && entry.Key != 920 {
			blueprintIDByOffset[entry.ValueAbs] = entry.Key
		}
	}
	headerOffsets := blueprintHeaderOffsets(ctx, data, typeNames)
	rows := make([]StaticBlueprintDecodedRow, 0, len(headerOffsets))
	seen := make(map[string]struct{}, len(headerOffsets))
	for index, offset := range headerOffsets {
		select {
		case <-ctx.Done():
			return rows
		default:
		}
		primaryTypeID := int(binary.LittleEndian.Uint32(data[offset+4 : offset+8]))
		primaryName := strings.TrimSpace(typeNames[primaryTypeID])
		runTime := int(binary.LittleEndian.Uint32(data[offset+8 : offset+12]))
		if primaryName == "" || runTime <= 0 {
			continue
		}
		end := int64(len(data))
		if index+1 < len(headerOffsets) {
			end = headerOffsets[index+1]
		}
		inputs, outputs := decodeBlueprintQuantityPairs(data, offset+16, end, primaryTypeID, typeNames)
		if len(outputs) == 0 {
			outputs = []StaticQuantityDecodedRow{{
				TypeID:   primaryTypeID,
				Name:     primaryName,
				Quantity: 1,
			}}
		}
		if len(inputs) == 0 && len(outputs) == 0 {
			continue
		}
		rowKey := fmt.Sprintf("offset:%d:primary:%d", offset, primaryTypeID)
		if _, ok := seen[rowKey]; ok {
			continue
		}
		seen[rowKey] = struct{}{}
		rows = append(rows, StaticBlueprintDecodedRow{
			BlueprintID:    blueprintIDByOffset[offset],
			RowKey:         rowKey,
			PrimaryTypeID:  primaryTypeID,
			PrimaryName:    primaryName,
			RunTimeSeconds: runTime,
			Inputs:         inputs,
			Outputs:        outputs,
			OffsetBytes:    offset,
			Basis:          "fsdbinary candidate blueprint row: zero marker, validated primaryTypeID, runTime, and typeID/quantity pairs",
		})
	}
	sortBlueprintRows(rows)
	return rows
}

func staticClientTypeNameMap(ctx context.Context, clientRoot string) (map[int]string, []StaticResourceEvidence, error) {
	entries, _, err := ReadResourceIndex(clientRoot)
	if err != nil {
		return nil, nil, err
	}
	typeEntry, ok := entries.Find("res:/staticdata/types.fsdbinary")
	if !ok {
		return nil, nil, errors.New("types.fsdbinary resource was not found")
	}
	typeEvidence, err := ResourceEvidence(clientRoot, typeEntry)
	if err != nil {
		return nil, nil, err
	}
	typeData, err := os.ReadFile(typeEvidence.Path)
	if err != nil {
		return nil, nil, err
	}
	locEntry, ok := entries.Find("res:/localizationfsd/localization_fsd_en-us.pickle")
	if !ok {
		return nil, nil, errors.New("localisation resource was not found")
	}
	locEvidence, err := ResourceEvidence(clientRoot, locEntry)
	if err != nil {
		return nil, nil, err
	}
	locData, err := os.ReadFile(locEvidence.Path)
	if err != nil {
		return nil, nil, err
	}
	names := make(map[int]string)
	for _, row := range DecodeFSDBinaryTypeRows(ctx, typeData, DecodeLocalizationStrings(locData)) {
		if row.TypeID > 0 && strings.TrimSpace(row.Name) != "" {
			names[row.TypeID] = repairStaticClientText(strings.TrimSpace(row.Name))
		}
	}
	if len(names) == 0 {
		return nil, nil, errors.New("native type decoder found no names for production validation")
	}
	return names, []StaticResourceEvidence{typeEvidence, locEvidence}, nil
}

func decodeFSDBinaryDictionaryEntries(data []byte) []fsdDictionaryEntry {
	if len(data) < 64 {
		return nil
	}
	base := int64(24)
	declaredRows := binary.LittleEndian.Uint64(data[40:48])
	bucketCount := binary.LittleEndian.Uint64(data[48:56])
	if declaredRows == 0 || declaredRows > 100000 || bucketCount == 0 || bucketCount > 100000 {
		return nil
	}
	if 56+bucketCount*8 > uint64(len(data)) {
		return nil
	}
	out := make([]fsdDictionaryEntry, 0, min(int(declaredRows), len(data)/16))
	for bucketIndex := 0; bucketIndex < int(bucketCount); bucketIndex++ {
		bucketRel := int64(binary.LittleEndian.Uint64(data[56+bucketIndex*8 : 64+bucketIndex*8]))
		if bucketRel <= 0 {
			continue
		}
		bucketAbs := base + bucketRel
		if bucketAbs < 0 || bucketAbs+8 > int64(len(data)) {
			continue
		}
		entryCount := binary.LittleEndian.Uint64(data[bucketAbs : bucketAbs+8])
		if entryCount > declaredRows || entryCount > 10000 {
			continue
		}
		position := bucketAbs + 8
		for entryIndex := uint64(0); entryIndex < entryCount; entryIndex++ {
			if position+16 > int64(len(data)) {
				break
			}
			key := int64(binary.LittleEndian.Uint64(data[position : position+8]))
			valueRel := int64(binary.LittleEndian.Uint64(data[position+8 : position+16]))
			valueAbs := base + valueRel
			if key > 0 && key <= int64(maxNativeStaticTypeID) && valueRel > 0 && valueAbs >= 0 && valueAbs < int64(len(data)) {
				out = append(out, fsdDictionaryEntry{
					Key:         int(key),
					ValueRel:    valueRel,
					ValueAbs:    valueAbs,
					BucketIndex: bucketIndex,
					EntryOffset: position,
				})
			}
			position += 16
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Key == out[j].Key {
			return out[i].ValueAbs < out[j].ValueAbs
		}
		return out[i].Key < out[j].Key
	})
	return out
}

func decodeStaticQuantityList(data []byte, offset int64, typeNames map[int]string) ([]StaticQuantityDecodedRow, bool) {
	if offset < 0 || offset+8 > int64(len(data)) {
		return nil, false
	}
	count := int(binary.LittleEndian.Uint64(data[offset : offset+8]))
	if count <= 0 || count > 256 {
		return nil, false
	}
	if offset+8+int64(count)*8 > int64(len(data)) {
		return nil, false
	}
	out := make([]StaticQuantityDecodedRow, 0, count)
	for index := 0; index < count; index++ {
		pairOffset := offset + 8 + int64(index)*8
		typeID := int(binary.LittleEndian.Uint32(data[pairOffset : pairOffset+4]))
		quantity := int(binary.LittleEndian.Uint32(data[pairOffset+4 : pairOffset+8]))
		name := strings.TrimSpace(typeNames[typeID])
		if typeID <= 0 || name == "" || quantity <= 0 {
			return nil, false
		}
		out = append(out, StaticQuantityDecodedRow{
			TypeID:   typeID,
			Name:     name,
			Quantity: quantity,
		})
	}
	sortQuantityRows(out)
	return out, true
}

func blueprintHeaderOffsets(ctx context.Context, data []byte, typeNames map[int]string) []int64 {
	var offsets []int64
	for offset := int64(32); offset+16 <= int64(len(data)); offset += 4 {
		if offset%1048576 == 0 {
			select {
			case <-ctx.Done():
				return offsets
			default:
			}
		}
		leading := binary.LittleEndian.Uint32(data[offset : offset+4])
		primaryTypeID := int(binary.LittleEndian.Uint32(data[offset+4 : offset+8]))
		runTime := int(binary.LittleEndian.Uint32(data[offset+8 : offset+12]))
		marker := binary.LittleEndian.Uint32(data[offset+12 : offset+16])
		if leading != 0 || primaryTypeID < 1000 || strings.TrimSpace(typeNames[primaryTypeID]) == "" {
			continue
		}
		if runTime <= 0 || runTime > 86400 || marker != 8 {
			continue
		}
		offsets = append(offsets, offset)
	}
	sort.Slice(offsets, func(i, j int) bool { return offsets[i] < offsets[j] })
	return offsets
}

func decodeBlueprintQuantityPairs(data []byte, start, end int64, primaryTypeID int, typeNames map[int]string) ([]StaticQuantityDecodedRow, []StaticQuantityDecodedRow) {
	if start < 0 {
		start = 0
	}
	if end > int64(len(data)) {
		end = int64(len(data))
	}
	seenInputs := make(map[int]struct{})
	seenOutputs := make(map[int]struct{})
	var inputs []StaticQuantityDecodedRow
	var outputs []StaticQuantityDecodedRow
	for offset := start; offset+8 <= end; offset += 4 {
		typeID := int(binary.LittleEndian.Uint32(data[offset : offset+4]))
		quantity := int(binary.LittleEndian.Uint32(data[offset+4 : offset+8]))
		if typeID < 1000 || quantity <= 0 || quantity > 10000000 {
			continue
		}
		name := strings.TrimSpace(typeNames[typeID])
		if name == "" {
			continue
		}
		row := StaticQuantityDecodedRow{TypeID: typeID, Name: name, Quantity: quantity}
		if typeID == primaryTypeID {
			if _, ok := seenOutputs[typeID]; !ok {
				outputs = append(outputs, row)
				seenOutputs[typeID] = struct{}{}
			}
			continue
		}
		if _, ok := seenInputs[typeID]; ok {
			continue
		}
		inputs = append(inputs, row)
		seenInputs[typeID] = struct{}{}
	}
	sortQuantityRows(inputs)
	sortQuantityRows(outputs)
	return inputs, outputs
}

func buildRecipeCandidatesFromBlueprints(rows []StaticBlueprintDecodedRow) []StaticProductionRecipeDecodedRow {
	recipes := make([]StaticProductionRecipeDecodedRow, 0, len(rows))
	for _, row := range rows {
		if len(row.Outputs) == 0 || len(row.Inputs) == 0 {
			continue
		}
		recipes = append(recipes, StaticProductionRecipeDecodedRow{
			RecipeID:       row.RowKey,
			BlueprintID:    row.BlueprintID,
			Output:         row.Outputs[0],
			Inputs:         row.Inputs,
			RunTimeSeconds: row.RunTimeSeconds,
			OffsetBytes:    row.OffsetBytes,
			Basis:          "derived from native static-client blueprint candidate row",
		})
	}
	sort.Slice(recipes, func(i, j int) bool {
		if recipes[i].Output.TypeID == recipes[j].Output.TypeID {
			return recipes[i].OffsetBytes < recipes[j].OffsetBytes
		}
		return recipes[i].Output.TypeID < recipes[j].Output.TypeID
	})
	return recipes
}

func sortQuantityRows(rows []StaticQuantityDecodedRow) {
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].TypeID == rows[j].TypeID {
			return rows[i].Quantity < rows[j].Quantity
		}
		return rows[i].TypeID < rows[j].TypeID
	})
}

func sortBlueprintRows(rows []StaticBlueprintDecodedRow) {
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].PrimaryTypeID == rows[j].PrimaryTypeID {
			return rows[i].OffsetBytes < rows[j].OffsetBytes
		}
		return rows[i].PrimaryTypeID < rows[j].PrimaryTypeID
	})
}

func sortMaterialRequirementRows(rows []StaticMaterialRequirementDecodedRow) {
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].TypeID == rows[j].TypeID {
			return rows[i].OffsetBytes < rows[j].OffsetBytes
		}
		return rows[i].TypeID < rows[j].TypeID
	})
}
