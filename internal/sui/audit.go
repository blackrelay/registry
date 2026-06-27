package sui

import (
	"context"
	"errors"
	"strings"

	"github.com/blackrelay/registry/internal/db"
	"github.com/blackrelay/registry/internal/model"
)

var ErrCursorStoreRequired = errors.New("cursor store is required")

type CoverageAuditOptions struct {
	Environment         model.Environment
	Network             string
	Cycles              []int
	IncludeUncycled     bool
	MaxPages            int
	PackageIDs          []string
	ModuleNames         []string
	EventTypes          []string
	ObjectTypeNames     []string
	ObjectTypeReprs     []string
	CheckpointFrom      uint64
	CheckpointTo        uint64
	CheckpointShardSize uint64
	NoModuleShards      bool
}

type CursorReader interface {
	GetSyncCursor(ctx context.Context, id string) (db.CursorStatus, bool, error)
}

func AuditCoverage(ctx context.Context, store CursorReader, manifest Manifest, options CoverageAuditOptions) (model.SuiCoverageSummary, error) {
	if store == nil {
		return model.SuiCoverageSummary{}, ErrCursorStoreRequired
	}
	if options.Environment == "" {
		options.Environment = model.EnvironmentStillness
	}
	if options.Network == "" {
		options.Network = "sui-testnet"
	}
	eventTargets, err := EventStreamTargets(manifest, TargetOptions{
		Environment:         options.Environment,
		Network:             options.Network,
		Cycles:              options.Cycles,
		IncludeUncycled:     options.IncludeUncycled,
		PackageIDs:          options.PackageIDs,
		ModuleNames:         options.ModuleNames,
		EventTypes:          options.EventTypes,
		CheckpointFrom:      options.CheckpointFrom,
		CheckpointTo:        options.CheckpointTo,
		CheckpointShardSize: options.CheckpointShardSize,
		NoModuleShards:      options.NoModuleShards,
	})
	if err != nil {
		return model.SuiCoverageSummary{}, err
	}
	objectTargets, err := ObjectTypeTargets(manifest, ObjectTargetOptions{
		Environment:     options.Environment,
		Network:         options.Network,
		Cycles:          options.Cycles,
		IncludeUncycled: options.IncludeUncycled,
		PackageIDs:      options.PackageIDs,
		ModuleNames:     options.ModuleNames,
		TypeNames:       options.ObjectTypeNames,
		TypeReprs:       options.ObjectTypeReprs,
	})
	if err != nil {
		return model.SuiCoverageSummary{}, err
	}
	summary := model.SuiCoverageSummary{
		Environment:        options.Environment,
		Network:            options.Network,
		CoverageBasis:      "manifest_cursor_audit",
		FullCoverageProven: false,
		EventTargets:       len(eventTargets),
		ObjectTargets:      len(objectTargets),
	}
	for _, target := range eventTargets {
		cursorID := EventCursorID(target)
		cursorStatus, ok, err := store.GetSyncCursor(ctx, cursorID)
		if err != nil {
			return model.SuiCoverageSummary{}, err
		}
		item := model.SuiCoverageTarget{
			Kind:             "event",
			CursorID:         cursorID,
			Source:           EventCursorSource(target),
			Environment:      target.Environment,
			Network:          target.Network,
			PackageName:      target.PackageName,
			PackageID:        target.PackageID,
			Role:             string(target.Role),
			ModuleName:       target.ModuleName,
			EventType:        target.EventType,
			CheckpointAfter:  target.CheckpointAfter,
			CheckpointBefore: target.CheckpointBefore,
		}
		applyCursorCoverage(&item, cursorStatus, ok, options.MaxPages)
		addCoverageTarget(&summary, item)
	}
	for _, target := range objectTargets {
		cursorID := ObjectCursorID(target)
		cursorStatus, ok, err := store.GetSyncCursor(ctx, cursorID)
		if err != nil {
			return model.SuiCoverageSummary{}, err
		}
		item := model.SuiCoverageTarget{
			Kind:        "object",
			CursorID:    cursorID,
			Source:      ObjectCursorSource(target),
			Environment: target.Environment,
			Network:     target.Network,
			PackageName: target.PackageName,
			PackageID:   target.PackageID,
			Role:        string(target.Role),
			ModuleName:  target.ModuleName,
			TypeName:    target.TypeName,
			TypeRepr:    target.TypeRepr,
		}
		applyCursorCoverage(&item, cursorStatus, ok, options.MaxPages)
		addCoverageTarget(&summary, item)
	}
	return summary, nil
}

func ProviderRangeBlockedObjectTargets(ctx context.Context, store CursorReader, manifest Manifest, options CoverageAuditOptions) ([]ObjectTypeTarget, error) {
	if store == nil {
		return nil, ErrCursorStoreRequired
	}
	if options.Environment == "" {
		options.Environment = model.EnvironmentStillness
	}
	if options.Network == "" {
		options.Network = "sui-testnet"
	}
	targets, err := ObjectTypeTargets(manifest, ObjectTargetOptions{
		Environment:     options.Environment,
		Network:         options.Network,
		Cycles:          options.Cycles,
		IncludeUncycled: options.IncludeUncycled,
		PackageIDs:      options.PackageIDs,
		ModuleNames:     options.ModuleNames,
		TypeNames:       options.ObjectTypeNames,
		TypeReprs:       options.ObjectTypeReprs,
	})
	if err != nil {
		return nil, err
	}
	out := make([]ObjectTypeTarget, 0)
	for _, target := range targets {
		cursorStatus, ok, err := store.GetSyncCursor(ctx, ObjectCursorID(target))
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		if hasProviderRangeBlockedCursorError("object", cursorStatus) {
			out = append(out, target)
		}
	}
	return out, nil
}

