package report

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/blackrelay/registry/internal/db"
	"github.com/blackrelay/registry/internal/killmail"
	"github.com/blackrelay/registry/internal/model"
	"github.com/blackrelay/registry/internal/resolver"
)

type KillmailAuditOptions struct {
	Environment     model.Environment
	PageSize        int
	SampleLimit     int
	ExcludeFixtures bool
	Now             func() time.Time
}

type KillmailAudit struct {
	SchemaVersion   string                   `json:"schemaVersion"`
	Environment     model.Environment        `json:"environment,omitempty"`
	GeneratedAt     time.Time                `json:"generatedAt"`
	ExcludeFixtures bool                     `json:"excludeFixtures,omitempty"`
	Counts          KillmailResolutionCounts `json:"counts"`
	Evidence        KillmailEvidenceCounts   `json:"evidence"`
	Samples         KillmailAuditSamples     `json:"samples,omitempty"`
}

type KillmailEvidenceCounts struct {
	RawKillerIDs                         int64 `json:"rawKillerIds"`
	ExplicitKillerTypeIDs                int64 `json:"explicitKillerTypeIds"`
	RawKillerIDsWithoutTypeIDs           int64 `json:"rawKillerIdsWithoutTypeIds"`
	RawKillerIDsWarnedAsNotStaticNPCType int64 `json:"rawKillerIdsWarnedAsNotStaticNpcType"`
}

type KillmailAuditSamples struct {
	UnresolvedSystems   []KillmailSample `json:"unresolvedSystems,omitempty"`
	UnresolvedVictims   []KillmailSample `json:"unresolvedVictims,omitempty"`
	UnresolvedKillers   []KillmailSample `json:"unresolvedKillers,omitempty"`
	UnresolvedReporters []KillmailSample `json:"unresolvedReporters,omitempty"`
	NPCKillers          []KillmailSample `json:"npcKillers,omitempty"`
	CharacterKillers    []KillmailSample `json:"characterKillers,omitempty"`
}

type KillmailSample struct {
	ID           string    `json:"id"`
	OccurredAt   time.Time `json:"occurredAt"`
	System       string    `json:"system,omitempty"`
	Victim       string    `json:"victim,omitempty"`
	Killer       string    `json:"killer,omitempty"`
	KillerTypeID string    `json:"killerTypeId,omitempty"`
	Reporter     string    `json:"reporter,omitempty"`
	SourceIDs    []string  `json:"sourceIds,omitempty"`
	Warnings     []string  `json:"warnings,omitempty"`
}

type CurrentStateAuditOptions struct {
	Environment model.Environment
	PageSize    int
	Now         func() time.Time
}

type CurrentStateAudit struct {
	SchemaVersion string                  `json:"schemaVersion"`
	Environment   model.Environment       `json:"environment,omitempty"`
	GeneratedAt   time.Time               `json:"generatedAt"`
	Counts        CurrentStateAuditCounts `json:"counts"`
}

