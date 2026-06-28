package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/blackrelay/registry/internal/artefacts"
	cyclepkg "github.com/blackrelay/registry/internal/cycles"
	"github.com/blackrelay/registry/internal/db"
	"github.com/blackrelay/registry/internal/importer"
	"github.com/blackrelay/registry/internal/killmail"
	"github.com/blackrelay/registry/internal/model"
	"github.com/blackrelay/registry/internal/resolver"
	"github.com/blackrelay/registry/internal/sui"
)

type Store interface {
	Ping(ctx context.Context) error
	ListEntities(ctx context.Context, query db.EntityQuery) (db.EntityPage, error)
	ListCurrentEntities(ctx context.Context, query db.CurrentEntityQuery) (db.CurrentEntityPage, error)
	ListCurrentRelations(ctx context.Context, query db.CurrentRelationQuery) (db.CurrentRelationPage, error)
	GetEntity(ctx context.Context, idOrSlug string) (model.Entity, bool, error)
	ListEntityFacts(ctx context.Context, entityID string) ([]model.Fact, error)
	ListEntityRelations(ctx context.Context, entityID string) ([]model.Relation, error)
	ListEntitySources(ctx context.Context, entityID string) ([]model.Source, error)
	GetSource(ctx context.Context, id string) (model.Source, bool, error)
	GetArtefact(ctx context.Context, id string) (model.SourceArtefact, bool, error)
	ListEvents(ctx context.Context, query db.EventQuery) (db.EventPage, error)
	GetEvent(ctx context.Context, id string) (db.EventRecord, bool, error)
	ListKillmailRaw(ctx context.Context, query db.KillmailQuery) ([]model.KillmailRaw, string, error)
	GetKillmailRaw(ctx context.Context, id string) (model.KillmailRaw, bool, error)
	ListFreshness(ctx context.Context) ([]db.FreshnessStatus, error)
	ListCursors(ctx context.Context) ([]db.CursorStatus, error)
	ListSourceGaps(ctx context.Context, environment model.Environment) ([]model.SourceGap, error)
	CreateReview(ctx context.Context, draft db.ReviewDraft) (model.Review, error)
	ListReviews(ctx context.Context, status model.ReviewStatus) ([]model.Review, error)
	UpdateReviewStatus(ctx context.Context, id string, status model.ReviewStatus, update db.ReviewUpdate) (model.Review, bool, error)
	resolver.Store
	importer.StaticEnemyStore
}

type Server struct {
	Store           Store
	ArtefactStore   artefacts.Store
	AdminToken      string
	AdminAuthorizer AdminAuthorizer
	RegistryID      string
	APIVersion      string
	Logger          *slog.Logger
}

type AdminAuthorizer interface {
	AuthorizeAdmin(ctx context.Context, r *http.Request) error
}

