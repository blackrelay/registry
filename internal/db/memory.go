package db

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/blackrelay/registry/internal/cursor"
	"github.com/blackrelay/registry/internal/model"
	"github.com/blackrelay/registry/internal/staticdata"
)

type MemoryStore struct {
	mu        sync.RWMutex
	Sources   map[string]model.Source
	Artefacts map[string]model.SourceArtefact
	Entities  map[string]model.Entity
	Facts     map[string][]model.Fact
	Killmails map[string]model.KillmailRaw
	Events    map[string]EventRecord
	Objects   map[string]SuiObjectRecord
	Relations map[string]RelationDraft
	Reviews   map[string]model.Review
	Cursors   map[string]CursorStatus
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		Sources:   make(map[string]model.Source),
		Artefacts: make(map[string]model.SourceArtefact),
		Entities:  make(map[string]model.Entity),
		Facts:     make(map[string][]model.Fact),
		Killmails: make(map[string]model.KillmailRaw),
		Events:    make(map[string]EventRecord),
		Objects:   make(map[string]SuiObjectRecord),
		Relations: make(map[string]RelationDraft),
		Reviews:   make(map[string]model.Review),
		Cursors:   make(map[string]CursorStatus),
	}
}

func (s *MemoryStore) Ping(context.Context) error {
	return nil
}

func (s *MemoryStore) CountRegistryRows(ctx context.Context, environment model.Environment) (RegistryCountSnapshot, error) {
	_ = ctx
	s.mu.RLock()
	defer s.mu.RUnlock()
	snapshot := RegistryCountSnapshot{
		EntitiesByType:       make(map[model.EntityType]int64),
		EventsByModule:       make(map[string]int64),
		SuiObjectsByType:     make(map[string]int64),
		RelationsByPredicate: make(map[string]int64),
	}
	for _, source := range s.Sources {
		if environment != "" && source.Environment != environment {
			continue
		}
		snapshot.Counts.Sources++
	}
	for _, artefact := range s.Artefacts {
		if environment != "" && artefact.Environment != environment {
			continue
		}
		snapshot.Counts.SourceArtefacts++
	}
	for _, review := range s.Reviews {
		_ = review
		snapshot.Counts.Reviews++
	}
	for _, entity := range s.Entities {
		if environment != "" && entity.Environment != environment {
			continue
		}
		snapshot.Counts.Entities++
		snapshot.EntitiesByType[entity.Type]++
	}
	for _, facts := range s.Facts {
		for _, fact := range facts {
			if environment != "" && fact.Environment != environment {
				continue
			}
			snapshot.Counts.Facts++
		}
	}
	for _, relation := range s.Relations {
		if environment != "" && relation.Environment != environment {
			continue
		}
		snapshot.Counts.Relations++
		snapshot.RelationsByPredicate[relation.Predicate]++
	}
	for _, event := range s.Events {
		if environment != "" && event.Environment != environment {
			continue
		}
		snapshot.Counts.RawSuiEvents++
		module := event.Module
		if module == "" {
			module = "(none)"
		}
		snapshot.EventsByModule[module]++
	}
	for _, object := range s.Objects {
		if environment != "" && object.Environment != environment {
			continue
		}
		snapshot.Counts.RawSuiObjects++
		typeName := object.TypeName
		if typeName == "" {
			typeName = object.TypeRepr
		}
		if typeName == "" {
			typeName = "(unknown)"
		}
		snapshot.SuiObjectsByType[typeName]++
		if object.Module == "character" && object.TypeName == "PlayerProfile" {
			snapshot.Counts.PlayerProfiles++
		}
	}
	for _, killmail := range s.Killmails {
		if environment != "" && killmail.Environment != environment {
			continue
		}
		snapshot.Counts.Killmails++
	}
	for _, cursor := range s.Cursors {
		if environment != "" && cursor.Environment != environment {
			continue
		}
		snapshot.Counts.SyncCursors++
	}
	snapshot.Counts.SearchTerms = snapshot.Counts.Entities
	return snapshot, nil
}

func (s *MemoryStore) RecordImport(ctx context.Context, importID string, source model.Source, artefact model.SourceArtefact, summary map[string]any) error {
	_ = ctx
	_ = importID
	_ = summary
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Sources[source.ID] = source
	s.Artefacts[artefact.ID] = artefact
	return nil
}

func (s *MemoryStore) UpsertStaticEnemy(ctx context.Context, importID string, source model.Source, artefact model.SourceArtefact, candidate staticdata.EnemyCandidate) error {
	_ = ctx
	_ = importID
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Sources[source.ID] = source
	s.Artefacts[artefact.ID] = artefact
	entity := model.Entity{
		ID:          staticdata.EntityID(source.Environment, candidate.TypeID),
		Slug:        staticdata.Slug(candidate.Name, candidate.TypeID, source.Environment),
		Type:        model.EntityTypeEnemy,
		Name:        candidate.Name,
		DisplayName: staticdata.DisplayName(candidate.Name),
		Summary:     candidate.Basis,
		Environment: source.Environment,
		Cycle:       artefact.Cycle,
		UpdatedAt:   time.Now().UTC(),
	}
	s.Entities[entity.ID] = entity
	return nil
}

func (s *MemoryStore) UpsertKillmail(ctx context.Context, raw model.KillmailRaw) error {
	_ = ctx
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Killmails[raw.ID] = raw
	return nil
}

