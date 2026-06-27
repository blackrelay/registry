package sui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/blackrelay/registry/internal/db"
	"github.com/blackrelay/registry/internal/model"
)

type PackageRole string

const (
	PackageRoleOriginal  PackageRole = "original"
	PackageRolePublished PackageRole = "published"
	PackageRoleExplicit  PackageRole = "explicit"
)

type EventStreamTarget struct {
	Environment      model.Environment `json:"environment"`
	Network          string            `json:"network"`
	PackageName      string            `json:"packageName"`
	PackageID        string            `json:"packageId"`
	Role             PackageRole       `json:"role"`
	ModuleName       string            `json:"moduleName,omitempty"`
	EventType        string            `json:"eventType,omitempty"`
	CheckpointAfter  *uint64           `json:"checkpointAfter,omitempty"`
	CheckpointBefore *uint64           `json:"checkpointBefore,omitempty"`
}

type TargetOptions struct {
	Environment         model.Environment
	Network             string
	Cycles              []int
	IncludeUncycled     bool
	PackageIDs          []string
	ModuleNames         []string
	EventTypes          []string
	CheckpointFrom      uint64
	CheckpointTo        uint64
	CheckpointShardSize uint64
	NoModuleShards      bool
}

type BackfillOptions struct {
	Environment         model.Environment
	Network             string
	Endpoint            string
	Cycles              []int
	IncludeUncycled     bool
	First               int
	MaxPages            int
	Concurrency         int
	ResetCursors        bool
	OnlyIncomplete      bool
	PackageIDs          []string
	ModuleNames         []string
	EventTypes          []string
	CheckpointFrom      uint64
	CheckpointTo        uint64
	CheckpointShardSize uint64
	NoModuleShards      bool
}

type EventStore interface {
	EnsureSource(ctx context.Context, source model.Source) error
	UpsertSuiEvent(ctx context.Context, event db.EventRecord) error
	GetSyncCursor(ctx context.Context, id string) (db.CursorStatus, bool, error)
	SaveSyncCursor(ctx context.Context, item db.CursorStatus) error
}

type EventFetcher interface {
	FetchEvents(ctx context.Context, query EventsQuery) (EventsPage, error)
}

type BackfillSummary struct {
	Environment      model.Environment `json:"environment"`
	Network          string            `json:"network"`
	Endpoint         string            `json:"endpoint"`
	Targets          int               `json:"targets"`
	PagesFetched     int64             `json:"pagesFetched"`
	EventsProcessed  int64             `json:"eventsProcessed"`
	ObjectsProcessed int64             `json:"objectsProcessed"`
	Errors           int64             `json:"errors"`
	SkippedTargets   int               `json:"skippedTargets"`
	StartedAt        time.Time         `json:"startedAt"`
	FinishedAt       time.Time         `json:"finishedAt"`
}