func CursorCoverageSummary(cursors []db.CursorStatus) model.SuiCoverageSummary {
	summary := model.SuiCoverageSummary{
		CoverageBasis:      "cursor_table",
		FullCoverageProven: false,
	}
	for _, cursor := range cursors {
		item := model.SuiCoverageTarget{
			Kind:                 cursorCoverageKind(cursor),
			CursorID:             cursor.ID,
			Source:               cursor.Source,
			CursorKind:           cursor.CursorKind,
			Environment:          cursor.Environment,
			Network:              cursorCoverageNetwork(cursor.Source),
			RowsProcessed:        cursor.EventsProcessed,
			LastSuccessfulIngest: cursor.LastSuccessfulIngest,
			LastCheckpoint:       cursor.LastCheckpoint,
			ErrorCount:           cursor.ErrorCount,
			LastErrorSummary:     cursor.LastErrorSummary,
			UpdatedAt:            cursor.UpdatedAt,
			EmptyStream:          cursor.LastSuccessfulIngest != nil && cursor.EventsProcessed == 0 && cursor.LastErrorSummary == "",
		}
		if hasProviderRangeBlockedCursorError(item.Kind, cursor) {
			item.Status = model.CoverageStatusRangeBlocked
			item.ProviderRangeBlocked = true
		} else if hasActiveCursorError(cursor) {
			item.Status = model.CoverageStatusErrored
		} else {
			item.Status = model.CoverageStatusIndexed
		}
		addCoverageTarget(&summary, item)
	}
	return summary
}

func applyCursorCoverage(item *model.SuiCoverageTarget, cursor db.CursorStatus, ok bool, maxPages int) {
	if !ok {
		item.Status = model.CoverageStatusNotSeen
		item.MissingCursor = true
		return
	}
	item.CursorKind = cursor.CursorKind
	item.RowsProcessed = cursor.EventsProcessed
	item.LastSuccessfulIngest = cursor.LastSuccessfulIngest
	item.LastCheckpoint = cursor.LastCheckpoint
	item.ErrorCount = cursor.ErrorCount
	item.LastErrorSummary = cursor.LastErrorSummary
	item.UpdatedAt = cursor.UpdatedAt
	item.EmptyStream = cursor.LastSuccessfulIngest != nil && cursor.EventsProcessed == 0 && cursor.LastErrorSummary == ""
	switch {
	case hasProviderRangeBlockedCursorError(item.Kind, cursor):
		item.Status = model.CoverageStatusRangeBlocked
		item.ProviderRangeBlocked = true
	case hasActiveCursorError(cursor):
		item.Status = model.CoverageStatusErrored
	case maxPages > 0:
		item.Status = model.CoverageStatusLimited
		item.LimitedByMaxPages = true
	default:
		item.Status = model.CoverageStatusIndexed
	}
}

func addCoverageTarget(s *model.SuiCoverageSummary, item model.SuiCoverageTarget) {
	s.Targets = append(s.Targets, item)
	s.TargetCount++
	s.RowsProcessed += item.RowsProcessed
	if item.Environment != "" {
		if s.Environment == "" {
			s.Environment = item.Environment
		} else if s.Environment != item.Environment {
			s.Environment = model.EnvironmentUnknown
		}
	}
	if item.Network != "" {
		if s.Network == "" {
			s.Network = item.Network
		} else if s.Network != item.Network {
			s.Network = "mixed"
		}
	}
	if item.LastSuccessfulIngest != nil && (s.LastSuccessfulIngest == nil || item.LastSuccessfulIngest.After(*s.LastSuccessfulIngest)) {
		s.LastSuccessfulIngest = item.LastSuccessfulIngest
	}
	switch item.Kind {
	case "event":
		if s.CoverageBasis == "cursor_table" {
			s.EventTargets++
		}
	case "object":
		if s.CoverageBasis == "cursor_table" {
			s.ObjectTargets++
		}
	case "derivation":
		s.DerivationTargets++
	}
	switch item.Status {
	case model.CoverageStatusIndexed:
		s.IndexedTargets++
	case model.CoverageStatusErrored:
		s.ErroredTargets++
	case model.CoverageStatusLimited:
		s.LimitedTargets++
	case model.CoverageStatusNotSeen:
		s.NotSeenTargets++
	case model.CoverageStatusRangeBlocked:
		s.RangeBlockedTargets++
	}
}

func hasActiveCursorError(cursor db.CursorStatus) bool {
	return strings.TrimSpace(cursor.LastErrorSummary) != ""
}

func hasProviderRangeBlockedCursorError(kind string, cursor db.CursorStatus) bool {
	if kind != "object" && cursor.CursorKind != "sui_object" && !strings.Contains(cursor.Source, ":objects:") {
		return false
	}
	return isSuiProviderRangeBlockedError(cursor.LastErrorSummary)
}

func isSuiProviderRangeBlockedError(message string) bool {
	return strings.Contains(strings.ToLower(message), "outside consistent range")
}

func cursorCoverageKind(cursor db.CursorStatus) string {
	switch {
	case strings.Contains(cursor.CursorKind, "derivation"):
		return "derivation"
	case cursor.CursorKind == "sui_object" || strings.Contains(cursor.Source, ":objects:"):
		return "object"
	case cursor.CursorKind == "sui_event" || strings.Contains(cursor.Source, ":events:"):
		return "event"
	default:
		return "cursor"
	}
}

func cursorCoverageNetwork(source string) string {
	parts := strings.Split(source, ":")
	if len(parts) >= 2 && parts[0] == "sui" {
		return parts[1]
	}
	return ""
}
