package resolver

import (
	"context"
	"fmt"
	"strings"

	"github.com/blackrelay/registry/internal/model"
)

type Store interface {
	ResolveCharacter(ctx context.Context, idOrName string, environment model.Environment) (model.ResolvedValue, bool, error)
	ResolveEnemyType(ctx context.Context, typeID string, environment model.Environment) (model.ResolvedValue, bool, error)
	ResolveSystem(ctx context.Context, idOrName string, environment model.Environment) (model.ResolvedValue, bool, error)
}

type Resolver struct {
	Store Store
}

func (r Resolver) Character(ctx context.Context, idOrName string, environment model.Environment) model.ResolvedValue {
	if strings.TrimSpace(idOrName) == "" {
		return unresolved(model.EntityTypeCharacter, idOrName, "character id is empty")
	}
	if r.Store == nil {
		return fallbackCharacter(idOrName)
	}
	value, ok, err := r.Store.ResolveCharacter(ctx, idOrName, environment)
	if err != nil {
		out := fallbackCharacter(idOrName)
		out.Warnings = append(out.Warnings, fmt.Sprintf("character lookup failed: %s", err.Error()))
		return out
	}
	if !ok {
		return fallbackCharacter(idOrName)
	}
	return value
}

func (r Resolver) EnemyType(ctx context.Context, typeID string, environment model.Environment) model.ResolvedValue {
	if strings.TrimSpace(typeID) == "" {
		return unresolved(model.EntityTypeEnemy, typeID, "enemy type id is empty")
	}
	if r.Store == nil {
		return unresolved(model.EntityTypeUnknown, typeID, "killer could not be resolved")
	}
	value, ok, err := r.Store.ResolveEnemyType(ctx, typeID, environment)
	if err != nil {
		return unresolved(model.EntityTypeUnknown, typeID, fmt.Sprintf("enemy lookup failed: %s", err.Error()))
	}
	if !ok {
		return unresolved(model.EntityTypeUnknown, typeID, "killer could not be resolved")
	}
	return value
}

func (r Resolver) System(ctx context.Context, idOrName string, environment model.Environment) model.ResolvedValue {
	if strings.TrimSpace(idOrName) == "" {
		return unresolved(model.EntityTypeSystem, idOrName, "system id is empty")
	}
	if r.Store == nil {
		return fallbackSystem(idOrName)
	}
	value, ok, err := r.Store.ResolveSystem(ctx, idOrName, environment)
	if err != nil {
		out := fallbackSystem(idOrName)
		out.Warnings = append(out.Warnings, fmt.Sprintf("system lookup failed: %s", err.Error()))
		return out
	}
	if !ok {
		return fallbackSystem(idOrName)
	}
	return value
}

func fallbackCharacter(id string) model.ResolvedValue {
	return model.ResolvedValue{
		EntityType:  model.EntityTypeCharacter,
		RawID:       id,
		DisplayName: id,
		Confidence:  model.ConfidenceUnknown,
		Warnings:    []string{"character could not be resolved"},
	}
}

func fallbackSystem(id string) model.ResolvedValue {
	return model.ResolvedValue{
		EntityType:  model.EntityTypeSystem,
		RawID:       id,
		DisplayName: id,
		Confidence:  model.ConfidenceUnknown,
		Warnings:    []string{"system could not be resolved"},
	}
}

func unresolved(entityType model.EntityType, rawID, warning string) model.ResolvedValue {
	return model.ResolvedValue{
		EntityType:  entityType,
		RawID:       rawID,
		DisplayName: "Unknown",
		Confidence:  model.ConfidenceUnknown,
		Warnings:    []string{warning},
	}
}