func (s Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/health", s.health)
	mux.HandleFunc("GET /v1/ready", s.ready)
	mux.HandleFunc("GET /v1/metrics", s.metrics)
	mux.HandleFunc("GET /metrics", s.metrics)
	mux.HandleFunc("GET /v1/search", s.search)
	mux.HandleFunc("GET /v1/entities", s.entities)
	mux.HandleFunc("GET /v1/entities/{id}", s.entity)
	mux.HandleFunc("GET /v1/entities/{id}/facts", s.entityFacts)
	mux.HandleFunc("GET /v1/entities/{id}/relations", s.entityRelations)
	mux.HandleFunc("GET /v1/entities/{id}/sources", s.entitySources)
	mux.HandleFunc("GET /v1/entities/{id}/history", s.entityHistory)
	mux.HandleFunc("GET /v1/types", s.types)
	mux.HandleFunc("GET /v1/types/{typeID}", s.typeByID)
	mux.HandleFunc("GET /v1/current/characters", s.currentEntities(model.EntityTypeCharacter))
	mux.HandleFunc("GET /v1/current/tribes", s.currentEntities(model.EntityTypeTribe))
	mux.HandleFunc("GET /v1/current/assemblies", s.currentEntities(model.EntityTypeAssembly))
	mux.HandleFunc("GET /v1/current/gates", s.currentEntities(model.EntityTypeGate))
	mux.HandleFunc("GET /v1/current/storage", s.currentEntities(model.EntityTypeStorage))
	mux.HandleFunc("GET /v1/current/turrets", s.currentEntities(model.EntityTypeTurret))
	mux.HandleFunc("GET /v1/current/regions", s.currentEntities(model.EntityTypeRegion))
	mux.HandleFunc("GET /v1/current/constellations", s.currentEntities(model.EntityTypeConstellation))
	mux.HandleFunc("GET /v1/current/items", s.currentEntities(model.EntityTypeItem))
	mux.HandleFunc("GET /v1/current/materials", s.currentEntities(model.EntityTypeMaterial))
	mux.HandleFunc("GET /v1/current/enemies", s.currentEntities(model.EntityTypeEnemy))
	mux.HandleFunc("GET /v1/current/recipes", s.currentEntities(model.EntityTypeRecipe))
	mux.HandleFunc("GET /v1/current/blueprints", s.currentEntities(model.EntityTypeBlueprint))
	mux.HandleFunc("GET /v1/current/ships", s.currentEntities(model.EntityTypeShip))
	mux.HandleFunc("GET /v1/current/structures", s.currentEntities(model.EntityTypeStructure))
	mux.HandleFunc("GET /v1/current/systems", s.currentEntities(model.EntityTypeSystem))
	mux.HandleFunc("GET /v1/current/routes", s.currentEntities(model.EntityTypeRoute))
	mux.HandleFunc("GET /v1/current/ownership", s.currentRelations("owned_by"))
	mux.HandleFunc("GET /v1/current/route-edges", s.currentRelations("links_to", "observed_between"))
	mux.HandleFunc("GET /v1/events", s.events)
	mux.HandleFunc("GET /v1/events/{id}", s.event)
	mux.HandleFunc("GET /v1/killmails", s.killmails)
	mux.HandleFunc("GET /v1/killmails/{id}", s.killmail)
	mux.HandleFunc("GET /v1/killmails/{id}/raw", s.killmailRaw)
	mux.HandleFunc("GET /v1/systems", s.entitiesOfType(model.EntityTypeSystem))
	mux.HandleFunc("GET /v1/systems/{id}", s.entityOfType(model.EntityTypeSystem))
	mux.HandleFunc("GET /v1/characters", s.entitiesOfType(model.EntityTypeCharacter))
	mux.HandleFunc("GET /v1/characters/{id}", s.entityOfType(model.EntityTypeCharacter))
	mux.HandleFunc("GET /v1/tribes", s.entitiesOfType(model.EntityTypeTribe))
	mux.HandleFunc("GET /v1/tribes/{id}", s.entityOfType(model.EntityTypeTribe))
	mux.HandleFunc("GET /v1/assemblies", s.entitiesOfType(model.EntityTypeAssembly))
	mux.HandleFunc("GET /v1/assemblies/{id}", s.entityOfType(model.EntityTypeAssembly))
	mux.HandleFunc("GET /v1/gates", s.entitiesOfType(model.EntityTypeGate))
	mux.HandleFunc("GET /v1/gates/{id}", s.entityOfType(model.EntityTypeGate))
	mux.HandleFunc("GET /v1/regions", s.entitiesOfType(model.EntityTypeRegion))
	mux.HandleFunc("GET /v1/regions/{id}", s.entityOfType(model.EntityTypeRegion))
	mux.HandleFunc("GET /v1/constellations", s.entitiesOfType(model.EntityTypeConstellation))
	mux.HandleFunc("GET /v1/constellations/{id}", s.entityOfType(model.EntityTypeConstellation))
	mux.HandleFunc("GET /v1/items", s.entitiesOfType(model.EntityTypeItem))
	mux.HandleFunc("GET /v1/items/{id}", s.entityOfType(model.EntityTypeItem))
	mux.HandleFunc("GET /v1/materials", s.entitiesOfType(model.EntityTypeMaterial))
	mux.HandleFunc("GET /v1/materials/{id}", s.entityOfType(model.EntityTypeMaterial))
	mux.HandleFunc("GET /v1/enemies", s.entitiesOfType(model.EntityTypeEnemy))
	mux.HandleFunc("GET /v1/enemies/{id}", s.entityOfType(model.EntityTypeEnemy))
	mux.HandleFunc("GET /v1/recipes", s.entitiesOfType(model.EntityTypeRecipe))
	mux.HandleFunc("GET /v1/recipes/{id}", s.entityOfType(model.EntityTypeRecipe))
	mux.HandleFunc("GET /v1/blueprints", s.entitiesOfType(model.EntityTypeBlueprint))
	mux.HandleFunc("GET /v1/blueprints/{id}", s.entityOfType(model.EntityTypeBlueprint))
	mux.HandleFunc("GET /v1/ships", s.entitiesOfType(model.EntityTypeShip))
	mux.HandleFunc("GET /v1/ships/{id}", s.entityOfType(model.EntityTypeShip))
	mux.HandleFunc("GET /v1/structures", s.entitiesOfType(model.EntityTypeStructure))
	mux.HandleFunc("GET /v1/structures/{id}", s.entityOfType(model.EntityTypeStructure))
	mux.HandleFunc("GET /v1/sources/{id}", s.source)
	mux.HandleFunc("GET /v1/artefacts/{id}", s.artefact)
	mux.HandleFunc("GET /v1/ops/freshness", s.freshness)
	mux.HandleFunc("GET /v1/ops/cursors", s.cursors)
	mux.HandleFunc("GET /v1/ops/sui-coverage", s.suiCoverage)
	mux.HandleFunc("GET /v1/ops/source-gaps", s.sourceGaps)
	mux.HandleFunc("POST /v1/admin/static-enemies/import", s.adminStaticEnemiesImport)
	mux.HandleFunc("POST /v1/admin/imports", s.adminCreateReview)
	mux.HandleFunc("GET /v1/admin/reviews", s.adminReviews)
	mux.HandleFunc("POST /v1/admin/reviews/{id}/publish", s.adminUpdateReview(model.ReviewStatusPublished))
	mux.HandleFunc("POST /v1/admin/reviews/{id}/reject", s.adminUpdateReview(model.ReviewStatusRejected))
	return requestLog(s.log(), mux)
}