func (s *MemoryStore) EnsureSource(ctx context.Context, source model.Source) error {
	_ = ctx
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Sources[source.ID] = source
	return nil
}

func (s *MemoryStore) UpsertSuiEvent(ctx context.Context, event EventRecord) error {
	_ = ctx
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Events[event.ID] = event
	return nil
}

func (s *MemoryStore) UpsertSuiObject(ctx context.Context, object SuiObjectRecord) error {
	_ = ctx
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Objects[object.ID] = object
	return nil
}

func (s *MemoryStore) ListEntities(ctx context.Context, query EntityQuery) (EntityPage, error) {
	_ = ctx
	s.mu.RLock()
	defer s.mu.RUnlock()
	limit := saneLimit(query.Limit, 50, 200)
	var after cursor.Keyset
	var hasCursor bool
	if query.Cursor != "" {
		decoded, err := cursor.Decode(query.Cursor)
		if err != nil {
			return EntityPage{}, err
		}
		after = decoded
		hasCursor = true
	}
	items := make([]model.Entity, 0, len(s.Entities))
	for _, entity := range s.Entities {
		if query.Type != "" && entity.Type != query.Type {
			continue
		}
		if query.Environment != "" && entity.Environment != query.Environment {
			continue
		}
		if !cycleInScope(entity.Cycle, query.Cycles, query.IncludeUncycled) {
			continue
		}
		if query.Q != "" && !strings.Contains(strings.ToLower(entity.Name+" "+entity.Slug+" "+entity.DisplayName), strings.ToLower(query.Q)) {
			continue
		}
		if !s.entityFactsMatchQueryLocked(entity.ID, query) {
			continue
		}
		items = append(items, entity)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].UpdatedAt.Equal(items[j].UpdatedAt) {
			return items[i].ID > items[j].ID
		}
		return items[i].UpdatedAt.After(items[j].UpdatedAt)
	})
	if hasCursor {
		items = filterEntitiesAfter(items, after)
	}
	next := ""
	if len(items) > limit {
		items = items[:limit]
		last := items[len(items)-1]
		encoded, err := cursor.Encode(cursor.Keyset{Time: last.UpdatedAt, ID: last.ID})
		if err != nil {
			return EntityPage{}, err
		}
		next = encoded
	}
	return EntityPage{Items: items, NextCursor: next}, nil
}

