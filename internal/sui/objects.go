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

type ObjectTypeTarget struct {
	Environment model.Environment `json:"environment"`
	Network     string            `json:"network"`
	PackageName string            `json:"packageName"`
	PackageID   string            `json:"packageId"`
	Role        PackageRole       `json:"role"`
	ModuleName  string            `json:"moduleName"`
	TypeName    string            `json:"typeName"`
	TypeRepr    string            `json:"typeRepr"`
}

type ObjectTargetOptions struct {
	Environment     model.Environment
	Network         string
	Cycles          []int
	IncludeUncycled bool
	PackageIDs      []string
	ModuleNames     []string
	TypeNames       []string
	TypeReprs       []string
}

type ObjectBackfillOptions struct {
	Environment       model.Environment
	Network           string
	Endpoint          string
	Cycles            []int
	IncludeUncycled   bool
	First             int
	MaxPages          int
	Concurrency       int
	ResetCursors      bool
	OnlyIncomplete    bool
	PackageIDs        []string
	ModuleNames       []string
	TypeNames         []string
	TypeReprs         []string
	Targets           []ObjectTypeTarget
	AllowTargetErrors bool
}

type ObjectStore interface {
	EnsureSource(ctx context.Context, source model.Source) error
	UpsertSuiObject(ctx context.Context, object db.SuiObjectRecord) error
	GetSyncCursor(ctx context.Context, id string) (db.CursorStatus, bool, error)
	SaveSyncCursor(ctx context.Context, item db.CursorStatus) error
}

type ObjectFetcher interface {
	FetchObjects(ctx context.Context, query ObjectsQuery) (ObjectsPage, error)
}

func ObjectTypeTargets(manifest Manifest, options ObjectTargetOptions) ([]ObjectTypeTarget, error) {
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
	typeFilter := lowerSet(options.TypeNames)
	var out []ObjectTypeTarget
	seen := make(map[string]struct{})
	if len(options.TypeReprs) > 0 {
		for _, typeRepr := range options.TypeReprs {
			parts, err := parseMoveType(strings.TrimSpace(typeRepr))
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
			if len(typeFilter) > 0 {
				if _, ok := typeFilter[strings.ToLower(parts.TypeName)]; !ok {
					continue
				}
			}
			key := fmt.Sprintf("%s:%s:%s", environment, parts.PackageID, typeRepr)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, ObjectTypeTarget{
				Environment: environment,
				Network:     network,
				PackageName: "explicit",
				PackageID:   parts.PackageID,
				Role:        PackageRoleExplicit,
				ModuleName:  parts.Module,
				TypeName:    parts.TypeName,
				TypeRepr:    typeRepr,
			})
		}
		if len(out) == 0 {
			return nil, fmt.Errorf("no Sui object type targets matched the requested filters")
		}
		return out, nil
	}
	for _, pkg := range manifest.Packages {
		if pkg.Network != network {
			continue
		}
		if !packageMatchesCycleScope(pkg, options.Cycles, options.IncludeUncycled) {
			continue
		}
		roles := packageRoles(pkg)
		objectTypes := append([]ObjectTypeManifest(nil), pkg.ObjectTypes...)
		sort.Slice(objectTypes, func(i, j int) bool {
			if objectTypes[i].ModuleName == objectTypes[j].ModuleName {
				return objectTypes[i].TypeName < objectTypes[j].TypeName
			}
			return objectTypes[i].ModuleName < objectTypes[j].ModuleName
		})
		for _, item := range roles {
			if len(packageFilter) > 0 {
				if _, ok := packageFilter[strings.ToLower(item.id)]; !ok {
					continue
				}
			}
			for _, objectType := range objectTypes {
				if len(moduleFilter) > 0 {
					if _, ok := moduleFilter[strings.ToLower(objectType.ModuleName)]; !ok {
						continue
					}
				}
				if len(typeFilter) > 0 {
					if _, ok := typeFilter[strings.ToLower(objectType.TypeName)]; !ok {
						continue
					}
				}
				typeRepr := fmt.Sprintf("%s::%s::%s", item.id, objectType.ModuleName, objectType.TypeName)
				key := fmt.Sprintf("%s:%s:%s", environment, item.role, typeRepr)
				if _, ok := seen[key]; ok {
					continue
				}
				seen[key] = struct{}{}
				out = append(out, ObjectTypeTarget{
					Environment: environment,
					Network:     network,
					PackageName: pkg.Name,
					PackageID:   item.id,
					Role:        item.role,
					ModuleName:  objectType.ModuleName,
					TypeName:    objectType.TypeName,
					TypeRepr:    typeRepr,
				})
			}
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no Sui object type targets matched the requested filters")
	}
	return out, nil
}