func (s Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"status": "ok",
			"time":   time.Now().UTC(),
		},
		"meta": s.meta(),
	})
}

func (s Server) ready(w http.ResponseWriter, r *http.Request) {
	if s.Store == nil {
		s.writeError(w, http.StatusServiceUnavailable, "not_ready", "Registry store is not configured.", nil)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.Store.Ping(ctx); err != nil {
		s.writeError(w, http.StatusServiceUnavailable, "not_ready", "Registry store is not ready.", map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "ready"}, "meta": s.meta()})
}

func (s Server) metrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	fmt.Fprintln(w, "# HELP blackrelay_registry_build_info Static Registry build marker.")
	fmt.Fprintln(w, "# TYPE blackrelay_registry_build_info gauge")
	fmt.Fprintln(w, "blackrelay_registry_build_info 1")
	if s.Store == nil {
		return
	}
	cursors, err := s.Store.ListCursors(r.Context())
	if err != nil {
		fmt.Fprintln(w, "blackrelay_registry_cursor_errors 1")
		return
	}
	for _, cursor := range cursors {
		fmt.Fprintf(w, "blackrelay_registry_events_processed{source=%q,environment=%q,cursor_kind=%q} %d\n", cursor.Source, cursor.Environment, cursor.CursorKind, cursor.EventsProcessed)
		fmt.Fprintf(w, "blackrelay_registry_cursor_error_count{source=%q,environment=%q,cursor_kind=%q} %d\n", cursor.Source, cursor.Environment, cursor.CursorKind, cursor.ErrorCount)
	}
}

func (s Server) entities(w http.ResponseWriter, r *http.Request) {
	query, ok := s.entityQueryFromRequest(w, r, model.EntityType(r.URL.Query().Get("type")))
	if !ok {
		return
	}
	page, err := s.Store.ListEntities(r.Context(), query)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_query", "Entity query is invalid.", map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": page.Items, "nextCursor": page.NextCursor, "meta": s.meta()})
}

func (s Server) search(w http.ResponseWriter, r *http.Request) {
	query, ok := s.entityQueryFromRequest(w, r, model.EntityType(r.URL.Query().Get("type")))
	if !ok {
		return
	}
	page, err := s.Store.ListEntities(r.Context(), query)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_query", "Search query is invalid.", map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": page.Items, "nextCursor": page.NextCursor, "meta": s.meta()})
}

func (s Server) entity(w http.ResponseWriter, r *http.Request) {
	entity, ok := s.lookupEntity(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": entity, "meta": s.meta()})
}

func (s Server) lookupEntity(w http.ResponseWriter, r *http.Request) (model.Entity, bool) {
	entity, ok, err := s.Store.GetEntity(r.Context(), r.PathValue("id"))
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "store_error", "Entity lookup failed.", map[string]any{"error": err.Error()})
		return model.Entity{}, false
	}
	if !ok {
		s.writeError(w, http.StatusNotFound, "not_found", "Entity not found.", nil)
		return model.Entity{}, false
	}
	return entity, true
}

