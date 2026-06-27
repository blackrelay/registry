package killmail

import (
	"context"
	"fmt"

	"github.com/blackrelay/registry/internal/model"
	"github.com/blackrelay/registry/internal/resolver"
)

type GraphStore interface {
	GetEntity(ctx context.Context, idOrSlug string) (model.Entity, bool, error)
	ListEntityFacts(ctx context.Context, entityID string) ([]model.Fact, error)
	ListEntityRelations(ctx context.Context, entityID string) ([]model.Relation, error)
}

type Service struct {
	Resolver   resolver.Resolver
	GraphStore GraphStore
}

func (s Service) Semantic(ctx context.Context, raw model.KillmailRaw) model.SemanticKillmail {
	raw = enrichFromRawPayload(raw)
	environment := raw.Environment
	if environment == "" {
		environment = model.EnvironmentUnknown
	}
	systemID := raw.SystemID
	if systemID == "" {
		systemID = raw.SystemName
	}
	victimID := raw.VictimCharacterID
	if victimID == "" {
		victimID = raw.VictimName
	}
	reporterID := raw.ReporterCharacterID
	if reporterID == "" {
		reporterID = raw.ReporterName
	}
	graph := s.graph(ctx, raw.ID)
	killer := s.resolveKiller(ctx, raw, environment, graph)
	out := model.SemanticKillmail{
		ID:         raw.ID,
		Kind:       "killmail",
		OccurredAt: raw.OccurredAt,
		System:     s.resolveSystem(ctx, systemID, environment, graph),
		Victim:     s.resolveCharacter(ctx, victimID, environment, graph.victim),
		Killer:     killer,
		Reporter:   s.resolveCharacter(ctx, reporterID, environment, graph.reporter),
		LossType:   raw.LossType,
		Sources:    mergeSources(raw.SourceIDs, graph.sources),
	}
	if out.Killer.EntityType == model.EntityTypeEnemy {
		out.Killer.IsNPC = true
	}
	out.SummaryText = killmailSummaryText(out.Killer, out.Victim)
	out.Warnings = appendResolvedWarnings(out.Warnings, out.System, out.Victim, out.Killer, out.Reporter)
	out.Warnings = append(out.Warnings, graph.warnings...)
	return out
}

func (s Service) resolveKiller(ctx context.Context, raw model.KillmailRaw, environment model.Environment, graph killmailGraph) model.ResolvedValue {
	if raw.KillerTypeID != "" {
		enemy := s.Resolver.EnemyType(ctx, raw.KillerTypeID, environment)
		if enemy.EntityType == model.EntityTypeEnemy && enemy.EntityID != "" && enemy.Confidence != model.ConfidenceUnknown {
			return enemy
		}
		if raw.KillerCharacterID == "" {
			return enemy
		}
	}
	if raw.KillerCharacterID != "" {
		character := s.Resolver.Character(ctx, raw.KillerCharacterID, environment)
		if character.EntityID != "" {
			return character
		}
		if killerID := killerItemIDFromRawPayload(raw.Raw); killerID != "" {
			character.Warnings = append(character.Warnings, "killer_id is not a static NPC type id; use explicit killer_type_id or a sourced killer relation to label NPC kills")
		}
		return character
	}
	if raw.KillerName != "" {
		return model.ResolvedValue{
			EntityType:  model.EntityTypeUnknown,
			RawID:       raw.KillerName,
			DisplayName: raw.KillerName,
			Confidence:  model.ConfidenceUnknown,
			Warnings:    []string{"killer type is not present"},
		}
	}
	if graph.killer.Entity.ID != "" {
		return graph.killer.resolved()
	}
	return model.ResolvedValue{
		EntityType:  model.EntityTypeUnknown,
		DisplayName: "Unknown",
		Confidence:  model.ConfidenceUnknown,
		Warnings:    []string{"killer could not be resolved"},
	}
}

func killerItemIDFromRawPayload(raw map[string]any) string {
	for _, payload := range rawPayloadCandidates(raw) {
		item, ok := payload["killer_id"].(map[string]any)
		if !ok {
			continue
		}
		if typeID := stringFromAny(item["item_id"]); typeID != "" {
			return typeID
		}
	}
	return ""
}

func killmailSummaryText(killer, victim model.ResolvedValue) string {
	killerName := killer.DisplayName
	if killerName == "" {
		killerName = "Unknown"
	}
	victimName := victim.DisplayName
	if victimName == "" {
		victimName = "Unknown"
	}
	return killerName + " killed " + victimName
}

func enrichFromRawPayload(raw model.KillmailRaw) model.KillmailRaw {
	for _, payload := range rawPayloadCandidates(raw.Raw) {
		if raw.KillerTypeID == "" {
			raw.KillerTypeID = stringFromAny(payload["killer_type_id"])
		}
		if raw.KillerCharacterID == "" {
			raw.KillerCharacterID = characterIDFromTenantItem(raw.Environment, payload["killer_id"])
		}
		if raw.VictimCharacterID == "" {
			raw.VictimCharacterID = characterIDFromTenantItem(raw.Environment, payload["victim_id"])
		}
		if raw.ReporterCharacterID == "" {
			raw.ReporterCharacterID = characterIDFromTenantItem(raw.Environment, payload["reported_by_character_id"])
		}
		if raw.SystemID == "" {
			raw.SystemID = systemIDFromTenantItem(raw.Environment, payload["solar_system_id"])
		}
		if raw.LossType == "" {
			raw.LossType = stringFromAny(payload["loss_type"])
		}
	}
	return raw
}