type CurrentStateAuditCounts struct {
	Characters                 int64 `json:"characters"`
	CharactersWithTribe        int64 `json:"charactersWithTribe"`
	CharactersWithActivity     int64 `json:"charactersWithActivity"`
	Tribes                     int64 `json:"tribes"`
	TribesWithMembers          int64 `json:"tribesWithMembers"`
	Assemblies                 int64 `json:"assemblies"`
	AssembliesWithOwner        int64 `json:"assembliesWithOwner"`
	AssembliesWithSystem       int64 `json:"assembliesWithSystem"`
	AssembliesWithOwnerCap     int64 `json:"assembliesWithOwnerCap"`
	AssembliesWithLocationHash int64 `json:"assembliesWithLocationHash"`
	Gates                      int64 `json:"gates"`
	GatesWithLinkedGate        int64 `json:"gatesWithLinkedGate"`
	GatesWithOwnerCap          int64 `json:"gatesWithOwnerCap"`
	GatesWithLocationHash      int64 `json:"gatesWithLocationHash"`
	Storage                    int64 `json:"storage"`
	StorageWithOwner           int64 `json:"storageWithOwner"`
	StorageWithSystem          int64 `json:"storageWithSystem"`
	StorageWithOwnerCap        int64 `json:"storageWithOwnerCap"`
	StorageWithLocationHash    int64 `json:"storageWithLocationHash"`
	Turrets                    int64 `json:"turrets"`
	TurretsWithOwner           int64 `json:"turretsWithOwner"`
	TurretsWithSystem          int64 `json:"turretsWithSystem"`
	TurretsWithOwnerCap        int64 `json:"turretsWithOwnerCap"`
	TurretsWithLocationHash    int64 `json:"turretsWithLocationHash"`
	Systems                    int64 `json:"systems"`
	SystemsWithActivity        int64 `json:"systemsWithActivity"`
	SystemsWithConnections     int64 `json:"systemsWithConnections"`
	RouteEntities              int64 `json:"routeEntities"`
	OwnershipRelations         int64 `json:"ownershipRelations"`
	OwnerCapRelations          int64 `json:"ownerCapRelations"`
	LocationHashRelations      int64 `json:"locationHashRelations"`
	RouteEdgeRelations         int64 `json:"routeEdgeRelations"`
}

type CharacterProfileAuditOptions struct {
	Environment model.Environment
	PageSize    int
	SampleLimit int
	Now         func() time.Time
}

type CharacterProfileAudit struct {
	SchemaVersion string                       `json:"schemaVersion"`
	Environment   model.Environment            `json:"environment,omitempty"`
	GeneratedAt   time.Time                    `json:"generatedAt"`
	Counts        CharacterProfileAuditCounts  `json:"counts"`
	Samples       CharacterProfileAuditSamples `json:"samples,omitempty"`
}

type CharacterProfileAuditCounts struct {
	Characters              int64 `json:"characters"`
	NamedCharacters         int64 `json:"namedCharacters"`
	PlaceholderDisplayNames int64 `json:"placeholderDisplayNames"`
	WithMetadataName        int64 `json:"withMetadataName"`
	WithMetadataDescription int64 `json:"withMetadataDescription"`
	WithMetadataURL         int64 `json:"withMetadataUrl"`
	WithTribe               int64 `json:"withTribe"`
	WithActivity            int64 `json:"withActivity"`
}

type CharacterProfileAuditSamples struct {
	PlaceholderDisplayNames []CurrentEntitySample `json:"placeholderDisplayNames,omitempty"`
}

type EvidenceBridgeAuditOptions struct {
	Environment model.Environment
	PageSize    int
	SampleLimit int
	Now         func() time.Time
}

type EvidenceBridgeAudit struct {
	SchemaVersion string                     `json:"schemaVersion"`
	Environment   model.Environment          `json:"environment,omitempty"`
	GeneratedAt   time.Time                  `json:"generatedAt"`
	Counts        EvidenceBridgeAuditCounts  `json:"counts"`
	Samples       EvidenceBridgeAuditSamples `json:"samples,omitempty"`
}

type EvidenceBridgeAuditCounts struct {
	InfrastructureWithOwnerCap       int64 `json:"infrastructureWithOwnerCap"`
	InfrastructureWithLocationHash   int64 `json:"infrastructureWithLocationHash"`
	InfrastructureWithResolvedOwner  int64 `json:"infrastructureWithResolvedOwner"`
	InfrastructureWithResolvedSystem int64 `json:"infrastructureWithResolvedSystem"`
	UniqueOwnerCapValues             int64 `json:"uniqueOwnerCapValues"`
	UniqueLocationHashValues         int64 `json:"uniqueLocationHashValues"`
	CharactersWithOwnerCap           int64 `json:"charactersWithOwnerCap"`
	SystemsWithLocationHash          int64 `json:"systemsWithLocationHash"`
}

type EvidenceBridgeAuditSamples struct {
	UnresolvedOwnerCaps      []EvidenceBridgeSample `json:"unresolvedOwnerCaps,omitempty"`
	UnresolvedLocationHashes []EvidenceBridgeSample `json:"unresolvedLocationHashes,omitempty"`
}

