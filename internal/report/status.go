package report

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/blackrelay/registry/internal/db"
	"github.com/blackrelay/registry/internal/exporter"
	"github.com/blackrelay/registry/internal/model"
	"github.com/blackrelay/registry/internal/sui"
)

type IndexerStatus string

const (
	IndexerStatusOK       IndexerStatus = "ok"
	IndexerStatusStale    IndexerStatus = "stale"
	IndexerStatusDegraded IndexerStatus = "degraded"
	IndexerStatusBlocked  IndexerStatus = "blocked"
)

type IndexerStatusOptions struct {
	Environment        model.Environment
	Now                func() time.Time
	StaleAfter         time.Duration
	ExportManifestPath string
}

type IndexerStatusReport struct {
	SchemaVersion        string                   `json:"schemaVersion"`
	Environment          model.Environment        `json:"environment"`
	GeneratedAt          time.Time                `json:"generatedAt"`
	Status               IndexerStatus            `json:"status"`
	Reasons              []string                 `json:"reasons,omitempty"`
	StaleAfterSeconds    int64                    `json:"staleAfterSeconds"`
	LastSuccessfulIngest *time.Time               `json:"lastSuccessfulIngest,omitempty"`
	MaxCursorLagSeconds  *int64                   `json:"maxCursorLagSeconds,omitempty"`
	Counts               db.RegistryRowCounts     `json:"counts"`
	CursorCounts         IndexerCursorCounts      `json:"cursorCounts"`
	Coverage             model.SuiCoverageSummary `json:"coverage"`
	Streams              []IndexerStreamStatus    `json:"streams"`
	Export               *IndexerExportStatus     `json:"export,omitempty"`
	SourceGaps           []model.SourceGap        `json:"sourceGaps,omitempty"`
}

type IndexerCursorCounts struct {
	Total         int   `json:"total"`
	Event         int   `json:"event"`
	Object        int   `json:"object"`
	Derivation    int   `json:"derivation"`
	Other         int   `json:"other"`
	Indexed       int   `json:"indexed"`
	Errored       int   `json:"errored"`
	RangeBlocked  int   `json:"rangeBlocked"`
	Stale         int   `json:"stale"`
	MissingIngest int   `json:"missingIngest"`
	RowsProcessed int64 `json:"rowsProcessed"`
}

type IndexerStreamStatus struct {
	Kind                 string               `json:"kind"`
	Status               model.CoverageStatus `json:"status"`
	CursorID             string               `json:"cursorId"`
	Source               string               `json:"source"`
	CursorKind           string               `json:"cursorKind,omitempty"`
	Environment          model.Environment    `json:"environment,omitempty"`
	Network              string               `json:"network,omitempty"`
	Role                 string               `json:"role,omitempty"`
	PackageID            string               `json:"packageId,omitempty"`
	ModuleName           string               `json:"moduleName,omitempty"`
	EventType            string               `json:"eventType,omitempty"`
	TypeName             string               `json:"typeName,omitempty"`
	TypeRepr             string               `json:"typeRepr,omitempty"`
	RowsProcessed        int64                `json:"rowsProcessed"`
	LastSuccessfulIngest *time.Time           `json:"lastSuccessfulIngest,omitempty"`
	LagSeconds           *int64               `json:"lagSeconds,omitempty"`
	LastCheckpoint       string               `json:"lastCheckpoint,omitempty"`
	ErrorCount           int64                `json:"errorCount"`
	LastErrorSummary     string               `json:"lastErrorSummary,omitempty"`
	EmptyStream          bool                 `json:"emptyStream,omitempty"`
	ProviderRangeBlocked bool                 `json:"providerRangeBlocked,omitempty"`
}

