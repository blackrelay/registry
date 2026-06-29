package db

import (
	"fmt"
	"sort"
	"strings"

	"github.com/blackrelay/registry/internal/model"
)

func deriveCurrentEntity(item *model.CurrentEntity) {
	if item == nil {
		return
	}
	derived := model.CurrentDerived{}
	derived.Profile = currentProfileFromFacts(item.Facts)
	for _, relation := range item.OutgoingRelations {
		switch relation.Predicate {
		case "belongs_to":
			if relation.ObjectEntityType == model.EntityTypeTribe {
				derived.Tribe = relatedObject(relation)
			}
		case "owned_by":
			derived.Owner = relatedObject(relation)
		case "has_owner_cap":
			if relation.ObjectEntityType == model.EntityTypeResourceObject {
				derived.OwnerCap = relatedObject(relation)
			}
		case "has_location_hash":
			if relation.ObjectEntityType == model.EntityTypeResourceObject {
				derived.LocationHash = relatedObject(relation)
			}
		case "located_in", "located_at", "deployed_in", "observed_in":
			switch relation.ObjectEntityType {
			case model.EntityTypeSystem:
				derived.System = relatedObject(relation)
			case model.EntityTypeConstellation:
				derived.Constellation = relatedObject(relation)
			case model.EntityTypeRegion:
				derived.Region = relatedObject(relation)
			}
		case "member_of_region":
			if relation.ObjectEntityType == model.EntityTypeRegion {
				derived.Region = relatedObject(relation)
			}
		case "links_to":
			if relation.ObjectEntityType == model.EntityTypeSystem {
				derived.ConnectedSystemCount++
			}
		case "observed_between":
			if relation.ObjectEntityType == model.EntityTypeSystem {
				derived.RouteEdgeCount++
				if derived.System == nil {
					derived.System = relatedObject(relation)
				}
			}
		}
		if relation.SubjectEntityType == model.EntityTypeKillmail {
			derived.PublicActivityCount++
		}
	}
	for _, relation := range item.IncomingRelations {
		switch relation.Predicate {
		case "belongs_to":
			if relation.SubjectEntityType == model.EntityTypeCharacter {
				derived.MemberCount++
			}
		case "owned_by":
			derived.OwnedObjectCount++
		case "links_to":
			if relation.SubjectEntityType == model.EntityTypeSystem {
				derived.ConnectedSystemCount++
			}
		case "observed_between":
			if relation.SubjectEntityType == model.EntityTypeRoute || relation.SubjectEntityType == model.EntityTypeSystem {
				derived.RouteEdgeCount++
			}
		case "occurred_in":
			if relation.SubjectEntityType == model.EntityTypeKillmail {
				derived.KillmailCount++
				derived.PublicActivityCount++
			}
		case "victim", "killer", "reported_by":
			if relation.SubjectEntityType == model.EntityTypeKillmail {
				derived.PublicActivityCount++
			}
		}
	}
	if hasDerivedState(derived) {
		item.Derived = &derived
	}
}

func relatedObject(relation model.CurrentRelation) *model.CurrentRelatedEntity {
	return &model.CurrentRelatedEntity{
		EntityID:    relation.ObjectEntityID,
		EntityType:  relation.ObjectEntityType,
		DisplayName: relation.ObjectDisplayName,
	}
}

func hasDerivedState(derived model.CurrentDerived) bool {
	return derived.Profile != nil ||
		derived.Tribe != nil ||
		derived.Owner != nil ||
		derived.System != nil ||
		derived.Constellation != nil ||
		derived.Region != nil ||
		derived.OwnerCap != nil ||
		derived.LocationHash != nil ||
		derived.MemberCount != 0 ||
		derived.OwnedObjectCount != 0 ||
		derived.ConnectedSystemCount != 0 ||
		derived.RouteEdgeCount != 0 ||
		derived.KillmailCount != 0 ||
		derived.PublicActivityCount != 0
}

