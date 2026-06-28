package staticclient

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/blackrelay/registry/internal/model"
)

const staticUniverseDecodeSchema = "registry.static-client-universe-decode.v1"

type StaticUniverseDecodeOptions struct {
	ClientRoot  string
	OutputDir   string
	Environment model.Environment
	ClientBuild string
	PatchLabel  string
	Now         func() time.Time
}

type StaticUniverseDecodeResult struct {
	SchemaVersion      string                   `json:"schemaVersion"`
	Environment        model.Environment        `json:"environment"`
	GeneratedAt        time.Time                `json:"generatedAt"`
	ClientBuild        string                   `json:"clientBuild,omitempty"`
	PatchLabel         string                   `json:"patchLabel,omitempty"`
	DecoderStatus      string                   `json:"decoderStatus"`
	OutputDir          string                   `json:"outputDir"`
	Resources          []StaticResourceEvidence `json:"resources"`
	RegionCount        int                      `json:"regionCount"`
	ConstellationCount int                      `json:"constellationCount"`
	SystemCount        int                      `json:"systemCount"`
	JumpCount          int                      `json:"jumpCount"`
}

type staticUniverseResources struct {
	regions        StaticResourceEvidence
	constellations StaticResourceEvidence
	systems        StaticResourceEvidence
	jumps          StaticResourceEvidence
	localisation   StaticResourceEvidence
}

type staticDictEntry struct {
	Key        int
	ValueStart int
	ValueEnd   int
}

func DecodeStaticClientUniverseFiles(ctx context.Context, opts StaticUniverseDecodeOptions) (StaticUniverseDecodeResult, error) {
	if strings.TrimSpace(opts.ClientRoot) == "" {
		return StaticUniverseDecodeResult{}, errors.New("client root is required")
	}
	if strings.TrimSpace(opts.OutputDir) == "" {
		return StaticUniverseDecodeResult{}, errors.New("output directory is required")
	}
	environment := opts.Environment
	if environment == "" {
		environment = model.EnvironmentStillness
	}
	now := time.Now().UTC()
	if opts.Now != nil {
		now = opts.Now().UTC()
	}
	resources, err := resolveStaticUniverseResources(opts.ClientRoot)
	if err != nil {
		return StaticUniverseDecodeResult{}, err
	}
	localisationData, err := os.ReadFile(resources.localisation.Path)
	if err != nil {
		return StaticUniverseDecodeResult{}, err
	}
	names := DecodeLocalizationStrings(localisationData)
	if len(names) == 0 {
		return StaticUniverseDecodeResult{}, errors.New("localisation resource did not contain decodable names")
	}
	regions, err := decodeNativeRegions(ctx, resources.regions.Path, names)
	if err != nil {
		return StaticUniverseDecodeResult{}, err
	}
	constellations, err := decodeNativeConstellations(ctx, resources.constellations.Path, names)
	if err != nil {
		return StaticUniverseDecodeResult{}, err
	}
	systems, err := decodeNativeSystems(ctx, resources.systems.Path, names)
	if err != nil {
		return StaticUniverseDecodeResult{}, err
	}
	jumps, err := decodeNativeJumps(ctx, resources.jumps.Path)
	if err != nil {
		return StaticUniverseDecodeResult{}, err
	}
	schemaDir := filepath.Join(opts.OutputDir, "fsd_binary_schema")
	if err := os.MkdirAll(schemaDir, 0o755); err != nil {
		return StaticUniverseDecodeResult{}, err
	}
	if err := writeJSONFile(filepath.Join(schemaDir, "regions.json"), regions); err != nil {
		return StaticUniverseDecodeResult{}, err
	}
	if err := writeJSONFile(filepath.Join(schemaDir, "constellations.json"), constellations); err != nil {
		return StaticUniverseDecodeResult{}, err
	}
	if err := writeJSONFile(filepath.Join(schemaDir, "systems.json"), systems); err != nil {
		return StaticUniverseDecodeResult{}, err
	}
	if err := writeJSONFile(filepath.Join(schemaDir, "jumps.json"), map[string]any{"Type: FSD List": jumps}); err != nil {
		return StaticUniverseDecodeResult{}, err
	}
	return StaticUniverseDecodeResult{
		SchemaVersion: staticUniverseDecodeSchema,
		Environment:   environment,
		GeneratedAt:   now,
		ClientBuild:   opts.ClientBuild,
		PatchLabel:    opts.PatchLabel,
		DecoderStatus: "native_static_universe_localisation_backed_rows",
		OutputDir:     opts.OutputDir,
		Resources: []StaticResourceEvidence{
			resources.regions,
			resources.constellations,
			resources.systems,
			resources.jumps,
			resources.localisation,
		},
		RegionCount:        len(regions),
		ConstellationCount: len(constellations),
		SystemCount:        len(systems),
		JumpCount:          len(jumps),
	}, nil
}