type EvidenceBridgeSample struct {
	EntityID    string   `json:"entityId"`
	DisplayName string   `json:"displayName,omitempty"`
	Value       string   `json:"value"`
	SourceIDs   []string `json:"sourceIds,omitempty"`
}

type CurrentEntitySample struct {
	EntityID    string   `json:"entityId"`
	DisplayName string   `json:"displayName,omitempty"`
	SourceIDs   []string `json:"sourceIds,omitempty"`
}

type CurrentStateStore interface {
	ListCurrentEntities(ctx context.Context, query db.CurrentEntityQuery) (db.CurrentEntityPage, error)
	ListCurrentRelations(ctx context.Context, query db.CurrentRelationQuery) (db.CurrentRelationPage, error)
}

func BuildKillmailAudit(ctx context.Context, store Store, options KillmailAuditOptions) (KillmailAudit, error) {
	if options.Environment == "" {
		options.Environment = model.EnvironmentStillness
	}
	now := time.Now().UTC()
	if options.Now != nil {
		now = options.Now().UTC()
	}
	pageSize := boundedPageSize(options.PageSize, 200)
	sampleLimit := options.SampleLimit
	if sampleLimit <= 0 {
		sampleLimit = 10
	}
	service := killmail.Service{Resolver: resolver.Resolver{Store: store}, GraphStore: store}
	cursor := ""
	var counts KillmailResolutionCounts
	var evidence KillmailEvidenceCounts
	var samples KillmailAuditSamples
	for {
		items, next, err := store.ListKillmailRaw(ctx, db.KillmailQuery{
			Environment:     options.Environment,
			ExcludeFixtures: options.ExcludeFixtures,
			Limit:           pageSize,
			Cursor:          cursor,
		})
		if err != nil {
			return KillmailAudit{}, err
		}
		for _, item := range items {
			semantic := service.Semantic(ctx, item)
			counts.Total++
			countKillmailEvidence(&evidence, item, semantic)
			countResolved(&counts.ResolvedSystems, &counts.UnresolvedSystems, semantic.System)
			countResolved(&counts.ResolvedVictims, &counts.UnresolvedVictims, semantic.Victim)
			countResolved(&counts.ResolvedKillers, &counts.UnresolvedKillers, semantic.Killer)
			countResolved(&counts.ResolvedReporters, &counts.UnresolvedReporters, semantic.Reporter)
			sample := killmailSample(item, semantic)
			if !isResolved(semantic.System) {
				appendSample(&samples.UnresolvedSystems, sample, sampleLimit)
			}
			if !isResolved(semantic.Victim) {
				appendSample(&samples.UnresolvedVictims, sample, sampleLimit)
			}
			if !isResolved(semantic.Killer) {
				appendSample(&samples.UnresolvedKillers, sample, sampleLimit)
			}
			if !isResolved(semantic.Reporter) {
				appendSample(&samples.UnresolvedReporters, sample, sampleLimit)
			}
			switch {
			case isResolved(semantic.Killer) && semantic.Killer.EntityType == model.EntityTypeEnemy:
				counts.NPCKillers++
				appendSample(&samples.NPCKillers, sample, sampleLimit)
			case isResolved(semantic.Killer) && semantic.Killer.EntityType == model.EntityTypeCharacter:
				counts.CharacterKillers++
				appendSample(&samples.CharacterKillers, sample, sampleLimit)
			}
		}
		if next == "" || len(items) == 0 {
			break
		}
		cursor = next
	}
	return KillmailAudit{
		SchemaVersion:   "registry.killmail-audit.v1",
		Environment:     options.Environment,
		GeneratedAt:     now,
		ExcludeFixtures: options.ExcludeFixtures,
		Counts:          counts,
		Evidence:        evidence,
		Samples:         samples,
	}, nil
}