func currentProfileFromFacts(facts map[string]any) *model.CurrentProfile {
	if len(facts) == 0 {
		return nil
	}
	profile := model.CurrentProfile{
		MetadataName:        factString(facts["metadata_name"]),
		MetadataDescription: factString(facts["metadata_description"]),
		MetadataURL:         factString(facts["metadata_url"]),
		Tag:                 factString(facts["tag"]),
		Description:         factString(facts["description"]),
		URL:                 factString(facts["url"]),
		Aliases:             factStringSlice(facts["aliases"]),
	}
	if profile.MetadataName == "" &&
		profile.MetadataDescription == "" &&
		profile.MetadataURL == "" &&
		profile.Tag == "" &&
		profile.Description == "" &&
		profile.URL == "" &&
		len(profile.Aliases) == 0 {
		return nil
	}
	return &profile
}

func factString(value any) string {
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(text)
}

func factStringSlice(value any) []string {
	seen := make(map[string]struct{})
	var out []string
	add := func(candidate string) {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			return
		}
		if _, ok := seen[candidate]; ok {
			return
		}
		seen[candidate] = struct{}{}
		out = append(out, candidate)
	}
	switch typed := value.(type) {
	case []string:
		for _, item := range typed {
			add(item)
		}
	case []any:
		for _, item := range typed {
			if text, ok := item.(string); ok {
				add(text)
			}
		}
	case string:
		add(typed)
	}
	return out
}

func currentEntityMatchesQuery(item model.CurrentEntity, query CurrentEntityQuery) bool {
	if currentScopedCharacterEvidenceRequired(query) && item.Entity.Type == model.EntityTypeCharacter && !hasEventBackedCharacterEvidence(item) {
		return false
	}
	if currentScopedTribeProfileRequired(query) && item.Entity.Type == model.EntityTypeTribe && !hasPublicCurrentTribeProfile(item) {
		return false
	}
	if query.ProfileState != "" && !matchesProfileState(item, query.ProfileState) {
		return false
	}
	if query.Q != "" {
		haystack := strings.ToLower(item.Entity.ID + " " + item.Entity.Slug + " " + item.Entity.Name + " " + item.Entity.DisplayName + " " + item.Entity.Summary)
		if !strings.Contains(haystack, strings.ToLower(query.Q)) {
			return false
		}
	}
	if query.TribeID != "" && !hasOutgoingRelation(item, "belongs_to", query.TribeID) {
		return false
	}
	if query.OwnerID != "" && !hasOutgoingRelation(item, "owned_by", query.OwnerID) {
		return false
	}
	if query.SystemID != "" && item.Entity.ID != query.SystemID && !hasRelationInEitherDirection(item, systemRelationPredicates(), query.SystemID) {
		return false
	}
	if query.OwnerCapID != "" && !hasFactValue(item, "owner_cap_id", query.OwnerCapID) {
		return false
	}
	if query.LocationHash != "" && !hasFactValue(item, "location_hash", query.LocationHash) {
		return false
	}
	if query.ConnectedTo != "" && !hasConnectionRelation(item, query.ConnectedTo) {
		return false
	}
	if query.HasActivity != nil {
		hasActivity := hasActivityRelation(item)
		if hasActivity != *query.HasActivity {
			return false
		}
	}
	if query.HasTribe != nil && (item.Derived != nil && item.Derived.Tribe != nil) != *query.HasTribe {
		return false
	}
	if query.HasOwnerCap != nil && hasOwnerCapEvidence(item) != *query.HasOwnerCap {
		return false
	}
	if query.HasLocationHash != nil && hasLocationHashEvidence(item) != *query.HasLocationHash {
		return false
	}
	if query.HasResolvedOwner != nil && (item.Derived != nil && item.Derived.Owner != nil) != *query.HasResolvedOwner {
		return false
	}
	if query.HasResolvedSystem != nil && (item.Derived != nil && item.Derived.System != nil) != *query.HasResolvedSystem {
		return false
	}
	if query.SourceID != "" && !hasCurrentEntitySource(item, query.SourceID) {
		return false
	}
	return true
}