func resolveStaticUniverseResources(clientRoot string) (staticUniverseResources, error) {
	entries, _, err := ReadResourceIndex(clientRoot)
	if err != nil {
		return staticUniverseResources{}, err
	}
	resolve := func(resourcePath string) (StaticResourceEvidence, error) {
		entry, ok := entries.Find(resourcePath)
		if !ok {
			return StaticResourceEvidence{}, fmt.Errorf("%s resource was not found", resourcePath)
		}
		return ResourceEvidence(clientRoot, entry)
	}
	regions, err := resolve("res:/staticdata/regions.static")
	if err != nil {
		return staticUniverseResources{}, err
	}
	constellations, err := resolve("res:/staticdata/constellations.static")
	if err != nil {
		return staticUniverseResources{}, err
	}
	systems, err := resolve("res:/staticdata/systems.static")
	if err != nil {
		return staticUniverseResources{}, err
	}
	jumps, err := resolve("res:/staticdata/jumps.static")
	if err != nil {
		return staticUniverseResources{}, err
	}
	localisation, err := resolve("res:/localizationfsd/localization_fsd_en-us.pickle")
	if err != nil {
		return staticUniverseResources{}, err
	}
	return staticUniverseResources{
		regions:        regions,
		constellations: constellations,
		systems:        systems,
		jumps:          jumps,
		localisation:   localisation,
	}, nil
}

func decodeNativeRegions(ctx context.Context, path string, names map[int]string) (map[string]map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	entries, err := decodeStaticDictEntries(data)
	if err != nil {
		return nil, err
	}
	rows := make(map[string]map[string]any, len(entries))
	for index, entry := range entries {
		if index%4096 == 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}
		}
		row := staticDictRow(data, entry)
		if len(row) < 44 {
			continue
		}
		regionID := readInt32(row, 0)
		nameID := readInt32(row, 4)
		if regionID <= 0 {
			regionID = entry.Key
		}
		name := staticUniverseRowName(row, 44, 11, 0, names, nameID, "Region "+strconv.Itoa(regionID))
		id := strconv.Itoa(regionID)
		rows[id] = map[string]any{
			"regionID":  id,
			"nameID":    nameID,
			"name":      name,
			"center":    vector3Any(row, 8),
			"potential": readFloat32(row, 32),
			"nebulaID":  readInt32(row, 36),
			"sectorID":  readInt32(row, 40),
		}
	}
	return rows, nil
}

func decodeNativeConstellations(ctx context.Context, path string, names map[int]string) (map[string]map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	entries, err := decodeStaticDictEntries(data)
	if err != nil {
		return nil, err
	}
	rows := make(map[string]map[string]any, len(entries))
	for index, entry := range entries {
		if index%4096 == 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}
		}
		row := staticDictRow(data, entry)
		if len(row) < 36 {
			continue
		}
		constellationID := readInt32(row, 0)
		regionID := readInt32(row, 4)
		nameID := readInt32(row, 8)
		if constellationID <= 0 {
			constellationID = entry.Key
		}
		name := staticUniverseRowName(row, 36, 6, 1, names, nameID, "Constellation "+strconv.Itoa(constellationID))
		id := strconv.Itoa(constellationID)
		rows[id] = map[string]any{
			"constellationID": id,
			"regionID":        strconv.Itoa(regionID),
			"nameID":          nameID,
			"name":            name,
			"center":          vector3Any(row, 12),
		}
	}
	return rows, nil
}

