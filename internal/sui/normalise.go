package sui

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/blackrelay/registry/internal/cycles"
	"github.com/blackrelay/registry/internal/db"
	"github.com/blackrelay/registry/internal/model"
)

var moveTypePattern = regexp.MustCompile(`^(0x[0-9a-fA-F]+)::([A-Za-z_][A-Za-z0-9_]*)::([A-Za-z_][A-Za-z0-9_]*)(?:<.*>)?$`)

type NormalizeOptions struct {
	Environment model.Environment
	SourceID    string
	FetchedAt   time.Time
}

type moveTypeParts struct {
	PackageID string
	Module    string
	TypeName  string
}

func NormalizeMoveEvent(node MoveEventNode, options NormalizeOptions) (db.EventRecord, error) {
	if options.Environment == "" {
		options.Environment = model.EnvironmentUnknown
	}
	if options.FetchedAt.IsZero() {
		options.FetchedAt = time.Now().UTC()
	}
	typeRepr := ""
	if node.Contents != nil && node.Contents.Type != nil {
		typeRepr = node.Contents.Type.Repr
	}
	if typeRepr == "" {
		return db.EventRecord{}, fmt.Errorf("event does not include a Move event type")
	}
	parts, err := parseMoveType(typeRepr)
	if err != nil {
		return db.EventRecord{}, err
	}
	occurredAt := options.FetchedAt
	if node.Timestamp != "" {
		parsed, err := time.Parse(time.RFC3339Nano, node.Timestamp)
		if err != nil {
			return db.EventRecord{}, fmt.Errorf("parse Sui event timestamp: %w", err)
		}
		occurredAt = parsed.UTC()
	}
	module := parts.Module
	packageID := parts.PackageID
	if node.TransactionModule != nil {
		if node.TransactionModule.Name != "" {
			module = node.TransactionModule.Name
		}
		if node.TransactionModule.Package != nil && node.TransactionModule.Package.Address != "" {
			packageID = node.TransactionModule.Package.Address
		}
	}
	transactionDigest := ""
	if node.Transaction != nil {
		transactionDigest = node.Transaction.Digest
	}
	eventID := eventID(transactionDigest, node.SequenceNumber, typeRepr, node.Contents)
	payload := map[string]any{
		"confidence":        model.ConfidenceVerified,
		"json":              contentsJSON(node),
		"moveType":          typeRepr,
		"packageId":         packageID,
		"module":            module,
		"reviewStatus":      model.ReviewStatusCandidate,
		"sequenceNumber":    node.SequenceNumber,
		"sourceKind":        model.SourceKindSuiEvent,
		"transactionDigest": transactionDigest,
	}
	if node.Sender != nil && node.Sender.Address != "" {
		payload["sender"] = node.Sender.Address
	}
	if node.Timestamp != "" {
		payload["timestamp"] = node.Timestamp
	}
	if key := eventKey(node); key != nil {
		payload["key"] = key
	}
	return db.EventRecord{
		ID:                eventID,
		Kind:              eventKindForMoveType(parts),
		Environment:       options.Environment,
		OccurredAt:        occurredAt,
		Cycle:             cycles.FromTime(occurredAt),
		PackageID:         packageID,
		Module:            module,
		TransactionDigest: transactionDigest,
		SourceID:          options.SourceID,
		Payload:           payload,
	}, nil
}

func NormalizeMoveObject(node MoveObjectNode, options NormalizeOptions) (db.SuiObjectRecord, error) {
	if options.Environment == "" {
		options.Environment = model.EnvironmentUnknown
	}
	if options.FetchedAt.IsZero() {
		options.FetchedAt = time.Now().UTC()
	}
	if strings.TrimSpace(node.Address) == "" {
		return db.SuiObjectRecord{}, fmt.Errorf("object does not include an address")
	}
	if node.AsMoveObject == nil || node.AsMoveObject.Contents == nil || node.AsMoveObject.Contents.Type == nil {
		return db.SuiObjectRecord{}, fmt.Errorf("object %s does not include a Move object type", node.Address)
	}
	typeRepr := node.AsMoveObject.Contents.Type.Repr
	parts, err := parseMoveType(typeRepr)
	if err != nil {
		return db.SuiObjectRecord{}, err
	}
	version := node.Version.String()
	id := objectRecordID(node.Address, version, typeRepr, node.AsMoveObject.Contents.JSON)
	payload := map[string]any{
		"confidence":   model.ConfidenceVerified,
		"digest":       node.Digest,
		"json":         objectContentsJSON(node),
		"moveType":     typeRepr,
		"objectId":     node.Address,
		"packageId":    parts.PackageID,
		"module":       parts.Module,
		"reviewStatus": model.ReviewStatusCandidate,
		"sourceKind":   model.SourceKindSuiObject,
		"typeName":     parts.TypeName,
		"version":      version,
	}
	return db.SuiObjectRecord{
		ID:          id,
		ObjectID:    node.Address,
		Environment: options.Environment,
		TypeRepr:    typeRepr,
		PackageID:   parts.PackageID,
		Module:      parts.Module,
		TypeName:    parts.TypeName,
		Version:     version,
		Digest:      node.Digest,
		SourceID:    options.SourceID,
		Payload:     payload,
		ObservedAt:  options.FetchedAt,
	}, nil
}

