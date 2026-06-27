package staticclient

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/blackrelay/registry/internal/model"
)

const staticTypeExtractionSchema = "registry.static-client-types.v1"

type StaticTypeExtractionOptions struct {
	ClientRoot       string
	ResolvedJSONPath string
	OutputPath       string
	Environment      model.Environment
	ProbeTypeIDs     []int
	NativeFullScan   bool
	ClientBuild      string
	PatchLabel       string
	Now              func() time.Time
}

type StaticTypeExtractionResult struct {
	OutputPath  string                   `json:"outputPath"`
	RowCount    int                      `json:"rowCount"`
	Resources   []StaticResourceEvidence `json:"resources"`
	GeneratedAt time.Time                `json:"generatedAt"`
}

type staticTypeExtractionPayload struct {
	SchemaVersion string                   `json:"schemaVersion"`
	Environment   model.Environment        `json:"environment"`
	GeneratedAt   time.Time                `json:"generatedAt"`
	ClientBuild   string                   `json:"clientBuild,omitempty"`
	PatchLabel    string                   `json:"patchLabel,omitempty"`
	Resources     []StaticResourceEvidence `json:"resources,omitempty"`
	Candidates    []staticTypeRow          `json:"candidates"`
}

func ExtractStaticClientTypes(ctx context.Context, opts StaticTypeExtractionOptions) (StaticTypeExtractionResult, error) {
	if strings.TrimSpace(opts.ClientRoot) == "" {
		return StaticTypeExtractionResult{}, errors.New("client root is required")
	}
	if strings.TrimSpace(opts.OutputPath) == "" {
		return StaticTypeExtractionResult{}, errors.New("output path is required")
	}
	environment := opts.Environment
	if environment == "" {
		environment = model.EnvironmentStillness
	}
	now := time.Now().UTC()
	if opts.Now != nil {
		now = opts.Now().UTC()
	}
	rows, err := extractStaticTypeRows(ctx, opts)
	if err != nil {
		return StaticTypeExtractionResult{}, err
	}
	rows = normaliseStaticTypeRows(rows)
	resources, err := staticTypeResourceEvidence(opts.ClientRoot)
	if err != nil {
		return StaticTypeExtractionResult{}, err
	}
	payload := staticTypeExtractionPayload{
		SchemaVersion: staticTypeExtractionSchema,
		Environment:   environment,
		GeneratedAt:   now,
		ClientBuild:   opts.ClientBuild,
		PatchLabel:    opts.PatchLabel,
		Resources:     resources,
		Candidates:    rows,
	}
	if err := os.MkdirAll(filepath.Dir(opts.OutputPath), 0o755); err != nil {
		return StaticTypeExtractionResult{}, err
	}
	file, err := os.Create(opts.OutputPath)
	if err != nil {
		return StaticTypeExtractionResult{}, err
	}
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	encodeErr := encoder.Encode(payload)
	closeErr := file.Close()
	if encodeErr != nil {
		return StaticTypeExtractionResult{}, encodeErr
	}
	if closeErr != nil {
		return StaticTypeExtractionResult{}, closeErr
	}
	return StaticTypeExtractionResult{
		OutputPath:  opts.OutputPath,
		RowCount:    len(rows),
		Resources:   resources,
		GeneratedAt: now,
	}, nil
}

func extractStaticTypeRows(ctx context.Context, opts StaticTypeExtractionOptions) ([]staticTypeRow, error) {
	if strings.TrimSpace(opts.ResolvedJSONPath) != "" {
		data, err := os.ReadFile(opts.ResolvedJSONPath)
		if err != nil {
			return nil, err
		}
		return decodeStaticTypeRows(data)
	}
	if opts.NativeFullScan {
		return extractNativeFullScanTypeRows(ctx, opts.ClientRoot)
	}
	return extractNativeProbeTypeRows(ctx, opts.ClientRoot, opts.ProbeTypeIDs)
}