type IndexerExportStatus struct {
	ManifestPath      string                                  `json:"manifestPath"`
	BundleID          string                                  `json:"bundleId"`
	Registry          string                                  `json:"registry"`
	APIVersion        string                                  `json:"apiVersion"`
	GeneratedAt       time.Time                               `json:"generatedAt"`
	CycleScope        string                                  `json:"cycleScope"`
	Cycles            []int                                   `json:"cycles,omitempty"`
	IncludeUncycled   bool                                    `json:"includeUncycled,omitempty"`
	Files             []exporter.ExportFile                   `json:"files"`
	RowCounts         map[string]int                          `json:"rowCounts"`
	HighWaterMarks    map[string]exporter.ExportHighWaterMark `json:"highWaterMarks,omitempty"`
	TotalRows         int                                     `json:"totalRows"`
	TotalSizeBytes    int64                                   `json:"totalSizeBytes"`
	ManifestSHA256    string                                  `json:"manifestSha256"`
	ManifestSizeBytes int64                                   `json:"manifestSizeBytes"`
}

type IndexerStatusStore interface {
	CountRegistryRows(ctx context.Context, environment model.Environment) (db.RegistryCountSnapshot, error)
	ListCursors(ctx context.Context) ([]db.CursorStatus, error)
	ListSourceGaps(ctx context.Context, environment model.Environment) ([]model.SourceGap, error)
}

func BuildIndexerStatus(ctx context.Context, store IndexerStatusStore, options IndexerStatusOptions) (IndexerStatusReport, error) {
	if options.Environment == "" {
		options.Environment = model.EnvironmentStillness
	}
	now := time.Now().UTC()
	if options.Now != nil {
		now = options.Now().UTC()
	}
	staleAfter := options.StaleAfter
	if staleAfter <= 0 {
		staleAfter = 15 * time.Minute
	}
	counts, err := store.CountRegistryRows(ctx, options.Environment)
	if err != nil {
		return IndexerStatusReport{}, err
	}
	cursors, err := store.ListCursors(ctx)
	if err != nil {
		return IndexerStatusReport{}, err
	}
	sourceGaps, err := store.ListSourceGaps(ctx, options.Environment)
	if err != nil {
		return IndexerStatusReport{}, err
	}
	coverage := sui.CursorCoverageSummary(filterStatusCursors(cursors, options.Environment))
	cursorCounts, streams, lastIngest, maxLag := summarizeIndexerStreams(coverage.Targets, now, staleAfter)
	status, reasons := classifyIndexerStatus(cursorCounts)
	exportStatus, err := BuildIndexerExportStatus(options.ExportManifestPath)
	if err != nil {
		return IndexerStatusReport{}, err
	}
	return IndexerStatusReport{
		SchemaVersion:        "registry.indexer_status.v1",
		Environment:          options.Environment,
		GeneratedAt:          now,
		Status:               status,
		Reasons:              reasons,
		StaleAfterSeconds:    int64(staleAfter.Seconds()),
		LastSuccessfulIngest: lastIngest,
		MaxCursorLagSeconds:  maxLag,
		Counts:               counts.Counts,
		CursorCounts:         cursorCounts,
		Coverage:             coverage,
		Streams:              streams,
		Export:               exportStatus,
		SourceGaps:           sourceGaps,
	}, nil
}

func BuildIndexerExportStatus(manifestPath string) (*IndexerExportStatus, error) {
	manifestPath = strings.TrimSpace(manifestPath)
	if manifestPath == "" {
		return nil, nil
	}
	info, err := os.Stat(manifestPath)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		manifestPath = filepath.Join(manifestPath, "manifest.json")
	}
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, err
	}
	var manifest exporter.ExportManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("decode export manifest: %w", err)
	}
	if manifest.SchemaVersion != "registry.export_manifest.v1" {
		return nil, fmt.Errorf("unsupported export manifest schema version %q", manifest.SchemaVersion)
	}
	sum := sha256.Sum256(data)
	rowCounts := make(map[string]int, len(manifest.Files))
	totalRows := 0
	var totalSize int64
	for _, file := range manifest.Files {
		rowCounts[file.Path] = file.RowCount
		totalRows += file.RowCount
		totalSize += file.SizeBytes
	}
	return &IndexerExportStatus{
		ManifestPath:      manifestPath,
		BundleID:          hex.EncodeToString(sum[:]),
		Registry:          manifest.Registry,
		APIVersion:        manifest.APIVersion,
		GeneratedAt:       manifest.GeneratedAt,
		CycleScope:        manifest.CycleScope,
		Cycles:            append([]int(nil), manifest.Cycles...),
		IncludeUncycled:   manifest.IncludeUncycled,
		Files:             append([]exporter.ExportFile(nil), manifest.Files...),
		RowCounts:         rowCounts,
		HighWaterMarks:    manifest.HighWaterMarks,
		TotalRows:         totalRows,
		TotalSizeBytes:    totalSize,
		ManifestSHA256:    hex.EncodeToString(sum[:]),
		ManifestSizeBytes: int64(len(data)),
	}, nil
}

