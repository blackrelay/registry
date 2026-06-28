package sui

import (
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/blackrelay/registry/internal/cycles"
	"github.com/blackrelay/registry/internal/db"
	"github.com/blackrelay/registry/internal/model"
)

type DerivedObjectEntity struct {
	Entity model.Entity
	Facts  []db.EntityFactDraft
}

type DerivedObjectGraph struct {
	Entities  []DerivedObjectEntity
	Relations []db.RelationDraft
	Killmails []model.KillmailRaw
}

var slugPartPattern = regexp.MustCompile(`[^a-z0-9]+`)

func DeriveGraphFromObject(object db.SuiObjectRecord) DerivedObjectGraph {
	entity, ok := DeriveEntityFromObject(object)
	if !ok {
		return DerivedObjectGraph{}
	}
	builder := objectGraphBuilder{
		object:      object,
		entities:    make(map[string]DerivedObjectEntity),
		relationSet: make(map[string]struct{}),
	}
	builder.addEntity(entity)
	jsonValue := objectJSON(object)
	switch entity.Entity.Type {
	case model.EntityTypeCharacter:
		builder.characterRelations(entity.Entity.ID, jsonValue)
	case model.EntityTypeAssembly, model.EntityTypeStorage, model.EntityTypeTurret:
		builder.infrastructureEvidenceRelations(entity.Entity.ID, jsonValue)
	case model.EntityTypeGate:
		builder.infrastructureEvidenceRelations(entity.Entity.ID, jsonValue)
		builder.gateRelations(entity.Entity.ID, jsonValue)
	case model.EntityTypeSite:
		builder.riftRelations(entity.Entity.ID, jsonValue)
	case model.EntityTypeKillmail:
		builder.killmailRelations(entity.Entity.ID, jsonValue)
	}
	return builder.graph()
}