func currentScopedCharacterEvidenceRequired(query CurrentEntityQuery) bool {
	return len(query.Cycles) > 0
}

func currentScopedTribeProfileRequired(query CurrentEntityQuery) bool {
	return len(query.Cycles) > 0
}

func hasPublicCurrentTribeProfile(item model.CurrentEntity) bool {
	if item.Entity.Type != model.EntityTypeTribe {
		return true
	}
	name := strings.TrimSpace(nonEmpty(item.Entity.DisplayName, item.Entity.Name))
	if name == "" {
		return false
	}
	tribeID := tribeIdentityToken(item.Entity.ID)
	if tribeID == "" {
		tribeID = strings.TrimSpace(fmt.Sprint(item.Facts["tribe_id"]))
	}
	if tribeID != "" && strings.EqualFold(name, "Tribe "+tribeID) {
		return false
	}
	if shouldPreserveExistingEntityOnPlaceholder(item.Entity) {
		return false
	}
	return !isNPCTribeName(name)
}

func isNPCTribeName(name string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(name)), "npc corp ")
}

func dedupeCurrentEntities(items []model.CurrentEntity, query CurrentEntityQuery) []model.CurrentEntity {
	if len(items) < 2 {
		return items
	}
	out := make([]model.CurrentEntity, 0, len(items))
	tribeIndexes := make(map[string]int)
	changed := false
	for _, item := range items {
		if key := currentTribeIdentityKey(item); key != "" {
			if index, ok := tribeIndexes[key]; ok {
				changed = true
				existing := out[index]
				if preferCurrentTribeIdentity(item, existing) {
					out[index] = mergeCurrentTribeIdentityRows(item, existing)
				} else {
					out[index] = mergeCurrentTribeIdentityRows(existing, item)
				}
				continue
			}
			tribeIndexes[key] = len(out)
			out = append(out, item)
			continue
		}
		out = append(out, item)
	}
	if !changed {
		return items
	}
	return out
}

func currentTribeIdentityKey(item model.CurrentEntity) string {
	if item.Entity.Type != model.EntityTypeTribe {
		return ""
	}
	tribeID := strings.TrimSpace(fmt.Sprint(item.Facts["tribe_id"]))
	if tribeID == "" || tribeID == "<nil>" {
		tribeID = tribeIdentityToken(item.Entity.ID)
	}
	if tribeID == "" {
		return ""
	}
	return fmt.Sprintf("%s:%s", item.Entity.Environment, tribeID)
}

func preferCurrentTribeIdentity(candidate, existing model.CurrentEntity) bool {
	candidateScore := currentTribeIdentityScore(candidate)
	existingScore := currentTribeIdentityScore(existing)
	if candidateScore != existingScore {
		return candidateScore > existingScore
	}
	if !candidate.Entity.UpdatedAt.Equal(existing.Entity.UpdatedAt) {
		return candidate.Entity.UpdatedAt.After(existing.Entity.UpdatedAt)
	}
	return candidate.Entity.ID > existing.Entity.ID
}

func currentTribeIdentityScore(item model.CurrentEntity) int {
	score := 0
	tribeID := strings.TrimSpace(fmt.Sprint(item.Facts["tribe_id"]))
	if tribeID == "" || tribeID == "<nil>" {
		tribeID = tribeIdentityToken(item.Entity.ID)
	}
	name := strings.TrimSpace(nonEmpty(item.Entity.DisplayName, item.Entity.Name))
	if name != "" && name != "Tribe "+tribeID {
		score += 1000
	}
	for _, key := range []string{"tag", "aliases", "description", "url"} {
		if hasNonEmptyFact(item, key) {
			score += 25
		}
	}
	if entityScope(item.Entity.ID) == string(item.Entity.Environment) {
		score += 10
	}
	if item.Entity.Cycle != nil {
		score += *item.Entity.Cycle
	}
	return score
}

