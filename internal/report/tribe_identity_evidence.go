package report

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/blackrelay/registry/internal/db"
	"github.com/blackrelay/registry/internal/model"
)

type TribeIdentityEvidenceAuditOptions struct {
	Environment    model.Environment
	Module         string
	ObjectTypeName string
	ObjectTypeRepr string
	PageSize       int
	SampleLimit    int
	Now            func() time.Time
}

type TribeIdentityEvidenceAudit struct {
	SchemaVersion string                       `json:"schemaVersion"`
	Environment   model.Environment            `json:"environment,omitempty"`
	GeneratedAt   time.Time                    `json:"generatedAt"`
	Filters       TribeIdentityEvidenceFilters `json:"filters,omitempty"`
	Counts        TribeIdentityEvidenceCounts  `json:"counts"`
	Samples       TribeIdentityEvidenceSamples `json:"samples,omitempty"`
	Notes         []string                     `json:"notes,omitempty"`
}

type TribeIdentityEvidenceFilters struct {
	Module         string `json:"module,omitempty"`
	ObjectTypeName string `json:"objectTypeName,omitempty"`
	ObjectTypeRepr string `json:"objectTypeRepr,omitempty"`
}

type TribeIdentityEvidenceCounts struct {
	EventsScanned          int64 `json:"eventsScanned"`
	ObjectsScanned         int64 `json:"objectsScanned"`
	RowsWithTribeID        int64 `json:"rowsWithTribeId"`
	RowsWithTribeName      int64 `json:"rowsWithTribeName"`
	RowsWithTribeTicker    int64 `json:"rowsWithTribeTicker"`
	RowsWithProfileText    int64 `json:"rowsWithProfileText"`
	CandidateIdentityRows  int64 `json:"candidateIdentityRows"`
	RowsWithOnlyMembership int64 `json:"rowsWithOnlyMembership"`
}

type TribeIdentityEvidenceSamples struct {
	IdentityCandidates []TribeIdentityEvidenceSample `json:"identityCandidates,omitempty"`
	MembershipOnly     []TribeIdentityEvidenceSample `json:"membershipOnly,omitempty"`
}

type TribeIdentityEvidenceSample struct {
	SourceTable string    `json:"sourceTable"`
	ID          string    `json:"id"`
	ObservedAt  time.Time `json:"observedAt"`
	EventKind   string    `json:"eventKind,omitempty"`
	Module      string    `json:"module,omitempty"`
	TypeName    string    `json:"typeName,omitempty"`
	TypeRepr    string    `json:"typeRepr,omitempty"`
	TribeID     string    `json:"tribeId,omitempty"`
	TribeName   string    `json:"tribeName,omitempty"`
	TribeTicker string    `json:"tribeTicker,omitempty"`
	Description string    `json:"description,omitempty"`
	URL         string    `json:"url,omitempty"`
	MatchedKeys []string  `json:"matchedKeys,omitempty"`
	SourceID    string    `json:"sourceId,omitempty"`
	PackageID   string    `json:"packageId,omitempty"`
	Transaction string    `json:"transactionDigest,omitempty"`
	Checkpoint  string    `json:"checkpoint,omitempty"`
}

type TribeIdentityEvidenceStore interface {
	ListEvents(ctx context.Context, query db.EventQuery) (db.EventPage, error)
	ListSuiObjects(ctx context.Context, query db.SuiObjectQuery) (db.SuiObjectPage, error)
}

type tribeEvidenceFields struct {
	TribeID     string
	TribeName   string
	TribeTicker string
	Description string
	URL         string
	Keys        []string
}

