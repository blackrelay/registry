package sui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/blackrelay/registry/internal/cursor"
	"github.com/blackrelay/registry/internal/cycles"
	"github.com/blackrelay/registry/internal/db"
	"github.com/blackrelay/registry/internal/model"
)

type DerivedEventGraph struct {
	Entities  []DerivedEventEntity
	Relations []db.RelationDraft
	Killmails []model.KillmailRaw
}

type DerivedEventEntity struct {
	Entity model.Entity
	Facts  []db.EntityFactDraft
}

type EventDerivationOptions struct {
	Environment     model.Environment
	Network         string
	Cycles          []int
	IncludeUncycled bool
	Modules         []string
	BatchSize       int
	MaxBatches      int
	ResetCursors    bool
}

type EventDerivationSummary struct {
	Environment      model.Environment `json:"environment"`
	Network          string            `json:"network"`
	Modules          []string          `json:"modules,omitempty"`
	Cursor           string            `json:"cursor,omitempty"`
	Cursors          map[string]string `json:"cursors,omitempty"`
	Batches          int               `json:"batches"`
	EventsScanned    int64             `json:"eventsScanned"`
	EntitiesDerived  int64             `json:"entitiesDerived"`
	RelationsDerived int64             `json:"relationsDerived"`
	KillmailsDerived int64             `json:"killmailsDerived"`
}

type EventDerivationStore interface {
	ListEvents(ctx context.Context, query db.EventQuery) (db.EventPage, error)
	UpsertEntityFacts(ctx context.Context, entity model.Entity, facts []db.EntityFactDraft) error
	UpsertRelations(ctx context.Context, relations []db.RelationDraft) error
	UpsertKillmail(ctx context.Context, raw model.KillmailRaw) error
	GetSyncCursor(ctx context.Context, id string) (db.CursorStatus, bool, error)
	SaveSyncCursor(ctx context.Context, item db.CursorStatus) error
}

type EventDerivationBatchStore interface {
	UpsertEventDerivationBatch(ctx context.Context, entities []db.EntityFactSet, relations []db.RelationDraft, killmails []model.KillmailRaw) error
}

type eventDerivationBatch struct {
	Entities  []db.EntityFactSet
	Relations []db.RelationDraft
	Killmails []model.KillmailRaw
}

type tenantItem struct {
	Tenant string
	ItemID string
}

func DeriveEntitiesFromEvent(event db.EventRecord) DerivedEventGraph {
	payload := eventPayload(event)
	if payload == nil || event.SourceID == "" {
		return DerivedEventGraph{}
	}
	builder := eventGraphBuilder{
		event:       event,
		entities:    make(map[string]DerivedEventEntity),
		relationSet: make(map[string]struct{}),
	}
	switch event.Kind {
	case "character.created":
		builder.characterCreated(payload)
	case "killmail.created":
		builder.killmailCreated(payload)
	case "assembly.created", "gate.created", "network_node.created", "storage_unit.created", "turret.created":
		builder.assemblyLike(payload)
	case "gate.jump":
		builder.gateJump(payload)
	default:
		builder.genericState(payload)
	}
	return builder.graph()
}

func RunEventDerivation(ctx context.Context, store EventDerivationStore, options EventDerivationOptions) (EventDerivationSummary, error) {
	if store == nil {
		return EventDerivationSummary{}, fmt.Errorf("event derivation store is required")
	}
	if options.Environment == "" {
		options.Environment = model.EnvironmentStillness
	}
	if options.Network == "" {
		options.Network = "sui-testnet"
	}
	if options.BatchSize <= 0 {
		options.BatchSize = 1000
	}
	modules := normaliseDerivationModules(options.Modules)
	summary := EventDerivationSummary{Environment: options.Environment, Network: options.Network, Modules: modules}
	if len(modules) > 0 {
		summary.Cursors = make(map[string]string, len(modules))
	}
	targets := []string{""}
	if len(modules) > 0 {
		targets = modules
	}
	for _, module := range targets {
		if options.MaxBatches > 0 && summary.Batches >= options.MaxBatches {
			break
		}
		targetOptions := options
		if options.MaxBatches > 0 {
			targetOptions.MaxBatches = options.MaxBatches - summary.Batches
		}
		targetSummary, err := runEventDerivationTarget(ctx, store, targetOptions, module)
		summary.Batches += targetSummary.Batches
		summary.EventsScanned += targetSummary.EventsScanned
		summary.EntitiesDerived += targetSummary.EntitiesDerived
		summary.RelationsDerived += targetSummary.RelationsDerived
		summary.KillmailsDerived += targetSummary.KillmailsDerived
		if targetSummary.Cursor != "" {
			summary.Cursor = targetSummary.Cursor
			if module != "" {
				summary.Cursors[module] = targetSummary.Cursor
			}
		}
		if err != nil {
			return summary, err
		}
	}
	return summary, nil
}