func filterStatusCursors(cursors []db.CursorStatus, environment model.Environment) []db.CursorStatus {
	out := make([]db.CursorStatus, 0, len(cursors))
	for _, cursor := range cursors {
		if environment != "" && cursor.Environment != environment {
			continue
		}
		out = append(out, cursor)
	}
	return out
}

func summarizeIndexerStreams(targets []model.SuiCoverageTarget, now time.Time, staleAfter time.Duration) (IndexerCursorCounts, []IndexerStreamStatus, *time.Time, *int64) {
	counts := IndexerCursorCounts{Total: len(targets)}
	streams := make([]IndexerStreamStatus, 0, len(targets))
	var lastIngest *time.Time
	var maxLag *int64
	for _, target := range targets {
		stream := indexerStreamStatus(target, now)
		providerLimited := stream.ProviderRangeBlocked || stream.Status == model.CoverageStatusRangeBlocked
		if stream.LastSuccessfulIngest == nil {
			if !providerLimited {
				counts.MissingIngest++
			}
		} else {
			lag := int64(now.Sub(*stream.LastSuccessfulIngest).Seconds())
			if lag < 0 {
				lag = 0
			}
			stream.LagSeconds = &lag
			if !providerLimited {
				if maxLag == nil || lag > *maxLag {
					value := lag
					maxLag = &value
				}
				if target.LastSuccessfulIngest != nil && (lastIngest == nil || target.LastSuccessfulIngest.After(*lastIngest)) {
					value := *target.LastSuccessfulIngest
					lastIngest = &value
				}
				if time.Duration(lag)*time.Second > staleAfter {
					counts.Stale++
				}
			}
		}
		switch stream.Kind {
		case "event":
			counts.Event++
		case "object":
			counts.Object++
		case "derivation":
			counts.Derivation++
		default:
			counts.Other++
		}
		switch stream.Status {
		case model.CoverageStatusIndexed:
			counts.Indexed++
		case model.CoverageStatusErrored:
			counts.Errored++
		case model.CoverageStatusRangeBlocked:
			counts.RangeBlocked++
		}
		counts.RowsProcessed += stream.RowsProcessed
		streams = append(streams, stream)
	}
	sort.Slice(streams, func(i, j int) bool {
		if streams[i].Kind == streams[j].Kind {
			return streams[i].Source < streams[j].Source
		}
		return streams[i].Kind < streams[j].Kind
	})
	return counts, streams, lastIngest, maxLag
}

func indexerStreamStatus(target model.SuiCoverageTarget, now time.Time) IndexerStreamStatus {
	stream := IndexerStreamStatus{
		Kind:                 target.Kind,
		Status:               target.Status,
		CursorID:             target.CursorID,
		Source:               target.Source,
		CursorKind:           target.CursorKind,
		Environment:          target.Environment,
		Network:              target.Network,
		Role:                 target.Role,
		PackageID:            target.PackageID,
		ModuleName:           target.ModuleName,
		EventType:            target.EventType,
		TypeName:             target.TypeName,
		TypeRepr:             target.TypeRepr,
		RowsProcessed:        target.RowsProcessed,
		LastSuccessfulIngest: target.LastSuccessfulIngest,
		LastCheckpoint:       target.LastCheckpoint,
		ErrorCount:           target.ErrorCount,
		LastErrorSummary:     target.LastErrorSummary,
		EmptyStream:          target.EmptyStream,
		ProviderRangeBlocked: target.ProviderRangeBlocked,
	}
	applyIndexerSourceFields(&stream)
	return stream
}