func DeriveEntityFromObject(object db.SuiObjectRecord) (DerivedObjectEntity, bool) {
	if object.SourceID == "" || object.ObjectID == "" || object.TypeRepr == "" {
		return DerivedObjectEntity{}, false
	}
	entityType := entityTypeForObject(object.Module, object.TypeName)
	if entityType == model.EntityTypeUnknown {
		return DerivedObjectEntity{}, false
	}
	jsonValue := objectJSON(object)
	tenant, itemID := objectIdentity(jsonValue)
	if itemID == "" && object.Module == "character" && object.TypeName == "PlayerProfile" {
		itemID = stringFrom(jsonValue["character_id"])
	}
	identityScope := tenant
	if identityScope == "" {
		identityScope = string(object.Environment)
	}
	entityID := "onchain:" + object.ObjectID
	if itemID != "" && identityScope != "" {
		entityID = fmt.Sprintf("%s:%s:%s", entityType, identityScope, itemID)
	}
	displayName := objectDisplayName(jsonValue)
	name := objectName(entityType, displayName, itemID, object.ObjectID)
	entity := model.Entity{
		ID:          entityID,
		Slug:        objectSlug(entityType, itemID, identityScope, object.ObjectID),
		Type:        entityType,
		Name:        name,
		DisplayName: name,
		Summary:     objectSummary(entityType),
		Environment: object.Environment,
		Cycle:       cycles.FromTime(object.ObservedAt),
		UpdatedAt:   time.Now().UTC(),
	}
	facts := objectFactBuilder(object)
	facts.add("object_id", object.ObjectID)
	facts.add("object_type", object.TypeRepr)
	facts.add("object_version", object.Version)
	facts.add("object_digest", object.Digest)
	facts.add("package_id", object.PackageID)
	facts.add("module", object.Module)
	facts.add("type_name", object.TypeName)
	facts.add("tenant", tenant)
	facts.add("item_id", itemID)
	facts.add("metadata_name", displayName)
	facts.add("metadata_description", metadataField(jsonValue["metadata"], "description"))
	facts.add("metadata_url", metadataField(jsonValue["metadata"], "url"))

	switch entityType {
	case model.EntityTypeCharacter:
		facts.add("character_id", stringFrom(jsonValue["character_id"]))
		facts.add("tribe_id", numberOrString(jsonValue["tribe_id"]))
		facts.add("character_address", stringFrom(jsonValue["character_address"]))
		facts.add("owner_cap_id", stringFrom(jsonValue["owner_cap_id"]))
	case model.EntityTypeAssembly, model.EntityTypeGate, model.EntityTypeStorage, model.EntityTypeTurret:
		facts.add("owner_cap_id", stringFrom(jsonValue["owner_cap_id"]))
		facts.add("type_id", numberOrString(jsonValue["type_id"]))
		facts.add("status", variantValue(jsonValue["status"]))
		facts.add("location_hash", locationHash(jsonValue["location"]))
		if system, ok := locationSystem(jsonValue, object.Environment); ok {
			facts.add("solar_system_id", system.ItemID)
		}
		facts.add("x", locationCoordinate(jsonValue, "x"))
		facts.add("y", locationCoordinate(jsonValue, "y"))
		facts.add("z", locationCoordinate(jsonValue, "z"))
		facts.add("energy_source_id", stringFrom(jsonValue["energy_source_id"]))
		facts.add("linked_gate_id", stringFrom(jsonValue["linked_gate_id"]))
	case model.EntityTypeSite:
		facts.add("rift_id", object.ObjectID)
		facts.add("location_hash", locationHash(jsonValue["location"]))
	case model.EntityTypeKillmail:
		facts.add("killer_id", tenantItemIDValue(jsonValue["killer_id"]))
		facts.add("victim_id", tenantItemIDValue(jsonValue["victim_id"]))
		facts.add("reported_by_character_id", tenantItemIDValue(jsonValue["reported_by_character_id"]))
		facts.add("kill_timestamp", numberOrString(jsonValue["kill_timestamp"]))
		facts.add("loss_type", variantValue(jsonValue["loss_type"]))
		facts.add("solar_system_id", tenantItemIDValue(jsonValue["solar_system_id"]))
	}

	return DerivedObjectEntity{Entity: entity, Facts: facts.values()}, true
}

type objectGraphBuilder struct {
	object      db.SuiObjectRecord
	entities    map[string]DerivedObjectEntity
	relations   []db.RelationDraft
	relationSet map[string]struct{}
	killmails   []model.KillmailRaw
}

func (b *objectGraphBuilder) characterRelations(characterID string, jsonValue map[string]any) {
	tenant, _ := objectIdentity(jsonValue)
	tribeID := stringFrom(numberOrString(jsonValue["tribe_id"]))
	if tribeID == "" {
		return
	}
	scope := objectScope(tenant, b.object.Environment)
	tribeIDValue := entityID(model.EntityTypeTribe, scope, tribeID)
	b.addEntity(DerivedObjectEntity{
		Entity: model.Entity{
			ID:          tribeIDValue,
			Slug:        entitySlug(model.EntityTypeTribe, tenantItem{Tenant: scope, ItemID: tribeID}),
			Type:        model.EntityTypeTribe,
			Name:        "Tribe " + tribeID,
			DisplayName: "Tribe " + tribeID,
			Summary:     "Public on-chain tribe identity observed from Sui object data.",
			Environment: b.object.Environment,
			Cycle:       b.cycle(),
			UpdatedAt:   time.Now().UTC(),
		},
		Facts: []db.EntityFactDraft{
			b.fact("tribe_id", tribeID),
			b.fact("tenant", scope),
		},
	})
	b.relation(characterID, "belongs_to", tribeIDValue)
}