func runEventDerivationTarget(ctx context.Context, store EventDerivationStore, options EventDerivationOptions, module string) (EventDerivationSummary, error) {
	cursorID := DeriveEventsCursorID(options.Network, options.Environment)
	cursorSource := DeriveEventsCursorSource(options.Network)
	if module != "" {
		cursorID = DeriveEventsModuleCursorID(options.Network, options.Environment, module)
		cursorSource = DeriveEventsModuleCursorSource(options.Network, module)
	}
	cursorStatus := db.CursorStatus{
		ID:          cursorID,
		Source:      cursorSource,
		Environment: options.Environment,
		CursorKind:  "sui_event_derivation",
		UpdatedAt:   time.Now().UTC(),
	}
	after := ""
	if !options.ResetCursors {
		saved, ok, err := store.GetSyncCursor(ctx, cursorID)
		if err != nil {
			return EventDerivationSummary{}, err
		}
		if ok {
			cursorStatus = saved
			after = saved.CursorValue
		}
	}
	summary := EventDerivationSummary{Environment: options.Environment, Network: options.Network}
	if module != "" {
		summary.Modules = []string{module}
	}
	for {
		if options.MaxBatches > 0 && summary.Batches >= options.MaxBatches {
			break
		}
		page, err := store.ListEvents(ctx, db.EventQuery{
			Environment:     options.Environment,
			Cycles:          options.Cycles,
			IncludeUncycled: options.IncludeUncycled,
			Module:          module,
			Limit:           options.BatchSize,
			MaxLimit:        options.BatchSize,
			Cursor:          after,
			Ascending:       true,
		})
		if err != nil {
			cursorStatus.ErrorCount++
			cursorStatus.LastErrorSummary = err.Error()
			_ = store.SaveSyncCursor(ctx, cursorStatus)
			return summary, err
		}
		if len(page.Items) == 0 {
			break
		}
		summary.Batches++
		batch := deriveEventBatch(page.Items)
		summary.EventsScanned += int64(len(page.Items))
		summary.EntitiesDerived += int64(len(batch.Entities))
		summary.RelationsDerived += int64(len(batch.Relations))
		summary.KillmailsDerived += int64(len(batch.Killmails))
		if err := writeEventDerivationBatch(ctx, store, batch); err != nil {
			cursorStatus.ErrorCount++
			cursorStatus.LastErrorSummary = err.Error()
			_ = store.SaveSyncCursor(ctx, cursorStatus)
			return summary, err
		}
		last := page.Items[len(page.Items)-1]
		lastCursor, err := cursor.Encode(cursor.Keyset{Time: last.OccurredAt, ID: last.ID})
		if err != nil {
			return summary, err
		}
		after = lastCursor
		cursorStatus.CursorValue = lastCursor
		cursorStatus.EventsProcessed += int64(len(page.Items))
		now := time.Now().UTC()
		cursorStatus.LastSuccessfulIngest = &now
		cursorStatus.LastErrorSummary = ""
		if err := store.SaveSyncCursor(ctx, cursorStatus); err != nil {
			return summary, err
		}
		summary.Cursor = cursorStatus.CursorValue
		if page.NextCursor == "" {
			break
		}
	}
	return summary, nil
}

func deriveEventBatch(events []db.EventRecord) eventDerivationBatch {
	batch := eventDerivationBatch{}
	for _, event := range events {
		graph := DeriveEntitiesFromEvent(event)
		for _, entity := range graph.Entities {
			batch.Entities = append(batch.Entities, db.EntityFactSet{Entity: entity.Entity, Facts: entity.Facts})
		}
		batch.Relations = append(batch.Relations, graph.Relations...)
		batch.Killmails = append(batch.Killmails, graph.Killmails...)
	}
	return batch
}