func (s Server) entityFacts(w http.ResponseWriter, r *http.Request) {
	entity, ok := s.lookupEntity(w, r)
	if !ok {
		return
	}
	items, err := s.Store.ListEntityFacts(r.Context(), entity.ID)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "store_error", "Entity facts lookup failed.", map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": items, "meta": s.meta()})
}

func (s Server) entityRelations(w http.ResponseWriter, r *http.Request) {
	entity, ok := s.lookupEntity(w, r)
	if !ok {
		return
	}
	items, err := s.Store.ListEntityRelations(r.Context(), entity.ID)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "store_error", "Entity relations lookup failed.", map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": items, "meta": s.meta()})
}

func (s Server) entitySources(w http.ResponseWriter, r *http.Request) {
	entity, ok := s.lookupEntity(w, r)
	if !ok {
		return
	}
	items, err := s.Store.ListEntitySources(r.Context(), entity.ID)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "store_error", "Entity sources lookup failed.", map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": items, "meta": s.meta()})
}

func (s Server) entityHistory(w http.ResponseWriter, r *http.Request) {
	entity, ok := s.lookupEntity(w, r)
	if !ok {
		return
	}
	facts, err := s.Store.ListEntityFacts(r.Context(), entity.ID)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "store_error", "Entity facts lookup failed.", map[string]any{"error": err.Error()})
		return
	}
	relations, err := s.Store.ListEntityRelations(r.Context(), entity.ID)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "store_error", "Entity relations lookup failed.", map[string]any{"error": err.Error()})
		return
	}
	sources, err := s.Store.ListEntitySources(r.Context(), entity.ID)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "store_error", "Entity sources lookup failed.", map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"data": model.EntityHistory{
			Entity:    entity,
			Facts:     facts,
			Relations: relations,
			Sources:   sources,
		},
		"meta": s.meta(),
	})
}

func (s Server) currentEntities(entityType model.EntityType) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		scope, ok := s.cycleScopeFromRequest(w, r)
		if !ok {
			return
		}
		profileState := firstNonEmpty(
			r.URL.Query().Get("profile"),
			r.URL.Query().Get("profile_state"),
			r.URL.Query().Get("profileState"),
		)
		if entityType == model.EntityTypeCharacter && profileState == "" {
			profileState = "known"
		}
		page, err := s.Store.ListCurrentEntities(r.Context(), db.CurrentEntityQuery{
			Type:            entityType,
			Environment:     requestEnvironment(r),
			Cycles:          scope.Cycles,
			IncludeUncycled: scope.IncludeUncycled,
			Q:               r.URL.Query().Get("q"),
			ProfileState:    profileState,
			TribeID:         firstNonEmpty(r.URL.Query().Get("tribe"), r.URL.Query().Get("tribe_id"), r.URL.Query().Get("tribeId")),
			OwnerID:         firstNonEmpty(r.URL.Query().Get("owner"), r.URL.Query().Get("owner_id"), r.URL.Query().Get("ownerId")),
			SystemID:        firstNonEmpty(r.URL.Query().Get("system"), r.URL.Query().Get("system_id"), r.URL.Query().Get("systemId")),
			OwnerCapID:      firstNonEmpty(r.URL.Query().Get("owner_cap"), r.URL.Query().Get("owner_cap_id"), r.URL.Query().Get("ownerCap"), r.URL.Query().Get("ownerCapId")),
			LocationHash: firstNonEmpty(
				r.URL.Query().Get("location_hash"),
				r.URL.Query().Get("locationHash"),
			),
			ConnectedTo:       firstNonEmpty(r.URL.Query().Get("connected_to"), r.URL.Query().Get("connectedTo")),
			HasActivity:       parseOptionalBool(r.URL.Query().Get("has_activity"), r.URL.Query().Get("hasActivity")),
			HasTribe:          parseOptionalBool(r.URL.Query().Get("has_tribe"), r.URL.Query().Get("hasTribe")),
			HasOwnerCap:       parseOptionalBool(r.URL.Query().Get("has_owner_cap"), r.URL.Query().Get("hasOwnerCap")),
			HasLocationHash:   parseOptionalBool(r.URL.Query().Get("has_location_hash"), r.URL.Query().Get("hasLocationHash")),
			HasResolvedOwner:  parseOptionalBool(r.URL.Query().Get("has_resolved_owner"), r.URL.Query().Get("hasResolvedOwner")),
			HasResolvedSystem: parseOptionalBool(r.URL.Query().Get("has_resolved_system"), r.URL.Query().Get("hasResolvedSystem")),
			SourceID:          firstNonEmpty(r.URL.Query().Get("source_id"), r.URL.Query().Get("sourceId")),
			Limit:             parseLimit(r, 50),
			Cursor:            r.URL.Query().Get("cursor"),
		})
		if err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid_query", "Current-state query is invalid.", map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": page.Items, "nextCursor": page.NextCursor, "meta": s.meta()})
	}
}

