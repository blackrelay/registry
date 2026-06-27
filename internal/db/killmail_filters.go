package db

import (
	"fmt"
	"strings"

	"github.com/blackrelay/registry/internal/cycles"
	"github.com/blackrelay/registry/internal/model"
)

func killmailMatchesQuery(raw model.KillmailRaw, query KillmailQuery, relations map[string]RelationDraft, entities map[string]model.Entity) bool {
	if query.ExcludeFixtures && IsFixtureKillmail(raw) {
		return false
	}
	if query.Environment != "" && raw.Environment != query.Environment {
		return false
	}
	if !killmailMatchesCycleScope(raw, query.Cycles, query.IncludeUncycled) {
		return false
	}
	if query.From != nil && raw.OccurredAt.Before(*query.From) {
		return false
	}
	if query.To != nil && raw.OccurredAt.After(*query.To) {
		return false
	}
	if query.SystemID != "" && !killmailHasSystem(raw, query.SystemID, relations) {
		return false
	}
	if query.VictimID != "" && !killmailHasActor(raw, "victim", query.VictimID, relations) {
		return false
	}
	if query.KillerID != "" && !killmailHasActor(raw, "killer", query.KillerID, relations) {
		return false
	}
	if query.KillerTypeID != "" && killmailKillerType(raw) != query.KillerTypeID {
		return false
	}
	if query.ReporterID != "" && !killmailHasActor(raw, "reported_by", query.ReporterID, relations) {
		return false
	}
	if query.NPCOnly != nil {
		isNPC := killmailKillerType(raw) != "" || killmailHasKillerEntityType(raw.ID, model.EntityTypeEnemy, relations, entities)
		if isNPC != *query.NPCOnly {
			return false
		}
	}
	return true
}

func killmailMatchesCycleScope(raw model.KillmailRaw, cycleValues []int, includeUncycled bool) bool {
	if len(cycleValues) == 0 {
		return true
	}
	cycle := cycles.FromTime(raw.OccurredAt)
	if cycle == nil {
		return includeUncycled
	}
	for _, candidate := range cycleValues {
		if candidate == *cycle {
			return true
		}
	}
	return false
}

func IsFixtureKillmail(raw model.KillmailRaw) bool {
	if strings.Contains(strings.ToLower(raw.ID), ":fixture:") {
		return true
	}
	for _, sourceID := range raw.SourceIDs {
		if IsFixtureSourceID(sourceID) {
			return true
		}
	}
	return false
}

func IsFixtureSourceID(sourceID string) bool {
	value := strings.ToLower(strings.TrimSpace(sourceID))
	return value == "source:fixture" || strings.HasPrefix(value, "source:fixture:") || strings.Contains(value, ":fixture:")
}

func killmailHasSystem(raw model.KillmailRaw, systemID string, relations map[string]RelationDraft) bool {
	if raw.SystemID == systemID || raw.SystemName == systemID || tenantItemEntityID(model.EntityTypeSystem, raw.Environment, raw.Raw, "solar_system_id") == systemID {
		return true
	}
	return killmailHasRelation(raw.ID, "occurred_in", systemID, relations)
}

func killmailHasActor(raw model.KillmailRaw, predicate, actorID string, relations map[string]RelationDraft) bool {
	switch predicate {
	case "victim":
		if raw.VictimCharacterID == actorID || raw.VictimName == actorID || tenantItemEntityID(model.EntityTypeCharacter, raw.Environment, raw.Raw, "victim_id") == actorID {
			return true
		}
	case "killer":
		if raw.KillerCharacterID == actorID || raw.KillerName == actorID || tenantItemEntityID(model.EntityTypeCharacter, raw.Environment, raw.Raw, "killer_id") == actorID {
			return true
		}
	case "reported_by":
		if raw.ReporterCharacterID == actorID || raw.ReporterName == actorID || tenantItemEntityID(model.EntityTypeCharacter, raw.Environment, raw.Raw, "reported_by_character_id") == actorID {
			return true
		}
	}
	return killmailHasRelation(raw.ID, predicate, actorID, relations)
}

func killmailKillerType(raw model.KillmailRaw) string {
	if raw.KillerTypeID != "" {
		return raw.KillerTypeID
	}
	for _, payload := range payloadCandidates(raw.Raw) {
		if value := stringFromRaw(payload["killer_type_id"]); value != "" {
			return value
		}
	}
	return ""
}

func killmailHasRelation(killmailID, predicate, objectID string, relations map[string]RelationDraft) bool {
	for _, relation := range relations {
		if relation.SubjectEntityID == killmailID && relation.Predicate == predicate && relation.ObjectEntityID == objectID {
			return true
		}
	}
	return false
}

func killmailHasKillerEntityType(killmailID string, entityType model.EntityType, relations map[string]RelationDraft, entities map[string]model.Entity) bool {
	for _, relation := range relations {
		if relation.SubjectEntityID != killmailID || relation.Predicate != "killer" {
			continue
		}
		if entities[relation.ObjectEntityID].Type == entityType {
			return true
		}
	}
	return false
}

func tenantItemEntityID(entityType model.EntityType, fallbackEnvironment model.Environment, raw map[string]any, field string) string {
	for _, payload := range payloadCandidates(raw) {
		value, ok := payload[field].(map[string]any)
		if !ok {
			continue
		}
		itemID := stringFromRaw(value["item_id"])
		if itemID == "" {
			continue
		}
		tenant := stringFromRaw(value["tenant"])
		if tenant == "" {
			tenant = string(fallbackEnvironment)
		}
		if tenant == "" {
			tenant = string(model.EnvironmentUnknown)
		}
		return fmt.Sprintf("%s:%s:%s", entityType, tenant, itemID)
	}
	return ""
}

func payloadCandidates(raw map[string]any) []map[string]any {
	var out []map[string]any
	add := func(value any) {
		if candidate, ok := value.(map[string]any); ok {
			out = append(out, candidate)
			if nested, ok := candidate["json"].(map[string]any); ok {
				out = append(out, nested)
			}
		}
	}
	add(raw)
	add(raw["event"])
	add(raw["object"])
	return out
}

func stringFromRaw(value any) string {
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