func EventStreamTargets(manifest Manifest, options TargetOptions) ([]EventStreamTarget, error) {
	if err := manifest.Validate(); err != nil {
		return nil, err
	}
	environment := options.Environment
	if environment == "" {
		environment = model.EnvironmentStillness
	}
	network := options.Network
	if network == "" {
		network = "sui-testnet"
	}
	packageFilter := lowerSet(options.PackageIDs)
	moduleFilter := lowerSet(options.ModuleNames)
	if len(options.EventTypes) > 0 {
		return eventTypeTargets(environment, network, packageFilter, moduleFilter, options)
	}
	var out []EventStreamTarget
	seen := make(map[string]struct{})
	for _, pkg := range manifest.Packages {
		if pkg.Network != network {
			continue
		}
		if !packageMatchesCycleScope(pkg, options.Cycles, options.IncludeUncycled) {
			continue
		}
		checkpointAfter, checkpointBefore := packageCheckpoints(pkg, options)
		roles := packageRoles(pkg)
		for _, item := range roles {
			if len(packageFilter) > 0 {
				if _, ok := packageFilter[strings.ToLower(item.id)]; !ok {
					continue
				}
			}
			modules := []string{""}
			if !options.NoModuleShards {
				modules = append([]string(nil), pkg.Modules...)
				sort.Strings(modules)
			}
			for _, module := range modules {
				if module != "" && len(moduleFilter) > 0 {
					if _, ok := moduleFilter[strings.ToLower(module)]; !ok {
						continue
					}
				}
				key := fmt.Sprintf("%s:%s:%s:%s:%s:%s", environment, item.role, item.id, module, pointerValue(checkpointAfter), pointerValue(checkpointBefore))
				if _, ok := seen[key]; ok {
					continue
				}
				seen[key] = struct{}{}
				out = append(out, EventStreamTarget{
					Environment:      environment,
					Network:          network,
					PackageName:      pkg.Name,
					PackageID:        item.id,
					Role:             item.role,
					ModuleName:       module,
					CheckpointAfter:  checkpointAfter,
					CheckpointBefore: checkpointBefore,
				})
			}
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no Sui event stream targets matched the requested filters")
	}
	return out, nil
}

func packageMatchesCycleScope(pkg PackageManifest, values []int, includeUncycled bool) bool {
	if len(values) == 0 {
		return true
	}
	if pkg.Cycle == nil {
		return includeUncycled
	}
	for _, value := range values {
		if value == *pkg.Cycle {
			return true
		}
	}
	return false
}

type packageRoleTarget struct {
	role PackageRole
	id   string
}

func packageRoles(pkg PackageManifest) []packageRoleTarget {
	roles := []packageRoleTarget{{role: PackageRoleOriginal, id: pkg.OriginalPackageID}}
	if !strings.EqualFold(pkg.OriginalPackageID, pkg.PublishedPackageID) {
		roles = append(roles, packageRoleTarget{role: PackageRolePublished, id: pkg.PublishedPackageID})
	}
	return roles
}

func packageCheckpoints(pkg PackageManifest, options TargetOptions) (*uint64, *uint64) {
	var after *uint64
	switch {
	case options.CheckpointFrom > 0:
		value := options.CheckpointFrom - 1
		after = &value
	case pkg.StartingCheckpoint > 0:
		value := pkg.StartingCheckpoint - 1
		after = &value
	}
	var before *uint64
	if options.CheckpointTo > 0 {
		value := options.CheckpointTo + 1
		before = &value
	}
	return after, before
}

func eventTypeTargets(environment model.Environment, network string, packageFilter, moduleFilter map[string]struct{}, options TargetOptions) ([]EventStreamTarget, error) {
	var out []EventStreamTarget
	seen := make(map[string]struct{})
	for _, eventType := range options.EventTypes {
		eventType = strings.TrimSpace(eventType)
		if eventType == "" {
			continue
		}
		parts, err := parseMoveType(eventType)
		if err != nil {
			return nil, err
		}
		if len(packageFilter) > 0 {
			if _, ok := packageFilter[strings.ToLower(parts.PackageID)]; !ok {
				continue
			}
		}
		if len(moduleFilter) > 0 {
			if _, ok := moduleFilter[strings.ToLower(parts.Module)]; !ok {
				continue
			}
		}
		ranges := checkpointRanges(options.CheckpointFrom, options.CheckpointTo, options.CheckpointShardSize)
		for _, checkpointRange := range ranges {
			key := fmt.Sprintf("%s:%s:%s:%s:%s", environment, parts.PackageID, eventType, pointerValue(checkpointRange.after), pointerValue(checkpointRange.before))
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, EventStreamTarget{
				Environment:      environment,
				Network:          network,
				PackageName:      "explicit-event-type",
				PackageID:        parts.PackageID,
				Role:             PackageRoleExplicit,
				ModuleName:       parts.Module,
				EventType:        eventType,
				CheckpointAfter:  checkpointRange.after,
				CheckpointBefore: checkpointRange.before,
			})
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no Sui event type targets matched the requested filters")
	}
	return out, nil
}

type checkpointRange struct {
	after  *uint64
	before *uint64
}

func checkpointRanges(from, to, size uint64) []checkpointRange {
	if size == 0 || to <= from {
		return []checkpointRange{{}}
	}
	var out []checkpointRange
	for start := from; start <= to; {
		end := start + size - 1
		if end < start || end > to {
			end = to
		}
		var after *uint64
		if start > 0 {
			value := start - 1
			after = &value
		}
		beforeValue := end + 1
		before := &beforeValue
		out = append(out, checkpointRange{after: after, before: before})
		if end == to {
			break
		}
		start = end + 1
	}
	return out
}

func RunEventBackfill(ctx context.Context, store EventStore, fetcher EventFetcher, manifest Manifest, options BackfillOptions) (BackfillSummary, error) {
	if store == nil {
		return BackfillSummary{}, fmt.Errorf("event store is required")
	}
	if fetcher == nil {
		return BackfillSummary{}, fmt.Errorf("event fetcher is required")
	}
	if options.Environment == "" {
		options.Environment = model.EnvironmentStillness
	}
	if options.Network == "" {
		options.Network = "sui-testnet"
	}
	if options.First <= 0 {
		options.First = 50
	}
	if options.Concurrency <= 0 {
		options.Concurrency = 8
	}
	targets, err := EventStreamTargets(manifest, TargetOptions{
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
		return BackfillSummary{}, err
	}
	plannedTargets := len(targets)
	if options.OnlyIncomplete && !options.ResetCursors {
		targets, err = incompleteEventTargets(ctx, store, targets)
		if err != nil {
			return BackfillSummary{}, err
		}
	}
	source := sourceForNetwork(options.Network, options.Endpoint, options.Environment)
	if err := store.EnsureSource(ctx, source); err != nil {
		return BackfillSummary{}, err
	}
	startedAt := time.Now().UTC()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	targetCh := make(chan EventStreamTarget)
	errCh := make(chan error, 1)
	var mu sync.Mutex
	summary := BackfillSummary{
		Environment:    options.Environment,
		Network:        options.Network,
		Endpoint:       options.Endpoint,
		Targets:        len(targets),
		SkippedTargets: plannedTargets - len(targets),
		StartedAt:      startedAt,
	}
	workerCount := options.Concurrency
	if workerCount > len(targets) {
		workerCount = len(targets)
	}
	var wg sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for target := range targetCh {
				result, err := backfillTarget(ctx, store, fetcher, source, target, options)
				mu.Lock()
				summary.PagesFetched += result.PagesFetched
				summary.EventsProcessed += result.EventsProcessed
				summary.Errors += result.Errors
				mu.Unlock()
				if err != nil {
					select {
					case errCh <- err:
						cancel()
					default:
					}
					return
				}
			}
		}()
	}
sendLoop:
	for _, target := range targets {
		select {
		case <-ctx.Done():
			break sendLoop
		case targetCh <- target:
		}
	}
	close(targetCh)
	wg.Wait()
	summary.FinishedAt = time.Now().UTC()
	select {
	case err := <-errCh:
		return summary, err
	default:
		return summary, nil
	}
}

