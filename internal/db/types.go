package db

import (
	"fmt"
	"time"

	"github.com/blackrelay/registry/internal/model"
)

type EntityQuery struct {
	Q                string
	Type             model.EntityType
	Environment      model.Environment
	Cycles           []int
	IncludeUncycled  bool
	TypeID           string
	GroupID          string
	CategoryID       string
	MarketGroupID    string
	WreckTypeID      string
	SourceArtefactID string
	StaticEntityType string
	Limit            int
	Cursor           string
}

type EntityPage struct {
	Items      []model.Entity `json:"items"`
	NextCursor string         `json:"nextCursor,omitempty"`
}

type CurrentEntityQuery struct {
	Type              model.EntityType
	Environment       model.Environment
	Cycles            []int
	IncludeUncycled   bool
	Q                 string
	ProfileState      string
	TribeID           string
	OwnerID           string
	SystemID          string
	OwnerCapID        string
	LocationHash      string
	ConnectedTo       string
	HasActivity       *bool
	HasTribe          *bool
	HasOwnerCap       *bool
	HasLocationHash   *bool
	HasResolvedOwner  *bool
	HasResolvedSystem *bool
	SourceID          string
	Limit             int
	Cursor            string
}

type CurrentEntityPage struct {
	Items      []model.CurrentEntity `json:"items"`
	NextCursor string                `json:"nextCursor,omitempty"`
}

type CurrentRelationQuery struct {
	Predicates      []string
	Environment     model.Environment
	Cycles          []int
	IncludeUncycled bool
	SystemID        string
	SourceID        string
	Limit           int
	Cursor          string
}

type CurrentRelationPage struct {
	Items      []model.CurrentRelation `json:"items"`
	NextCursor string                  `json:"nextCursor,omitempty"`
}

type SourceQuery struct {
	Environment     model.Environment
	Cycles          []int
	IncludeUncycled bool
	Limit           int
	Cursor          string
}

type SourcePage struct {
	Items      []model.Source `json:"items"`
	NextCursor string         `json:"nextCursor,omitempty"`
}

type DatabaseIdentity struct {
	Engine         string   `json:"engine"`
	Database       string   `json:"database,omitempty"`
	ServerVersion  string   `json:"serverVersion,omitempty"`
	SchemaVersions []string `json:"schemaVersions,omitempty"`
}

type RegistryCountSnapshot struct {
	Counts               RegistryRowCounts          `json:"counts"`
	EntitiesByType       map[model.EntityType]int64 `json:"entitiesByType,omitempty"`
	EventsByModule       map[string]int64           `json:"eventsByModule,omitempty"`
	SuiObjectsByType     map[string]int64           `json:"suiObjectsByType,omitempty"`
	RelationsByPredicate map[string]int64           `json:"relationsByPredicate,omitempty"`
}

type RegistryRowCounts struct {
	Sources         int64 `json:"sources"`
	SourceArtefacts int64 `json:"sourceArtefacts"`
	Imports         int64 `json:"imports"`
	Reviews         int64 `json:"reviews"`
	RawSuiEvents    int64 `json:"rawSuiEvents"`
	RawSuiObjects   int64 `json:"rawSuiObjects"`
	Entities        int64 `json:"entities"`
	Facts           int64 `json:"facts"`
	Relations       int64 `json:"relations"`
	Killmails       int64 `json:"killmails"`
	SearchTerms     int64 `json:"searchTerms"`
	SyncCursors     int64 `json:"syncCursors"`
	PlayerProfiles  int64 `json:"playerProfiles"`
}

type KillmailResolutionCounts struct {
	Total               int64 `json:"total"`
	ResolvedSystems     int64 `json:"resolvedSystems"`
	UnresolvedSystems   int64 `json:"unresolvedSystems"`
	ResolvedVictims     int64 `json:"resolvedVictims"`
	UnresolvedVictims   int64 `json:"unresolvedVictims"`
	ResolvedKillers     int64 `json:"resolvedKillers"`
	UnresolvedKillers   int64 `json:"unresolvedKillers"`
	ResolvedReporters   int64 `json:"resolvedReporters"`
	UnresolvedReporters int64 `json:"unresolvedReporters"`
	CharacterKillers    int64 `json:"characterKillers"`
	NPCKillers          int64 `json:"npcKillers"`
}

type EvidenceRelationResolutionCounts struct {
	OwnershipRelations int64 `json:"ownershipRelations"`
	LocationRelations  int64 `json:"locationRelations"`
}

type EntityFactDraft struct {
	Key          string             `json:"key"`
	Value        any                `json:"value"`
	SourceID     string             `json:"sourceId"`
	Confidence   model.Confidence   `json:"confidence"`
	Environment  model.Environment  `json:"environment"`
	Cycle        *int               `json:"cycle,omitempty"`
	ReviewStatus model.ReviewStatus `json:"reviewStatus"`
}

type EntityFactSet struct {
	Entity model.Entity      `json:"entity"`
	Facts  []EntityFactDraft `json:"facts"`
}

