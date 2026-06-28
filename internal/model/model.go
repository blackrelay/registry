package model

import "time"

type Environment string

const (
	EnvironmentStillness Environment = "stillness"
	EnvironmentUtopia    Environment = "utopia"
	EnvironmentUnknown   Environment = "unknown"
)

type SourceKind string

const (
	SourceKindOnChain           SourceKind = "onchain"
	SourceKindSuiEvent          SourceKind = "sui_event"
	SourceKindSuiObject         SourceKind = "sui_object"
	SourceKindWorldAPI          SourceKind = "world_api"
	SourceKindDatahub           SourceKind = "datahub"
	SourceKindStaticClientData  SourceKind = "static_client_data"
	SourceKindReverseEngineered SourceKind = "reverse_engineered"
	SourceKindObservedGameplay  SourceKind = "observed_gameplay"
	SourceKindCommunityReport   SourceKind = "community_report"
	SourceKindManualInference   SourceKind = "manual_inference"
)

type Confidence string

const (
	ConfidenceVerified Confidence = "verified"
	ConfidenceProbable Confidence = "probable"
	ConfidenceReported Confidence = "reported"
	ConfidenceStale    Confidence = "stale"
	ConfidenceUnknown  Confidence = "unknown"
)

type ReviewStatus string

const (
	ReviewStatusCandidate  ReviewStatus = "candidate"
	ReviewStatusReviewed   ReviewStatus = "reviewed"
	ReviewStatusPublished  ReviewStatus = "published"
	ReviewStatusRejected   ReviewStatus = "rejected"
	ReviewStatusSuperseded ReviewStatus = "superseded"
)

type Freshness string

const (
	FreshnessLiveIndexed       Freshness = "live_indexed"
	FreshnessCachedSnapshot    Freshness = "cached_snapshot"
	FreshnessStaticCycleData   Freshness = "static_cycle_data"
	FreshnessCycleArchive      Freshness = "cycle_archive"
	FreshnessManualObservation Freshness = "manual_observation"
	FreshnessUnknown           Freshness = "unknown"
)

type EntityType string

const (
	EntityTypeCharacter      EntityType = "character"
	EntityTypePlayer         EntityType = "player"
	EntityTypeTribe          EntityType = "tribe"
	EntityTypeAlliance       EntityType = "alliance"
	EntityTypeItem           EntityType = "item"
	EntityTypeMaterial       EntityType = "material"
	EntityTypeRecipe         EntityType = "recipe"
	EntityTypeBlueprint      EntityType = "blueprint"
	EntityTypeShip           EntityType = "ship"
	EntityTypeStructure      EntityType = "structure"
	EntityTypeAssembly       EntityType = "assembly"
	EntityTypeGate           EntityType = "gate"
	EntityTypeStorage        EntityType = "storage"
	EntityTypeMarket         EntityType = "market"
	EntityTypeTurret         EntityType = "turret"
	EntityTypeSystem         EntityType = "system"
	EntityTypeRegion         EntityType = "region"
	EntityTypeConstellation  EntityType = "constellation"
	EntityTypeSite           EntityType = "site"
	EntityTypeResourceObject EntityType = "resource_object"
	EntityTypeEnemy          EntityType = "enemy"
	EntityTypeKillmail       EntityType = "killmail"
	EntityTypeEvent          EntityType = "event"
	EntityTypeRoute          EntityType = "route"
	EntityTypeUnknown        EntityType = "unknown"
)