func extractNativeFullScanTypeRows(ctx context.Context, clientRoot string) ([]staticTypeRow, error) {
	entries, _, err := ReadResourceIndex(clientRoot)
	if err != nil {
		return nil, err
	}
	typeEntry, ok := entries.Find("res:/staticdata/types.fsdbinary")
	if !ok {
		return nil, errors.New("types.fsdbinary resource was not found")
	}
	typeEvidence, err := ResourceEvidence(clientRoot, typeEntry)
	if err != nil {
		return nil, err
	}
	typeData, err := os.ReadFile(typeEvidence.Path)
	if err != nil {
		return nil, err
	}
	locEntry, ok := entries.Find("res:/localizationfsd/localization_fsd_en-us.pickle")
	if !ok {
		return nil, errors.New("localisation resource was not found")
	}
	locEvidence, err := ResourceEvidence(clientRoot, locEntry)
	if err != nil {
		return nil, err
	}
	locData, err := os.ReadFile(locEvidence.Path)
	if err != nil {
		return nil, err
	}
	decoded := DecodeFSDBinaryTypeRows(ctx, typeData, DecodeLocalizationStrings(locData))
	rows := make([]staticTypeRow, 0, len(decoded))
	for _, row := range decoded {
		if row.TypeID <= 0 || row.GroupID <= 0 || strings.TrimSpace(row.Name) == "" {
			continue
		}
		rows = append(rows, staticTypeRow{
			GroupID:     row.GroupID,
			Name:        row.Name,
			Reason:      "native static-client full scan with localisation-backed name",
			TypeID:      row.TypeID,
			TypeNameID:  row.TypeNameID,
			WreckTypeID: row.WreckTypeID,
		})
	}
	if len(rows) == 0 {
		return nil, errors.New("native full scan extraction found no localisation-backed type rows")
	}
	return rows, nil
}

func extractNativeProbeTypeRows(ctx context.Context, clientRoot string, probeTypeIDs []int) ([]staticTypeRow, error) {
	if len(uniquePositiveInts(probeTypeIDs)) == 0 {
		return nil, errors.New("resolved JSON path or at least one native probe type id is required")
	}
	inspection, err := InspectStaticClientTypes(ctx, StaticTypeInspectionOptions{
		ClientRoot:   clientRoot,
		ProbeTypeIDs: probeTypeIDs,
	})
	if err != nil {
		return nil, err
	}
	rows := make([]staticTypeRow, 0, len(inspection.DecodedRows))
	for _, row := range inspection.DecodedRows {
		if row.TypeID <= 0 || row.GroupID <= 0 || strings.TrimSpace(row.Name) == "" {
			continue
		}
		rows = append(rows, staticTypeRow{
			GroupID:     row.GroupID,
			Name:        row.Name,
			Reason:      "native static-client numeric row probe with localisation-backed name",
			TypeID:      row.TypeID,
			TypeNameID:  row.TypeNameID,
			WreckTypeID: row.WreckTypeID,
		})
	}
	if len(rows) == 0 {
		return nil, errors.New("native probe extraction found no localisation-backed type rows")
	}
	return rows, nil
}

func staticTypeResourceEvidence(clientRoot string) ([]StaticResourceEvidence, error) {
	entries, _, err := ReadResourceIndex(clientRoot)
	if err != nil {
		return nil, err
	}
	wanted := []string{
		"res:/staticdata/types.fsdbinary",
		"res:/staticdata/groups.fsdbinary",
		"res:/staticdata/categories.fsdbinary",
		"res:/localizationfsd/localization_fsd_en-us.pickle",
	}
	resources := make([]StaticResourceEvidence, 0, len(wanted))
	for _, resourcePath := range wanted {
		entry, ok := entries.Find(resourcePath)
		if !ok {
			continue
		}
		evidence, err := ResourceEvidence(clientRoot, entry)
		if err != nil {
			if resourcePath == "res:/staticdata/types.fsdbinary" {
				return nil, err
			}
			continue
		}
		resources = append(resources, evidence)
	}
	if len(resources) == 0 {
		return nil, errors.New("types.fsdbinary resource evidence was not found")
	}
	return resources, nil
}

func normaliseStaticTypeRows(rows []staticTypeRow) []staticTypeRow {
	out := make([]staticTypeRow, 0, len(rows))
	seen := make(map[int]struct{}, len(rows))
	for _, row := range rows {
		if row.TypeID <= 0 {
			continue
		}
		if _, ok := seen[row.TypeID]; ok {
			continue
		}
		seen[row.TypeID] = struct{}{}
		row.Name = repairStaticClientText(strings.TrimSpace(row.Name))
		row.Description = repairStaticClientText(strings.TrimSpace(row.Description))
		row.GroupName = repairStaticClientText(strings.TrimSpace(row.GroupName))
		row.CategoryName = repairStaticClientText(strings.TrimSpace(row.CategoryName))
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].GroupID == out[j].GroupID {
			if out[i].TypeID == out[j].TypeID {
				return out[i].Name < out[j].Name
			}
			return out[i].TypeID < out[j].TypeID
		}
		return out[i].GroupID < out[j].GroupID
	})
	return out
}

func repairStaticClientText(value string) string {
	runes := []rune(value)
	for i, r := range runes {
		if r != '\uFFFD' {
			continue
		}
		if i > 0 && i+1 < len(runes) && isStaticTextWordRune(runes[i-1]) && isStaticTextWordRune(runes[i+1]) {
			runes[i] = '\''
			continue
		}
		runes[i] = ' '
	}
	return strings.Join(strings.Fields(string(runes)), " ")
}

func isStaticTextWordRune(r rune) bool {
	return r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9'
}
