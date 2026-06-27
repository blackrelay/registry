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

const staticProductionResourcesSchema = "registry.static-client-production-resources.v1"

type StaticProductionExtractionOptions struct {
	ClientRoot  string
	OutputPath  string
	Environment model.Environment
	ClientBuild string
	PatchLabel  string
	Now         func() time.Time
}

type StaticProductionExtractionResult struct {
	OutputPath    string                   `json:"outputPath"`
	ResourceCount int                      `json:"resourceCount"`
	Resources     []StaticResourceEvidence `json:"resources"`
	GeneratedAt   time.Time                `json:"generatedAt"`
}

type staticProductionResourcesPayload struct {
	SchemaVersion string                    `json:"schemaVersion"`
	Environment   model.Environment         `json:"environment"`
	GeneratedAt   time.Time                 `json:"generatedAt"`
	ClientBuild   string                    `json:"clientBuild,omitempty"`
	PatchLabel    string                    `json:"patchLabel,omitempty"`
	Resources     []StaticResourceDiscovery `json:"resources"`
}

type StaticProductionCompareResult struct {
	SchemaVersion     string                          `json:"schemaVersion"`
	BeforePath        string                          `json:"beforePath"`
	AfterPath         string                          `json:"afterPath"`
	BeforeCount       int                             `json:"beforeCount"`
	AfterCount        int                             `json:"afterCount"`
	SemanticallyEqual bool                            `json:"semanticallyEqual"`
	Added             []StaticProductionResourceDelta `json:"added,omitempty"`
	Removed           []StaticProductionResourceDelta `json:"removed,omitempty"`
	Changed           []StaticProductionResourceDelta `json:"changed,omitempty"`
}

type StaticProductionResourceSummary struct {
	SchemaVersion  string         `json:"schemaVersion"`
	Path           string         `json:"path"`
	ResourceCount  int            `json:"resourceCount"`
	TotalSizeBytes int64          `json:"totalSizeBytes"`
	Kinds          map[string]int `json:"kinds"`
	Resources      []string       `json:"resources"`
}

type StaticProductionResourceDelta struct {
	ResourcePath string                       `json:"resourcePath"`
	Kind         string                       `json:"kind"`
	Before       *StaticProductionResourceRow `json:"before,omitempty"`
	After        *StaticProductionResourceRow `json:"after,omitempty"`
}

type StaticProductionResourceRow struct {
	ResourcePath string `json:"resourcePath"`
	Kind         string `json:"kind"`
	SHA256       string `json:"sha256"`
	SizeBytes    int64  `json:"sizeBytes"`
	IndexSize    int64  `json:"indexSize,omitempty"`
	PackedSize   int64  `json:"packedSize,omitempty"`
}

func ExtractStaticClientProductionResources(ctx context.Context, opts StaticProductionExtractionOptions) (StaticProductionExtractionResult, error) {
	_ = ctx
	if strings.TrimSpace(opts.ClientRoot) == "" {
		return StaticProductionExtractionResult{}, errors.New("client root is required")
	}
	if strings.TrimSpace(opts.OutputPath) == "" {
		return StaticProductionExtractionResult{}, errors.New("output path is required")
	}
	environment := opts.Environment
	if environment == "" {
		environment = model.EnvironmentStillness
	}
	now := time.Now().UTC()
	if opts.Now != nil {
		now = opts.Now().UTC()
	}
	discoveries, err := DiscoverStaticClientResources(opts.ClientRoot)
	if err != nil {
		return StaticProductionExtractionResult{}, err
	}
	production := make([]StaticResourceDiscovery, 0, len(discoveries))
	for _, item := range discoveries {
		if isProductionResourceKind(item.Kind) {
			production = append(production, item)
		}
	}
	if len(production) == 0 {
		return StaticProductionExtractionResult{}, errors.New("no static-client production resources were discovered")
	}
	payload := staticProductionResourcesPayload{
		SchemaVersion: staticProductionResourcesSchema,
		Environment:   environment,
		GeneratedAt:   now,
		ClientBuild:   opts.ClientBuild,
		PatchLabel:    opts.PatchLabel,
		Resources:     production,
	}
	if err := os.MkdirAll(filepath.Dir(opts.OutputPath), 0o755); err != nil {
		return StaticProductionExtractionResult{}, err
	}
	file, err := os.Create(opts.OutputPath)
	if err != nil {
		return StaticProductionExtractionResult{}, err
	}
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	encodeErr := encoder.Encode(payload)
	closeErr := file.Close()
	if encodeErr != nil {
		return StaticProductionExtractionResult{}, encodeErr
	}
	if closeErr != nil {
		return StaticProductionExtractionResult{}, closeErr
	}
	evidence := make([]StaticResourceEvidence, 0, len(production))
	for _, item := range production {
		evidence = append(evidence, item.Evidence)
	}
	return StaticProductionExtractionResult{
		OutputPath:    opts.OutputPath,
		ResourceCount: len(production),
		Resources:     evidence,
		GeneratedAt:   now,
	}, nil
}

func isProductionResourceKind(kind string) bool {
	switch kind {
	case "blueprint_metadata", "recipe_metadata", "material_requirement_metadata":
		return true
	default:
		return false
	}
}