func incompleteEventTargets(ctx context.Context, store EventStore, targets []EventStreamTarget) ([]EventStreamTarget, error) {
	out := make([]EventStreamTarget, 0, len(targets))
	for _, target := range targets {
		cursorStatus, ok, err := store.GetSyncCursor(ctx, EventCursorID(target))
		if err != nil {
			return nil, err
		}
		if !ok || hasActiveCursorError(cursorStatus) {
			out = append(out, target)
		}
	}
	return out, nil
}

type targetResult struct {
	PagesFetched    int64
	EventsProcessed int64
	Errors          int64
}

func backfillTarget(ctx context.Context, store EventStore, fetcher EventFetcher, source model.Source, target EventStreamTarget, options BackfillOptions) (targetResult, error) {
	cursorID := EventCursorID(target)
	cursorStatus := db.CursorStatus{
		ID:          cursorID,
		Source:      EventCursorSource(target),
		Environment: target.Environment,
		CursorKind:  "sui_event",
		UpdatedAt:   time.Now().UTC(),
	}
	after := ""
	if !options.ResetCursors {
		saved, ok, err := store.GetSyncCursor(ctx, cursorID)
		if err != nil {
			return targetResult{Errors: 1}, err
		}
		if ok {
			cursorStatus = saved
			after = saved.CursorValue
		}
	}
	var result targetResult
	for page := 1; ; page++ {
		events, err := fetcher.FetchEvents(ctx, EventsQuery{
			PackageID:        target.PackageID,
			ModuleName:       target.ModuleName,
			EventType:        target.EventType,
			AfterCheckpoint:  target.CheckpointAfter,
			BeforeCheckpoint: target.CheckpointBefore,
			After:            after,
			First:            options.First,
		})
		if err != nil {
			result.Errors++
			cursorStatus.ErrorCount++
			cursorStatus.LastErrorSummary = err.Error()
			_ = store.SaveSyncCursor(ctx, cursorStatus)
			return result, err
		}
		result.PagesFetched++
		for _, node := range events.Nodes {
			record, err := NormalizeMoveEvent(node, NormalizeOptions{
				Environment: target.Environment,
				SourceID:    source.ID,
				FetchedAt:   time.Now().UTC(),
			})
			if err != nil {
				result.Errors++
				cursorStatus.ErrorCount++
				cursorStatus.LastErrorSummary = err.Error()
				_ = store.SaveSyncCursor(ctx, cursorStatus)
				return result, err
			}
			if err := store.UpsertSuiEvent(ctx, record); err != nil {
				result.Errors++
				cursorStatus.ErrorCount++
				cursorStatus.LastErrorSummary = err.Error()
				_ = store.SaveSyncCursor(ctx, cursorStatus)
				return result, err
			}
			result.EventsProcessed++
			cursorStatus.EventsProcessed++
		}
		if events.EndCursor != "" {
			after = events.EndCursor
			cursorStatus.CursorValue = events.EndCursor
		}
		now := time.Now().UTC()
		cursorStatus.LastSuccessfulIngest = &now
		cursorStatus.LastErrorSummary = ""
		if err := store.SaveSyncCursor(ctx, cursorStatus); err != nil {
			return result, err
		}
		reachedMaxPages := options.MaxPages > 0 && page >= options.MaxPages
		if !events.HasNextPage || reachedMaxPages {
			break
		}
		if events.EndCursor == "" {
			err := fmt.Errorf("sui GraphQL reported another page for %s without an end cursor", target.PackageID)
			result.Errors++
			cursorStatus.ErrorCount++
			cursorStatus.LastErrorSummary = err.Error()
			_ = store.SaveSyncCursor(ctx, cursorStatus)
			return result, err
		}
	}
	return result, nil
}