func (s *MemoryStore) entityFactsMatchQueryLocked(entityID string, query EntityQuery) bool {
	filters := map[string]string{
		"type_id":            query.TypeID,
		"group_id":           query.GroupID,
		"category_id":        query.CategoryID,
		"market_group_id":    query.MarketGroupID,
		"wreck_type_id":      query.WreckTypeID,
		"source_artefact_id": query.SourceArtefactID,
		"static_entity_type": query.StaticEntityType,
	}
	for key, expected := range filters {
		expected = strings.TrimSpace(expected)
		if expected == "" {
			continue
		}
		found := false
		for _, fact := range s.Facts[entityID] {
			if fact.Key == key && fmt.Sprint(fact.Value) == expected {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func (s *MemoryStore) ListCurrentEntities(ctx context.Context, query CurrentEntityQuery) (CurrentEntityPage, error) {
	_ = ctx
	s.mu.RLock()
	defer s.mu.RUnlock()
	limit := saneLimit(query.Limit, 50, 200)
	var after cursor.Keyset
	var hasCursor bool
	if query.Cursor != "" {
		decoded, err := cursor.Decode(query.Cursor)
		if err != nil {
			return CurrentEntityPage{}, err
		}
		after = decoded
		hasCursor = true
	}
	items := make([]model.CurrentEntity, 0, len(s.Entities))
	for _, entity := range s.Entities {
		if query.Type != "" && entity.Type != query.Type {
			continue
		}
		if query.Environment != "" && entity.Environment != query.Environment {
			continue
		}
		if !cycleInScope(entity.Cycle, query.Cycles, query.IncludeUncycled) {
			continue
		}
		item := s.currentEntityLocked(entity, query.Cycles, query.IncludeUncycled)
		if !currentEntityMatchesQuery(item, query) {
			continue
		}
		items = append(items, item)
	}
	items = dedupeCurrentEntities(items, query)
	sortCurrentEntities(items, query)
	if hasCursor {
		items = filterCurrentEntitiesAfter(items, after)
	}
	next := ""
	if len(items) > limit {
		items = items[:limit]
		last := items[len(items)-1].Entity
		encoded, err := cursor.Encode(cursor.Keyset{Time: last.UpdatedAt, ID: last.ID})
		if err != nil {
			return CurrentEntityPage{}, err
		}
		next = encoded
	}
	return CurrentEntityPage{Items: items, NextCursor: next}, nil
}

func (s *MemoryStore) ListCurrentRelations(ctx context.Context, query CurrentRelationQuery) (CurrentRelationPage, error) {
	_ = ctx
	s.mu.RLock()
	defer s.mu.RUnlock()
	limit := saneLimit(query.Limit, 50, 200)
	predicateSet := make(map[string]struct{}, len(query.Predicates))
	for _, predicate := range query.Predicates {
		if predicate != "" {
			predicateSet[predicate] = struct{}{}
		}
	}
	var items []model.CurrentRelation
	for _, relation := range s.Relations {
		if query.Environment != "" && relation.Environment != query.Environment {
			continue
		}
		if !s.relationMatchesCycleScopeLocked(relation, query.Cycles, query.IncludeUncycled) {
			continue
		}
		if len(predicateSet) > 0 {
			if _, ok := predicateSet[relation.Predicate]; !ok {
				continue
			}
		}
		item := s.currentRelationLocked(relation)
		if !currentRelationMatchesQuery(item, query) {
			continue
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].ID > items[j].ID
		}
		return items[i].CreatedAt.After(items[j].CreatedAt)
	})
	if query.Cursor != "" {
		decoded, err := cursor.Decode(query.Cursor)
		if err != nil {
			return CurrentRelationPage{}, err
		}
		items = filterCurrentRelationsAfter(items, decoded)
	}
	next := ""
	if len(items) > limit {
		items = items[:limit]
		last := items[len(items)-1]
		encoded, err := cursor.Encode(cursor.Keyset{Time: last.CreatedAt, ID: last.ID})
		if err != nil {
			return CurrentRelationPage{}, err
		}
		next = encoded
	}
	return CurrentRelationPage{Items: items, NextCursor: next}, nil
}

func (s *MemoryStore) GetEntity(ctx context.Context, idOrSlug string) (model.Entity, bool, error) {
	_ = ctx
	s.mu.RLock()
	defer s.mu.RUnlock()
	if entity, ok := s.Entities[idOrSlug]; ok {
		return entity, true, nil
	}
	for _, entity := range s.Entities {
		if entity.Slug == idOrSlug {
			return entity, true, nil
		}
	}
	return model.Entity{}, false, nil
}

func (s *MemoryStore) GetSource(ctx context.Context, id string) (model.Source, bool, error) {
	_ = ctx
	s.mu.RLock()
	defer s.mu.RUnlock()
	source, ok := s.Sources[id]
	return source, ok, nil
}

func (s *MemoryStore) ExportDatabaseIdentity(ctx context.Context) (DatabaseIdentity, error) {
	_ = ctx
	return DatabaseIdentity{Engine: "memory", Database: "memory"}, nil
}

func (s *MemoryStore) ListSources(ctx context.Context, limit int) ([]model.Source, error) {
	if limit <= 0 {
		limit = 200
	}
	page, err := s.ListSourcesPage(ctx, SourceQuery{Limit: limit})
	if err != nil {
		return nil, err
	}
	return page.Items, nil
}

func (s *MemoryStore) ListSourcesPage(ctx context.Context, query SourceQuery) (SourcePage, error) {
	_ = ctx
	s.mu.RLock()
	defer s.mu.RUnlock()
	limit := saneLimit(query.Limit, 200, 5000)
	out := make([]model.Source, 0, len(s.Sources))
	for _, source := range s.Sources {
		if query.Environment != "" && source.Environment != query.Environment {
			continue
		}
		if !cycleInScope(source.Cycle, query.Cycles, query.IncludeUncycled) {
			continue
		}
		out = append(out, source)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID > out[j].ID
		}
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	if query.Cursor != "" {
		decoded, err := cursor.Decode(query.Cursor)
		if err != nil {
			return SourcePage{}, err
		}
		out = filterSourcesAfter(out, decoded)
	}
	next := ""
	if len(out) > limit {
		out = out[:limit]
		last := out[len(out)-1]
		encoded, err := cursor.Encode(cursor.Keyset{Time: last.CreatedAt, ID: last.ID})
		if err != nil {
			return SourcePage{}, err
		}
		next = encoded
	}
	return SourcePage{Items: out, NextCursor: next}, nil
}

func (s *MemoryStore) GetArtefact(ctx context.Context, id string) (model.SourceArtefact, bool, error) {
	_ = ctx
	s.mu.RLock()
	defer s.mu.RUnlock()
	artefact, ok := s.Artefacts[id]
	return artefact, ok, nil
}

func (s *MemoryStore) ListSourceArtefactsPage(ctx context.Context, query SourceArtefactQuery) (SourceArtefactPage, error) {
	_ = ctx
	limit := saneLimit(query.Limit, 200, 5000)
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]model.SourceArtefact, 0, len(s.Artefacts))
	for _, artefact := range s.Artefacts {
		if query.Environment != "" && artefact.Environment != query.Environment {
			continue
		}
		if !cycleInScope(artefact.Cycle, query.Cycles, query.IncludeUncycled) {
			continue
		}
		out = append(out, artefact)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID > out[j].ID
		}
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	if query.Cursor != "" {
		decoded, err := cursor.Decode(query.Cursor)
		if err != nil {
			return SourceArtefactPage{}, err
		}
		out = filterArtefactsAfter(out, decoded)
	}
	next := ""
	if len(out) > limit {
		out = out[:limit]
		last := out[len(out)-1]
		encoded, err := cursor.Encode(cursor.Keyset{Time: last.CreatedAt, ID: last.ID})
		if err != nil {
			return SourceArtefactPage{}, err
		}
		next = encoded
	}
	return SourceArtefactPage{Items: out, NextCursor: next}, nil
}

