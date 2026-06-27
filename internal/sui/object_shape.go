package sui

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/blackrelay/registry/internal/db"
	"github.com/blackrelay/registry/internal/model"
)

type ObjectShapeStore interface {
	ListSuiObjects(ctx context.Context, query db.SuiObjectQuery) (db.SuiObjectPage, error)
}

type ObjectShapeAuditOptions struct {
	Environment model.Environment
	PackageID   string
	Module      string
	TypeName    string
	TypeRepr    string
	Limit       int
	SampleLimit int
	Now         func() time.Time
}

type ObjectShapeAudit struct {
	SchemaVersion  string               `json:"schemaVersion"`
	Environment    model.Environment    `json:"environment,omitempty"`
	GeneratedAt    time.Time            `json:"generatedAt"`
	PackageID      string               `json:"packageId,omitempty"`
	Module         string               `json:"module,omitempty"`
	TypeName       string               `json:"typeName,omitempty"`
	TypeRepr       string               `json:"typeRepr,omitempty"`
	ObjectsScanned int                  `json:"objectsScanned"`
	KeyPaths       []ObjectShapeKeyPath `json:"keyPaths,omitempty"`
	Samples        []ObjectShapeSample  `json:"samples,omitempty"`
}

type ObjectShapeKeyPath struct {
	Path  string   `json:"path"`
	Count int      `json:"count"`
	Types []string `json:"types,omitempty"`
}

type ObjectShapeSample struct {
	ID         string    `json:"id"`
	ObjectID   string    `json:"objectId"`
	Module     string    `json:"module,omitempty"`
	TypeName   string    `json:"typeName,omitempty"`
	TypeRepr   string    `json:"typeRepr,omitempty"`
	ObservedAt time.Time `json:"observedAt,omitempty"`
}

func BuildObjectShapeAudit(ctx context.Context, store ObjectShapeStore, options ObjectShapeAuditOptions) (ObjectShapeAudit, error) {
	if store == nil {
		return ObjectShapeAudit{}, errors.New("object shape store is required")
	}
	if options.Environment == "" {
		options.Environment = model.EnvironmentStillness
	}
	now := time.Now().UTC()
	if options.Now != nil {
		now = options.Now().UTC()
	}
	limit := options.Limit
	if limit <= 0 {
		limit = 1000
	}
	sampleLimit := options.SampleLimit
	if sampleLimit <= 0 {
		sampleLimit = 5
	}
	page, err := store.ListSuiObjects(ctx, db.SuiObjectQuery{
		Environment: options.Environment,
		PackageID:   options.PackageID,
		Module:      options.Module,
		TypeName:    options.TypeName,
		TypeRepr:    options.TypeRepr,
		Limit:       limit,
	})
	if err != nil {
		return ObjectShapeAudit{}, err
	}
	paths := make(map[string]*ObjectShapeKeyPath)
	samples := make([]ObjectShapeSample, 0, sampleLimit)
	for _, object := range page.Items {
		walkObjectShape("", objectJSON(object), paths)
		if len(samples) < sampleLimit {
			samples = append(samples, ObjectShapeSample{
				ID:         object.ID,
				ObjectID:   object.ObjectID,
				Module:     object.Module,
				TypeName:   object.TypeName,
				TypeRepr:   object.TypeRepr,
				ObservedAt: object.ObservedAt,
			})
		}
	}
	keyPaths := make([]ObjectShapeKeyPath, 0, len(paths))
	for _, item := range paths {
		sort.Strings(item.Types)
		keyPaths = append(keyPaths, *item)
	}
	sort.Slice(keyPaths, func(i, j int) bool { return keyPaths[i].Path < keyPaths[j].Path })
	return ObjectShapeAudit{
		SchemaVersion:  "registry.sui-object-shape-audit.v1",
		Environment:    options.Environment,
		GeneratedAt:    now,
		PackageID:      options.PackageID,
		Module:         options.Module,
		TypeName:       options.TypeName,
		TypeRepr:       options.TypeRepr,
		ObjectsScanned: len(page.Items),
		KeyPaths:       keyPaths,
		Samples:        samples,
	}, nil
}

func walkObjectShape(prefix string, value any, paths map[string]*ObjectShapeKeyPath) {
	switch typed := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			child := key
			if prefix != "" {
				child = prefix + "." + key
			}
			walkObjectShape(child, typed[key], paths)
		}
	case []any:
		path := prefix + "[]"
		addShapePath(path, typed, paths)
		for _, item := range typed {
			walkObjectShape(path, item, paths)
		}
	default:
		addShapePath(prefix, value, paths)
	}
}

func addShapePath(path string, value any, paths map[string]*ObjectShapeKeyPath) {
	if path == "" {
		return
	}
	item := paths[path]
	if item == nil {
		item = &ObjectShapeKeyPath{Path: path}
		paths[path] = item
	}
	item.Count++
	shapeType := shapeValueType(value)
	for _, existing := range item.Types {
		if existing == shapeType {
			return
		}
	}
	item.Types = append(item.Types, shapeType)
}

func shapeValueType(value any) string {
	switch value.(type) {
	case nil:
		return "null"
	case bool:
		return "boolean"
	case float64, float32, int, int64, int32, uint64, uint32:
		return "number"
	case string:
		return "string"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	default:
		return "unknown"
	}
}