func parseMoveType(value string) (moveTypeParts, error) {
	match := moveTypePattern.FindStringSubmatch(value)
	if match == nil {
		return moveTypeParts{}, fmt.Errorf("malformed Move type %s", value)
	}
	return moveTypeParts{PackageID: match[1], Module: match[2], TypeName: match[3]}, nil
}

func eventKindForMoveType(parts moveTypeParts) string {
	typeName := parts.TypeName
	if strings.HasSuffix(typeName, "EventV2") {
		typeName = strings.TrimSuffix(typeName, "EventV2") + "V2"
	} else {
		typeName = strings.TrimSuffix(typeName, "Event")
	}
	moduleName := toSnake(parts.Module)
	eventName := toSnake(typeName)
	prefix := moduleName + "_"
	eventName = strings.TrimPrefix(eventName, prefix)
	return moduleName + "." + strings.ReplaceAll(eventName, "_", ".")
}

func eventID(digest string, sequenceNumber int64, typeRepr string, contents *MoveEventContents) string {
	if digest != "" {
		return fmt.Sprintf("event:%s:%d", digest, sequenceNumber)
	}
	encoded, _ := json.Marshal(struct {
		Type     string             `json:"type"`
		Seq      int64              `json:"seq"`
		Contents *MoveEventContents `json:"contents,omitempty"`
	}{Type: typeRepr, Seq: sequenceNumber, Contents: contents})
	sum := sha256.Sum256(encoded)
	return "event:unknown:" + hex.EncodeToString(sum[:12])
}

func contentsJSON(node MoveEventNode) any {
	if node.Contents == nil {
		return map[string]any{}
	}
	if node.Contents.JSON == nil {
		return map[string]any{}
	}
	return node.Contents.JSON
}

func objectContentsJSON(node MoveObjectNode) any {
	if node.AsMoveObject == nil || node.AsMoveObject.Contents == nil {
		return map[string]any{}
	}
	if node.AsMoveObject.Contents.JSON == nil {
		return map[string]any{}
	}
	return node.AsMoveObject.Contents.JSON
}

func objectRecordID(objectID, version, typeRepr string, contents any) string {
	if objectID != "" && version != "" {
		return fmt.Sprintf("object:%s:%s", objectID, version)
	}
	encoded, _ := json.Marshal(struct {
		ObjectID string `json:"objectId"`
		Type     string `json:"type"`
		Contents any    `json:"contents,omitempty"`
	}{ObjectID: objectID, Type: typeRepr, Contents: contents})
	sum := sha256.Sum256(encoded)
	return "object:unknown:" + hex.EncodeToString(sum[:12])
}

func eventKey(node MoveEventNode) map[string]any {
	jsonValue, ok := contentsJSON(node).(map[string]any)
	if !ok {
		return nil
	}
	value, ok := jsonValue["key"].(map[string]any)
	if !ok {
		return nil
	}
	out := make(map[string]any)
	if itemID, ok := value["item_id"]; ok {
		out["item_id"] = itemID
	}
	if tenant, ok := value["tenant"].(string); ok && tenant != "" {
		out["tenant"] = tenant
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func toSnake(value string) string {
	var out []rune
	for i, r := range value {
		if r >= 'A' && r <= 'Z' {
			if i > 0 {
				out = append(out, '_')
			}
			out = append(out, r+'a'-'A')
			continue
		}
		out = append(out, r)
	}
	return string(out)
}