func (s *MemoryStore) UpsertEntityFacts(ctx context.Context, entity model.Entity, facts []EntityFactDraft) error {
	_ = ctx
	s.mu.Lock()
	defer s.mu.Unlock()
	if entity.UpdatedAt.IsZero() {
		entity.UpdatedAt = time.Now().UTC()
	}
	if existing, ok := s.Entities[entity.ID]; ok && shouldPreserveExistingEntityOnPlaceholder(entity) {
		entity.Slug = nonEmpty(existing.Slug, entity.Slug)
		entity.Name = nonEmpty(existing.Name, entity.Name)
		entity.DisplayName = nonEmpty(existing.DisplayName, entity.DisplayName)
		entity.Summary = nonEmpty(existing.Summary, entity.Summary)
	}
	s.Entities[entity.ID] = entity
	for _, fact := range facts {
		if fact.Confidence == "" {
			fact.Confidence = model.ConfidenceUnknown
		}
		if fact.Environment == "" {
			fact.Environment = entity.Environment
		}
		if fact.Cycle == nil {
			fact.Cycle = entity.Cycle
		}
		if fact.ReviewStatus == "" {
			fact.ReviewStatus = model.ReviewStatusCandidate
		}
		s.Facts[entity.ID] = upsertMemoryFact(s.Facts[entity.ID], model.Fact{
			EntityID:     entity.ID,
			Key:          fact.Key,
			Value:        fact.Value,
			SourceID:     fact.SourceID,
			Confidence:   fact.Confidence,
			Environment:  fact.Environment,
			Cycle:        fact.Cycle,
			ReviewStatus: fact.ReviewStatus,
		})
	}
	return nil
}

func (s *MemoryStore) ListEntityFacts(ctx context.Context, entityID string) ([]model.Fact, error) {
	_ = ctx
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := append([]model.Fact(nil), s.Facts[entityID]...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Key == out[j].Key {
			return out[i].SourceID < out[j].SourceID
		}
		return out[i].Key < out[j].Key
	})
	return out, nil
}

func (s *MemoryStore) UpsertRelations(ctx context.Context, relations []RelationDraft) error {
	_ = ctx
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, relation := range relations {
		if relation.SubjectEntityID == "" || relation.Predicate == "" || relation.ObjectEntityID == "" {
			continue
		}
		key := RelationID(relation)
		s.Relations[key] = relation
	}
	return nil
}

func (s *MemoryStore) ListEntityRelations(ctx context.Context, entityID string) ([]model.Relation, error) {
	_ = ctx
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []model.Relation
	for _, relation := range s.Relations {
		if relation.SubjectEntityID != entityID && relation.ObjectEntityID != entityID {
			continue
		}
		out = append(out, model.Relation{
			SubjectEntityID: relation.SubjectEntityID,
			Predicate:       relation.Predicate,
			ObjectEntityID:  relation.ObjectEntityID,
			SourceID:        relation.SourceID,
			Confidence:      relation.Confidence,
			Environment:     relation.Environment,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Predicate == out[j].Predicate {
			return out[i].ObjectEntityID < out[j].ObjectEntityID
		}
		return out[i].Predicate < out[j].Predicate
	})
	return out, nil
}

func (s *MemoryStore) ListEntitySources(ctx context.Context, entityID string) ([]model.Source, error) {
	_ = ctx
	s.mu.RLock()
	defer s.mu.RUnlock()
	sourceIDs := make(map[string]struct{})
	for _, fact := range s.Facts[entityID] {
		if fact.SourceID != "" {
			sourceIDs[fact.SourceID] = struct{}{}
		}
	}
	for _, relation := range s.Relations {
		if relation.SubjectEntityID == entityID || relation.ObjectEntityID == entityID {
			if relation.SourceID != "" {
				sourceIDs[relation.SourceID] = struct{}{}
			}
		}
	}
	out := make([]model.Source, 0, len(sourceIDs))
	for sourceID := range sourceIDs {
		if source, ok := s.Sources[sourceID]; ok {
			out = append(out, source)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (s *MemoryStore) ListEvents(ctx context.Context, query EventQuery) (EventPage, error) {
	_ = ctx
	s.mu.RLock()
	defer s.mu.RUnlock()
	maxLimit := query.MaxLimit
	if maxLimit <= 0 {
		maxLimit = 200
	}
	limit := saneLimit(query.Limit, 50, maxLimit)
	items := make([]EventRecord, 0, len(s.Events))
	for _, event := range s.Events {
		if query.Kind != "" && event.Kind != query.Kind {
			continue
		}
		if query.Environment != "" && event.Environment != query.Environment {
			continue
		}
		if !cycleInScope(event.Cycle, effectiveEventCycles(query), query.IncludeUncycled) {
			continue
		}
		if query.PackageID != "" && event.PackageID != query.PackageID {
			continue
		}
		if query.Module != "" && event.Module != query.Module {
			continue
		}
		if query.TransactionDigest != "" && event.TransactionDigest != query.TransactionDigest {
			continue
		}
		if query.SourceID != "" && event.SourceID != query.SourceID {
			continue
		}
		items = append(items, event)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].OccurredAt.Equal(items[j].OccurredAt) {
			if query.Ascending {
				return items[i].ID < items[j].ID
			}
			return items[i].ID > items[j].ID
		}
		if query.Ascending {
			return items[i].OccurredAt.Before(items[j].OccurredAt)
		}
		return items[i].OccurredAt.After(items[j].OccurredAt)
	})
	if query.Cursor != "" {
		decoded, err := cursor.Decode(query.Cursor)
		if err != nil {
			return EventPage{}, err
		}
		if query.Ascending {
			items = filterEventsAfterAscending(items, decoded)
		} else {
			items = filterEventsAfter(items, decoded)
		}
	}
	next := ""
	if len(items) > limit {
		items = items[:limit]
		last := items[len(items)-1]
		encoded, err := cursor.Encode(cursor.Keyset{Time: last.OccurredAt, ID: last.ID})
		if err != nil {
			return EventPage{}, err
		}
		next = encoded
	}
	return EventPage{Items: items, NextCursor: next}, nil
}

func (s *MemoryStore) ListSuiObjects(ctx context.Context, query SuiObjectQuery) (SuiObjectPage, error) {
	_ = ctx
	s.mu.RLock()
	defer s.mu.RUnlock()
	limit := saneLimit(query.Limit, 1000, 5000)
	items := make([]SuiObjectRecord, 0, len(s.Objects))
	for _, object := range s.Objects {
		if query.Environment != "" && object.Environment != query.Environment {
			continue
		}
		if !timeCycleInScope(object.ObservedAt, query.Cycles, query.IncludeUncycled) {
			continue
		}
		if query.PackageID != "" && object.PackageID != query.PackageID {
			continue
		}
		if query.Module != "" && object.Module != query.Module {
			continue
		}
		if query.TypeName != "" && object.TypeName != query.TypeName {
			continue
		}
		if query.TypeRepr != "" && object.TypeRepr != query.TypeRepr {
			continue
		}
		items = append(items, object)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].ObservedAt.Equal(items[j].ObservedAt) {
			return items[i].ID < items[j].ID
		}
		return items[i].ObservedAt.Before(items[j].ObservedAt)
	})
	if query.Cursor != "" {
		decoded, err := cursor.Decode(query.Cursor)
		if err != nil {
			return SuiObjectPage{}, err
		}
		items = filterObjectsAfter(items, decoded)
	}
	next := ""
	if len(items) > limit {
		items = items[:limit]
		last := items[len(items)-1]
		encoded, err := cursor.Encode(cursor.Keyset{Time: last.ObservedAt, ID: last.ID})
		if err != nil {
			return SuiObjectPage{}, err
		}
		next = encoded
	}
	return SuiObjectPage{Items: items, NextCursor: next}, nil
}