func writeEventDerivationBatch(ctx context.Context, store EventDerivationStore, batch eventDerivationBatch) error {
	if bulkStore, ok := store.(EventDerivationBatchStore); ok {
		return bulkStore.UpsertEventDerivationBatch(ctx, batch.Entities, batch.Relations, batch.Killmails)
	}
	for _, entity := range batch.Entities {
		if err := store.UpsertEntityFacts(ctx, entity.Entity, entity.Facts); err != nil {
			return err
		}
	}
	for _, raw := range batch.Killmails {
		if err := store.UpsertKillmail(ctx, raw); err != nil {
			return err
		}
	}
	return store.UpsertRelations(ctx, batch.Relations)
}

func DeriveEventsCursorID(network string, environment model.Environment) string {
	return "cursor:" + DeriveEventsCursorSource(network) + ":" + string(environment)
}

func DeriveEventsModuleCursorID(network string, environment model.Environment, module string) string {
	module = strings.TrimSpace(module)
	if module == "" {
		return DeriveEventsCursorID(network, environment)
	}
	return "cursor:" + DeriveEventsModuleCursorSource(network, module) + ":" + string(environment)
}

func DeriveEventsCursorSource(network string) string {
	if network == "" {
		network = "sui-testnet"
	}
	return "registry:derive:sui-events:" + network
}

func DeriveEventsModuleCursorSource(network, module string) string {
	module = strings.TrimSpace(module)
	if module == "" {
		return DeriveEventsCursorSource(network)
	}
	return DeriveEventsCursorSource(network) + ":module:" + module
}

func normaliseDerivationModules(modules []string) []string {
	seen := make(map[string]struct{}, len(modules))
	out := make([]string, 0, len(modules))
	for _, module := range modules {
		module = strings.TrimSpace(module)
		if module == "" {
			continue
		}
		if _, ok := seen[module]; ok {
			continue
		}
		seen[module] = struct{}{}
		out = append(out, module)
	}
	return out
}

type eventGraphBuilder struct {
	event       db.EventRecord
	entities    map[string]DerivedEventEntity
	relations   []db.RelationDraft
	relationSet map[string]struct{}
	killmails   []model.KillmailRaw
}

func (b *eventGraphBuilder) characterCreated(payload map[string]any) {
	key, ok := tenantItemID(payload["key"])
	if !ok {
		return
	}
	displayName := objectDisplayName(payload)
	characterID := b.character(key, [][2]any{
		{"character_id", stringFrom(payload["character_id"])},
		{"tribe_id", numberOrString(payload["tribe_id"])},
		{"character_address", stringFrom(payload["character_address"])},
		{"owner_cap_id", stringFrom(payload["owner_cap_id"])},
		{"metadata_name", metadataName(payload["metadata"])},
		{"metadata_description", metadataField(payload["metadata"], "description")},
		{"metadata_url", metadataField(payload["metadata"], "url")},
	}, displayName)
	if tribeID := stringFrom(numberOrString(payload["tribe_id"])); tribeID != "" {
		tribe := tenantItem{Tenant: tenantOrEnvironment(key.Tenant, b.event.Environment), ItemID: tribeID}
		tribeEntityID := b.tribe(tribe, [][2]any{
			{"observed_character_item_id", key.ItemID},
			{"observed_character_id", stringFrom(payload["character_id"])},
		})
		b.relation(characterID, "belongs_to", tribeEntityID)
	}
}