func hasEventBackedCharacterEvidence(item model.CurrentEntity) bool {
	return hasNonEmptyFact(item, "source_event_kind") ||
		hasNonEmptyFact(item, "source_event_id") ||
		hasNonEmptyFact(item, "transaction_digest")
}

func mergeCurrentTribeIdentityRows(winner, loser model.CurrentEntity) model.CurrentEntity {
	merged := winner
	merged.Facts = mergeCurrentFacts(winner.Facts, loser.Facts)
	merged.SourceIDs = mergeSourceIDs(winner.SourceIDs, loser.SourceIDs)
	winningTribeToken := tribeIdentityToken(winner.Entity.ID)
	merged.OutgoingRelations = mergeCurrentRelations(winner.OutgoingRelations, loser.OutgoingRelations, func(relation model.CurrentRelation) bool {
		return relation.SubjectEntityID == winner.Entity.ID
	})
	merged.IncomingRelations = mergeCurrentRelations(winner.IncomingRelations, loser.IncomingRelations, func(relation model.CurrentRelation) bool {
		if relation.ObjectEntityID == winner.Entity.ID {
			return true
		}
		return relation.ObjectEntityType == model.EntityTypeTribe && tribeIdentityToken(relation.ObjectEntityID) == winningTribeToken
	})
	merged.Derived = nil
	deriveCurrentEntity(&merged)
	return merged
}

func mergeCurrentFacts(primary, secondary map[string]any) map[string]any {
	merged := make(map[string]any, len(primary)+len(secondary))
	for key, value := range primary {
		merged[key] = value
	}
	for key, value := range secondary {
		if _, ok := merged[key]; !ok || strings.TrimSpace(fmt.Sprint(merged[key])) == "" {
			merged[key] = value
		}
	}
	return merged
}

func mergeSourceIDs(primary, secondary []string) []string {
	seen := make(map[string]struct{}, len(primary)+len(secondary))
	var out []string
	for _, sourceID := range append(primary, secondary...) {
		sourceID = strings.TrimSpace(sourceID)
		if sourceID == "" {
			continue
		}
		if _, ok := seen[sourceID]; ok {
			continue
		}
		seen[sourceID] = struct{}{}
		out = append(out, sourceID)
	}
	sort.Strings(out)
	return out
}

func mergeCurrentRelations(primary, secondary []model.CurrentRelation, keep func(model.CurrentRelation) bool) []model.CurrentRelation {
	seen := make(map[string]struct{}, len(primary)+len(secondary))
	out := make([]model.CurrentRelation, 0, len(primary)+len(secondary))
	add := func(relation model.CurrentRelation) {
		if keep != nil && !keep(relation) {
			return
		}
		key := relation.ID
		if key == "" {
			key = fmt.Sprintf("%s:%s:%s:%s", relation.SubjectEntityID, relation.Predicate, relation.ObjectEntityID, relation.SourceID)
		}
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, relation)
	}
	for _, relation := range primary {
		add(relation)
	}
	for _, relation := range secondary {
		add(relation)
	}
	return out
}

func matchesProfileState(item model.CurrentEntity, state string) bool {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "known":
		return item.Derived != nil && item.Derived.Profile != nil
	case "placeholder":
		return shouldPreserveExistingEntityOnPlaceholder(item.Entity)
	default:
		return false
	}
}

func currentRelationMatchesQuery(item model.CurrentRelation, query CurrentRelationQuery) bool {
	if query.SystemID != "" && item.SubjectEntityID != query.SystemID && item.ObjectEntityID != query.SystemID {
		return false
	}
	if query.SourceID != "" && item.SourceID != query.SourceID {
		return false
	}
	return true
}

