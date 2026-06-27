package report

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/blackrelay/registry/internal/model"
	"github.com/blackrelay/registry/internal/staticclient"
)

type SystemReconciliationOptions struct {
	Environment model.Environment
	PageSize    int
	Now         func() time.Time
}

type SystemReconciliation struct {
	SchemaVersion       string            `json:"schemaVersion"`
	Environment         model.Environment `json:"environment,omitempty"`
	GeneratedAt         time.Time         `json:"generatedAt"`
	SourceSystemCount   int               `json:"sourceSystemCount"`
	RegistrySystemCount int               `json:"registrySystemCount"`
	MatchedSystemCount  int               `json:"matchedSystemCount"`
	MissingInRegistry   []string          `json:"missingInRegistry,omitempty"`
	MissingInSource     []string          `json:"missingInSource,omitempty"`
}

func BuildSystemReconciliation(ctx context.Context, store CurrentStateStore, staticUniverseDir string, options SystemReconciliationOptions) (SystemReconciliation, error) {
	if options.Environment == "" {
		options.Environment = model.EnvironmentStillness
	}
	now := time.Now().UTC()
	if options.Now != nil {
		now = options.Now().UTC()
	}
	sourceIDs, err := staticclient.ReadUniverseSystemIDs(staticUniverseDir)
	if err != nil {
		return SystemReconciliation{}, err
	}
	registryItems, err := listAllCurrentEntities(ctx, store, options.Environment, model.EntityTypeSystem, boundedPageSize(options.PageSize, 200))
	if err != nil {
		return SystemReconciliation{}, err
	}
	sourceSet := stringSet(sourceIDs)
	registryIDs := make([]string, 0, len(registryItems))
	for _, item := range registryItems {
		if id := currentSystemID(item); id != "" {
			registryIDs = append(registryIDs, id)
		}
	}
	registrySet := stringSet(registryIDs)
	return SystemReconciliation{
		SchemaVersion:       "registry.system-reconciliation.v1",
		Environment:         options.Environment,
		GeneratedAt:         now,
		SourceSystemCount:   len(sourceSet),
		RegistrySystemCount: len(registrySet),
		MatchedSystemCount:  intersectionCount(sourceSet, registrySet),
		MissingInRegistry:   setDifference(sourceSet, registrySet),
		MissingInSource:     setDifference(registrySet, sourceSet),
	}, nil
}

func currentSystemID(item model.CurrentEntity) string {
	for _, key := range []string{"solar_system_id", "system_id"} {
		if value, ok := item.Facts[key]; ok {
			return fmt.Sprint(value)
		}
	}
	prefix := "system:" + string(item.Entity.Environment) + ":"
	return strings.TrimPrefix(item.Entity.ID, prefix)
}

func stringSet(values []string) map[string]struct{} {
	out := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out[value] = struct{}{}
		}
	}
	return out
}

func intersectionCount(left, right map[string]struct{}) int {
	total := 0
	for value := range left {
		if _, ok := right[value]; ok {
			total++
		}
	}
	return total
}

func setDifference(left, right map[string]struct{}) []string {
	out := make([]string, 0)
	for value := range left {
		if _, ok := right[value]; !ok {
			out = append(out, value)
		}
	}
	sort.Strings(out)
	return out
}