func (s Server) currentRelations(predicates ...string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		scope, ok := s.cycleScopeFromRequest(w, r)
		if !ok {
			return
		}
		page, err := s.Store.ListCurrentRelations(r.Context(), db.CurrentRelationQuery{
			Predicates:      predicates,
			Environment:     requestEnvironment(r),
			Cycles:          scope.Cycles,
			IncludeUncycled: scope.IncludeUncycled,
			SystemID:        firstNonEmpty(r.URL.Query().Get("system"), r.URL.Query().Get("system_id"), r.URL.Query().Get("systemId")),
			SourceID:        firstNonEmpty(r.URL.Query().Get("source_id"), r.URL.Query().Get("sourceId")),
			Limit:           parseLimit(r, 50),
			Cursor:          r.URL.Query().Get("cursor"),
		})
		if err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid_query", "Current relation query is invalid.", map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": page.Items, "nextCursor": page.NextCursor, "meta": s.meta()})
	}
}

func (s Server) entitiesOfType(entityType model.EntityType) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query, ok := s.entityQueryFromRequest(w, r, entityType)
		if !ok {
			return
		}
		page, err := s.Store.ListEntities(r.Context(), query)
		if err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid_query", "Entity query is invalid.", map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": page.Items, "nextCursor": page.NextCursor, "meta": s.meta()})
	}
}

func (s Server) types(w http.ResponseWriter, r *http.Request) {
	query, ok := s.entityQueryFromRequest(w, r, model.EntityType(r.URL.Query().Get("type")))
	if !ok {
		return
	}
	page, err := s.Store.ListEntities(r.Context(), query)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_query", "Type query is invalid.", map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": page.Items, "nextCursor": page.NextCursor, "meta": s.meta()})
}

func (s Server) typeByID(w http.ResponseWriter, r *http.Request) {
	query, ok := s.entityQueryFromRequest(w, r, model.EntityType(r.URL.Query().Get("type")))
	if !ok {
		return
	}
	query.TypeID = r.PathValue("typeID")
	query.Limit = 1
	page, err := s.Store.ListEntities(r.Context(), query)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_query", "Type query is invalid.", map[string]any{"error": err.Error()})
		return
	}
	if len(page.Items) == 0 {
		s.writeError(w, http.StatusNotFound, "not_found", "Type entity not found.", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": page.Items[0], "meta": s.meta()})
}

func (s Server) entityOfType(entityType model.EntityType) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		entity, ok, err := s.Store.GetEntity(r.Context(), r.PathValue("id"))
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, "store_error", "Entity lookup failed.", map[string]any{"error": err.Error()})
			return
		}
		if !ok || entity.Type != entityType {
			s.writeError(w, http.StatusNotFound, "not_found", "Entity not found.", nil)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": entity, "meta": s.meta()})
	}
}