func (b *objectGraphBuilder) gateRelations(gateID string, jsonValue map[string]any) {
	tenant, _ := objectIdentity(jsonValue)
	linkedGateID := stringFrom(numberOrString(jsonValue["linked_gate_id"]))
	if linkedGateID == "" {
		return
	}
	scope := objectScope(tenant, b.object.Environment)
	linkedID := entityID(model.EntityTypeGate, scope, linkedGateID)
	b.addEntity(DerivedObjectEntity{
		Entity: model.Entity{
			ID:          linkedID,
			Slug:        entitySlug(model.EntityTypeGate, tenantItem{Tenant: scope, ItemID: linkedGateID}),
			Type:        model.EntityTypeGate,
			Name:        "Gate " + compactIdentityLabel(linkedGateID),
			DisplayName: "Gate " + compactIdentityLabel(linkedGateID),
			Summary:     "Public on-chain gate identity referenced from Sui object data.",
			Environment: b.object.Environment,
			Cycle:       b.cycle(),
			UpdatedAt:   time.Now().UTC(),
		},
		Facts: []db.EntityFactDraft{
			b.fact("item_id", linkedGateID),
			b.fact("tenant", scope),
			b.fact("linked_gate_placeholder", true),
		},
	})
	b.relation(gateID, "links_to", linkedID)
}

func (b *objectGraphBuilder) infrastructureEvidenceRelations(entityID string, jsonValue map[string]any) {
	tenant, _ := objectIdentity(jsonValue)
	scope := objectScope(tenant, b.object.Environment)
	if ownerCapID := stringFrom(jsonValue["owner_cap_id"]); ownerCapID != "" {
		resourceID := resourceObjectID(scope, "owner-cap", ownerCapID)
		b.resourceObject(resourceID, "owner-cap", ownerCapID, scope, "Public on-chain owner capability identifier observed from Sui object data.")
		b.relation(entityID, "has_owner_cap", resourceID)
	}
	if locationHash := locationHash(jsonValue["location"]); locationHash != "" {
		resourceID := resourceObjectID(scope, "location-hash", locationHash)
		b.resourceObject(resourceID, "location-hash", locationHash, scope, "Public on-chain location hash observed from Sui object data.")
		b.relation(entityID, "has_location_hash", resourceID)
	}
	if system, ok := locationSystem(jsonValue, b.object.Environment); ok {
		systemID := b.system(system)
		b.relation(entityID, "located_in", systemID)
	}
}

func (b *objectGraphBuilder) riftRelations(entityID string, jsonValue map[string]any) {
	tenant, _ := objectIdentity(jsonValue)
	scope := objectScope(tenant, b.object.Environment)
	if locationHash := locationHash(jsonValue["location"]); locationHash != "" {
		resourceID := resourceObjectID(scope, "location-hash", locationHash)
		b.resourceObject(resourceID, "location-hash", locationHash, scope, "Public on-chain rift location hash observed from Sui object data.")
		b.relation(entityID, "has_location_hash", resourceID)
	}
}

func (b *objectGraphBuilder) resourceObject(entityID, resourceKind, value, scope, summary string) {
	name := resourceKindLabel(resourceKind) + " " + value
	b.addEntity(DerivedObjectEntity{
		Entity: model.Entity{
			ID:          entityID,
			Slug:        slugify(strings.Join(compactStrings("resource-object", resourceKind, value, scope), "-")),
			Type:        model.EntityTypeResourceObject,
			Name:        name,
			DisplayName: name,
			Summary:     summary,
			Environment: b.object.Environment,
			Cycle:       b.cycle(),
			UpdatedAt:   time.Now().UTC(),
		},
		Facts: []db.EntityFactDraft{
			b.fact("resource_kind", resourceKind),
			b.fact("value", value),
			b.fact("tenant", scope),
		},
	})
}