func (s *MemoryStore) GetEvent(ctx context.Context, id string) (EventRecord, bool, error) {
	_ = ctx
	s.mu.RLock()
	defer s.mu.RUnlock()
	event, ok := s.Events[id]
	return event, ok, nil
}

func (s *MemoryStore) ListKillmailRaw(ctx context.Context, query KillmailQuery) ([]model.KillmailRaw, string, error) {
	_ = ctx
	s.mu.RLock()
	defer s.mu.RUnlock()
	limit := saneLimit(query.Limit, 50, 200)
	items := make([]model.KillmailRaw, 0, len(s.Killmails))
	for _, item := range s.Killmails {
		if !killmailMatchesQuery(item, query, s.Relations, s.Entities) {
			continue
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].OccurredAt.Equal(items[j].OccurredAt) {
			return items[i].ID > items[j].ID
		}
		return items[i].OccurredAt.After(items[j].OccurredAt)
	})
	if query.Cursor != "" {
		decoded, err := cursor.Decode(query.Cursor)
		if err != nil {
			return nil, "", err
		}
		items = filterKillmailsAfter(items, decoded)
	}
	next := ""
	if len(items) > limit {
		items = items[:limit]
		last := items[len(items)-1]
		encoded, err := cursor.Encode(cursor.Keyset{Time: last.OccurredAt, ID: last.ID})
		if err != nil {
			return nil, "", err
		}
		next = encoded
	}
	return items, next, nil
}

func (s *MemoryStore) GetKillmailRaw(ctx context.Context, id string) (model.KillmailRaw, bool, error) {
	_ = ctx
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.Killmails[id]
	return item, ok, nil
}

func (s *MemoryStore) ResolveCharacter(ctx context.Context, idOrName string, environment model.Environment) (model.ResolvedValue, bool, error) {
	return s.resolveEntity(ctx, idOrName, environment, model.EntityTypeCharacter)
}

func (s *MemoryStore) ResolveSystem(ctx context.Context, idOrName string, environment model.Environment) (model.ResolvedValue, bool, error) {
	return s.resolveEntity(ctx, idOrName, environment, model.EntityTypeSystem)
}

func (s *MemoryStore) ResolveEnemyType(ctx context.Context, typeID string, environment model.Environment) (model.ResolvedValue, bool, error) {
	id := "enemy:" + string(environment) + ":type:" + typeID
	entity, ok, err := s.GetEntity(ctx, id)
	if err != nil || !ok {
		return model.ResolvedValue{}, ok, err
	}
	return model.ResolvedValue{
		EntityID:    entity.ID,
		EntityType:  entity.Type,
		TypeID:      typeID,
		DisplayName: nonEmpty(entity.DisplayName, entity.Name),
		Confidence:  model.ConfidenceProbable,
		SourceIDs:   []string{"source:static-client:stillness:reviewed-enemies"},
	}, true, nil
}

func (s *MemoryStore) ListCursors(ctx context.Context) ([]CursorStatus, error) {
	_ = ctx
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]CursorStatus, 0, len(s.Cursors))
	for _, cursor := range s.Cursors {
		out = append(out, cursor)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	return out, nil
}

func (s *MemoryStore) GetSyncCursor(ctx context.Context, id string) (CursorStatus, bool, error) {
	_ = ctx
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.Cursors[id]
	return item, ok, nil
}