func countKillmailEvidence(counts *KillmailEvidenceCounts, raw model.KillmailRaw, semantic model.SemanticKillmail) {
	if raw.KillerCharacterID != "" {
		counts.RawKillerIDs++
	}
	if raw.KillerTypeID != "" {
		counts.ExplicitKillerTypeIDs++
	}
	if raw.KillerCharacterID != "" && raw.KillerTypeID == "" {
		counts.RawKillerIDsWithoutTypeIDs++
	}
	for _, warning := range semantic.Killer.Warnings {
		if strings.Contains(warning, "killer_id is not a static NPC type id") {
			counts.RawKillerIDsWarnedAsNotStaticNPCType++
			return
		}
	}
}

func BuildCharacterProfileAudit(ctx context.Context, store CurrentStateStore, options CharacterProfileAuditOptions) (CharacterProfileAudit, error) {
	if options.Environment == "" {
		options.Environment = model.EnvironmentStillness
	}
	now := time.Now().UTC()
	if options.Now != nil {
		now = options.Now().UTC()
	}
	pageSize := boundedPageSize(options.PageSize, 200)
	sampleLimit := options.SampleLimit
	if sampleLimit <= 0 {
		sampleLimit = 10
	}
	items, err := listAllCurrentEntities(ctx, store, options.Environment, model.EntityTypeCharacter, pageSize)
	if err != nil {
		return CharacterProfileAudit{}, err
	}
	var counts CharacterProfileAuditCounts
	var samples CharacterProfileAuditSamples
	for _, item := range items {
		counts.Characters++
		if !db.IsPlaceholderEntity(item.Entity) {
			counts.NamedCharacters++
		} else {
			counts.PlaceholderDisplayNames++
			appendCurrentEntitySample(&samples.PlaceholderDisplayNames, currentEntitySample(item), sampleLimit)
		}
		if nonEmptyFactString(item, "metadata_name") != "" {
			counts.WithMetadataName++
		}
		if nonEmptyFactString(item, "metadata_description") != "" {
			counts.WithMetadataDescription++
		}
		if nonEmptyFactString(item, "metadata_url") != "" {
			counts.WithMetadataURL++
		}
		if item.Derived != nil && item.Derived.Tribe != nil {
			counts.WithTribe++
		}
		if item.Derived != nil && item.Derived.PublicActivityCount > 0 {
			counts.WithActivity++
		}
	}
	return CharacterProfileAudit{
		SchemaVersion: "registry.character-profile-audit.v1",
		Environment:   options.Environment,
		GeneratedAt:   now,
		Counts:        counts,
		Samples:       samples,
	}, nil
}

