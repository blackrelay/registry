package db

import (
	"strings"

	"github.com/blackrelay/registry/internal/model"
)

func shouldPreserveExistingEntityOnPlaceholder(entity model.Entity) bool {
	switch entity.Type {
	case model.EntityTypeCharacter:
		return isEntityPlaceholder(entity, "Character")
	case model.EntityTypeTribe:
		return isEntityPlaceholder(entity, "Tribe")
	case model.EntityTypeSystem:
		return isEntityPlaceholder(entity, "System")
	case model.EntityTypeItem, model.EntityTypeMaterial, model.EntityTypeShip, model.EntityTypeStructure, model.EntityTypeBlueprint:
		return isStaticTypePlaceholder(entity)
	default:
		return false
	}
}

func IsPlaceholderEntity(entity model.Entity) bool {
	return shouldPreserveExistingEntityOnPlaceholder(entity)
}

func isEntityPlaceholder(entity model.Entity, label string) bool {
	localID := entity.ID
	if index := strings.LastIndex(localID, ":"); index >= 0 {
		localID = localID[index+1:]
	}
	placeholder := label + " " + localID
	return entity.Name == placeholder && (entity.DisplayName == "" || entity.DisplayName == placeholder)
}

func isStaticTypePlaceholder(entity model.Entity) bool {
	localID := entity.ID
	if index := strings.LastIndex(localID, ":"); index >= 0 {
		localID = entity.ID[index+1:]
	}
	if localID == "" {
		return false
	}
	name := strings.ToLower(strings.TrimSpace(entity.Name))
	displayName := strings.ToLower(strings.TrimSpace(entity.DisplayName))
	if displayName != "" && displayName != name {
		return false
	}
	for _, prefix := range []string{
		"blueprint type ",
		"facility type ",
		"input type ",
		"item type ",
		"material type ",
		"ship type ",
		"structure type ",
		"type ",
	} {
		if name == prefix+localID {
			return true
		}
	}
	return false
}