func CompareStaticProductionResourceFiles(beforePath, afterPath string) (StaticProductionCompareResult, error) {
	before, err := decodeStaticProductionResourceFile(beforePath)
	if err != nil {
		return StaticProductionCompareResult{}, err
	}
	after, err := decodeStaticProductionResourceFile(afterPath)
	if err != nil {
		return StaticProductionCompareResult{}, err
	}
	beforeByKey := productionRowsByKey(before)
	afterByKey := productionRowsByKey(after)
	result := StaticProductionCompareResult{
		SchemaVersion: "registry.static-client-production-resource-compare.v1",
		BeforePath:    beforePath,
		AfterPath:     afterPath,
		BeforeCount:   len(before),
		AfterCount:    len(after),
	}
	for key, beforeRow := range beforeByKey {
		afterRow, ok := afterByKey[key]
		if !ok {
			result.Removed = append(result.Removed, StaticProductionResourceDelta{
				ResourcePath: beforeRow.ResourcePath,
				Kind:         beforeRow.Kind,
				Before:       productionRowPtr(beforeRow),
			})
			continue
		}
		if beforeRow.SHA256 != afterRow.SHA256 || beforeRow.SizeBytes != afterRow.SizeBytes || beforeRow.IndexSize != afterRow.IndexSize || beforeRow.PackedSize != afterRow.PackedSize {
			result.Changed = append(result.Changed, StaticProductionResourceDelta{
				ResourcePath: beforeRow.ResourcePath,
				Kind:         beforeRow.Kind,
				Before:       productionRowPtr(beforeRow),
				After:        productionRowPtr(afterRow),
			})
		}
	}
	for key, afterRow := range afterByKey {
		if _, ok := beforeByKey[key]; ok {
			continue
		}
		result.Added = append(result.Added, StaticProductionResourceDelta{
			ResourcePath: afterRow.ResourcePath,
			Kind:         afterRow.Kind,
			After:        productionRowPtr(afterRow),
		})
	}
	sortProductionDeltas(result.Added)
	sortProductionDeltas(result.Removed)
	sortProductionDeltas(result.Changed)
	result.SemanticallyEqual = len(result.Added) == 0 && len(result.Removed) == 0 && len(result.Changed) == 0
	return result, nil
}

func SummariseStaticProductionResourceFile(path string) (StaticProductionResourceSummary, error) {
	rows, err := decodeStaticProductionResourceFile(path)
	if err != nil {
		return StaticProductionResourceSummary{}, err
	}
	summary := StaticProductionResourceSummary{
		SchemaVersion: "registry.static-client-production-resource-summary.v1",
		Path:          path,
		ResourceCount: len(rows),
		Kinds:         make(map[string]int),
		Resources:     make([]string, 0, len(rows)),
	}
	for _, row := range rows {
		summary.Kinds[row.Kind]++
		summary.TotalSizeBytes += row.SizeBytes
		summary.Resources = append(summary.Resources, row.ResourcePath)
	}
	sort.Strings(summary.Resources)
	return summary, nil
}

func decodeStaticProductionResourceFile(path string) ([]StaticProductionResourceRow, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var payload staticProductionResourcesPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, err
	}
	rows := make([]StaticProductionResourceRow, 0, len(payload.Resources))
	for _, item := range payload.Resources {
		resourcePath := strings.TrimSpace(item.ResourcePath)
		if resourcePath == "" {
			resourcePath = strings.TrimSpace(item.Evidence.ResourcePath)
		}
		if resourcePath == "" || strings.TrimSpace(item.Kind) == "" {
			continue
		}
		rows = append(rows, StaticProductionResourceRow{
			ResourcePath: resourcePath,
			Kind:         item.Kind,
			SHA256:       item.Evidence.SHA256,
			SizeBytes:    item.Evidence.SizeBytes,
			IndexSize:    item.Evidence.IndexSize,
			PackedSize:   item.Evidence.PackedSize,
		})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].ResourcePath == rows[j].ResourcePath {
			return rows[i].Kind < rows[j].Kind
		}
		return rows[i].ResourcePath < rows[j].ResourcePath
	})
	return rows, nil
}

func productionRowsByKey(rows []StaticProductionResourceRow) map[string]StaticProductionResourceRow {
	out := make(map[string]StaticProductionResourceRow, len(rows))
	for _, row := range rows {
		out[productionRowKey(row)] = row
	}
	return out
}

func productionRowKey(row StaticProductionResourceRow) string {
	return strings.ToLower(strings.TrimSpace(row.ResourcePath)) + "\x00" + strings.ToLower(strings.TrimSpace(row.Kind))
}

func productionRowPtr(row StaticProductionResourceRow) *StaticProductionResourceRow {
	return &StaticProductionResourceRow{
		ResourcePath: row.ResourcePath,
		Kind:         row.Kind,
		SHA256:       row.SHA256,
		SizeBytes:    row.SizeBytes,
		IndexSize:    row.IndexSize,
		PackedSize:   row.PackedSize,
	}
}

func sortProductionDeltas(items []StaticProductionResourceDelta) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].ResourcePath == items[j].ResourcePath {
			return items[i].Kind < items[j].Kind
		}
		return items[i].ResourcePath < items[j].ResourcePath
	})
}