func (b *eventGraphBuilder) killmailCreated(payload map[string]any) {
	key, ok := tenantItemID(payload["key"])
	if !ok {
		return
	}
	killmailID := entityID(model.EntityTypeKillmail, tenantOrEnvironment(key.Tenant, b.event.Environment), key.ItemID)
	facts := b.facts()
	facts.add("item_id", key.ItemID)
	facts.add("tenant", tenantOrEnvironment(key.Tenant, b.event.Environment))
	facts.add("killer_character_item_id", tenantItemFact(payload["killer_id"]))
	facts.add("victim_character_item_id", tenantItemFact(payload["victim_id"]))
	facts.add("reported_by_character_item_id", tenantItemFact(payload["reported_by_character_id"]))
	facts.add("loss_type", variantValue(payload["loss_type"]))
	facts.add("kill_timestamp", numberOrString(payload["kill_timestamp"]))
	facts.add("solar_system_id", tenantItemFact(payload["solar_system_id"]))
	b.addEntity(model.Entity{
		ID:          killmailID,
		Slug:        entitySlug(model.EntityTypeKillmail, key),
		Type:        model.EntityTypeKillmail,
		Name:        "Killmail " + key.ItemID,
		DisplayName: "Killmail " + key.ItemID,
		Summary:     "Public on-chain killmail observed from Sui event data.",
		Environment: b.event.Environment,
		UpdatedAt:   time.Now().UTC(),
	}, facts.values())

	raw := model.KillmailRaw{
		ID:          killmailID,
		Environment: b.event.Environment,
		OccurredAt:  b.event.OccurredAt,
		LossType:    variantValue(payload["loss_type"]),
		SourceIDs:   []string{b.event.SourceID},
		Raw:         map[string]any{"sourceEventId": b.event.ID, "event": b.event.Payload},
	}
	if victim, ok := tenantItemID(payload["victim_id"]); ok {
		victimID := b.character(victim, [][2]any{{"victim_id", true}})
		raw.VictimCharacterID = victimID
		b.relation(killmailID, "victim", victimID)
	}
	if killer, ok := tenantItemID(payload["killer_id"]); ok {
		killerID := b.character(killer, [][2]any{{"killer_id", true}})
		raw.KillerCharacterID = killerID
		b.relation(killmailID, "killer", killerID)
	}
	if reporter, ok := tenantItemID(payload["reported_by_character_id"]); ok {
		reporterID := b.character(reporter, [][2]any{{"reported_by_character_id", true}})
		raw.ReporterCharacterID = reporterID
		b.relation(killmailID, "reported_by", reporterID)
	}
	if system, ok := tenantItemID(payload["solar_system_id"]); ok {
		systemID := b.system(system)
		raw.SystemID = systemID
		b.relation(killmailID, "occurred_in", systemID)
	}
	if typeID := stringFrom(numberOrString(payload["killer_type_id"])); typeID != "" {
		raw.KillerTypeID = typeID
	}
	if occurredAt := killTimestamp(payload["kill_timestamp"]); !occurredAt.IsZero() {
		raw.OccurredAt = occurredAt
	}
	b.killmails = append(b.killmails, raw)
}

func (b *eventGraphBuilder) assemblyLike(payload map[string]any) {
	entityType, label, keyNames := assemblyLikeShape(b.event.Kind, b.event.Module)
	key, ok := firstTenantItem(payload, keyNames)
	if !ok {
		return
	}
	entityID := b.objectLike(entityType, label, key, payload, nil)
	if linked := stringFrom(numberOrString(payload["linked_gate_id"])); linked != "" && entityType == model.EntityTypeGate {
		target := tenantItem{Tenant: tenantOrEnvironment(key.Tenant, b.event.Environment), ItemID: linked}
		targetID := b.objectLike(model.EntityTypeGate, "Gate", target, map[string]any{}, [][2]any{{"linked_gate_placeholder", true}})
		b.relation(entityID, "links_to", targetID)
	}
	if owner, ok := firstTenantItem(payload, []string{"owner_character_id", "owner_id"}); ok {
		ownerID := b.character(owner, [][2]any{{"owner_observed", true}})
		b.relation(entityID, "owned_by", ownerID)
	}
}