func BuildEvidenceBridgeAudit(ctx context.Context, store CurrentStateStore, options EvidenceBridgeAuditOptions) (EvidenceBridgeAudit, error) {
	if options.Environment == "" {
		options.Environment = model.EnvironmentStillness
	}
	now := time.Now().UTC()
	if options.Now != nil {
		now = options.Now().UTC()
	}
	pageSize := boundedPageSize(options.PageSize, 200)
	sampleLimit := options.SampleLimit
	if sampleLimit <= 0 {
		sampleLimit = 10
	}
	var counts EvidenceBridgeAuditCounts
	var samples EvidenceBridgeAuditSamples
	ownerCaps := make(map[string]struct{})
	locationHashes := make(map[string]struct{})
	for _, entityType := range []model.EntityType{
		model.EntityTypeAssembly,
		model.EntityTypeGate,
		model.EntityTypeStorage,
		model.EntityTypeMarket,
		model.EntityTypeTurret,
		model.EntityTypeStructure,
	} {
		items, err := listAllCurrentEntities(ctx, store, options.Environment, entityType, pageSize)
		if err != nil {
			return EvidenceBridgeAudit{}, err
		}
		for _, item := range items {
			ownerCap := nonEmptyFactString(item, "owner_cap_id")
			locationHash := nonEmptyFactString(item, "location_hash")
			if ownerCap != "" || hasDerivedRelated(item, "owner_cap") {
				counts.InfrastructureWithOwnerCap++
			}
			if locationHash != "" || hasDerivedRelated(item, "location_hash") {
				counts.InfrastructureWithLocationHash++
			}
			if ownerCap != "" {
				ownerCaps[ownerCap] = struct{}{}
				if item.Derived == nil || item.Derived.Owner == nil {
					appendEvidenceBridgeSample(&samples.UnresolvedOwnerCaps, evidenceBridgeSample(item, ownerCap), sampleLimit)
				}
			}
			if locationHash != "" {
				locationHashes[locationHash] = struct{}{}
				if item.Derived == nil || item.Derived.System == nil {
					appendEvidenceBridgeSample(&samples.UnresolvedLocationHashes, evidenceBridgeSample(item, locationHash), sampleLimit)
				}
			}
			if item.Derived != nil && item.Derived.Owner != nil {
				counts.InfrastructureWithResolvedOwner++
			}
			if item.Derived != nil && item.Derived.System != nil {
				counts.InfrastructureWithResolvedSystem++
			}
		}
	}
	characters, err := listAllCurrentEntities(ctx, store, options.Environment, model.EntityTypeCharacter, pageSize)
	if err != nil {
		return EvidenceBridgeAudit{}, err
	}
	for _, item := range characters {
		if ownerCap := nonEmptyFactString(item, "owner_cap_id"); ownerCap != "" {
			counts.CharactersWithOwnerCap++
			ownerCaps[ownerCap] = struct{}{}
		}
	}
	systems, err := listAllCurrentEntities(ctx, store, options.Environment, model.EntityTypeSystem, pageSize)
	if err != nil {
		return EvidenceBridgeAudit{}, err
	}
	for _, item := range systems {
		if locationHash := nonEmptyFactString(item, "location_hash"); locationHash != "" {
			counts.SystemsWithLocationHash++
			locationHashes[locationHash] = struct{}{}
		}
	}
	counts.UniqueOwnerCapValues = int64(len(ownerCaps))
	counts.UniqueLocationHashValues = int64(len(locationHashes))
	return EvidenceBridgeAudit{
		SchemaVersion: "registry.evidence-bridge-audit.v1",
		Environment:   options.Environment,
		GeneratedAt:   now,
		Counts:        counts,
		Samples:       samples,
	}, nil
}

func BuildCurrentStateAudit(ctx context.Context, store CurrentStateStore, options CurrentStateAuditOptions) (CurrentStateAudit, error) {
	if options.Environment == "" {
		options.Environment = model.EnvironmentStillness
	}
	now := time.Now().UTC()
	if options.Now != nil {
		now = options.Now().UTC()
	}
	pageSize := boundedPageSize(options.PageSize, 200)
	var counts CurrentStateAuditCounts
	for _, entityType := range []model.EntityType{
		model.EntityTypeCharacter,
		model.EntityTypeTribe,
		model.EntityTypeAssembly,
		model.EntityTypeGate,
		model.EntityTypeStorage,
		model.EntityTypeTurret,
		model.EntityTypeSystem,
		model.EntityTypeRoute,
	} {
		items, err := listAllCurrentEntities(ctx, store, options.Environment, entityType, pageSize)
		if err != nil {
			return CurrentStateAudit{}, err
		}
		countCurrentEntities(&counts, entityType, items)
	}
	ownership, err := countCurrentRelations(ctx, store, db.CurrentRelationQuery{
		Environment: options.Environment,
		Predicates:  []string{"owned_by"},
		Limit:       pageSize,
	})
	if err != nil {
		return CurrentStateAudit{}, err
	}
	routeEdges, err := countCurrentRelations(ctx, store, db.CurrentRelationQuery{
		Environment: options.Environment,
		Predicates:  []string{"links_to", "observed_between"},
		Limit:       pageSize,
	})
	if err != nil {
		return CurrentStateAudit{}, err
	}
	ownerCaps, err := countCurrentRelations(ctx, store, db.CurrentRelationQuery{
		Environment: options.Environment,
		Predicates:  []string{"has_owner_cap"},
		Limit:       pageSize,
	})
	if err != nil {
		return CurrentStateAudit{}, err
	}
	locationHashes, err := countCurrentRelations(ctx, store, db.CurrentRelationQuery{
		Environment: options.Environment,
		Predicates:  []string{"has_location_hash"},
		Limit:       pageSize,
	})
	if err != nil {
		return CurrentStateAudit{}, err
	}
	counts.OwnershipRelations = ownership
	counts.OwnerCapRelations = ownerCaps
	counts.LocationHashRelations = locationHashes
	counts.RouteEdgeRelations = routeEdges
	return CurrentStateAudit{
		SchemaVersion: "registry.current-state-audit.v1",
		Environment:   options.Environment,
		GeneratedAt:   now,
		Counts:        counts,
	}, nil
}

