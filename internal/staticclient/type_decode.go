package staticclient

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/blackrelay/registry/internal/model"
)

const staticTypeDecodeSchema = "registry.static-client-type-decode.v1"

type StaticTypeDecodeOptions struct {
	ClientRoot  string
	OutputPath  string
	Environment model.Environment
	ClientBuild string
	PatchLabel  string
	Now         func() time.Time
}

type StaticTypeDecodeResult struct {
	SchemaVersion string                   `json:"schemaVersion"`
	Environment   model.Environment        `json:"environment"`
	GeneratedAt   time.Time                `json:"generatedAt"`
	ClientBuild   string                   `json:"clientBuild,omitempty"`
	PatchLabel    string                   `json:"patchLabel,omitempty"`
	DecoderStatus string                   `json:"decoderStatus"`
	OutputPath    string                   `json:"outputPath,omitempty"`
	RowCount      int                      `json:"rowCount"`
	Resources     []StaticResourceEvidence `json:"resources"`
	Rows          []StaticTypeDecodedRow   `json:"rows"`
}

type StaticTypeDecodedRow struct {
	TypeID      int    `json:"typeId"`
	GroupID     int    `json:"groupId"`
	Name        string `json:"name"`
	TypeNameID  int    `json:"typeNameId,omitempty"`
	WreckTypeID int    `json:"wreckTypeId,omitempty"`
	OffsetBytes int64  `json:"offsetBytes"`
	Basis       string `json:"basis"`
}

func DecodeStaticClientTypeFile(ctx context.Context, opts StaticTypeDecodeOptions) (StaticTypeDecodeResult, error) {
	if strings.TrimSpace(opts.ClientRoot) == "" {
		return StaticTypeDecodeResult{}, errors.New("client root is required")
	}
	environment := opts.Environment
	if environment == "" {
		environment = model.EnvironmentStillness
	}
	now := time.Now().UTC()
	if opts.Now != nil {
		now = opts.Now().UTC()
	}
	entries, _, err := ReadResourceIndex(opts.ClientRoot)
	if err != nil {
		return StaticTypeDecodeResult{}, err
	}
	typeEntry, ok := entries.Find("res:/staticdata/types.fsdbinary")
	if !ok {
		return StaticTypeDecodeResult{}, errors.New("types.fsdbinary resource was not found")
	}
	typeEvidence, err := ResourceEvidence(opts.ClientRoot, typeEntry)
	if err != nil {
		return StaticTypeDecodeResult{}, err
	}
	typeData, err := os.ReadFile(typeEvidence.Path)
	if err != nil {
		return StaticTypeDecodeResult{}, err
	}
	locEntry, ok := entries.Find("res:/localizationfsd/localization_fsd_en-us.pickle")
	if !ok {
		return StaticTypeDecodeResult{}, errors.New("localisation resource was not found")
	}
	locEvidence, err := ResourceEvidence(opts.ClientRoot, locEntry)
	if err != nil {
		return StaticTypeDecodeResult{}, err
	}
	locData, err := os.ReadFile(locEvidence.Path)
	if err != nil {
		return StaticTypeDecodeResult{}, err
	}
	decodedRows := DecodeFSDBinaryTypeRows(ctx, typeData, DecodeLocalizationStrings(locData))
	rows := make([]StaticTypeDecodedRow, 0, len(decodedRows))
	for _, row := range decodedRows {
		if row.TypeID <= 0 || row.GroupID <= 0 || strings.TrimSpace(row.Name) == "" {
			continue
		}
		rows = append(rows, StaticTypeDecodedRow{
			TypeID:      row.TypeID,
			GroupID:     row.GroupID,
			Name:        repairStaticClientName(strings.TrimSpace(row.Name)),
			TypeNameID:  row.TypeNameID,
			WreckTypeID: row.WreckTypeID,
			OffsetBytes: row.OffsetBytes,
			Basis:       row.Basis,
		})
	}
	if len(rows) == 0 {
		return StaticTypeDecodeResult{}, errors.New("native type decoder found no localisation-backed type rows")
	}
	result := StaticTypeDecodeResult{
		SchemaVersion: staticTypeDecodeSchema,
		Environment:   environment,
		GeneratedAt:   now,
		ClientBuild:   opts.ClientBuild,
		PatchLabel:    opts.PatchLabel,
		DecoderStatus: "native_localisation_backed_type_rows",
		OutputPath:    opts.OutputPath,
		RowCount:      len(rows),
		Resources:     []StaticResourceEvidence{typeEvidence, locEvidence},
		Rows:          rows,
	}
	if strings.TrimSpace(opts.OutputPath) == "" {
		return result, nil
	}
	if err := os.MkdirAll(filepath.Dir(opts.OutputPath), 0o755); err != nil {
		return StaticTypeDecodeResult{}, err
	}
	file, err := os.Create(opts.OutputPath)
	if err != nil {
		return StaticTypeDecodeResult{}, err
	}
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	encodeErr := encoder.Encode(result)
	closeErr := file.Close()
	if encodeErr != nil {
		return StaticTypeDecodeResult{}, encodeErr
	}
	if closeErr != nil {
		return StaticTypeDecodeResult{}, closeErr
	}
	return result, nil
}
