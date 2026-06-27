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