func (b *eventGraphBuilder) gateJump(payload map[string]any) {
	sourceGate, sourceOK := firstTenantItem(payload, []string{"source_gate_key", "from_gate_key"})
	destinationGate, destinationOK := firstTenantItem(payload, []string{"destination_gate_key", "to_gate_key"})
	if !sourceOK || !destinationOK {
		return
	}
	sourceGateID := b.objectLike(model.EntityTypeGate, "Gate", sourceGate, payload, [][2]any{{"jump_observed", true}})
	destinationGateID := b.objectLike(model.EntityTypeGate, "Gate", destinationGate, payload, [][2]any{{"jump_observed", true}})
	b.relation(sourceGateID, "links_to", destinationGateID)
	tenant := tenantOrEnvironment(sourceGate.Tenant, b.event.Environment)
	routeID := fmt.Sprintf("route:%s:%s:%s", tenant, sourceGate.ItemID, destinationGate.ItemID)
	facts := b.facts()
	facts.add("tenant", tenant)
	facts.add("source_gate_item_id", sourceGate.ItemID)
	facts.add("destination_gate_item_id", destinationGate.ItemID)
	if character, ok := tenantItemID(payload["character_id"]); ok {
		facts.add("character_item_id", character.ItemID)
		characterID := b.character(character, [][2]any{{"gate_jump_observed", true}})
		b.relation(characterID, "traversed", routeID)
	}
	b.addEntity(model.Entity{
		ID:          routeID,
		Slug:        slugify(fmt.Sprintf("route-%s-to-%s-%s", sourceGate.ItemID, destinationGate.ItemID, tenant)),
		Type:        model.EntityTypeRoute,
		Name:        fmt.Sprintf("Route %s to %s", sourceGate.ItemID, destinationGate.ItemID),
		DisplayName: fmt.Sprintf("Route %s to %s", sourceGate.ItemID, destinationGate.ItemID),
		Summary:     "Public on-chain gate traversal observed from Sui event data.",
		Environment: b.event.Environment,
		UpdatedAt:   time.Now().UTC(),
	}, facts.values())
	b.relation(routeID, "observed_between", sourceGateID)
	b.relation(routeID, "observed_between", destinationGateID)
}

func (b *eventGraphBuilder) genericState(payload map[string]any) {
	entityType, label, keyNames := assemblyLikeShape("", b.event.Module)
	if entityType == model.EntityTypeUnknown {
		return
	}
	key, ok := firstTenantItem(payload, append([]string{"key"}, keyNames...))
	if !ok {
		return
	}
	b.objectLike(entityType, label, key, payload, [][2]any{{"observed_event_payload", payload}})
}

func (b *eventGraphBuilder) character(key tenantItem, extra [][2]any, displayName ...string) string {
	entityID := entityID(model.EntityTypeCharacter, tenantOrEnvironment(key.Tenant, b.event.Environment), key.ItemID)
	name := strings.TrimSpace(firstStringValue(displayName...))
	if name == "" {
		name = "Character " + key.ItemID
	}
	facts := b.facts()
	facts.add("item_id", key.ItemID)
	facts.add("tenant", tenantOrEnvironment(key.Tenant, b.event.Environment))
	for _, fact := range extra {
		facts.add(fact[0].(string), fact[1])
	}
	b.addEntity(model.Entity{
		ID:          entityID,
		Slug:        entitySlug(model.EntityTypeCharacter, key),
		Type:        model.EntityTypeCharacter,
		Name:        name,
		DisplayName: name,
		Summary:     "Public on-chain character identity observed from Sui event data.",
		Environment: b.event.Environment,
		UpdatedAt:   time.Now().UTC(),
	}, facts.values())
	return entityID
}

func (b *eventGraphBuilder) tribe(key tenantItem, extra [][2]any) string {
	entityID := entityID(model.EntityTypeTribe, tenantOrEnvironment(key.Tenant, b.event.Environment), key.ItemID)
	facts := b.facts()
	facts.add("tribe_id", key.ItemID)
	facts.add("tenant", tenantOrEnvironment(key.Tenant, b.event.Environment))
	for _, fact := range extra {
		facts.add(fact[0].(string), fact[1])
	}
	b.addEntity(model.Entity{
		ID:          entityID,
		Slug:        entitySlug(model.EntityTypeTribe, key),
		Type:        model.EntityTypeTribe,
		Name:        "Tribe " + key.ItemID,
		DisplayName: "Tribe " + key.ItemID,
		Summary:     "Public on-chain tribe identity observed from Sui event data.",
		Environment: b.event.Environment,
		UpdatedAt:   time.Now().UTC(),
	}, facts.values())
	return entityID
}

func (b *eventGraphBuilder) system(key tenantItem) string {
	entityID := entityID(model.EntityTypeSystem, tenantOrEnvironment(key.Tenant, b.event.Environment), key.ItemID)
	facts := b.facts()
	facts.add("item_id", key.ItemID)
	facts.add("tenant", tenantOrEnvironment(key.Tenant, b.event.Environment))
	b.addEntity(model.Entity{
		ID:          entityID,
		Slug:        entitySlug(model.EntityTypeSystem, key),
		Type:        model.EntityTypeSystem,
		Name:        "System " + key.ItemID,
		DisplayName: "System " + key.ItemID,
		Summary:     "Public on-chain solar system reference observed from Sui event data.",
		Environment: b.event.Environment,
		UpdatedAt:   time.Now().UTC(),
	}, facts.values())
	return entityID
}