func hasOutgoingRelation(item model.CurrentEntity, predicate, objectID string) bool {
	for _, relation := range item.OutgoingRelations {
		if relation.Predicate == predicate && currentRelationObjectMatches(relation, objectID) {
			return true
		}
	}
	return false
}

func currentRelationObjectMatches(relation model.CurrentRelation, objectID string) bool {
	if relation.ObjectEntityID == objectID {
		return true
	}
	if relation.ObjectEntityType != model.EntityTypeTribe {
		return false
	}
	return sameTribeIdentity(relation.ObjectEntityID, objectID)
}

func sameTribeIdentity(left, right string) bool {
	left = tribeIdentityToken(left)
	right = tribeIdentityToken(right)
	return left != "" && left == right
}

func tribeIdentityToken(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if index := strings.LastIndex(value, ":"); index >= 0 {
		value = value[index+1:]
	}
	return strings.TrimSpace(value)
}

func entityScope(value string) string {
	parts := strings.Split(value, ":")
	if len(parts) < 3 {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

func hasRelationInEitherDirection(item model.CurrentEntity, predicates map[string]struct{}, relatedID string) bool {
	for _, relation := range item.OutgoingRelations {
		if _, ok := predicates[relation.Predicate]; ok && relation.ObjectEntityID == relatedID {
			return true
		}
	}
	for _, relation := range item.IncomingRelations {
		if _, ok := predicates[relation.Predicate]; ok && relation.SubjectEntityID == relatedID {
			return true
		}
	}
	return false
}

func hasFactValue(item model.CurrentEntity, key, value string) bool {
	found, ok := item.Facts[key]
	return ok && fmt.Sprint(found) == value
}

func hasNonEmptyFact(item model.CurrentEntity, key string) bool {
	found, ok := item.Facts[key]
	return ok && strings.TrimSpace(fmt.Sprint(found)) != ""
}

func hasOwnerCapEvidence(item model.CurrentEntity) bool {
	return hasNonEmptyFact(item, "owner_cap_id") || (item.Derived != nil && item.Derived.OwnerCap != nil)
}

func hasLocationHashEvidence(item model.CurrentEntity) bool {
	return hasNonEmptyFact(item, "location_hash") || (item.Derived != nil && item.Derived.LocationHash != nil)
}

func hasCurrentEntitySource(item model.CurrentEntity, sourceID string) bool {
	for _, candidate := range item.SourceIDs {
		if candidate == sourceID {
			return true
		}
	}
	return false
}

func hasConnectionRelation(item model.CurrentEntity, otherID string) bool {
	for _, relation := range item.OutgoingRelations {
		if isRouteEdgePredicate(relation.Predicate) && relation.ObjectEntityID == otherID {
			return true
		}
	}
	for _, relation := range item.IncomingRelations {
		if isRouteEdgePredicate(relation.Predicate) && relation.SubjectEntityID == otherID {
			return true
		}
	}
	return false
}

func hasActivityRelation(item model.CurrentEntity) bool {
	for _, relation := range item.OutgoingRelations {
		if isActivityPredicate(relation.Predicate) {
			return true
		}
	}
	for _, relation := range item.IncomingRelations {
		if isActivityPredicate(relation.Predicate) {
			return true
		}
	}
	return false
}

func systemRelationPredicates() map[string]struct{} {
	return map[string]struct{}{
		"located_in":       {},
		"located_at":       {},
		"deployed_in":      {},
		"observed_in":      {},
		"occurred_in":      {},
		"observed_between": {},
		"member_of_region": {},
	}
}

func isRouteEdgePredicate(predicate string) bool {
	return predicate == "links_to" || predicate == "observed_between"
}

func isActivityPredicate(predicate string) bool {
	switch predicate {
	case "victim", "killer", "reported_by", "occurred_in":
		return true
	default:
		return false
	}
}
