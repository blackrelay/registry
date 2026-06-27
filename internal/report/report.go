package report

import (
	"context"
	"time"

	"github.com/blackrelay/registry/internal/db"
	"github.com/blackrelay/registry/internal/killmail"
	"github.com/blackrelay/registry/internal/model"
	"github.com/blackrelay/registry/internal/resolver"
)

type Options struct {
	Environment      model.Environment
	KillmailPageSize int
	ExcludeFixtures  bool
	Now              func() time.Time
}

type Report struct {
	SchemaVersion        string                     `json:"schemaVersion"`
	Environment          model.Environment          `json:"environment,omitempty"`
	GeneratedAt          time.Time                  `json:"generatedAt"`
	ExcludeFixtures      bool                       `json:"excludeFixtures,omitempty"`
	Counts               db.RegistryRowCounts       `json:"counts"`
	EntitiesByType       map[model.EntityType]int64 `json:"entitiesByType"`
	EventsByModule       map[string]int64           `json:"eventsByModule"`
	SuiObjectsByType     map[string]int64           `json:"suiObjectsByType"`
	RelationsByPredicate map[string]int64           `json:"relationsByPredicate"`
	Killmails            KillmailResolutionCounts   `json:"killmails"`
	SourceGaps           []model.SourceGap          `json:"sourceGaps,omitempty"`
}

type KillmailResolutionCounts = db.KillmailResolutionCounts

type Store interface {
	CountRegistryRows(ctx context.Context, environment model.Environment) (db.RegistryCountSnapshot, error)
	ListKillmailRaw(ctx context.Context, query db.KillmailQuery) ([]model.KillmailRaw, string, error)
	ListSourceGaps(ctx context.Context, environment model.Environment) ([]model.SourceGap, error)
	resolver.Store
	killmail.GraphStore
}

type killmailResolutionCounter interface {
	CountKillmailResolution(ctx context.Context, environment model.Environment) (db.KillmailResolutionCounts, error)
}

type filteredKillmailResolutionCounter interface {
	CountKillmailResolutionFiltered(ctx context.Context, query db.KillmailQuery) (db.KillmailResolutionCounts, error)
}

func Build(ctx context.Context, store Store, options Options) (Report, error) {
	if options.Environment == "" {
		options.Environment = model.EnvironmentStillness
	}
	now := time.Now().UTC()
	if options.Now != nil {
		now = options.Now().UTC()
	}
	counts, err := store.CountRegistryRows(ctx, options.Environment)
	if err != nil {
		return Report{}, err
	}
	killmails, err := countKillmailResolution(ctx, store, options)
	if err != nil {
		return Report{}, err
	}
	sourceGaps, err := store.ListSourceGaps(ctx, options.Environment)
	if err != nil {
		return Report{}, err
	}
	return Report{
		SchemaVersion:        "registry.report.v1",
		Environment:          options.Environment,
		GeneratedAt:          now,
		ExcludeFixtures:      options.ExcludeFixtures,
		Counts:               counts.Counts,
		EntitiesByType:       counts.EntitiesByType,
		EventsByModule:       counts.EventsByModule,
		SuiObjectsByType:     counts.SuiObjectsByType,
		RelationsByPredicate: counts.RelationsByPredicate,
		Killmails:            killmails,
		SourceGaps:           sourceGaps,
	}, nil
}

func countKillmailResolution(ctx context.Context, store Store, options Options) (KillmailResolutionCounts, error) {
	if options.ExcludeFixtures {
		if counter, ok := store.(filteredKillmailResolutionCounter); ok {
			return counter.CountKillmailResolutionFiltered(ctx, db.KillmailQuery{
				Environment:     options.Environment,
				ExcludeFixtures: true,
			})
		}
	} else if counter, ok := store.(killmailResolutionCounter); ok {
		return counter.CountKillmailResolution(ctx, options.Environment)
	}
	pageSize := options.KillmailPageSize
	if pageSize <= 0 {
		pageSize = 200
	}
	if pageSize > 200 {
		pageSize = 200
	}
	service := killmail.Service{Resolver: resolver.Resolver{Store: store}, GraphStore: store}
	cursor := ""
	var counts KillmailResolutionCounts
	for {
		items, next, err := store.ListKillmailRaw(ctx, db.KillmailQuery{
			Environment:     options.Environment,
			ExcludeFixtures: options.ExcludeFixtures,
			Limit:           pageSize,
			Cursor:          cursor,
		})
		if err != nil {
			return counts, err
		}
		for _, item := range items {
			semantic := service.Semantic(ctx, item)
			counts.Total++
			countResolved(&counts.ResolvedSystems, &counts.UnresolvedSystems, semantic.System)
			countResolved(&counts.ResolvedVictims, &counts.UnresolvedVictims, semantic.Victim)
			countResolved(&counts.ResolvedKillers, &counts.UnresolvedKillers, semantic.Killer)
			countResolved(&counts.ResolvedReporters, &counts.UnresolvedReporters, semantic.Reporter)
			switch {
			case isResolved(semantic.Killer) && semantic.Killer.EntityType == model.EntityTypeEnemy:
				counts.NPCKillers++
			case isResolved(semantic.Killer) && semantic.Killer.EntityType == model.EntityTypeCharacter:
				counts.CharacterKillers++
			}
		}
		if next == "" || len(items) == 0 {
			break
		}
		cursor = next
	}
	return counts, nil
}

func countResolved(resolved, unresolved *int64, value model.ResolvedValue) {
	if isResolved(value) {
		(*resolved)++
		return
	}
	(*unresolved)++
}

func isResolved(value model.ResolvedValue) bool {
	return value.EntityID != "" && value.Confidence != model.ConfidenceUnknown
}