func (b *objectGraphBuilder) killmailRelations(killmailID string, jsonValue map[string]any) {
	raw := model.KillmailRaw{
		ID:          killmailID,
		Environment: b.object.Environment,
		OccurredAt:  b.object.ObservedAt,
		LossType:    variantValue(jsonValue["loss_type"]),
		SourceIDs:   []string{b.object.SourceID},
		Raw:         map[string]any{"sourceObjectId": b.object.ID, "object": b.object.Payload},
	}
	if key, ok := tenantItemID(jsonValue["victim_id"]); ok {
		victimID := b.character(key, "victim_id")
		raw.VictimCharacterID = victimID
		b.relation(killmailID, "victim", victimID)
	}
	if key, ok := tenantItemID(jsonValue["killer_id"]); ok {
		killerID := b.character(key, "killer_id")
		raw.KillerCharacterID = killerID
		b.relation(killmailID, "killer", killerID)
	}
	if key, ok := tenantItemID(jsonValue["reported_by_character_id"]); ok {
		reporterID := b.character(key, "reported_by_character_id")
		raw.ReporterCharacterID = reporterID
		b.relation(killmailID, "reported_by", reporterID)
	}
	if key, ok := tenantItemID(jsonValue["solar_system_id"]); ok {
		systemID := b.system(key)
		raw.SystemID = systemID
		b.relation(killmailID, "occurred_in", systemID)
	}
	if occurredAt := killTimestamp(jsonValue["kill_timestamp"]); !occurredAt.IsZero() {
		raw.OccurredAt = occurredAt
	}
	if typeID := stringFrom(numberOrString(jsonValue["killer_type_id"])); typeID != "" {
		raw.KillerTypeID = typeID
	}
	b.killmails = append(b.killmails, raw)
}

func (b *objectGraphBuilder) character(key tenantItem, roleFact string) string {
	scope := objectScope(key.Tenant, b.object.Environment)
	id := entityID(model.EntityTypeCharacter, scope, key.ItemID)
	b.addEntity(DerivedObjectEntity{
		Entity: model.Entity{
			ID:          id,
			Slug:        entitySlug(model.EntityTypeCharacter, tenantItem{Tenant: scope, ItemID: key.ItemID}),
			Type:        model.EntityTypeCharacter,
			Name:        "Character " + key.ItemID,
			DisplayName: "Character " + key.ItemID,
			Summary:     "Public on-chain character identity referenced from Sui object data.",
			Environment: b.object.Environment,
			Cycle:       b.cycle(),
			UpdatedAt:   time.Now().UTC(),
		},
		Facts: []db.EntityFactDraft{
			b.fact("item_id", key.ItemID),
			b.fact("tenant", scope),
			b.fact(roleFact, true),
		},
	})
	return id
}

func (b *objectGraphBuilder) system(key tenantItem) string {
	scope := objectScope(key.Tenant, b.object.Environment)
	id := entityID(model.EntityTypeSystem, scope, key.ItemID)
	b.addEntity(DerivedObjectEntity{
		Entity: model.Entity{
			ID:          id,
			Slug:        entitySlug(model.EntityTypeSystem, tenantItem{Tenant: scope, ItemID: key.ItemID}),
			Type:        model.EntityTypeSystem,
			Name:        "System " + key.ItemID,
			DisplayName: "System " + key.ItemID,
			Summary:     "Public on-chain solar system reference observed from Sui object data.",
			Environment: b.object.Environment,
			Cycle:       b.cycle(),
			UpdatedAt:   time.Now().UTC(),
		},
		Facts: []db.EntityFactDraft{
			b.fact("item_id", key.ItemID),
			b.fact("tenant", scope),
		},
	})
	return id
}

func (b *objectGraphBuilder) addEntity(entity DerivedObjectEntity) {
	existing, ok := b.entities[entity.Entity.ID]
	if !ok {
		b.entities[entity.Entity.ID] = entity
		return
	}
	seen := make(map[string]struct{}, len(existing.Facts))
	for _, fact := range existing.Facts {
		seen[fact.Key+":"+fmt.Sprint(fact.Value)] = struct{}{}
	}
	for _, fact := range entity.Facts {
		key := fact.Key + ":" + fmt.Sprint(fact.Value)
		if _, ok := seen[key]; ok {
			continue
		}
		existing.Facts = append(existing.Facts, fact)
	}
	b.entities[entity.Entity.ID] = existing
}