func (s Server) source(w http.ResponseWriter, r *http.Request) {
	source, ok, err := s.Store.GetSource(r.Context(), r.PathValue("id"))
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "store_error", "Source lookup failed.", map[string]any{"error": err.Error()})
		return
	}
	if !ok {
		s.writeError(w, http.StatusNotFound, "not_found", "Source not found.", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": source, "meta": s.meta()})
}

func (s Server) artefact(w http.ResponseWriter, r *http.Request) {
	artefact, ok, err := s.Store.GetArtefact(r.Context(), r.PathValue("id"))
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "store_error", "Artefact lookup failed.", map[string]any{"error": err.Error()})
		return
	}
	if !ok {
		s.writeError(w, http.StatusNotFound, "not_found", "Artefact not found.", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": artefact, "meta": s.meta()})
}

func (s Server) events(w http.ResponseWriter, r *http.Request) {
	scope, ok := s.cycleScopeFromRequest(w, r)
	if !ok {
		return
	}
	page, err := s.Store.ListEvents(r.Context(), db.EventQuery{
		Kind:              r.URL.Query().Get("kind"),
		Environment:       requestEnvironment(r),
		Cycles:            scope.Cycles,
		IncludeUncycled:   scope.IncludeUncycled,
		PackageID:         firstNonEmpty(r.URL.Query().Get("package_id"), r.URL.Query().Get("packageId")),
		Module:            r.URL.Query().Get("module"),
		TransactionDigest: firstNonEmpty(r.URL.Query().Get("transaction_digest"), r.URL.Query().Get("transactionDigest")),
		SourceID:          firstNonEmpty(r.URL.Query().Get("source_id"), r.URL.Query().Get("sourceId")),
		Limit:             parseLimit(r, 50),
		Cursor:            r.URL.Query().Get("cursor"),
	})
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_query", "Event query is invalid.", map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": page.Items, "nextCursor": page.NextCursor, "meta": s.meta()})
}

func (s Server) event(w http.ResponseWriter, r *http.Request) {
	event, ok, err := s.Store.GetEvent(r.Context(), r.PathValue("id"))
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "store_error", "Event lookup failed.", map[string]any{"error": err.Error()})
		return
	}
	if !ok {
		s.writeError(w, http.StatusNotFound, "not_found", "Event not found.", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": event, "meta": s.meta()})
}

func (s Server) killmails(w http.ResponseWriter, r *http.Request) {
	scope, ok := s.cycleScopeFromRequest(w, r)
	if !ok {
		return
	}
	from, ok := s.parseOptionalTime(w, r, "from")
	if !ok {
		return
	}
	to, ok := s.parseOptionalTime(w, r, "to")
	if !ok {
		return
	}
	raw, next, err := s.Store.ListKillmailRaw(r.Context(), db.KillmailQuery{
		Environment:     requestEnvironment(r),
		Cycles:          scope.Cycles,
		IncludeUncycled: scope.IncludeUncycled,
		SystemID:        firstNonEmpty(r.URL.Query().Get("system"), r.URL.Query().Get("system_id"), r.URL.Query().Get("systemId")),
		VictimID:        firstNonEmpty(r.URL.Query().Get("victim"), r.URL.Query().Get("victim_id"), r.URL.Query().Get("victimId")),
		KillerID:        firstNonEmpty(r.URL.Query().Get("killer"), r.URL.Query().Get("killer_id"), r.URL.Query().Get("killerId")),
		KillerTypeID:    firstNonEmpty(r.URL.Query().Get("killer_type_id"), r.URL.Query().Get("killerTypeId")),
		ReporterID:      firstNonEmpty(r.URL.Query().Get("reporter"), r.URL.Query().Get("reporter_id"), r.URL.Query().Get("reporterId")),
		NPCOnly:         parseOptionalBool(r.URL.Query().Get("npc"), r.URL.Query().Get("npc_only"), r.URL.Query().Get("npcOnly")),
		From:            from,
		To:              to,
		ExcludeFixtures: parseBoolDefault(false, r.URL.Query().Get("exclude_fixtures"), r.URL.Query().Get("excludeFixtures")),
		Limit:           parseLimit(r, 50),
		Cursor:          r.URL.Query().Get("cursor"),
	})
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_query", "Killmail query is invalid.", map[string]any{"error": err.Error()})
		return
	}
	service := killmail.Service{Resolver: resolver.Resolver{Store: s.Store}, GraphStore: s.Store}
	items := make([]model.SemanticKillmail, 0, len(raw))
	for _, item := range raw {
		items = append(items, service.Semantic(r.Context(), item))
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": items, "nextCursor": next, "meta": s.meta()})
}

func (s Server) killmail(w http.ResponseWriter, r *http.Request) {
	raw, ok, err := s.Store.GetKillmailRaw(r.Context(), r.PathValue("id"))
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "store_error", "Killmail lookup failed.", map[string]any{"error": err.Error()})
		return
	}
	if !ok {
		s.writeError(w, http.StatusNotFound, "not_found", "Killmail not found.", nil)
		return
	}
	service := killmail.Service{Resolver: resolver.Resolver{Store: s.Store}, GraphStore: s.Store}
	writeJSON(w, http.StatusOK, map[string]any{"data": service.Semantic(r.Context(), raw), "meta": s.meta()})
}