type RelationDraft struct {
	SubjectEntityID string            `json:"subjectEntityId"`
	Predicate       string            `json:"predicate"`
	ObjectEntityID  string            `json:"objectEntityId"`
	SourceID        string            `json:"sourceId"`
	Confidence      model.Confidence  `json:"confidence"`
	Environment     model.Environment `json:"environment"`
	ValidFrom       *time.Time        `json:"validFrom,omitempty"`
	ValidTo         *time.Time        `json:"validTo,omitempty"`
}

func RelationID(relation RelationDraft) string {
	return fmt.Sprintf("relation:%s:%s:%s:%s", relation.SubjectEntityID, relation.Predicate, relation.ObjectEntityID, relation.SourceID)
}

func ReviewID(targetKind, targetID string) string {
	return fmt.Sprintf("review:%s:%s", targetKind, targetID)
}

type KillmailQuery struct {
	Environment     model.Environment
	Cycles          []int
	IncludeUncycled bool
	SystemID        string
	VictimID        string
	KillerID        string
	KillerTypeID    string
	ReporterID      string
	NPCOnly         *bool
	From            *time.Time
	To              *time.Time
	ExcludeFixtures bool
	Limit           int
	Cursor          string
}

type KillmailPage struct {
	Items      []model.SemanticKillmail `json:"items"`
	NextCursor string                   `json:"nextCursor,omitempty"`
}

type EventRecord struct {
	ID                string            `json:"id"`
	Kind              string            `json:"kind"`
	Environment       model.Environment `json:"environment"`
	OccurredAt        time.Time         `json:"occurredAt"`
	Cycle             *int              `json:"cycle,omitempty"`
	PackageID         string            `json:"packageId,omitempty"`
	Module            string            `json:"module,omitempty"`
	TransactionDigest string            `json:"transactionDigest,omitempty"`
	Checkpoint        string            `json:"checkpoint,omitempty"`
	SourceID          string            `json:"sourceId,omitempty"`
	Payload           map[string]any    `json:"payload"`
}

type SuiObjectRecord struct {
	ID          string            `json:"id"`
	ObjectID    string            `json:"objectId"`
	Environment model.Environment `json:"environment"`
	TypeRepr    string            `json:"typeRepr"`
	PackageID   string            `json:"packageId,omitempty"`
	Module      string            `json:"module,omitempty"`
	TypeName    string            `json:"typeName,omitempty"`
	Version     string            `json:"version,omitempty"`
	Digest      string            `json:"digest,omitempty"`
	SourceID    string            `json:"sourceId,omitempty"`
	Payload     map[string]any    `json:"payload"`
	ObservedAt  time.Time         `json:"observedAt"`
}

type SuiObjectQuery struct {
	Environment     model.Environment
	Cycles          []int
	IncludeUncycled bool
	PackageID       string
	Module          string
	TypeName        string
	TypeRepr        string
	Limit           int
	Cursor          string
}

type SuiObjectPage struct {
	Items      []SuiObjectRecord `json:"items"`
	NextCursor string            `json:"nextCursor,omitempty"`
}

type EventQuery struct {
	Kind              string
	Environment       model.Environment
	Cycle             *int
	Cycles            []int
	IncludeUncycled   bool
	PackageID         string
	Module            string
	TransactionDigest string
	SourceID          string
	Limit             int
	MaxLimit          int
	Cursor            string
	Ascending         bool
}

type EventPage struct {
	Items      []EventRecord `json:"items"`
	NextCursor string        `json:"nextCursor,omitempty"`
}

type CursorStatus struct {
	ID                   string            `json:"id"`
	Source               string            `json:"source"`
	Environment          model.Environment `json:"environment"`
	CursorValue          string            `json:"cursorValue"`
	CursorKind           string            `json:"cursorKind"`
	LastSuccessfulIngest *time.Time        `json:"lastSuccessfulIngest,omitempty"`
	LastCheckpoint       string            `json:"lastCheckpoint,omitempty"`
	EventsProcessed      int64             `json:"eventsProcessed"`
	ErrorCount           int64             `json:"errorCount"`
	LastErrorSummary     string            `json:"lastErrorSummary,omitempty"`
	UpdatedAt            time.Time         `json:"updatedAt"`
}

type FreshnessStatus struct {
	Source               string            `json:"source"`
	Environment          model.Environment `json:"environment"`
	LastSuccessfulIngest *time.Time        `json:"lastSuccessfulIngest,omitempty"`
	LastCheckpoint       string            `json:"lastCheckpoint,omitempty"`
	EventsProcessed      int64             `json:"eventsProcessed"`
	ErrorCount           int64             `json:"errorCount"`
	LastErrorSummary     string            `json:"lastErrorSummary,omitempty"`
	StalenessStatus      model.Freshness   `json:"stalenessStatus"`
	UpdatedAt            time.Time         `json:"updatedAt"`
}

type ReviewDraft struct {
	TargetKind string `json:"targetKind"`
	TargetID   string `json:"targetId"`
	Notes      string `json:"notes,omitempty"`
}

type ReviewUpdate struct {
	Reviewer string `json:"reviewer,omitempty"`
	Notes    string `json:"notes,omitempty"`
}