func classifyIndexerStatus(counts IndexerCursorCounts) (IndexerStatus, []string) {
	var reasons []string
	if counts.Total == 0 {
		return IndexerStatusDegraded, []string{"no sync cursors recorded"}
	}
	if counts.RangeBlocked > 0 {
		reasons = append(reasons, fmt.Sprintf("%d object cursor(s) are limited by the Sui provider range", counts.RangeBlocked))
	}
	if counts.Errored > 0 {
		reasons = append(reasons, fmt.Sprintf("%d cursor(s) have retryable errors", counts.Errored))
	}
	if counts.MissingIngest > 0 {
		reasons = append(reasons, fmt.Sprintf("%d cursor(s) have no successful ingest timestamp", counts.MissingIngest))
	}
	if counts.Stale > 0 {
		reasons = append(reasons, fmt.Sprintf("%d cursor(s) are stale", counts.Stale))
	}
	switch {
	case counts.Errored > 0 || counts.MissingIngest > 0:
		return IndexerStatusDegraded, reasons
	case counts.Stale > 0:
		return IndexerStatusStale, reasons
	default:
		return IndexerStatusOK, reasons
	}
}

func applyIndexerSourceFields(stream *IndexerStreamStatus) {
	parts := strings.Split(stream.Source, ":")
	if len(parts) >= 3 && parts[0] == "sui" {
		stream.Network = nonEmpty(stream.Network, parts[1])
		switch parts[2] {
		case "events":
			applyEventSourceFields(stream, parts)
		case "objects":
			applyObjectSourceFields(stream, parts)
		}
		return
	}
	if len(parts) >= 4 && parts[0] == "registry" && parts[1] == "derive" {
		stream.Network = nonEmpty(stream.Network, parts[3])
		if len(parts) >= 6 && parts[4] == "module" {
			stream.ModuleName = nonEmpty(stream.ModuleName, parts[5])
		}
	}
}

func applyEventSourceFields(stream *IndexerStreamStatus, parts []string) {
	if len(parts) < 4 {
		return
	}
	if parts[3] == "type" {
		if len(parts) >= 7 {
			stream.EventType = nonEmpty(stream.EventType, strings.Join(parts[4:len(parts)-2], ":"))
		} else if len(parts) > 4 {
			stream.EventType = nonEmpty(stream.EventType, strings.Join(parts[4:], ":"))
		}
		return
	}
	if len(parts) >= 6 {
		stream.Role = nonEmpty(stream.Role, parts[3])
		stream.PackageID = nonEmpty(stream.PackageID, parts[4])
		stream.ModuleName = nonEmpty(stream.ModuleName, parts[5])
	}
}

func applyObjectSourceFields(stream *IndexerStreamStatus, parts []string) {
	if len(parts) < 5 {
		return
	}
	stream.Role = nonEmpty(stream.Role, parts[3])
	stream.TypeRepr = nonEmpty(stream.TypeRepr, strings.Join(parts[4:], ":"))
	if stream.TypeRepr != "" {
		typeParts := strings.Split(stream.TypeRepr, "::")
		stream.TypeName = nonEmpty(stream.TypeName, typeParts[len(typeParts)-1])
		if stream.ModuleName == "" && len(typeParts) >= 2 {
			stream.ModuleName = typeParts[len(typeParts)-2]
		}
	}
}

func nonEmpty(first, fallback string) string {
	if strings.TrimSpace(first) != "" {
		return first
	}
	return fallback
}