func boundedPageSize(value, max int) int {
	if value <= 0 {
		return max
	}
	if value > max {
		return max
	}
	return value
}

func appendSample(samples *[]KillmailSample, sample KillmailSample, limit int) {
	if len(*samples) >= limit {
		return
	}
	*samples = append(*samples, sample)
}

func appendCurrentEntitySample(samples *[]CurrentEntitySample, sample CurrentEntitySample, limit int) {
	if len(*samples) >= limit {
		return
	}
	*samples = append(*samples, sample)
}

func appendEvidenceBridgeSample(samples *[]EvidenceBridgeSample, sample EvidenceBridgeSample, limit int) {
	if len(*samples) >= limit {
		return
	}
	*samples = append(*samples, sample)
}

func currentEntitySample(item model.CurrentEntity) CurrentEntitySample {
	return CurrentEntitySample{
		EntityID:    item.Entity.ID,
		DisplayName: displayName(item.Entity),
		SourceIDs:   append([]string(nil), item.SourceIDs...),
	}
}

func evidenceBridgeSample(item model.CurrentEntity, value string) EvidenceBridgeSample {
	return EvidenceBridgeSample{
		EntityID:    item.Entity.ID,
		DisplayName: displayName(item.Entity),
		Value:       value,
		SourceIDs:   append([]string(nil), item.SourceIDs...),
	}
}

func displayName(entity model.Entity) string {
	if strings.TrimSpace(entity.DisplayName) != "" {
		return entity.DisplayName
	}
	return entity.Name
}