func (s Server) killmailRaw(w http.ResponseWriter, r *http.Request) {
	raw, ok, err := s.Store.GetKillmailRaw(r.Context(), r.PathValue("id"))
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "store_error", "Killmail lookup failed.", map[string]any{"error": err.Error()})
		return
	}
	if !ok {
		s.writeError(w, http.StatusNotFound, "not_found", "Killmail not found.", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": raw, "meta": s.meta()})
}

func (s Server) freshness(w http.ResponseWriter, r *http.Request) {
	items, err := s.Store.ListFreshness(r.Context())
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "store_error", "Freshness lookup failed.", map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": items, "meta": s.meta()})
}

func (s Server) cursors(w http.ResponseWriter, r *http.Request) {
	items, err := s.Store.ListCursors(r.Context())
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "store_error", "Cursor lookup failed.", map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": items, "meta": s.meta()})
}

func (s Server) suiCoverage(w http.ResponseWriter, r *http.Request) {
	items, err := s.Store.ListCursors(r.Context())
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "store_error", "Sui coverage lookup failed.", map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": sui.CursorCoverageSummary(items), "meta": s.meta()})
}

func (s Server) sourceGaps(w http.ResponseWriter, r *http.Request) {
	items, err := s.Store.ListSourceGaps(r.Context(), requestEnvironment(r))
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "store_error", "Source-gap lookup failed.", map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": items, "meta": s.meta()})
}

func (s Server) adminStaticEnemiesImport(w http.ResponseWriter, r *http.Request) {
	if !s.authorised(r) {
		s.writeError(w, http.StatusUnauthorized, "unauthorised", "Admin token is required.", nil)
		return
	}
	var body struct {
		Path        string            `json:"path"`
		Environment model.Environment `json:"environment"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_body", "Request body is invalid JSON.", map[string]any{"error": err.Error()})
		return
	}
	if body.Path == "" {
		s.writeError(w, http.StatusBadRequest, "invalid_body", "Static enemy import path is required.", nil)
		return
	}
	artefactStore := s.ArtefactStore
	if artefactStore == nil {
		artefactStore = artefacts.LocalStore{Root: "artefacts"}
	}
	if body.Environment == "" {
		body.Environment = model.EnvironmentStillness
	}
	result, err := importer.ImportStaticEnemies(r.Context(), s.Store, artefactStore, body.Path, importer.StaticEnemyOptions{
		Environment:     body.Environment,
		AllowedRootDirs: []string{"testdata", "local-extract", "."},
		Notes:           "Imported through local admin endpoint.",
	})
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "import_failed", "Static enemy import failed.", map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": result, "meta": s.meta()})
}

func (s Server) adminCreateReview(w http.ResponseWriter, r *http.Request) {
	if !s.authorised(r) {
		s.writeError(w, http.StatusUnauthorized, "unauthorised", "Admin token is required.", nil)
		return
	}
	var body db.ReviewDraft
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_body", "Request body is invalid JSON.", map[string]any{"error": err.Error()})
		return
	}
	review, err := s.Store.CreateReview(r.Context(), body)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "review_failed", "Review entry could not be created.", map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": review, "meta": s.meta()})
}

func (s Server) adminReviews(w http.ResponseWriter, r *http.Request) {
	if !s.authorised(r) {
		s.writeError(w, http.StatusUnauthorized, "unauthorised", "Admin token is required.", nil)
		return
	}
	items, err := s.Store.ListReviews(r.Context(), model.ReviewStatus(r.URL.Query().Get("status")))
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "store_error", "Review lookup failed.", map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": items, "meta": s.meta()})
}

func (s Server) adminUpdateReview(status model.ReviewStatus) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.authorised(r) {
			s.writeError(w, http.StatusUnauthorized, "unauthorised", "Admin token is required.", nil)
			return
		}
		var body db.ReviewUpdate
		if r.Body != nil {
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
				s.writeError(w, http.StatusBadRequest, "invalid_body", "Request body is invalid JSON.", map[string]any{"error": err.Error()})
				return
			}
		}
		review, ok, err := s.Store.UpdateReviewStatus(r.Context(), r.PathValue("id"), status, body)
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, "store_error", "Review update failed.", map[string]any{"error": err.Error()})
			return
		}
		if !ok {
			s.writeError(w, http.StatusNotFound, "not_found", "Review not found.", nil)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": review, "meta": s.meta()})
	}
}

func (s Server) authorised(r *http.Request) bool {
	if s.AdminAuthorizer != nil {
		return s.AdminAuthorizer.AuthorizeAdmin(r.Context(), r) == nil
	}
	if s.AdminToken == "" {
		return false
	}
	header := r.Header.Get("Authorization")
	const prefix = "Bearer "
	return strings.HasPrefix(header, prefix) && strings.TrimPrefix(header, prefix) == s.AdminToken
}

func (s Server) log() *slog.Logger {
	if s.Logger != nil {
		return s.Logger
	}
	return slog.Default()
}

func (s Server) meta() model.Meta {
	return model.ResponseMeta(s.RegistryID, s.APIVersion)
}

func parseLimit(r *http.Request, fallback int) int {
	value := r.URL.Query().Get("limit")
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	if parsed > 200 {
		return 200
	}
	return parsed
}

func (s Server) entityQueryFromRequest(w http.ResponseWriter, r *http.Request, entityType model.EntityType) (db.EntityQuery, bool) {
	scope, ok := s.cycleScopeFromRequest(w, r)
	if !ok {
		return db.EntityQuery{}, false
	}
	return db.EntityQuery{
		Q:                r.URL.Query().Get("q"),
		Type:             entityType,
		Environment:      requestEnvironment(r),
		Cycles:           scope.Cycles,
		IncludeUncycled:  scope.IncludeUncycled,
		TypeID:           firstNonEmpty(r.URL.Query().Get("type_id"), r.URL.Query().Get("typeId")),
		GroupID:          firstNonEmpty(r.URL.Query().Get("group_id"), r.URL.Query().Get("groupId")),
		CategoryID:       firstNonEmpty(r.URL.Query().Get("category_id"), r.URL.Query().Get("categoryId")),
		MarketGroupID:    firstNonEmpty(r.URL.Query().Get("market_group_id"), r.URL.Query().Get("marketGroupId")),
		WreckTypeID:      firstNonEmpty(r.URL.Query().Get("wreck_type_id"), r.URL.Query().Get("wreckTypeId")),
		SourceArtefactID: firstNonEmpty(r.URL.Query().Get("source_artefact_id"), r.URL.Query().Get("sourceArtefactId")),
		StaticEntityType: firstNonEmpty(r.URL.Query().Get("static_entity_type"), r.URL.Query().Get("staticEntityType")),
		Limit:            parseLimit(r, 50),
		Cursor:           r.URL.Query().Get("cursor"),
	}, true
}

func requestEnvironment(r *http.Request) model.Environment {
	environment := model.Environment(strings.TrimSpace(r.URL.Query().Get("environment")))
	if environment == "" {
		return model.EnvironmentStillness
	}
	return environment
}

func parseOptionalBool(values ...string) *bool {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return nil
		}
		return &parsed
	}
	return nil
}

func parseBoolDefault(fallback bool, values ...string) bool {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return fallback
		}
		return parsed
	}
	return fallback
}

func (s Server) parseOptionalTime(w http.ResponseWriter, r *http.Request, key string) (*time.Time, bool) {
	value := strings.TrimSpace(r.URL.Query().Get(key))
	if value == "" {
		return nil, true
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_query", "Time query parameter must be RFC3339.", map[string]any{"parameter": key})
		return nil, false
	}
	return &parsed, true
}

func (s Server) cycleScopeFromRequest(w http.ResponseWriter, r *http.Request) (cyclepkg.Scope, bool) {
	scope, err := cyclepkg.ParseScope(firstNonEmpty(r.URL.Query().Get("cycles"), r.URL.Query().Get("cycle")), true)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_query", "Cycle query parameter must be all, current, or comma-separated positive integers.", map[string]any{"error": err.Error()})
		return cyclepkg.Scope{}, false
	}
	return scope, true
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func (s Server) writeError(w http.ResponseWriter, status int, code, message string, details map[string]any) {
	writeJSON(w, status, model.ErrorEnvelope{
		Error: model.ErrorBody{
			Code:    code,
			Message: message,
			Details: details,
		},
		Meta: s.meta(),
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(value)
}

func requestLog(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		logger.Info("registry request", "method", r.Method, "path", r.URL.Path, "elapsed", time.Since(start).String())
	})
}