func RunObjectBackfill(ctx context.Context, store ObjectStore, fetcher ObjectFetcher, manifest Manifest, options ObjectBackfillOptions) (BackfillSummary, error) {
	if store == nil {
		return BackfillSummary{}, fmt.Errorf("object store is required")
	}
	if fetcher == nil {
		return BackfillSummary{}, fmt.Errorf("object fetcher is required")
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
	targets := append([]ObjectTypeTarget(nil), options.Targets...)
	var err error
	if len(targets) == 0 {
		targets, err = ObjectTypeTargets(manifest, ObjectTargetOptions{
			Environment:     options.Environment,
			Network:         options.Network,
			Cycles:          options.Cycles,
			IncludeUncycled: options.IncludeUncycled,
			PackageIDs:      options.PackageIDs,
			ModuleNames:     options.ModuleNames,
			TypeNames:       options.TypeNames,
			TypeReprs:       options.TypeReprs,
		})
		if err != nil {
			return BackfillSummary{}, err
		}
	}
	plannedTargets := len(targets)
	if options.OnlyIncomplete && !options.ResetCursors {
		targets, err = incompleteObjectTargets(ctx, store, targets)
		if err != nil {
			return BackfillSummary{}, err
		}
	}
	source := ObjectSourceForNetwork(options.Network, options.Endpoint, options.Environment)
	if err := store.EnsureSource(ctx, source); err != nil {
		return BackfillSummary{}, err
	}
	startedAt := time.Now().UTC()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	targetCh := make(chan ObjectTypeTarget)
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
	var firstErr error
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for target := range targetCh {
				result, err := backfillObjectTarget(ctx, store, fetcher, source, target, options)
				mu.Lock()
				summary.PagesFetched += result.PagesFetched
				summary.ObjectsProcessed += result.ObjectsProcessed
				summary.Errors += result.Errors
				if err != nil && firstErr == nil {
					firstErr = err
				}
				mu.Unlock()
				if err != nil {
					continue
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
	if firstErr != nil && !options.AllowTargetErrors {
		return summary, firstErr
	}
	select {
	case err := <-errCh:
		return summary, err
	default:
		return summary, nil
	}
}

func incompleteObjectTargets(ctx context.Context, store ObjectStore, targets []ObjectTypeTarget) ([]ObjectTypeTarget, error) {
	out := make([]ObjectTypeTarget, 0, len(targets))
	for _, target := range targets {
		cursorStatus, ok, err := store.GetSyncCursor(ctx, ObjectCursorID(target))
		if err != nil {
			return nil, err
		}
		if !ok || hasRetriableObjectCursorError(cursorStatus) {
			out = append(out, target)
		}
	}
	return out, nil
}

func hasRetriableObjectCursorError(cursor db.CursorStatus) bool {
	return hasActiveCursorError(cursor) && !hasProviderRangeBlockedCursorError("object", cursor)
}

type objectTargetResult struct {
	PagesFetched     int64
	ObjectsProcessed int64
	Errors           int64
}

func backfillObjectTarget(ctx context.Context, store ObjectStore, fetcher ObjectFetcher, source model.Source, target ObjectTypeTarget, options ObjectBackfillOptions) (objectTargetResult, error) {
	cursorID := ObjectCursorID(target)
	cursorStatus := db.CursorStatus{
		ID:          cursorID,
		Source:      ObjectCursorSource(target),
		Environment: target.Environment,
		CursorKind:  "sui_object",
		UpdatedAt:   time.Now().UTC(),
	}
	after := ""
	if !options.ResetCursors {
		saved, ok, err := store.GetSyncCursor(ctx, cursorID)
		if err != nil {
			return objectTargetResult{Errors: 1}, err
		}
		if ok {
			cursorStatus = saved
			after = saved.CursorValue
		}
	}
	var result objectTargetResult
	for page := 1; ; page++ {
		objects, err := fetcher.FetchObjects(ctx, ObjectsQuery{
			Type:  target.TypeRepr,
			After: after,
			First: options.First,
		})
		if err != nil {
			result.Errors++
			cursorStatus.ErrorCount++
			cursorStatus.LastErrorSummary = err.Error()
			_ = store.SaveSyncCursor(ctx, cursorStatus)
			return result, err
		}
		result.PagesFetched++
		for _, node := range objects.Nodes {
			record, err := NormalizeMoveObject(node, NormalizeOptions{
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
			if err := store.UpsertSuiObject(ctx, record); err != nil {
				result.Errors++
				cursorStatus.ErrorCount++
				cursorStatus.LastErrorSummary = err.Error()
				_ = store.SaveSyncCursor(ctx, cursorStatus)
				return result, err
			}
			result.ObjectsProcessed++
			cursorStatus.EventsProcessed++
		}
		if objects.EndCursor != "" {
			after = objects.EndCursor
			cursorStatus.CursorValue = objects.EndCursor
		}
		now := time.Now().UTC()
		cursorStatus.LastSuccessfulIngest = &now
		cursorStatus.LastErrorSummary = ""
		if err := store.SaveSyncCursor(ctx, cursorStatus); err != nil {
			return result, err
		}
		reachedMaxPages := options.MaxPages > 0 && page >= options.MaxPages
		if !objects.HasNextPage || reachedMaxPages {
			break
		}
		if objects.EndCursor == "" {
			err := fmt.Errorf("sui GraphQL reported another object page for %s without an end cursor", target.TypeRepr)
			result.Errors++
			cursorStatus.ErrorCount++
			cursorStatus.LastErrorSummary = err.Error()
			_ = store.SaveSyncCursor(ctx, cursorStatus)
			return result, err
		}
	}
	return result, nil
}

func ObjectCursorID(target ObjectTypeTarget) string {
	return "cursor:" + ObjectCursorSource(target)
}

func ObjectCursorSource(target ObjectTypeTarget) string {
	return fmt.Sprintf("sui:%s:objects:%s:%s", target.Network, target.Role, target.TypeRepr)
}

func ObjectSourceForNetwork(network, endpoint string, environment model.Environment) model.Source {
	return model.Source{
		ID:          fmt.Sprintf("source:sui:%s:graphql:objects", network),
		Kind:        model.SourceKindSuiObject,
		Title:       fmt.Sprintf("Sui %s GraphQL objects", network),
		Locator:     endpoint,
		URL:         endpoint,
		Environment: environment,
		Metadata: map[string]any{
			"network": network,
		},
		CreatedAt: time.Now().UTC(),
	}
}