func (s *MemoryStore) SaveSyncCursor(ctx context.Context, item CursorStatus) error {
	_ = ctx
	s.mu.Lock()
	defer s.mu.Unlock()
	item.UpdatedAt = time.Now().UTC()
	s.Cursors[item.ID] = item
	return nil
}

func (s *MemoryStore) ListFreshness(ctx context.Context) ([]FreshnessStatus, error) {
	cursors, err := s.ListCursors(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]FreshnessStatus, 0, len(cursors))
	for _, cursor := range cursors {
		status := model.FreshnessUnknown
		if cursor.LastSuccessfulIngest != nil {
			status = model.FreshnessCachedSnapshot
		}
		out = append(out, FreshnessStatus{
			Source:               cursor.Source,
			Environment:          cursor.Environment,
			LastSuccessfulIngest: cursor.LastSuccessfulIngest,
			LastCheckpoint:       cursor.LastCheckpoint,
			EventsProcessed:      cursor.EventsProcessed,
			ErrorCount:           cursor.ErrorCount,
			LastErrorSummary:     cursor.LastErrorSummary,
			StalenessStatus:      status,
			UpdatedAt:            cursor.UpdatedAt,
		})
	}
	return out, nil
}

func (s *MemoryStore) ListSourceGaps(ctx context.Context, environment model.Environment) ([]model.SourceGap, error) {
	_ = ctx
	s.mu.RLock()
	defer s.mu.RUnlock()
	ownership := int64(0)
	location := int64(0)
	for _, entity := range s.Entities {
		if environment != "" && entity.Environment != environment {
			continue
		}
		if !isInfrastructureEntity(entity.Type) {
			continue
		}
		if memoryHasFactValueForKey(s.Facts[entity.ID], "owner_cap_id") && !s.hasOutgoingRelationLocked(entity.ID, "owned_by") {
			ownership++
		}
		if memoryHasFactValueForKey(s.Facts[entity.ID], "location_hash") && !s.hasOutgoingRelationLocked(entity.ID, "located_in") {
			location++
		}
	}
	unresolvedKillmails := int64(0)
	for _, item := range s.Killmails {
		if environment != "" && item.Environment != environment {
			continue
		}
		if item.SystemID == "" || item.VictimCharacterID == "" || (item.KillerCharacterID == "" && item.KillerTypeID == "") || item.ReporterCharacterID == "" {
			unresolvedKillmails++
		}
	}
	recipeCount := int64(0)
	placeholderTribeNames := int64(0)
	tribeProfileGaps := int64(0)
	for _, entity := range s.Entities {
		if entity.Type == model.EntityTypeRecipe && (environment == "" || entity.Environment == environment) {
			recipeCount++
		}
		if entity.Type == model.EntityTypeTribe &&
			(environment == "" || entity.Environment == environment) &&
			shouldPreserveExistingEntityOnPlaceholder(entity) {
			placeholderTribeNames++
		}
		if entity.Type == model.EntityTypeTribe &&
			(environment == "" || entity.Environment == environment) &&
			!memoryHasAnyFactValueForKeys(s.Facts[entity.ID], "tag", "aliases", "description", "url") {
			tribeProfileGaps++
		}
	}
	suiObjectRangeBlocked := int64(0)
	for _, cursor := range s.Cursors {
		if environment != "" && cursor.Environment != environment {
			continue
		}
		if isProviderRangeBlockedCursor(cursor) {
			suiObjectRangeBlocked++
		}
	}
	return sourceGapRows(environment, ownership, location, unresolvedKillmails, recipeCount, suiObjectRangeBlocked, placeholderTribeNames, tribeProfileGaps), nil
}

func (s *MemoryStore) CreateReview(ctx context.Context, draft ReviewDraft) (model.Review, error) {
	_ = ctx
	if strings.TrimSpace(draft.TargetKind) == "" || strings.TrimSpace(draft.TargetID) == "" {
		return model.Review{}, errors.New("review target kind and id are required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	review := model.Review{
		ID:           ReviewID(draft.TargetKind, draft.TargetID),
		TargetKind:   draft.TargetKind,
		TargetID:     draft.TargetID,
		ReviewStatus: model.ReviewStatusCandidate,
		Notes:        draft.Notes,
		CreatedAt:    now,
	}
	s.Reviews[review.ID] = review
	return review, nil
}

func isInfrastructureEntity(entityType model.EntityType) bool {
	switch entityType {
	case model.EntityTypeAssembly, model.EntityTypeGate, model.EntityTypeStorage, model.EntityTypeTurret:
		return true
	default:
		return false
	}
}

func memoryHasFactValueForKey(facts []model.Fact, key string) bool {
	for _, fact := range facts {
		if fact.Key == key && memoryFactValueIsPresent(fact.Value) {
			return true
		}
	}
	return false
}

func memoryHasAnyFactValueForKeys(facts []model.Fact, keys ...string) bool {
	keySet := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		keySet[key] = struct{}{}
	}
	for _, fact := range facts {
		if _, ok := keySet[fact.Key]; ok && memoryFactValueIsPresent(fact.Value) {
			return true
		}
	}
	return false
}

func memoryFactValueIsPresent(value any) bool {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed) != ""
	case []string:
		for _, item := range typed {
			if strings.TrimSpace(item) != "" {
				return true
			}
		}
		return false
	case []any:
		for _, item := range typed {
			if text, ok := item.(string); ok && strings.TrimSpace(text) != "" {
				return true
			}
		}
		return false
	default:
		return strings.TrimSpace(fmt.Sprint(value)) != ""
	}
}