func EventCursorID(target EventStreamTarget) string {
	return "cursor:" + EventCursorSource(target)
}

func EventCursorSource(target EventStreamTarget) string {
	if target.EventType != "" {
		return fmt.Sprintf("sui:%s:events:type:%s:%s:%s", target.Network, target.EventType, pointerValue(target.CheckpointAfter), pointerValue(target.CheckpointBefore))
	}
	module := target.ModuleName
	if module == "" {
		module = "*"
	}
	if target.CheckpointAfter != nil || target.CheckpointBefore != nil {
		return fmt.Sprintf("sui:%s:events:%s:%s:%s:%s:%s", target.Network, target.Role, target.PackageID, module, pointerValue(target.CheckpointAfter), pointerValue(target.CheckpointBefore))
	}
	return fmt.Sprintf("sui:%s:events:%s:%s:%s", target.Network, target.Role, target.PackageID, module)
}

func pointerValue(value *uint64) string {
	if value == nil {
		return "*"
	}
	return fmt.Sprint(*value)
}

func sourceForNetwork(network, endpoint string, environment model.Environment) model.Source {
	return model.Source{
		ID:          fmt.Sprintf("source:sui:%s:graphql", network),
		Kind:        model.SourceKindSuiEvent,
		Title:       fmt.Sprintf("Sui %s GraphQL", network),
		Locator:     endpoint,
		URL:         endpoint,
		Environment: environment,
		Metadata: map[string]any{
			"network": network,
		},
		CreatedAt: time.Now().UTC(),
	}
}

func lowerSet(values []string) map[string]struct{} {
	out := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		out[strings.ToLower(value)] = struct{}{}
	}
	return out
}