func (b *eventGraphBuilder) objectLike(entityType model.EntityType, label string, key tenantItem, payload map[string]any, extra [][2]any) string {
	entityID := entityID(entityType, tenantOrEnvironment(key.Tenant, b.event.Environment), key.ItemID)
	name := metadataName(payload["metadata"])
	if name == "" {
		name = label + " " + key.ItemID
	}
	facts := b.facts()
	facts.add("item_id", key.ItemID)
	facts.add("tenant", tenantOrEnvironment(key.Tenant, b.event.Environment))
	facts.add("object_id", firstString(payload, []string{"assembly_id", "gate_id", "network_node_id", "storage_unit_id", "turret_id"}))
	facts.add("owner_cap_id", stringFrom(payload["owner_cap_id"]))
	facts.add("type_id", numberOrString(payload["type_id"]))
	facts.add("status", variantValue(payload["status"]))
	facts.add("location_hash", locationHash(payload["location"]))
	facts.add("metadata_name", metadataName(payload["metadata"]))
	facts.add("metadata_description", metadataField(payload["metadata"], "description"))
	facts.add("metadata_url", metadataField(payload["metadata"], "url"))
	for _, fact := range extra {
		facts.add(fact[0].(string), fact[1])
	}
	b.addEntity(model.Entity{
		ID:          entityID,
		Slug:        entitySlug(entityType, key),
		Type:        entityType,
		Name:        name,
		DisplayName: name,
		Summary:     fmt.Sprintf("Public on-chain %s identity observed from Sui event data.", strings.ToLower(label)),
		Environment: b.event.Environment,
		UpdatedAt:   time.Now().UTC(),
	}, facts.values())
	return entityID
}

func (b *eventGraphBuilder) addEntity(entity model.Entity, facts []db.EntityFactDraft) {
	if entity.Cycle == nil {
		entity.Cycle = b.cycle()
	}
	existing, ok := b.entities[entity.ID]
	if !ok {
		b.entities[entity.ID] = DerivedEventEntity{Entity: entity, Facts: facts}
		return
	}
	if existing.Entity.Cycle == nil && entity.Cycle != nil {
		existing.Entity.Cycle = entity.Cycle
	}
	seen := make(map[string]struct{}, len(existing.Facts))
	for _, fact := range existing.Facts {
		seen[fact.Key+":"+fmt.Sprint(fact.Value)] = struct{}{}
	}
	for _, fact := range facts {
		key := fact.Key + ":" + fmt.Sprint(fact.Value)
		if _, ok := seen[key]; ok {
			continue
		}
		existing.Facts = append(existing.Facts, fact)
	}
	b.entities[entity.ID] = existing
}

func (b *eventGraphBuilder) relation(subject, predicate, object string) {
	if subject == "" || predicate == "" || object == "" {
		return
	}
	relation := db.RelationDraft{
		SubjectEntityID: subject,
		Predicate:       predicate,
		ObjectEntityID:  object,
		SourceID:        b.event.SourceID,
		Confidence:      model.ConfidenceVerified,
		Environment:     b.event.Environment,
		ValidFrom:       &b.event.OccurredAt,
	}
	id := db.RelationID(relation)
	if _, ok := b.relationSet[id]; ok {
		return
	}
	b.relationSet[id] = struct{}{}
	b.relations = append(b.relations, relation)
}

func (b *eventGraphBuilder) facts() *eventFactBuilder {
	return &eventFactBuilder{event: b.event}
}

func (b *eventGraphBuilder) cycle() *int {
	if b.event.Cycle != nil {
		return b.event.Cycle
	}
	return cycles.FromTime(b.event.OccurredAt)
}

func (b *eventGraphBuilder) graph() DerivedEventGraph {
	entities := make([]DerivedEventEntity, 0, len(b.entities))
	for _, entity := range b.entities {
		entities = append(entities, entity)
	}
	return DerivedEventGraph{Entities: entities, Relations: b.relations, Killmails: b.killmails}
}

type eventFactBuilder struct {
	event db.EventRecord
	items []db.EntityFactDraft
}