func nonEmptyFactString(item model.CurrentEntity, key string) string {
	value, ok := item.Facts[key]
	if !ok || value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func hasDerivedRelated(item model.CurrentEntity, name string) bool {
	if item.Derived == nil {
		return false
	}
	switch name {
	case "owner_cap":
		return item.Derived.OwnerCap != nil
	case "location_hash":
		return item.Derived.LocationHash != nil
	default:
		return false
	}
}

func killmailSample(raw model.KillmailRaw, semantic model.SemanticKillmail) KillmailSample {
	return KillmailSample{
		ID:           raw.ID,
		OccurredAt:   raw.OccurredAt,
		System:       rawValue(semantic.System.DisplayName, raw.SystemID, raw.SystemName),
		Victim:       rawValue(semantic.Victim.DisplayName, raw.VictimCharacterID, raw.VictimName),
		Killer:       rawValue(semantic.Killer.DisplayName, raw.KillerCharacterID, raw.KillerName),
		KillerTypeID: raw.KillerTypeID,
		Reporter:     rawValue(semantic.Reporter.DisplayName, raw.ReporterCharacterID, raw.ReporterName),
		SourceIDs:    raw.SourceIDs,
		Warnings:     semantic.Warnings,
	}
}

func rawValue(values ...string) string {
	for _, value := range values {
		if value != "" && value != "Unknown" {
			return value
		}
	}
	return ""
}

func listAllCurrentEntities(ctx context.Context, store CurrentStateStore, environment model.Environment, entityType model.EntityType, pageSize int) ([]model.CurrentEntity, error) {
	cursor := ""
	var out []model.CurrentEntity
	for {
		page, err := store.ListCurrentEntities(ctx, db.CurrentEntityQuery{
			Type:        entityType,
			Environment: environment,
			Limit:       pageSize,
			Cursor:      cursor,
		})
		if err != nil {
			return nil, err
		}
		out = append(out, page.Items...)
		if page.NextCursor == "" || len(page.Items) == 0 {
			break
		}
		cursor = page.NextCursor
	}
	return out, nil
}

func countCurrentEntities(counts *CurrentStateAuditCounts, entityType model.EntityType, items []model.CurrentEntity) {
	for _, item := range items {
		switch entityType {
		case model.EntityTypeCharacter:
			counts.Characters++
			if item.Derived != nil && item.Derived.Tribe != nil {
				counts.CharactersWithTribe++
			}
			if item.Derived != nil && item.Derived.PublicActivityCount > 0 {
				counts.CharactersWithActivity++
			}
		case model.EntityTypeTribe:
			counts.Tribes++
			if item.Derived != nil && item.Derived.MemberCount > 0 {
				counts.TribesWithMembers++
			}
		case model.EntityTypeAssembly:
			counts.Assemblies++
			countOwnerAndSystem(item, &counts.AssembliesWithOwner, &counts.AssembliesWithSystem)
			countEvidenceRelations(item, &counts.AssembliesWithOwnerCap, &counts.AssembliesWithLocationHash)
		case model.EntityTypeGate:
			counts.Gates++
			if hasCurrentRelation(item, "links_to", model.EntityTypeGate) {
				counts.GatesWithLinkedGate++
			}
			countEvidenceRelations(item, &counts.GatesWithOwnerCap, &counts.GatesWithLocationHash)
		case model.EntityTypeStorage:
			counts.Storage++
			countOwnerAndSystem(item, &counts.StorageWithOwner, &counts.StorageWithSystem)
			countEvidenceRelations(item, &counts.StorageWithOwnerCap, &counts.StorageWithLocationHash)
		case model.EntityTypeTurret:
			counts.Turrets++
			countOwnerAndSystem(item, &counts.TurretsWithOwner, &counts.TurretsWithSystem)
			countEvidenceRelations(item, &counts.TurretsWithOwnerCap, &counts.TurretsWithLocationHash)
		case model.EntityTypeSystem:
			counts.Systems++
			if item.Derived != nil && item.Derived.PublicActivityCount > 0 {
				counts.SystemsWithActivity++
			}
			if item.Derived != nil && item.Derived.ConnectedSystemCount > 0 {
				counts.SystemsWithConnections++
			}
		case model.EntityTypeRoute:
			counts.RouteEntities++
		}
	}
}

func countEvidenceRelations(item model.CurrentEntity, ownerCapCount, locationHashCount *int64) {
	if hasOutgoingResourceRelation(item, "has_owner_cap") {
		(*ownerCapCount)++
	}
	if hasOutgoingResourceRelation(item, "has_location_hash") {
		(*locationHashCount)++
	}
}

func countOwnerAndSystem(item model.CurrentEntity, ownerCount, systemCount *int64) {
	if item.Derived != nil && item.Derived.Owner != nil {
		(*ownerCount)++
	}
	if item.Derived != nil && item.Derived.System != nil {
		(*systemCount)++
	}
}

func hasOutgoingResourceRelation(item model.CurrentEntity, predicate string) bool {
	for _, relation := range item.OutgoingRelations {
		if relation.Predicate == predicate && relation.ObjectEntityType == model.EntityTypeResourceObject {
			return true
		}
	}
	return false
}

func hasCurrentRelation(item model.CurrentEntity, predicate string, relatedType model.EntityType) bool {
	for _, relation := range item.OutgoingRelations {
		if relation.Predicate == predicate && relation.ObjectEntityType == relatedType {
			return true
		}
	}
	for _, relation := range item.IncomingRelations {
		if relation.Predicate == predicate && relation.SubjectEntityType == relatedType {
			return true
		}
	}
	return false
}

func countCurrentRelations(ctx context.Context, store CurrentStateStore, query db.CurrentRelationQuery) (int64, error) {
	cursor := ""
	var total int64
	for {
		query.Cursor = cursor
		page, err := store.ListCurrentRelations(ctx, query)
		if err != nil {
			return 0, err
		}
		total += int64(len(page.Items))
		if page.NextCursor == "" || len(page.Items) == 0 {
			break
		}
		cursor = page.NextCursor
	}
	return total, nil
}