func BuildTribeIdentityEvidenceAudit(ctx context.Context, store TribeIdentityEvidenceStore, options TribeIdentityEvidenceAuditOptions) (TribeIdentityEvidenceAudit, error) {
	if options.Environment == "" {
		options.Environment = model.EnvironmentStillness
	}
	now := time.Now().UTC()
	if options.Now != nil {
		now = options.Now().UTC()
	}
	pageSize := boundedPageSize(options.PageSize, 5000)
	sampleLimit := options.SampleLimit
	if sampleLimit <= 0 {
		sampleLimit = 10
	}
	result := TribeIdentityEvidenceAudit{
		SchemaVersion: "registry.tribe-identity-evidence-audit.v1",
		Environment:   options.Environment,
		GeneratedAt:   now,
		Filters: TribeIdentityEvidenceFilters{
			Module:         options.Module,
			ObjectTypeName: options.ObjectTypeName,
			ObjectTypeRepr: options.ObjectTypeRepr,
		},
		Notes: []string{
			"Rows with only tribe_id or corpId prove membership/id evidence, not human-readable tribe profile text.",
			"Promote names, tags, descriptions or URLs only when the row itself contains those claims or another reviewed public artefact proves them.",
		},
	}

	cursor := ""
	for {
		page, err := store.ListEvents(ctx, db.EventQuery{
			Environment: options.Environment,
			Module:      options.Module,
			Limit:       pageSize,
			MaxLimit:    5000,
			Cursor:      cursor,
			Ascending:   true,
		})
		if err != nil {
			return TribeIdentityEvidenceAudit{}, err
		}
		for _, event := range page.Items {
			result.Counts.EventsScanned++
			fields := analyseTribeIdentityPayload(event.Payload)
			countTribeEvidence(&result.Counts, fields)
			sample := TribeIdentityEvidenceSample{
				SourceTable: "events",
				ID:          event.ID,
				ObservedAt:  event.OccurredAt,
				EventKind:   event.Kind,
				Module:      event.Module,
				TribeID:     fields.TribeID,
				TribeName:   fields.TribeName,
				TribeTicker: fields.TribeTicker,
				Description: fields.Description,
				URL:         fields.URL,
				MatchedKeys: fields.Keys,
				SourceID:    event.SourceID,
				PackageID:   event.PackageID,
				Transaction: event.TransactionDigest,
				Checkpoint:  event.Checkpoint,
			}
			appendTribeEvidenceSample(&result.Samples, sample, fields, sampleLimit)
		}
		if page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
	}

	cursor = ""
	for {
		page, err := store.ListSuiObjects(ctx, db.SuiObjectQuery{
			Environment: options.Environment,
			Module:      options.Module,
			TypeName:    options.ObjectTypeName,
			TypeRepr:    options.ObjectTypeRepr,
			Limit:       pageSize,
			Cursor:      cursor,
		})
		if err != nil {
			return TribeIdentityEvidenceAudit{}, err
		}
		for _, object := range page.Items {
			result.Counts.ObjectsScanned++
			fields := analyseTribeIdentityPayload(object.Payload)
			countTribeEvidence(&result.Counts, fields)
			sample := TribeIdentityEvidenceSample{
				SourceTable: "sui_objects",
				ID:          object.ID,
				ObservedAt:  object.ObservedAt,
				Module:      object.Module,
				TypeName:    object.TypeName,
				TypeRepr:    object.TypeRepr,
				TribeID:     fields.TribeID,
				TribeName:   fields.TribeName,
				TribeTicker: fields.TribeTicker,
				Description: fields.Description,
				URL:         fields.URL,
				MatchedKeys: fields.Keys,
				SourceID:    object.SourceID,
				PackageID:   object.PackageID,
			}
			appendTribeEvidenceSample(&result.Samples, sample, fields, sampleLimit)
		}
		if page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
	}

	return result, nil
}

func analyseTribeIdentityPayload(payload map[string]any) tribeEvidenceFields {
	fields := tribeEvidenceFields{}
	visitTribeIdentityValue(&fields, nil, "", payload)
	if len(fields.Keys) > 1 {
		sort.Strings(fields.Keys)
		fields.Keys = compactStrings(fields.Keys)
	}
	return fields
}

func visitTribeIdentityValue(fields *tribeEvidenceFields, path []string, key string, value any) {
	if key != "" {
		recordTribeIdentityScalar(fields, path, key, value)
		path = append(path, key)
	}
	switch typed := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for childKey := range typed {
			keys = append(keys, childKey)
		}
		sort.Strings(keys)
		for _, childKey := range keys {
			visitTribeIdentityValue(fields, path, childKey, typed[childKey])
		}
	case []any:
		for i, item := range typed {
			visitTribeIdentityValue(fields, append(path, strconv.Itoa(i)), "", item)
		}
	}
}