func decodeNativeSystems(ctx context.Context, path string, names map[int]string) (map[string]map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	entries, err := decodeStaticDictEntries(data)
	if err != nil {
		return nil, err
	}
	rows := make(map[string]map[string]any, len(entries))
	for index, entry := range entries {
		if index%4096 == 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}
		}
		row := staticDictRow(data, entry)
		if len(row) < 56 {
			continue
		}
		systemID := readInt32(row, 0)
		nameID := readInt32(row, 24)
		if systemID <= 0 {
			systemID = entry.Key
		}
		name := staticUniverseRowName(row, 56, 11, 2, names, nameID, "System "+strconv.Itoa(systemID))
		id := strconv.Itoa(systemID)
		rows[id] = map[string]any{
			"solarSystemID":   id,
			"systemID":        id,
			"nameID":          nameID,
			"name":            name,
			"securityStatus":  readFloat32(row, 4),
			"frostLine":       readFloat32(row, 8),
			"potential":       readFloat32(row, 12),
			"constellationID": strconv.Itoa(readInt32(row, 16)),
			"regionID":        strconv.Itoa(readInt32(row, 20)),
			"center":          vector3Any(row, 28),
			"pseudoSecurity":  readFloat32(row, 52),
		}
	}
	return rows, nil
}

func decodeNativeJumps(ctx context.Context, path string) ([]map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(data) < 4 {
		return nil, errors.New("static jump list is too short")
	}
	count := int(readUint32(data, 0))
	if count < 0 || 4+count*65 != len(data) {
		return nil, fmt.Errorf("invalid static jump list length: count=%d size=%d", count, len(data))
	}
	rows := make([]map[string]any, 0, count)
	for index := 0; index < count; index++ {
		if index%4096 == 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}
		}
		offset := 4 + index*65
		row := map[string]any{
			"jump": map[string]any{
				"jumpID":         strconv.Itoa(readInt32(data, offset)),
				"stargateID":     strconv.Itoa(readInt32(data, offset+4)),
				"fromSystemID":   strconv.Itoa(readInt32(data, offset+8)),
				"toSystemID":     strconv.Itoa(readInt32(data, offset+12)),
				"jumpType":       int(data[offset+16]),
				"fromCoordinate": vector3Any(data, offset+17),
				"toCoordinate":   vector3Any(data, offset+41),
			},
		}
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool {
		left := rows[i]["jump"].(map[string]any)
		right := rows[j]["jump"].(map[string]any)
		if left["fromSystemID"] == right["fromSystemID"] {
			return fmt.Sprint(left["toSystemID"]) < fmt.Sprint(right["toSystemID"])
		}
		return fmt.Sprint(left["fromSystemID"]) < fmt.Sprint(right["fromSystemID"])
	})
	return rows, nil
}

func decodeStaticDictEntries(data []byte) ([]staticDictEntry, error) {
	if len(data) < 12 {
		return nil, errors.New("static dictionary is too short")
	}
	footerLen := int(readUint32(data, len(data)-4))
	if footerLen < 12 || footerLen > len(data) || (footerLen-4)%8 != 0 {
		return nil, fmt.Errorf("invalid static dictionary footer length: %d", footerLen)
	}
	footerStart := len(data) - footerLen
	entryCount := (footerLen - 4) / 8
	entries := make([]staticDictEntry, 0, entryCount)
	for index := 0; index < entryCount; index++ {
		offset := footerStart + index*8
		key := readInt32(data, offset)
		valueOffset := readInt32(data, offset+4)
		valueStart := valueOffset + 4
		if key <= 0 || valueStart < 4 || valueStart >= footerStart {
			continue
		}
		entries = append(entries, staticDictEntry{Key: key, ValueStart: valueStart})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].ValueStart < entries[j].ValueStart })
	for index := range entries {
		valueEnd := footerStart
		if index+1 < len(entries) {
			valueEnd = entries[index+1].ValueStart
		}
		if valueEnd <= entries[index].ValueStart {
			entries[index].ValueEnd = entries[index].ValueStart
			continue
		}
		entries[index].ValueEnd = valueEnd
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Key < entries[j].Key })
	if len(entries) == 0 {
		return nil, errors.New("static dictionary contained no valid entries")
	}
	return entries, nil
}