func (b *objectGraphBuilder) relation(subject, predicate, object string) {
	if subject == "" || predicate == "" || object == "" {
		return
	}
	relation := db.RelationDraft{
		SubjectEntityID: subject,
		Predicate:       predicate,
		ObjectEntityID:  object,
		SourceID:        b.object.SourceID,
		Confidence:      model.ConfidenceVerified,
		Environment:     b.object.Environment,
	}
	if !b.object.ObservedAt.IsZero() {
		observedAt := b.object.ObservedAt
		relation.ValidFrom = &observedAt
	}
	id := db.RelationID(relation)
	if _, ok := b.relationSet[id]; ok {
		return
	}
	b.relationSet[id] = struct{}{}
	b.relations = append(b.relations, relation)
}

func (b *objectGraphBuilder) fact(key string, value any) db.EntityFactDraft {
	return db.EntityFactDraft{
		Key:          key,
		Value:        value,
		SourceID:     b.object.SourceID,
		Confidence:   model.ConfidenceVerified,
		Environment:  b.object.Environment,
		Cycle:        b.cycle(),
		ReviewStatus: model.ReviewStatusReviewed,
	}
}

func (b *objectGraphBuilder) cycle() *int {
	return cycles.FromTime(b.object.ObservedAt)
}

func (b *objectGraphBuilder) graph() DerivedObjectGraph {
	entities := make([]DerivedObjectEntity, 0, len(b.entities))
	for _, entity := range b.entities {
		entities = append(entities, entity)
	}
	return DerivedObjectGraph{Entities: entities, Relations: b.relations, Killmails: b.killmails}
}

func entityTypeForObject(moduleName, typeName string) model.EntityType {
	switch {
	case moduleName == "character" && (typeName == "Character" || typeName == "PlayerProfile"):
		return model.EntityTypeCharacter
	case moduleName == "assembly" && typeName == "Assembly":
		return model.EntityTypeAssembly
	case moduleName == "network_node" && typeName == "NetworkNode":
		return model.EntityTypeAssembly
	case moduleName == "gate" && typeName == "Gate":
		return model.EntityTypeGate
	case moduleName == "storage_unit" && typeName == "StorageUnit":
		return model.EntityTypeStorage
	case moduleName == "turret" && typeName == "Turret":
		return model.EntityTypeTurret
	case moduleName == "killmail" && typeName == "Killmail":
		return model.EntityTypeKillmail
	case moduleName == "rift" && typeName == "Rift":
		return model.EntityTypeSite
	default:
		return model.EntityTypeUnknown
	}
}

func resourceObjectID(scope, resourceKind, value string) string {
	return fmt.Sprintf("%s:%s:%s:%s", model.EntityTypeResourceObject, scope, resourceKind, slugify(value))
}

func resourceKindLabel(resourceKind string) string {
	switch resourceKind {
	case "owner-cap":
		return "Owner capability"
	case "location-hash":
		return "Location hash"
	default:
		return "Resource object"
	}
}

type factBuilder struct {
	object db.SuiObjectRecord
	items  []db.EntityFactDraft
}

func objectFactBuilder(object db.SuiObjectRecord) *factBuilder {
	return &factBuilder{object: object}
}

func (b *factBuilder) add(key string, value any) {
	if value == nil {
		return
	}
	if valueString, ok := value.(string); ok && valueString == "" {
		return
	}
	b.items = append(b.items, db.EntityFactDraft{
		Key:          key,
		Value:        value,
		SourceID:     b.object.SourceID,
		Confidence:   model.ConfidenceVerified,
		Environment:  b.object.Environment,
		Cycle:        cycles.FromTime(b.object.ObservedAt),
		ReviewStatus: model.ReviewStatusReviewed,
	})
}