type Source struct {
	ID          string         `json:"id"`
	Kind        SourceKind     `json:"kind"`
	Title       string         `json:"title"`
	Locator     string         `json:"locator"`
	URL         string         `json:"url,omitempty"`
	Environment Environment    `json:"environment,omitempty"`
	Cycle       *int           `json:"cycle,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	CreatedAt   time.Time      `json:"createdAt,omitempty"`
}

type SourceArtefact struct {
	ID                     string       `json:"id"`
	SourceID               string       `json:"sourceId"`
	SourceKind             SourceKind   `json:"sourceKind,omitempty"`
	Kind                   string       `json:"kind"`
	ArtefactKind           string       `json:"artefactKind,omitempty"`
	Environment            Environment  `json:"environment"`
	PathOrURI              string       `json:"pathOrUri"`
	SHA256                 string       `json:"sha256"`
	SizeBytes              int64        `json:"sizeBytes"`
	RowCount               int64        `json:"rowCount,omitempty"`
	ContentType            string       `json:"contentType"`
	ExtractedAt            time.Time    `json:"extractedAt"`
	ImporterName           string       `json:"importerName"`
	ImporterVersion        string       `json:"importerVersion"`
	ClientBuild            string       `json:"clientBuild,omitempty"`
	PatchLabel             string       `json:"patchLabel,omitempty"`
	Cycle                  *int         `json:"cycle,omitempty"`
	ReviewStatus           ReviewStatus `json:"reviewStatus"`
	SupersededByArtefactID string       `json:"supersededByArtefactId,omitempty"`
	Notes                  string       `json:"notes,omitempty"`
	CreatedAt              time.Time    `json:"createdAt,omitempty"`
}

type Entity struct {
	ID          string      `json:"id"`
	Slug        string      `json:"slug"`
	Type        EntityType  `json:"entityType"`
	Name        string      `json:"name"`
	DisplayName string      `json:"displayName,omitempty"`
	Summary     string      `json:"summary,omitempty"`
	Environment Environment `json:"environment"`
	Cycle       *int        `json:"cycle,omitempty"`
	UpdatedAt   time.Time   `json:"updatedAt,omitempty"`
}

type Fact struct {
	EntityID     string       `json:"entityId"`
	Key          string       `json:"key"`
	Value        any          `json:"value"`
	SourceID     string       `json:"sourceId"`
	Confidence   Confidence   `json:"confidence"`
	Environment  Environment  `json:"environment"`
	Cycle        *int         `json:"cycle,omitempty"`
	ReviewStatus ReviewStatus `json:"reviewStatus"`
	ImportID     string       `json:"importId,omitempty"`
	PublishedAt  *time.Time   `json:"publishedAt,omitempty"`
}

type Relation struct {
	SubjectEntityID string      `json:"subjectEntityId"`
	Predicate       string      `json:"predicate"`
	ObjectEntityID  string      `json:"objectEntityId"`
	SourceID        string      `json:"sourceId"`
	Confidence      Confidence  `json:"confidence"`
	Environment     Environment `json:"environment"`
}

type EntityHistory struct {
	Entity    Entity     `json:"entity"`
	Facts     []Fact     `json:"facts"`
	Relations []Relation `json:"relations"`
	Sources   []Source   `json:"sources"`
}

type CurrentEntity struct {
	Entity            Entity            `json:"entity"`
	Facts             map[string]any    `json:"facts"`
	OutgoingRelations []CurrentRelation `json:"outgoingRelations,omitempty"`
	IncomingRelations []CurrentRelation `json:"incomingRelations,omitempty"`
	SourceIDs         []string          `json:"sourceIds,omitempty"`
	Derived           *CurrentDerived   `json:"derived,omitempty"`
}

type CurrentRelation struct {
	ID                 string      `json:"id,omitempty"`
	SubjectEntityID    string      `json:"subjectEntityId"`
	SubjectEntityType  EntityType  `json:"subjectEntityType,omitempty"`
	SubjectDisplayName string      `json:"subjectDisplayName,omitempty"`
	Predicate          string      `json:"predicate"`
	ObjectEntityID     string      `json:"objectEntityId"`
	ObjectEntityType   EntityType  `json:"objectEntityType,omitempty"`
	ObjectDisplayName  string      `json:"objectDisplayName,omitempty"`
	SourceID           string      `json:"sourceId"`
	Confidence         Confidence  `json:"confidence"`
	Environment        Environment `json:"environment"`
	CreatedAt          time.Time   `json:"createdAt,omitempty"`
}

type CurrentRelatedEntity struct {
	EntityID    string     `json:"entityId"`
	EntityType  EntityType `json:"entityType,omitempty"`
	DisplayName string     `json:"displayName,omitempty"`
}

type CurrentProfile struct {
	MetadataName        string   `json:"metadataName,omitempty"`
	MetadataDescription string   `json:"metadataDescription,omitempty"`
	MetadataURL         string   `json:"metadataUrl,omitempty"`
	Tag                 string   `json:"tag,omitempty"`
	Description         string   `json:"description,omitempty"`
	URL                 string   `json:"url,omitempty"`
	Aliases             []string `json:"aliases,omitempty"`
}

type CurrentDerived struct {
	Profile              *CurrentProfile       `json:"profile,omitempty"`
	Tribe                *CurrentRelatedEntity `json:"tribe,omitempty"`
	Owner                *CurrentRelatedEntity `json:"owner,omitempty"`
	System               *CurrentRelatedEntity `json:"system,omitempty"`
	Constellation        *CurrentRelatedEntity `json:"constellation,omitempty"`
	Region               *CurrentRelatedEntity `json:"region,omitempty"`
	OwnerCap             *CurrentRelatedEntity `json:"ownerCap,omitempty"`
	LocationHash         *CurrentRelatedEntity `json:"locationHash,omitempty"`
	MemberCount          int                   `json:"memberCount,omitempty"`
	OwnedObjectCount     int                   `json:"ownedObjectCount,omitempty"`
	ConnectedSystemCount int                   `json:"connectedSystemCount,omitempty"`
	RouteEdgeCount       int                   `json:"routeEdgeCount,omitempty"`
	KillmailCount        int                   `json:"killmailCount,omitempty"`
	PublicActivityCount  int                   `json:"publicActivityCount,omitempty"`
}

type CoverageStatus string

const (
	CoverageStatusIndexed      CoverageStatus = "indexed"
	CoverageStatusErrored      CoverageStatus = "errored"
	CoverageStatusLimited      CoverageStatus = "limited"
	CoverageStatusNotSeen      CoverageStatus = "not_seen"
	CoverageStatusRangeBlocked CoverageStatus = "range_blocked"
)

type SuiCoverageTarget struct {
	Kind                 string         `json:"kind"`
	Status               CoverageStatus `json:"status"`
	CursorID             string         `json:"cursorId"`
	Source               string         `json:"source"`
	CursorKind           string         `json:"cursorKind,omitempty"`
	Environment          Environment    `json:"environment,omitempty"`
	Network              string         `json:"network,omitempty"`
	PackageName          string         `json:"packageName,omitempty"`
	PackageID            string         `json:"packageId,omitempty"`
	Role                 string         `json:"role,omitempty"`
	ModuleName           string         `json:"moduleName,omitempty"`
	EventType            string         `json:"eventType,omitempty"`
	TypeName             string         `json:"typeName,omitempty"`
	TypeRepr             string         `json:"typeRepr,omitempty"`
	CheckpointAfter      *uint64        `json:"checkpointAfter,omitempty"`
	CheckpointBefore     *uint64        `json:"checkpointBefore,omitempty"`
	RowsProcessed        int64          `json:"rowsProcessed"`
	LastSuccessfulIngest *time.Time     `json:"lastSuccessfulIngest,omitempty"`
	LastCheckpoint       string         `json:"lastCheckpoint,omitempty"`
	ErrorCount           int64          `json:"errorCount"`
	LastErrorSummary     string         `json:"lastErrorSummary,omitempty"`
	UpdatedAt            time.Time      `json:"updatedAt,omitempty"`
	MissingCursor        bool           `json:"missingCursor,omitempty"`
	EmptyStream          bool           `json:"emptyStream,omitempty"`
	LimitedByMaxPages    bool           `json:"limitedByMaxPages,omitempty"`
	ProviderRangeBlocked bool           `json:"providerRangeBlocked,omitempty"`
}

type SuiCoverageSummary struct {
	Environment          Environment         `json:"environment,omitempty"`
	Network              string              `json:"network,omitempty"`
	CoverageBasis        string              `json:"coverageBasis"`
	FullCoverageProven   bool                `json:"fullCoverageProven"`
	TargetCount          int                 `json:"targetCount"`
	EventTargets         int                 `json:"eventTargets"`
	ObjectTargets        int                 `json:"objectTargets"`
	DerivationTargets    int                 `json:"derivationTargets"`
	IndexedTargets       int                 `json:"indexedTargets"`
	ErroredTargets       int                 `json:"erroredTargets"`
	LimitedTargets       int                 `json:"limitedTargets"`
	NotSeenTargets       int                 `json:"notSeenTargets"`
	RangeBlockedTargets  int                 `json:"rangeBlockedTargets"`
	RowsProcessed        int64               `json:"rowsProcessed"`
	LastSuccessfulIngest *time.Time          `json:"lastSuccessfulIngest,omitempty"`
	Targets              []SuiCoverageTarget `json:"targets"`
	Warnings             []string            `json:"warnings,omitempty"`
}

type SourceGap struct {
	ID                string      `json:"id"`
	Kind              string      `json:"kind"`
	Category          string      `json:"category,omitempty"`
	Severity          string      `json:"severity"`
	Environment       Environment `json:"environment,omitempty"`
	Count             int64       `json:"count"`
	Summary           string      `json:"summary"`
	RecommendedAction string      `json:"recommendedAction"`
	SuggestedCommands []string    `json:"suggestedCommands,omitempty"`
}

type Review struct {
	ID           string       `json:"id"`
	TargetKind   string       `json:"targetKind"`
	TargetID     string       `json:"targetId"`
	ReviewStatus ReviewStatus `json:"reviewStatus"`
	Reviewer     string       `json:"reviewer,omitempty"`
	Notes        string       `json:"notes,omitempty"`
	ReviewedAt   *time.Time   `json:"reviewedAt,omitempty"`
	CreatedAt    time.Time    `json:"createdAt,omitempty"`
}

type KillmailRaw struct {
	ID                  string         `json:"id"`
	Environment         Environment    `json:"environment"`
	OccurredAt          time.Time      `json:"occurredAt"`
	SystemID            string         `json:"systemId,omitempty"`
	SystemName          string         `json:"systemName,omitempty"`
	VictimCharacterID   string         `json:"victimCharacterId,omitempty"`
	VictimName          string         `json:"victimName,omitempty"`
	KillerCharacterID   string         `json:"killerCharacterId,omitempty"`
	KillerName          string         `json:"killerName,omitempty"`
	KillerTypeID        string         `json:"killerTypeId,omitempty"`
	ReporterCharacterID string         `json:"reporterCharacterId,omitempty"`
	ReporterName        string         `json:"reporterName,omitempty"`
	LossType            string         `json:"lossType,omitempty"`
	SourceIDs           []string       `json:"sourceIds"`
	Raw                 map[string]any `json:"raw,omitempty"`
}

type ResolvedValue struct {
	EntityID    string     `json:"entityId,omitempty"`
	EntityType  EntityType `json:"entityType"`
	RawID       string     `json:"rawId,omitempty"`
	DisplayName string     `json:"displayName"`
	TypeID      string     `json:"typeId,omitempty"`
	IsNPC       bool       `json:"isNpc,omitempty"`
	Confidence  Confidence `json:"confidence"`
	SourceIDs   []string   `json:"sourceIds,omitempty"`
	Warnings    []string   `json:"warnings,omitempty"`
}

type SemanticKillmail struct {
	ID          string        `json:"id"`
	Kind        string        `json:"kind"`
	OccurredAt  time.Time     `json:"occurredAt"`
	System      ResolvedValue `json:"system"`
	Victim      ResolvedValue `json:"victim"`
	Killer      ResolvedValue `json:"killer"`
	Reporter    ResolvedValue `json:"reporter"`
	LossType    string        `json:"lossType,omitempty"`
	SummaryText string        `json:"summaryText,omitempty"`
	Sources     []string      `json:"sources,omitempty"`
	Warnings    []string      `json:"warnings,omitempty"`
}

type ErrorEnvelope struct {
	Error ErrorBody `json:"error"`
	Meta  Meta      `json:"meta"`
}

type ErrorBody struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

type Meta struct {
	Registry   string `json:"registry"`
	APIVersion string `json:"apiVersion"`
}

const (
	DefaultRegistryID = "black-relay-registry"
	DefaultAPIVersion = "v1"
)

func ResponseMeta(registryID, apiVersion string) Meta {
	if registryID == "" {
		registryID = DefaultRegistryID
	}
	if apiVersion == "" {
		apiVersion = DefaultAPIVersion
	}
	return Meta{
		Registry:   registryID,
		APIVersion: apiVersion,
	}
}