func (s *MemoryStore) hasOutgoingRelationLocked(subjectID, predicate string) bool {
	for _, relation := range s.Relations {
		if relation.SubjectEntityID == subjectID && relation.Predicate == predicate {
			return true
		}
	}
	return false
}

func (s *MemoryStore) ListReviews(ctx context.Context, status model.ReviewStatus) ([]model.Review, error) {
	_ = ctx
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]model.Review, 0, len(s.Reviews))
	for _, review := range s.Reviews {
		if status != "" && review.ReviewStatus != status {
			continue
		}
		out = append(out, review)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out, nil
}

func (s *MemoryStore) UpdateReviewStatus(ctx context.Context, id string, status model.ReviewStatus, update ReviewUpdate) (model.Review, bool, error) {
	_ = ctx
	s.mu.Lock()
	defer s.mu.Unlock()
	review, ok := s.Reviews[id]
	if !ok {
		return model.Review{}, false, nil
	}
	now := time.Now().UTC()
	review.ReviewStatus = status
	review.Reviewer = update.Reviewer
	review.Notes = update.Notes
	review.ReviewedAt = &now
	s.Reviews[id] = review
	return review, true, nil
}

func (s *MemoryStore) resolveEntity(ctx context.Context, idOrName string, environment model.Environment, entityType model.EntityType) (model.ResolvedValue, bool, error) {
	_ = ctx
	if strings.TrimSpace(idOrName) == "" {
		return model.ResolvedValue{}, false, errors.New("id or name is empty")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, entity := range s.Entities {
		if entity.Type != entityType {
			continue
		}
		if environment != "" && environment != model.EnvironmentUnknown && entity.Environment != environment {
			continue
		}
		if entity.ID == idOrName || entity.Slug == idOrName || strings.EqualFold(entity.Name, idOrName) || strings.EqualFold(entity.DisplayName, idOrName) {
			displayName, confidence, sourceIDs := s.preferredEntityDisplayLocked(entity)
			return model.ResolvedValue{
				EntityID:    entity.ID,
				EntityType:  entity.Type,
				RawID:       idOrName,
				DisplayName: displayName,
				Confidence:  confidence,
				SourceIDs:   sourceIDs,
			}, true, nil
		}
	}
	return model.ResolvedValue{}, false, nil
}

func filterEntitiesAfter(items []model.Entity, after cursor.Keyset) []model.Entity {
	var out []model.Entity
	for _, item := range items {
		if item.UpdatedAt.Before(after.Time) || (item.UpdatedAt.Equal(after.Time) && item.ID < after.ID) {
			out = append(out, item)
		}
	}
	return out
}

func filterCurrentEntitiesAfter(items []model.CurrentEntity, after cursor.Keyset) []model.CurrentEntity {
	var out []model.CurrentEntity
	for _, item := range items {
		if item.Entity.UpdatedAt.Before(after.Time) || (item.Entity.UpdatedAt.Equal(after.Time) && item.Entity.ID < after.ID) {
			out = append(out, item)
		}
	}
	return out
}

func sortCurrentEntities(items []model.CurrentEntity, query CurrentEntityQuery) {
	sort.Slice(items, func(i, j int) bool {
		if query.Type == model.EntityTypeTribe && query.Cursor == "" {
			leftRank := tribeCurrentSortRank(items[i])
			rightRank := tribeCurrentSortRank(items[j])
			if leftRank != rightRank {
				return leftRank < rightRank
			}
		}
		if items[i].Entity.UpdatedAt.Equal(items[j].Entity.UpdatedAt) {
			return items[i].Entity.ID > items[j].Entity.ID
		}
		return items[i].Entity.UpdatedAt.After(items[j].Entity.UpdatedAt)
	})
}

func tribeCurrentSortRank(item model.CurrentEntity) int {
	if shouldPreserveExistingEntityOnPlaceholder(item.Entity) {
		return 2
	}
	if currentEntityHasTribeProfile(item) {
		return 0
	}
	return 1
}

func currentEntityHasTribeProfile(item model.CurrentEntity) bool {
	return factString(item.Facts["tag"]) != "" ||
		factString(item.Facts["description"]) != "" ||
		factString(item.Facts["url"]) != "" ||
		len(factStringSlice(item.Facts["aliases"])) > 0
}

func filterSourcesAfter(items []model.Source, after cursor.Keyset) []model.Source {
	var out []model.Source
	for _, item := range items {
		if item.CreatedAt.Before(after.Time) || (item.CreatedAt.Equal(after.Time) && item.ID < after.ID) {
			out = append(out, item)
		}
	}
	return out
}

func filterArtefactsAfter(items []model.SourceArtefact, after cursor.Keyset) []model.SourceArtefact {
	var out []model.SourceArtefact
	for _, item := range items {
		if item.CreatedAt.Before(after.Time) || (item.CreatedAt.Equal(after.Time) && item.ID < after.ID) {
			out = append(out, item)
		}
	}
	return out
}

func filterEventsAfter(items []EventRecord, after cursor.Keyset) []EventRecord {
	var out []EventRecord
	for _, item := range items {
		if item.OccurredAt.Before(after.Time) || (item.OccurredAt.Equal(after.Time) && item.ID < after.ID) {
			out = append(out, item)
		}
	}
	return out
}

func filterEventsAfterAscending(items []EventRecord, after cursor.Keyset) []EventRecord {
	var out []EventRecord
	for _, item := range items {
		if item.OccurredAt.After(after.Time) || (item.OccurredAt.Equal(after.Time) && item.ID > after.ID) {
			out = append(out, item)
		}
	}
	return out
}

func filterObjectsAfter(items []SuiObjectRecord, after cursor.Keyset) []SuiObjectRecord {
	var out []SuiObjectRecord
	for _, item := range items {
		if item.ObservedAt.After(after.Time) || (item.ObservedAt.Equal(after.Time) && item.ID > after.ID) {
			out = append(out, item)
		}
	}
	return out
}

func filterKillmailsAfter(items []model.KillmailRaw, after cursor.Keyset) []model.KillmailRaw {
	var out []model.KillmailRaw
	for _, item := range items {
		if item.OccurredAt.Before(after.Time) || (item.OccurredAt.Equal(after.Time) && item.ID < after.ID) {
			out = append(out, item)
		}
	}
	return out
}

func nonEmpty(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

func (s *MemoryStore) currentEntityLocked(entity model.Entity, cycles []int, includeUncycled bool) model.CurrentEntity {
	facts := make(map[string]any, len(s.Facts[entity.ID]))
	sourceIDs := make(map[string]struct{})
	for _, fact := range s.Facts[entity.ID] {
		facts[fact.Key] = fact.Value
		if fact.SourceID != "" {
			sourceIDs[fact.SourceID] = struct{}{}
		}
	}
	var outgoing []model.CurrentRelation
	var incoming []model.CurrentRelation
	for _, relation := range s.Relations {
		if !s.relationMatchesCycleScopeLocked(relation, cycles, includeUncycled) {
			continue
		}
		if relation.SourceID != "" && (relation.SubjectEntityID == entity.ID || relation.ObjectEntityID == entity.ID) {
			sourceIDs[relation.SourceID] = struct{}{}
		}
		switch {
		case relation.SubjectEntityID == entity.ID:
			outgoing = append(outgoing, s.currentRelationLocked(relation))
		case relation.ObjectEntityID == entity.ID:
			incoming = append(incoming, s.currentRelationLocked(relation))
		}
	}
	sources := make([]string, 0, len(sourceIDs))
	for sourceID := range sourceIDs {
		sources = append(sources, sourceID)
	}
	sort.Strings(sources)
	item := model.CurrentEntity{
		Entity:            entity,
		Facts:             facts,
		OutgoingRelations: outgoing,
		IncomingRelations: incoming,
		SourceIDs:         sources,
	}
	deriveCurrentEntity(&item)
	return item
}

func (s *MemoryStore) relationMatchesCycleScopeLocked(relation RelationDraft, cycles []int, includeUncycled bool) bool {
	if len(cycles) == 0 {
		return true
	}
	subject := s.Entities[relation.SubjectEntityID]
	object := s.Entities[relation.ObjectEntityID]
	return cycleInScope(subject.Cycle, cycles, includeUncycled) && cycleInScope(object.Cycle, cycles, includeUncycled)
}

func (s *MemoryStore) currentRelationLocked(relation RelationDraft) model.CurrentRelation {
	subject := s.Entities[relation.SubjectEntityID]
	object := s.Entities[relation.ObjectEntityID]
	id := RelationID(relation)
	return model.CurrentRelation{
		ID:                 id,
		SubjectEntityID:    relation.SubjectEntityID,
		SubjectEntityType:  subject.Type,
		SubjectDisplayName: nonEmpty(subject.DisplayName, subject.Name),
		Predicate:          relation.Predicate,
		ObjectEntityID:     relation.ObjectEntityID,
		ObjectEntityType:   object.Type,
		ObjectDisplayName:  nonEmpty(object.DisplayName, object.Name),
		SourceID:           relation.SourceID,
		Confidence:         relation.Confidence,
		Environment:        relation.Environment,
	}
}

func (s *MemoryStore) preferredEntityDisplayLocked(entity model.Entity) (string, model.Confidence, []string) {
	displayName := nonEmpty(entity.DisplayName, entity.Name)
	confidence := model.ConfidenceProbable
	sourceSet := make(map[string]struct{})
	for _, fact := range s.Facts[entity.ID] {
		if fact.SourceID != "" {
			sourceSet[fact.SourceID] = struct{}{}
		}
		if fact.Key != "metadata_name" && fact.Key != "display_name" {
			continue
		}
		value, ok := fact.Value.(string)
		if !ok || strings.TrimSpace(value) == "" {
			continue
		}
		displayName = value
		if fact.Confidence != "" {
			confidence = fact.Confidence
		}
	}
	sources := make([]string, 0, len(sourceSet))
	for sourceID := range sourceSet {
		sources = append(sources, sourceID)
	}
	sort.Strings(sources)
	return displayName, confidence, sources
}

func filterCurrentRelationsAfter(items []model.CurrentRelation, after cursor.Keyset) []model.CurrentRelation {
	var out []model.CurrentRelation
	for _, item := range items {
		if item.CreatedAt.Before(after.Time) || (item.CreatedAt.Equal(after.Time) && item.ID < after.ID) {
			out = append(out, item)
		}
	}
	return out
}

func upsertMemoryFact(facts []model.Fact, next model.Fact) []model.Fact {
	for i, fact := range facts {
		if fact.Key == next.Key && fact.SourceID == next.SourceID {
			facts[i] = next
			return facts
		}
	}
	return append(facts, next)
}