func staticDictRow(data []byte, entry staticDictEntry) []byte {
	if entry.ValueStart < 0 || entry.ValueEnd <= entry.ValueStart || entry.ValueEnd > len(data) {
		return nil
	}
	return data[entry.ValueStart:entry.ValueEnd]
}

func staticUniverseRowName(row []byte, fixedSize, variableAttributeCount, attributeIndex int, names map[int]string, nameID int, fallback string) string {
	if name, ok := decodeStaticVariableString(row, fixedSize, variableAttributeCount, attributeIndex); ok {
		name = repairStaticClientName(strings.TrimSpace(name))
		if name != "" {
			return name
		}
	}
	return staticUniverseName(names, nameID, fallback)
}

func decodeStaticVariableString(row []byte, fixedSize, variableAttributeCount, attributeIndex int) (string, bool) {
	if fixedSize <= 0 || variableAttributeCount < 2 || attributeIndex < 0 || attributeIndex >= variableAttributeCount {
		return "", false
	}
	for _, offsetCount := range []int{variableAttributeCount - 1, variableAttributeCount} {
		if value, ok := decodeStaticVariableStringWithOffsetCount(row, fixedSize, attributeIndex, offsetCount); ok {
			return value, true
		}
	}
	return "", false
}

func decodeStaticVariableStringWithOffsetCount(row []byte, fixedSize, attributeIndex, offsetCount int) (string, bool) {
	tableStart := fixedSize + 4
	variableBase := tableStart + offsetCount*4
	startOffsetIndex := attributeIndex + 1
	endOffsetIndex := attributeIndex + 2
	if endOffsetIndex >= offsetCount || variableBase > len(row) || tableStart+offsetCount*4 > len(row) {
		return "", false
	}
	start := readInt32(row, tableStart+startOffsetIndex*4)
	end := readInt32(row, tableStart+endOffsetIndex*4)
	if start < 0 || end <= start || variableBase+end > len(row) {
		return "", false
	}
	return decodeStaticLengthPrefixedString(row[variableBase+start : variableBase+end])
}

func decodeStaticLengthPrefixedString(segment []byte) (string, bool) {
	if len(segment) < 4 {
		return "", false
	}
	length := int(readUint32(segment, 0))
	if length <= 0 || length > len(segment)-4 {
		return "", false
	}
	return decodeLocalisationUTF8(segment[4 : 4+length])
}

func staticUniverseName(names map[int]string, nameID int, fallback string) string {
	name := repairStaticClientName(strings.TrimSpace(names[nameID]))
	if name != "" && !isAllASCIIDigits(name) {
		return name
	}
	return fallback
}

func vector3Any(data []byte, offset int) []any {
	if offset < 0 || offset+24 > len(data) {
		return []any{map[string]any{"vector_schema": map[string]any{}}, 0, 0, 0}
	}
	return []any{
		map[string]any{"vector_schema": map[string]any{}},
		readFloat64(data, offset),
		readFloat64(data, offset+8),
		readFloat64(data, offset+16),
	}
}

func readInt32(data []byte, offset int) int {
	return int(int32(binary.LittleEndian.Uint32(data[offset : offset+4])))
}

func readUint32(data []byte, offset int) uint32 {
	return binary.LittleEndian.Uint32(data[offset : offset+4])
}

func readFloat32(data []byte, offset int) float32 {
	return math.Float32frombits(binary.LittleEndian.Uint32(data[offset : offset+4]))
}

func readFloat64(data []byte, offset int) float64 {
	return math.Float64frombits(binary.LittleEndian.Uint64(data[offset : offset+8]))
}

func writeJSONFile(path string, value any) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	encodeErr := encoder.Encode(value)
	closeErr := file.Close()
	if encodeErr != nil {
		return encodeErr
	}
	return closeErr
}