func (b *factBuilder) values() []db.EntityFactDraft {
	return b.items
}

func objectJSON(object db.SuiObjectRecord) map[string]any {
	value, ok := object.Payload["json"].(map[string]any)
	if ok && value != nil {
		return value
	}
	return map[string]any{}
}

func objectIdentity(value map[string]any) (string, string) {
	key, ok := value["key"].(map[string]any)
	if !ok {
		return "", ""
	}
	return stringFrom(key["tenant"]), stringFrom(numberOrString(key["item_id"]))
}

func objectScope(tenant string, environment model.Environment) string {
	if tenant != "" {
		return tenant
	}
	if environment != "" {
		return string(environment)
	}
	return string(model.EnvironmentUnknown)
}

func objectName(entityType model.EntityType, displayName, itemID, objectID string) string {
	if name := strings.TrimSpace(displayName); name != "" {
		return name
	}
	label := map[model.EntityType]string{
		model.EntityTypeAssembly:  "Assembly",
		model.EntityTypeCharacter: "Character",
		model.EntityTypeGate:      "Gate",
		model.EntityTypeKillmail:  "Killmail",
		model.EntityTypeSite:      "Rift",
		model.EntityTypeStorage:   "Storage",
		model.EntityTypeTurret:    "Turret",
	}[entityType]
	if label == "" {
		label = "On-chain object"
	}
	if itemID != "" {
		return label + " " + compactIdentityLabel(itemID)
	}
	return label + " " + shortObjectID(objectID)
}

func objectSlug(entityType model.EntityType, itemID, tenant, objectID string) string {
	label := string(entityType)
	identity := itemID
	if identity == "" {
		identity = shortObjectID(objectID)
	}
	return slugify(strings.Join(compactStrings(label, identity, tenant), "-"))
}

func objectSummary(entityType model.EntityType) string {
	label := string(entityType)
	if label == "" {
		label = "object"
	}
	return fmt.Sprintf("Public on-chain %s current state observed from Sui object data.", strings.ReplaceAll(label, "_", " "))
}

func metadataName(value any) string {
	return nameFromAny(value)
}

func metadataField(value any, field string) string {
	record, ok := value.(map[string]any)
	if !ok {
		return ""
	}
	if result := strings.TrimSpace(stringFrom(record[field])); result != "" {
		return result
	}
	for _, key := range []string{"fields", "value", "inner", "contents"} {
		if result := metadataField(record[key], field); result != "" {
			return result
		}
	}
	return ""
}

func objectDisplayName(value map[string]any) string {
	for _, candidate := range []any{
		value["metadata"],
		value["profile"],
		value["character"],
		value["display"],
		value,
	} {
		if name := nameFromAny(candidate); name != "" {
			return name
		}
	}
	return ""
}

func nameFromAny(value any) string {
	record, ok := value.(map[string]any)
	if !ok {
		return ""
	}
	for _, key := range []string{"name", "display_name", "displayName"} {
		if name := strings.TrimSpace(stringFrom(record[key])); name != "" {
			return name
		}
	}
	for _, key := range []string{"fields", "value", "inner", "contents"} {
		if name := nameFromAny(record[key]); name != "" {
			return name
		}
	}
	return ""
}

func locationHash(value any) string {
	record, ok := value.(map[string]any)
	if !ok {
		return ""
	}
	return locationHashValue(record["location_hash"])
}

func locationRecord(payload map[string]any) map[string]any {
	if payload == nil {
		return nil
	}
	record, ok := payload["location"].(map[string]any)
	if !ok {
		return nil
	}
	return record
}