func (b *eventFactBuilder) add(key string, value any) {
	if value == nil {
		return
	}
	if valueString, ok := value.(string); ok && valueString == "" {
		return
	}
	b.items = append(b.items, db.EntityFactDraft{
		Key:          key,
		Value:        value,
		SourceID:     b.event.SourceID,
		Confidence:   model.ConfidenceVerified,
		Environment:  b.event.Environment,
		Cycle:        b.cycle(),
		ReviewStatus: model.ReviewStatusReviewed,
	})
}

func (b *eventFactBuilder) cycle() *int {
	if b.event.Cycle != nil {
		return b.event.Cycle
	}
	return cycles.FromTime(b.event.OccurredAt)
}

func (b *eventFactBuilder) values() []db.EntityFactDraft {
	b.add("source_event_id", b.event.ID)
	b.add("source_event_kind", b.event.Kind)
	b.add("transaction_digest", b.event.TransactionDigest)
	b.add("package_id", b.event.PackageID)
	b.add("module", b.event.Module)
	b.add("observed_at", b.event.OccurredAt.Format(time.RFC3339Nano))
	return b.items
}

func eventPayload(event db.EventRecord) map[string]any {
	value, ok := event.Payload["json"].(map[string]any)
	if ok && value != nil {
		return value
	}
	return nil
}

func tenantItemID(value any) (tenantItem, bool) {
	record, ok := value.(map[string]any)
	if !ok {
		return tenantItem{}, false
	}
	itemID := stringFrom(numberOrString(record["item_id"]))
	if itemID == "" {
		return tenantItem{}, false
	}
	return tenantItem{Tenant: stringFrom(record["tenant"]), ItemID: itemID}, true
}

func tenantItemFact(value any) string {
	key, ok := tenantItemID(value)
	if !ok {
		return ""
	}
	return key.ItemID
}

func firstTenantItem(payload map[string]any, keys []string) (tenantItem, bool) {
	for _, key := range keys {
		if value, ok := tenantItemID(payload[key]); ok {
			return value, true
		}
	}
	return tenantItem{}, false
}

func firstString(payload map[string]any, keys []string) string {
	for _, key := range keys {
		if value := stringFrom(payload[key]); value != "" {
			return value
		}
	}
	return ""
}

func firstStringValue(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func entityID(entityType model.EntityType, tenant, itemID string) string {
	return fmt.Sprintf("%s:%s:%s", entityType, tenant, itemID)
}

func entitySlug(entityType model.EntityType, key tenantItem) string {
	return slugify(strings.Join(compactStrings(string(entityType), key.ItemID, key.Tenant), "-"))
}

func tenantOrEnvironment(tenant string, environment model.Environment) string {
	if tenant != "" {
		return tenant
	}
	if environment != "" {
		return string(environment)
	}
	return string(model.EnvironmentUnknown)
}

func assemblyLikeShape(eventKind, module string) (model.EntityType, string, []string) {
	switch {
	case eventKind == "assembly.created" || module == "assembly":
		return model.EntityTypeAssembly, "Assembly", []string{"assembly_key", "key"}
	case eventKind == "gate.created" || module == "gate":
		return model.EntityTypeGate, "Gate", []string{"gate_key", "assembly_key", "key"}
	case eventKind == "network_node.created" || module == "network_node":
		return model.EntityTypeAssembly, "Network node", []string{"network_node_key", "assembly_key", "key"}
	case eventKind == "storage_unit.created" || module == "storage_unit":
		return model.EntityTypeStorage, "Storage", []string{"storage_unit_key", "assembly_key", "key"}
	case eventKind == "turret.created" || module == "turret":
		return model.EntityTypeTurret, "Turret", []string{"turret_key", "assembly_key", "key"}
	default:
		return model.EntityTypeUnknown, "", nil
	}
}

func killTimestamp(value any) time.Time {
	switch item := numberOrString(value).(type) {
	case int64:
		return timestampFromInt(item)
	case int:
		return timestampFromInt(int64(item))
	case string:
		var parsed int64
		if _, err := fmt.Sscan(item, &parsed); err == nil {
			return timestampFromInt(parsed)
		}
	}
	return time.Time{}
}

func timestampFromInt(value int64) time.Time {
	if value <= 0 {
		return time.Time{}
	}
	if value > 10_000_000_000 {
		return time.UnixMilli(value).UTC()
	}
	return time.Unix(value, 0).UTC()
}