func recordTribeIdentityScalar(fields *tribeEvidenceFields, path []string, key string, value any) {
	text := scalarText(value)
	if text == "" {
		return
	}
	normalised := normaliseEvidenceKey(key)
	inTribeContext := pathHasTribeContext(path)
	fullPath := append(append([]string(nil), path...), key)
	switch {
	case normalised == "tribeid" || normalised == "corpid" || (normalised == "id" && inTribeContext):
		setFirst(&fields.TribeID, text)
		fields.Keys = append(fields.Keys, strings.Join(fullPath, "."))
	case normalised == "tribename" || normalised == "corpname" || (normalised == "name" && inTribeContext):
		setFirst(&fields.TribeName, text)
		fields.Keys = append(fields.Keys, strings.Join(fullPath, "."))
	case normalised == "tribeticker" || normalised == "corpticker" || (normalised == "ticker" && inTribeContext):
		setFirst(&fields.TribeTicker, text)
		fields.Keys = append(fields.Keys, strings.Join(fullPath, "."))
	case normalised == "description" && inTribeContext:
		setFirst(&fields.Description, text)
		fields.Keys = append(fields.Keys, strings.Join(fullPath, "."))
	case (normalised == "url" || normalised == "dappurl") && inTribeContext:
		setFirst(&fields.URL, text)
		fields.Keys = append(fields.Keys, strings.Join(fullPath, "."))
	}
}

func countTribeEvidence(counts *TribeIdentityEvidenceCounts, fields tribeEvidenceFields) {
	if fields.TribeID == "" && fields.TribeName == "" && fields.TribeTicker == "" && fields.Description == "" && fields.URL == "" {
		return
	}
	if fields.TribeID != "" {
		counts.RowsWithTribeID++
	}
	if fields.TribeName != "" {
		counts.RowsWithTribeName++
	}
	if fields.TribeTicker != "" {
		counts.RowsWithTribeTicker++
	}
	if fields.Description != "" || fields.URL != "" {
		counts.RowsWithProfileText++
	}
	if fields.TribeName != "" || fields.TribeTicker != "" || fields.Description != "" || fields.URL != "" {
		counts.CandidateIdentityRows++
		return
	}
	counts.RowsWithOnlyMembership++
}

func appendTribeEvidenceSample(samples *TribeIdentityEvidenceSamples, sample TribeIdentityEvidenceSample, fields tribeEvidenceFields, limit int) {
	if fields.TribeName != "" || fields.TribeTicker != "" || fields.Description != "" || fields.URL != "" {
		if len(samples.IdentityCandidates) < limit {
			samples.IdentityCandidates = append(samples.IdentityCandidates, sample)
		}
		return
	}
	if fields.TribeID != "" && len(samples.MembershipOnly) < limit {
		samples.MembershipOnly = append(samples.MembershipOnly, sample)
	}
}

func scalarText(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case float64:
		if typed == float64(int64(typed)) {
			return strconv.FormatInt(int64(typed), 10)
		}
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	case uint64:
		return strconv.FormatUint(typed, 10)
	case bool:
		return strconv.FormatBool(typed)
	default:
		return ""
	}
}

func normaliseEvidenceKey(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	replacer := strings.NewReplacer("_", "", "-", "", " ", "")
	return replacer.Replace(value)
}

func pathHasTribeContext(path []string) bool {
	for _, part := range path {
		normalised := normaliseEvidenceKey(part)
		if strings.Contains(normalised, "tribe") || strings.Contains(normalised, "corp") {
			return true
		}
	}
	return false
}

func setFirst(target *string, value string) {
	if *target == "" {
		*target = value
	}
}

func compactStrings(values []string) []string {
	out := values[:0]
	var last string
	for i, value := range values {
		if i == 0 || value != last {
			out = append(out, value)
			last = value
		}
	}
	return out
}