func locationSystem(payload map[string]any, environment model.Environment) (tenantItem, bool) {
	if key, ok := firstTenantItem(payload, []string{"solar_system_id", "solarSystemId", "system_id", "systemId", "solarsystem"}); ok {
		return tenantItem{Tenant: tenantOrEnvironment(key.Tenant, environment), ItemID: key.ItemID}, true
	}
	location := locationRecord(payload)
	if key, ok := firstTenantItem(location, []string{"solar_system_id", "solarSystemId", "system_id", "systemId", "solarsystem"}); ok {
		return tenantItem{Tenant: tenantOrEnvironment(key.Tenant, environment), ItemID: key.ItemID}, true
	}
	for _, key := range []string{"solar_system_id", "solarSystemId", "system_id", "systemId", "solarsystem"} {
		if value := stringFrom(numberOrString(payload[key])); value != "" {
			return tenantItem{Tenant: string(environment), ItemID: value}, true
		}
		if location != nil {
			if value := stringFrom(numberOrString(location[key])); value != "" {
				return tenantItem{Tenant: string(environment), ItemID: value}, true
			}
		}
	}
	return tenantItem{}, false
}

func locationCoordinate(payload map[string]any, key string) string {
	if value := stringFrom(numberOrString(payload[key])); value != "" {
		return value
	}
	if location := locationRecord(payload); location != nil {
		return stringFrom(numberOrString(location[key]))
	}
	return ""
}

func locationHashValue(value any) string {
	if hash := stringFrom(value); hash != "" {
		return hash
	}
	items, ok := value.([]any)
	if !ok || len(items) == 0 {
		return ""
	}
	bytes := make([]byte, 0, len(items))
	for _, item := range items {
		number, ok := numberOrString(item).(int64)
		if !ok || number < 0 || number > 255 {
			return ""
		}
		bytes = append(bytes, byte(number))
	}
	return "0x" + hex.EncodeToString(bytes)
}

func variantValue(value any) string {
	record, ok := value.(map[string]any)
	if !ok {
		return ""
	}
	if nested, ok := record["status"].(map[string]any); ok {
		record = nested
	}
	return strings.ToLower(stringFrom(record["@variant"]))
}

func tenantItemIDValue(value any) map[string]any {
	record, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	out := map[string]any{}
	if itemID := numberOrString(record["item_id"]); itemID != nil {
		out["item_id"] = itemID
	}
	if tenant := stringFrom(record["tenant"]); tenant != "" {
		out["tenant"] = tenant
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func numberOrString(value any) any {
	switch item := value.(type) {
	case int:
		return item
	case int64:
		return item
	case float64:
		if item == float64(int64(item)) {
			return int64(item)
		}
		return item
	case string:
		if item != "" {
			return item
		}
	}
	return nil
}

func stringFrom(value any) string {
	switch item := value.(type) {
	case string:
		return item
	case int:
		return fmt.Sprint(item)
	case int64:
		return fmt.Sprint(item)
	case float64:
		if item == float64(int64(item)) {
			return fmt.Sprint(int64(item))
		}
		return fmt.Sprint(item)
	default:
		return ""
	}
}

func shortObjectID(objectID string) string {
	value := strings.TrimPrefix(objectID, "0x")
	if len(value) > 12 {
		value = value[:12]
	}
	if value == "" {
		return "unknown"
	}
	return value
}

func compactIdentityLabel(value string) string {
	value = strings.TrimSpace(value)
	if isLongHexIdentity(value) {
		return shortObjectID(value)
	}
	return value
}

func isLongHexIdentity(value string) bool {
	value = strings.TrimSpace(value)
	trimmed := strings.TrimPrefix(value, "0x")
	if len(trimmed) <= 16 {
		return false
	}
	for _, char := range trimmed {
		if (char >= '0' && char <= '9') || (char >= 'a' && char <= 'f') || (char >= 'A' && char <= 'F') {
			continue
		}
		return false
	}
	return true
}

func compactStrings(values ...string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func slugify(value string) string {
	slug := strings.ToLower(value)
	slug = slugPartPattern.ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-")
	if len(slug) > 120 {
		slug = strings.Trim(slug[:120], "-")
	}
	if slug == "" {
		return "onchain-object"
	}
	return slug
}