func rawPayloadCandidates(raw map[string]any) []map[string]any {
	var out []map[string]any
	add := func(value any) {
		if candidate, ok := value.(map[string]any); ok {
			out = append(out, candidate)
			if jsonValue, ok := candidate["json"].(map[string]any); ok {
				out = append(out, jsonValue)
			}
		}
	}
	add(raw)
	add(raw["event"])
	add(raw["object"])
	return out
}

func characterIDFromTenantItem(environment model.Environment, value any) string {
	return entityIDFromTenantItem(model.EntityTypeCharacter, environment, value)
}

func systemIDFromTenantItem(environment model.Environment, value any) string {
	return entityIDFromTenantItem(model.EntityTypeSystem, environment, value)
}

func entityIDFromTenantItem(entityType model.EntityType, environment model.Environment, value any) string {
	item, ok := value.(map[string]any)
	if !ok {
		return ""
	}
	itemID := stringFromAny(item["item_id"])
	if itemID == "" {
		return ""
	}
	tenant := stringFromAny(item["tenant"])
	if tenant == "" {
		tenant = string(environment)
	}
	if tenant == "" {
		tenant = string(model.EnvironmentUnknown)
	}
	return fmt.Sprintf("%s:%s:%s", entityType, tenant, itemID)
}

func stringFromAny(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	case int:
		return fmt.Sprint(typed)
	case int64:
		return fmt.Sprint(typed)
	case uint64:
		return fmt.Sprint(typed)
	case float64:
		return fmt.Sprintf("%.0f", typed)
	default:
		return ""
	}
}

func (s Service) resolveSystem(ctx context.Context, rawID string, environment model.Environment, graph killmailGraph) model.ResolvedValue {
	if rawID != "" {
		return s.Resolver.System(ctx, rawID, environment)
	}
	if graph.system.Entity.ID != "" {
		return graph.system.resolved()
	}
	return s.Resolver.System(ctx, rawID, environment)
}

func (s Service) resolveCharacter(ctx context.Context, rawID string, environment model.Environment, graphValue graphValue) model.ResolvedValue {
	if rawID != "" {
		return s.Resolver.Character(ctx, rawID, environment)
	}
	if graphValue.Entity.ID != "" {
		return graphValue.resolved()
	}
	return s.Resolver.Character(ctx, rawID, environment)
}

type killmailGraph struct {
	system   graphValue
	victim   graphValue
	killer   graphValue
	reporter graphValue
	sources  []string
	warnings []string
}

type graphValue struct {
	Entity     model.Entity
	SourceID   string
	Confidence model.Confidence
	TypeID     string
}

func (v graphValue) resolved() model.ResolvedValue {
	confidence := v.Confidence
	if confidence == "" {
		confidence = model.ConfidenceUnknown
	}
	displayName := v.Entity.DisplayName
	if displayName == "" {
		displayName = v.Entity.Name
	}
	out := model.ResolvedValue{
		EntityID:    v.Entity.ID,
		EntityType:  v.Entity.Type,
		DisplayName: displayName,
		TypeID:      v.TypeID,
		Confidence:  confidence,
	}
	if out.EntityType == model.EntityTypeEnemy {
		out.IsNPC = true
	}
	if v.SourceID != "" {
		out.SourceIDs = []string{v.SourceID}
	}
	return out
}

func (s Service) graph(ctx context.Context, killmailID string) killmailGraph {
	if s.GraphStore == nil || killmailID == "" {
		return killmailGraph{}
	}
	relations, err := s.GraphStore.ListEntityRelations(ctx, killmailID)
	if err != nil {
		return killmailGraph{warnings: []string{fmt.Sprintf("killmail graph lookup failed: %s", err.Error())}}
	}
	out := killmailGraph{}
	sourceSet := make(map[string]struct{})
	for _, relation := range relations {
		if relation.SubjectEntityID != killmailID {
			continue
		}
		if relation.SourceID != "" {
			sourceSet[relation.SourceID] = struct{}{}
		}
		entity, ok, err := s.GraphStore.GetEntity(ctx, relation.ObjectEntityID)
		if err != nil {
			out.warnings = append(out.warnings, fmt.Sprintf("killmail relation target lookup failed: %s", err.Error()))
			continue
		}
		if !ok {
			out.warnings = append(out.warnings, fmt.Sprintf("killmail relation target %s could not be resolved", relation.ObjectEntityID))
			continue
		}
		value := graphValue{Entity: entity, SourceID: relation.SourceID, Confidence: relation.Confidence}
		if entity.Type == model.EntityTypeEnemy {
			value.TypeID = s.typeID(ctx, entity.ID)
		}
		switch relation.Predicate {
		case "occurred_in":
			out.system = value
		case "victim":
			out.victim = value
		case "killer":
			out.killer = value
		case "reported_by":
			out.reporter = value
		}
	}
	for source := range sourceSet {
		out.sources = append(out.sources, source)
	}
	return out
}

func (s Service) typeID(ctx context.Context, entityID string) string {
	if s.GraphStore == nil {
		return ""
	}
	facts, err := s.GraphStore.ListEntityFacts(ctx, entityID)
	if err != nil {
		return ""
	}
	for _, fact := range facts {
		if fact.Key == "type_id" {
			return fmt.Sprint(fact.Value)
		}
	}
	return ""
}

func appendResolvedWarnings(warnings []string, values ...model.ResolvedValue) []string {
	for _, value := range values {
		warnings = append(warnings, value.Warnings...)
	}
	return warnings
}

func mergeSources(primary, secondary []string) []string {
	seen := make(map[string]struct{}, len(primary)+len(secondary))
	out := make([]string, 0, len(primary)+len(secondary))
	for _, source := range append(primary, secondary...) {
		if source == "" {
			continue
		}
		if _, ok := seen[source]; ok {
			continue
		}
		seen[source] = struct{}{}
		out = append(out, source)
	}
	return out
}
