package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/blackrelay/registry/internal/db"
	"github.com/blackrelay/registry/internal/model"
	"github.com/blackrelay/registry/internal/staticdata"
)

func TestKillmailEndpointReturnsSemanticNPCRecord(t *testing.T) {
	store := db.NewMemoryStore()
	now := time.Date(2026, 6, 24, 10, 0, 0, 0, time.UTC)
	source := model.Source{ID: "source:static-client:stillness:reviewed-enemies", Kind: model.SourceKindStaticClientData, Title: "fixture", Locator: "fixture", Environment: model.EnvironmentStillness}
	artefact := model.SourceArtefact{ID: "artefact:fixture", SourceID: source.ID, Environment: model.EnvironmentStillness, SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}
	if err := store.RecordImport(context.Background(), "import:fixture", source, artefact, nil); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertStaticEnemy(context.Background(), "import:fixture", source, artefact, staticdata.EnemyCandidate{Name: "Caird", GroupID: 5033, TypeID: 92096, Confidence: string(model.ConfidenceProbable), Basis: "confirmed enemy group 5033"}); err != nil {
		t.Fatal(err)
	}
	store.Entities["character:stillness:victim"] = model.Entity{ID: "character:stillness:victim", Slug: "fixture-victim", Type: model.EntityTypeCharacter, Name: "Fixture Victim", Environment: model.EnvironmentStillness, UpdatedAt: now}
	if err := store.UpsertKillmail(context.Background(), model.KillmailRaw{
		ID:                "killmail:stillness:fixture:caird",
		Environment:       model.EnvironmentStillness,
		OccurredAt:        now,
		VictimCharacterID: "character:stillness:victim",
		KillerTypeID:      "92096",
		SourceIDs:         []string{"source:fixture"},
	}); err != nil {
		t.Fatal(err)
	}
	handler := Server{Store: store}.Handler()
	req := httptest.NewRequest(http.MethodGet, "/v1/killmails/killmail:stillness:fixture:caird", nil)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body %s", res.Code, res.Body.String())
	}
	var body struct {
		Data model.SemanticKillmail `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Data.Killer.DisplayName != "Caird [NPC]" {
		t.Fatalf("unexpected killer display name %q", body.Data.Killer.DisplayName)
	}
}

func TestKillmailEndpointReturnsSemanticMycenaNPCRecord(t *testing.T) {
	store := db.NewMemoryStore()
	now := time.Date(2026, 6, 25, 10, 0, 0, 0, time.UTC)
	source := model.Source{ID: "source:static-client:enemies:stillness", Kind: model.SourceKindStaticClientData, Title: "fixture", Locator: "fixture", Environment: model.EnvironmentStillness}
	artefact := model.SourceArtefact{ID: "artefact:fixture-mycena", SourceID: source.ID, Environment: model.EnvironmentStillness, SHA256: "abababababababababababababababababababababababababababababababab"}
	if err := store.RecordImport(context.Background(), "import:fixture-mycena", source, artefact, nil); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertStaticEnemy(context.Background(), "import:fixture-mycena", source, artefact, staticdata.EnemyCandidate{Name: "Mycena", GroupID: 5130, TypeID: 94167, Confidence: string(model.ConfidenceProbable), Basis: "static-client group 5130 with wreck type 81610"}); err != nil {
		t.Fatal(err)
	}
	store.Entities["character:stillness:victim"] = model.Entity{ID: "character:stillness:victim", Slug: "fixture-victim", Type: model.EntityTypeCharacter, Name: "Fixture Victim", Environment: model.EnvironmentStillness, UpdatedAt: now}
	if err := store.UpsertKillmail(context.Background(), model.KillmailRaw{
		ID:                "killmail:stillness:fixture:mycena",
		Environment:       model.EnvironmentStillness,
		OccurredAt:        now,
		VictimCharacterID: "character:stillness:victim",
		KillerTypeID:      "94167",
		SourceIDs:         []string{"source:fixture"},
	}); err != nil {
		t.Fatal(err)
	}
	handler := Server{Store: store}.Handler()
	req := httptest.NewRequest(http.MethodGet, "/v1/killmails/killmail:stillness:fixture:mycena", nil)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body %s", res.Code, res.Body.String())
	}
	var body struct {
		Data model.SemanticKillmail `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Data.Killer.EntityID != "enemy:stillness:type:94167" || body.Data.Killer.DisplayName != "Mycena [NPC]" {
		t.Fatalf("unexpected Mycena killer resolution: %#v", body.Data.Killer)
	}
}

func TestKillmailRoutesReturnSemanticListDetailAndRawEvidence(t *testing.T) {
	store := db.NewMemoryStore()
	now := time.Date(2026, 6, 25, 10, 0, 0, 0, time.UTC)
	source := model.Source{ID: "source:static-client:stillness:reviewed-enemies", Kind: model.SourceKindStaticClientData, Title: "fixture", Locator: "fixture", Environment: model.EnvironmentStillness}
	artefact := model.SourceArtefact{ID: "artefact:fixture", SourceID: source.ID, Environment: model.EnvironmentStillness, SHA256: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}
	if err := store.RecordImport(context.Background(), "import:fixture", source, artefact, nil); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertStaticEnemy(context.Background(), "import:fixture", source, artefact, staticdata.EnemyCandidate{Name: "Caird", GroupID: 5033, TypeID: 92096, Confidence: string(model.ConfidenceProbable), Basis: "confirmed enemy group 5033"}); err != nil {
		t.Fatal(err)
	}
	store.Entities["character:stillness:victim"] = model.Entity{ID: "character:stillness:victim", Slug: "fixture-victim", Type: model.EntityTypeCharacter, Name: "Fixture Victim", DisplayName: "Fixture Victim", Environment: model.EnvironmentStillness, UpdatedAt: now}
	store.Entities["character:stillness:reporter"] = model.Entity{ID: "character:stillness:reporter", Slug: "fixture-reporter", Type: model.EntityTypeCharacter, Name: "Fixture Reporter", DisplayName: "Fixture Reporter", Environment: model.EnvironmentStillness, UpdatedAt: now}
	store.Entities["system:stillness:30001001"] = model.Entity{ID: "system:stillness:30001001", Slug: "system-30001001-stillness", Type: model.EntityTypeSystem, Name: "NN0-Y-D5", DisplayName: "NN0-Y-D5", Environment: model.EnvironmentStillness, UpdatedAt: now}
	if err := store.UpsertKillmail(context.Background(), model.KillmailRaw{
		ID:          "killmail:stillness:fixture:caird",
		Environment: model.EnvironmentStillness,
		OccurredAt:  now,
		Raw: map[string]any{
			"event": map[string]any{
				"json": map[string]any{
					"killer_type_id":             "92096",
					"victim_id":                  map[string]any{"tenant": "stillness", "item_id": "victim"},
					"reported_by_character_id":   map[string]any{"tenant": "stillness", "item_id": "reporter"},
					"solar_system_id":            map[string]any{"tenant": "stillness", "item_id": "30001001"},
					"loss_type":                  "ship",
					"irrelevant_client_metadata": "kept as evidence only",
				},
			},
		},
		SourceIDs: []string{source.ID},
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertKillmail(context.Background(), model.KillmailRaw{
		ID:           "killmail:stillness:unknown",
		Environment:  model.EnvironmentStillness,
		OccurredAt:   now.Add(-time.Minute),
		KillerTypeID: "999999",
		SourceIDs:    []string{"source:sui:sui-testnet:graphql"},
	}); err != nil {
		t.Fatal(err)
	}

	handler := Server{Store: store}.Handler()
	listReq := httptest.NewRequest(http.MethodGet, "/v1/killmails?environment=stillness", nil)
	listRes := httptest.NewRecorder()
	handler.ServeHTTP(listRes, listReq)
	if listRes.Code != http.StatusOK {
		t.Fatalf("unexpected list status %d body %s", listRes.Code, listRes.Body.String())
	}
	var listBody struct {
		Data []model.SemanticKillmail `json:"data"`
	}
	if err := json.Unmarshal(listRes.Body.Bytes(), &listBody); err != nil {
		t.Fatal(err)
	}
	if len(listBody.Data) != 2 {
		t.Fatalf("expected two semantic killmails, got %#v", listBody.Data)
	}
	if listBody.Data[0].ID != "killmail:stillness:fixture:caird" || listBody.Data[0].Killer.DisplayName != "Caird [NPC]" {
		t.Fatalf("newest semantic killmail was not resolved: %#v", listBody.Data)
	}
	if listBody.Data[1].Killer.EntityType != model.EntityTypeUnknown || len(listBody.Data[1].Warnings) == 0 {
		t.Fatalf("unresolved killmail did not carry semantic warnings: %#v", listBody.Data[1])
	}

	liveReq := httptest.NewRequest(http.MethodGet, "/v1/killmails?environment=stillness&exclude_fixtures=true", nil)
	liveRes := httptest.NewRecorder()
	handler.ServeHTTP(liveRes, liveReq)
	if liveRes.Code != http.StatusOK {
		t.Fatalf("unexpected live list status %d body %s", liveRes.Code, liveRes.Body.String())
	}
	var liveBody struct {
		Data []model.SemanticKillmail `json:"data"`
	}
	if err := json.Unmarshal(liveRes.Body.Bytes(), &liveBody); err != nil {
		t.Fatal(err)
	}
	if len(liveBody.Data) != 1 || liveBody.Data[0].ID != "killmail:stillness:unknown" {
		t.Fatalf("fixture exclusion did not remove fixture-sourced killmails: %#v", liveBody.Data)
	}

	detailReq := httptest.NewRequest(http.MethodGet, "/v1/killmails/killmail:stillness:fixture:caird", nil)
	detailRes := httptest.NewRecorder()
	handler.ServeHTTP(detailRes, detailReq)
	if detailRes.Code != http.StatusOK {
		t.Fatalf("unexpected detail status %d body %s", detailRes.Code, detailRes.Body.String())
	}
	var detailBody struct {
		Data model.SemanticKillmail `json:"data"`
	}
	if err := json.Unmarshal(detailRes.Body.Bytes(), &detailBody); err != nil {
		t.Fatal(err)
	}
	if detailBody.Data.System.DisplayName != "NN0-Y-D5" || detailBody.Data.Victim.DisplayName != "Fixture Victim" || detailBody.Data.Reporter.DisplayName != "Fixture Reporter" {
		t.Fatalf("detail route did not resolve semantic IDs from raw payload: %#v", detailBody.Data)
	}
	if detailBody.Data.LossType != "ship" {
		t.Fatalf("raw payload loss type was not normalised into semantic killmail: %#v", detailBody.Data)
	}

	rawReq := httptest.NewRequest(http.MethodGet, "/v1/killmails/killmail:stillness:fixture:caird/raw", nil)
	rawRes := httptest.NewRecorder()
	handler.ServeHTTP(rawRes, rawReq)
	if rawRes.Code != http.StatusOK {
		t.Fatalf("unexpected raw status %d body %s", rawRes.Code, rawRes.Body.String())
	}
	var rawBody struct {
		Data model.KillmailRaw `json:"data"`
	}
	if err := json.Unmarshal(rawRes.Body.Bytes(), &rawBody); err != nil {
		t.Fatal(err)
	}
	eventPayload, ok := rawBody.Data.Raw["event"].(map[string]any)
	if !ok {
		t.Fatalf("raw evidence payload was not returned: %#v", rawBody.Data.Raw)
	}
	jsonPayload, ok := eventPayload["json"].(map[string]any)
	if !ok || jsonPayload["irrelevant_client_metadata"] != "kept as evidence only" {
		t.Fatalf("raw evidence details were not preserved: %#v", rawBody.Data.Raw)
	}
}

func TestResponseMetaUsesConfiguredRegistryID(t *testing.T) {
	handler := Server{Store: db.NewMemoryStore(), RegistryID: "frontier-community-registry"}.Handler()
	req := httptest.NewRequest(http.MethodGet, "/v1/health", nil)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body %s", res.Code, res.Body.String())
	}
	var body struct {
		Meta model.Meta `json:"meta"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Meta.Registry != "frontier-community-registry" {
		t.Fatalf("expected configured registry id, got %#v", body.Meta)
	}
	if body.Meta.APIVersion != "v1" {
		t.Fatalf("expected default API version, got %#v", body.Meta)
	}
}

func TestAdminImportRequiresBearerToken(t *testing.T) {
	handler := Server{Store: db.NewMemoryStore(), AdminToken: "secret"}.Handler()
	req := httptest.NewRequest(http.MethodPost, "/v1/admin/static-enemies/import", strings.NewReader(`{}`))
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", res.Code)
	}
}

func TestAdminReviewWorkflowCreatesListsPublishesAndRejects(t *testing.T) {
	store := db.NewMemoryStore()
	handler := Server{Store: store, AdminToken: "secret"}.Handler()
	createReq := httptest.NewRequest(http.MethodPost, "/v1/admin/imports", strings.NewReader(`{"targetKind":"source_artefact","targetId":"artefact:fixture","notes":"needs review"}`))
	createReq.Header.Set("Authorization", "Bearer secret")
	createRes := httptest.NewRecorder()
	handler.ServeHTTP(createRes, createReq)
	if createRes.Code != http.StatusCreated {
		t.Fatalf("unexpected create status %d body %s", createRes.Code, createRes.Body.String())
	}
	var createBody struct {
		Data model.Review `json:"data"`
	}
	if err := json.Unmarshal(createRes.Body.Bytes(), &createBody); err != nil {
		t.Fatal(err)
	}
	if createBody.Data.ReviewStatus != model.ReviewStatusCandidate {
		t.Fatalf("unexpected created review %#v", createBody.Data)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/v1/admin/reviews?status=candidate", nil)
	listReq.Header.Set("Authorization", "Bearer secret")
	listRes := httptest.NewRecorder()
	handler.ServeHTTP(listRes, listReq)
	if listRes.Code != http.StatusOK || !strings.Contains(listRes.Body.String(), createBody.Data.ID) {
		t.Fatalf("review list failed status=%d body=%s", listRes.Code, listRes.Body.String())
	}

	publishReq := httptest.NewRequest(http.MethodPost, "/v1/admin/reviews/"+createBody.Data.ID+"/publish", strings.NewReader(`{"reviewer":"tao","notes":"source checked"}`))
	publishReq.Header.Set("Authorization", "Bearer secret")
	publishRes := httptest.NewRecorder()
	handler.ServeHTTP(publishRes, publishReq)
	if publishRes.Code != http.StatusOK {
		t.Fatalf("review publish failed status=%d body=%s", publishRes.Code, publishRes.Body.String())
	}
	var publishBody struct {
		Data model.Review `json:"data"`
	}
	if err := json.Unmarshal(publishRes.Body.Bytes(), &publishBody); err != nil {
		t.Fatal(err)
	}
	if publishBody.Data.ReviewStatus != model.ReviewStatusPublished {
		t.Fatalf("review was not published: %#v", publishBody.Data)
	}

	rejectReq := httptest.NewRequest(http.MethodPost, "/v1/admin/reviews/"+createBody.Data.ID+"/reject", strings.NewReader(`{"reviewer":"tao","notes":"superseded"}`))
	rejectReq.Header.Set("Authorization", "Bearer secret")
	rejectRes := httptest.NewRecorder()
	handler.ServeHTTP(rejectRes, rejectReq)
	if rejectRes.Code != http.StatusOK {
		t.Fatalf("review reject failed status=%d body=%s", rejectRes.Code, rejectRes.Body.String())
	}
	var rejectBody struct {
		Data model.Review `json:"data"`
	}
	if err := json.Unmarshal(rejectRes.Body.Bytes(), &rejectBody); err != nil {
		t.Fatal(err)
	}
	if rejectBody.Data.ReviewStatus != model.ReviewStatusRejected {
		t.Fatalf("review was not rejected: %#v", rejectBody.Data)
	}
}

func TestAdminReviewWorkflowRequiresBearerToken(t *testing.T) {
	handler := Server{Store: db.NewMemoryStore(), AdminToken: "secret"}.Handler()
	req := httptest.NewRequest(http.MethodGet, "/v1/admin/reviews", nil)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", res.Code)
	}
}

func TestAdminRoutesUseConfiguredProductionAuthorizer(t *testing.T) {
	store := db.NewMemoryStore()
	handler := Server{Store: store, AdminAuthorizer: fakeAdminAuthorizer{allow: true}}.Handler()
	req := httptest.NewRequest(http.MethodPost, "/v1/admin/imports", strings.NewReader(`{"targetKind":"source_artefact","targetId":"artefact:fixture"}`))
	req.Header.Set("Cf-Access-Jwt-Assertion", "valid")
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusCreated {
		t.Fatalf("expected configured authorizer to allow admin request, got %d body %s", res.Code, res.Body.String())
	}
}

func TestAdminRoutesRejectFailedProductionAuthorizer(t *testing.T) {
	handler := Server{Store: db.NewMemoryStore(), AdminAuthorizer: fakeAdminAuthorizer{allow: false}}.Handler()
	req := httptest.NewRequest(http.MethodGet, "/v1/admin/reviews", nil)
	req.Header.Set("Cf-Access-Jwt-Assertion", "invalid")
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("expected failed authorizer to reject admin request, got %d body %s", res.Code, res.Body.String())
	}
}

type fakeAdminAuthorizer struct {
	allow bool
}

func (a fakeAdminAuthorizer) AuthorizeAdmin(_ context.Context, _ *http.Request) error {
	if a.allow {
		return nil
	}
	return errFakeUnauthorized
}

var errFakeUnauthorized = &fakeUnauthorizedError{}

type fakeUnauthorizedError struct{}

func (e *fakeUnauthorizedError) Error() string {
	return "unauthorised"
}

func TestEventsEndpointFiltersBySuiModule(t *testing.T) {
	store := db.NewMemoryStore()
	now := time.Date(2026, 6, 24, 10, 0, 0, 0, time.UTC)
	if err := store.UpsertSuiEvent(context.Background(), db.EventRecord{
		ID:          "event:character",
		Kind:        "character.created",
		Environment: model.EnvironmentStillness,
		OccurredAt:  now,
		PackageID:   "0xabc",
		Module:      "character",
		Payload:     map[string]any{"module": "character"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertSuiEvent(context.Background(), db.EventRecord{
		ID:          "event:gate",
		Kind:        "gate.created",
		Environment: model.EnvironmentStillness,
		OccurredAt:  now.Add(-time.Minute),
		PackageID:   "0xabc",
		Module:      "gate",
		Payload:     map[string]any{"module": "gate"},
	}); err != nil {
		t.Fatal(err)
	}
	handler := Server{Store: store}.Handler()
	req := httptest.NewRequest(http.MethodGet, "/v1/events?environment=stillness&module=character", nil)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body %s", res.Code, res.Body.String())
	}
	var body struct {
		Data []db.EventRecord `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Data) != 1 || body.Data[0].ID != "event:character" {
		t.Fatalf("unexpected events %#v", body.Data)
	}
}

func TestEventsEndpointFiltersByCycle(t *testing.T) {
	store := db.NewMemoryStore()
	cycle5 := 5
	cycle6 := 6
	if err := store.UpsertSuiEvent(context.Background(), db.EventRecord{
		ID:          "event:cycle5",
		Kind:        "character.created",
		Environment: model.EnvironmentStillness,
		OccurredAt:  time.Date(2026, 6, 24, 10, 0, 0, 0, time.UTC),
		Cycle:       &cycle5,
		PackageID:   "0xabc",
		Module:      "character",
		Payload:     map[string]any{"cycle": 5},
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertSuiEvent(context.Background(), db.EventRecord{
		ID:          "event:cycle6",
		Kind:        "character.created",
		Environment: model.EnvironmentStillness,
		OccurredAt:  time.Date(2026, 6, 25, 9, 0, 0, 0, time.UTC),
		Cycle:       &cycle6,
		PackageID:   "0xabc",
		Module:      "character",
		Payload:     map[string]any{"cycle": 6},
	}); err != nil {
		t.Fatal(err)
	}
	handler := Server{Store: store}.Handler()
	req := httptest.NewRequest(http.MethodGet, "/v1/events?environment=stillness&cycle=6", nil)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body %s", res.Code, res.Body.String())
	}
	var body struct {
		Data []db.EventRecord `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Data) != 1 || body.Data[0].ID != "event:cycle6" {
		t.Fatalf("unexpected events %#v", body.Data)
	}
}

func TestReadEndpointsDefaultToCurrentCycleAndAllowArchiveOptIn(t *testing.T) {
	store := db.NewMemoryStore()
	ctx := context.Background()
	now := time.Date(2026, 6, 26, 8, 0, 0, 0, time.UTC)
	cycle5 := 5
	cycle6 := 6
	source := model.Source{ID: "source:fixture:cycles", Kind: model.SourceKindSuiEvent, Title: "fixture", Locator: "fixture", Environment: model.EnvironmentStillness, CreatedAt: now}
	store.Sources[source.ID] = source
	for _, entity := range []model.Entity{
		{ID: "tribe:stillness:cycle5", Slug: "tribe-cycle5-stillness", Type: model.EntityTypeTribe, Name: "Cycle 5 Tribe", DisplayName: "Cycle 5 Tribe", Environment: model.EnvironmentStillness, Cycle: &cycle5, UpdatedAt: now.Add(-time.Hour)},
		{ID: "tribe:stillness:cycle6", Slug: "tribe-cycle6-stillness", Type: model.EntityTypeTribe, Name: "Cycle 6 Tribe", DisplayName: "Cycle 6 Tribe", Environment: model.EnvironmentStillness, Cycle: &cycle6, UpdatedAt: now},
		{ID: "tribe:stillness:unlabelled", Slug: "tribe-unlabelled-stillness", Type: model.EntityTypeTribe, Name: "Unlabelled Tribe", DisplayName: "Unlabelled Tribe", Environment: model.EnvironmentStillness, UpdatedAt: now.Add(-time.Minute)},
		{ID: "tribe:utopia:cycle6", Slug: "tribe-cycle6-utopia", Type: model.EntityTypeTribe, Name: "Utopia Tribe", DisplayName: "Utopia Tribe", Environment: model.EnvironmentUtopia, Cycle: &cycle6, UpdatedAt: now.Add(time.Minute)},
	} {
		if err := store.UpsertEntityFacts(ctx, entity, []db.EntityFactDraft{{
			Key:          "metadata_name",
			Value:        entity.DisplayName,
			SourceID:     source.ID,
			Confidence:   model.ConfidenceVerified,
			Environment:  entity.Environment,
			ReviewStatus: model.ReviewStatusReviewed,
		}}); err != nil {
			t.Fatal(err)
		}
	}
	for _, event := range []db.EventRecord{
		{ID: "event:cycle5", Kind: "character.created", Environment: model.EnvironmentStillness, OccurredAt: time.Date(2026, 6, 24, 10, 0, 0, 0, time.UTC), Cycle: &cycle5, Module: "character", Payload: map[string]any{"cycle": 5}},
		{ID: "event:cycle6", Kind: "character.created", Environment: model.EnvironmentStillness, OccurredAt: time.Date(2026, 6, 25, 10, 0, 0, 0, time.UTC), Cycle: &cycle6, Module: "character", Payload: map[string]any{"cycle": 6}},
		{ID: "event:utopia:cycle6", Kind: "character.created", Environment: model.EnvironmentUtopia, OccurredAt: time.Date(2026, 6, 25, 10, 0, 0, 0, time.UTC), Cycle: &cycle6, Module: "character", Payload: map[string]any{"cycle": 6}},
	} {
		if err := store.UpsertSuiEvent(ctx, event); err != nil {
			t.Fatal(err)
		}
	}
	for _, raw := range []model.KillmailRaw{
		{ID: "killmail:stillness:cycle5", Environment: model.EnvironmentStillness, OccurredAt: time.Date(2026, 6, 24, 10, 0, 0, 0, time.UTC), SourceIDs: []string{source.ID}},
		{ID: "killmail:stillness:cycle6", Environment: model.EnvironmentStillness, OccurredAt: time.Date(2026, 6, 25, 10, 0, 0, 0, time.UTC), SourceIDs: []string{source.ID}},
		{ID: "killmail:utopia:cycle6", Environment: model.EnvironmentUtopia, OccurredAt: time.Date(2026, 6, 25, 10, 0, 0, 0, time.UTC), SourceIDs: []string{source.ID}},
	} {
		if err := store.UpsertKillmail(ctx, raw); err != nil {
			t.Fatal(err)
		}
	}
	handler := Server{Store: store}.Handler()

	assertEntityIDs := func(path string, want ...string) {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != http.StatusOK {
			t.Fatalf("%s returned %d body %s", path, res.Code, res.Body.String())
		}
		var body struct {
			Data []model.CurrentEntity `json:"data"`
		}
		if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		got := make([]string, 0, len(body.Data))
		for _, item := range body.Data {
			got = append(got, item.Entity.ID)
		}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("%s returned IDs %#v, want %#v", path, got, want)
		}
	}
	assertEventIDs := func(path string, want ...string) {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != http.StatusOK {
			t.Fatalf("%s returned %d body %s", path, res.Code, res.Body.String())
		}
		var body struct {
			Data []db.EventRecord `json:"data"`
		}
		if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		got := make([]string, 0, len(body.Data))
		for _, item := range body.Data {
			got = append(got, item.ID)
		}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("%s returned IDs %#v, want %#v", path, got, want)
		}
	}
	assertKillmailIDs := func(path string, want ...string) {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != http.StatusOK {
			t.Fatalf("%s returned %d body %s", path, res.Code, res.Body.String())
		}
		var body struct {
			Data []model.SemanticKillmail `json:"data"`
		}
		if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		got := make([]string, 0, len(body.Data))
		for _, item := range body.Data {
			got = append(got, item.ID)
		}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("%s returned IDs %#v, want %#v", path, got, want)
		}
	}

	assertEntityIDs("/v1/current/tribes", "tribe:stillness:cycle6", "tribe:stillness:unlabelled")
	assertEntityIDs("/v1/current/tribes?environment=stillness", "tribe:stillness:cycle6", "tribe:stillness:unlabelled")
	assertEntityIDs("/v1/current/tribes?cycles=current", "tribe:stillness:cycle6")
	assertEntityIDs("/v1/current/tribes?environment=stillness&cycles=6", "tribe:stillness:cycle6")
	assertEntityIDs("/v1/current/tribes?environment=stillness&cycles=5,6", "tribe:stillness:cycle6", "tribe:stillness:cycle5")
	assertEntityIDs("/v1/current/tribes?environment=stillness&cycles=all", "tribe:stillness:cycle6", "tribe:stillness:unlabelled", "tribe:stillness:cycle5")
	assertEventIDs("/v1/events", "event:cycle6")
	assertEventIDs("/v1/events?environment=stillness", "event:cycle6")
	assertEventIDs("/v1/events?cycles=current", "event:cycle6")
	assertEventIDs("/v1/events?environment=stillness&cycles=all", "event:cycle6", "event:cycle5")
	assertKillmailIDs("/v1/killmails", "killmail:stillness:cycle6")
	assertKillmailIDs("/v1/killmails?environment=stillness", "killmail:stillness:cycle6")
	assertKillmailIDs("/v1/killmails?cycles=current", "killmail:stillness:cycle6")
	assertKillmailIDs("/v1/killmails?environment=stillness&cycles=5", "killmail:stillness:cycle5")
	assertKillmailIDs("/v1/killmails?environment=stillness&cycles=all", "killmail:stillness:cycle6", "killmail:stillness:cycle5")
}

func TestOpsSuiCoverageSummarisesCursorHealth(t *testing.T) {
	store := db.NewMemoryStore()
	now := time.Date(2026, 6, 24, 10, 0, 0, 0, time.UTC)
	if err := store.SaveSyncCursor(context.Background(), db.CursorStatus{
		ID:                   "cursor:sui:sui-testnet:events:original:0xabc:character",
		Source:               "sui:sui-testnet:events:original:0xabc:character",
		Environment:          model.EnvironmentStillness,
		CursorValue:          "event-cursor",
		CursorKind:           "sui_event",
		LastSuccessfulIngest: &now,
		EventsProcessed:      50,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveSyncCursor(context.Background(), db.CursorStatus{
		ID:                   "cursor:sui:sui-testnet:objects:original:0xabc::character::PlayerProfile",
		Source:               "sui:sui-testnet:objects:original:0xabc::character::PlayerProfile",
		Environment:          model.EnvironmentStillness,
		CursorValue:          "object-cursor",
		CursorKind:           "sui_object",
		LastSuccessfulIngest: &now,
		EventsProcessed:      3,
		ErrorCount:           1,
		LastErrorSummary:     "context cancelled",
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveSyncCursor(context.Background(), db.CursorStatus{
		ID:               "cursor:sui:sui-testnet:objects:published:0xdef::assembly::Assembly",
		Source:           "sui:sui-testnet:objects:published:0xdef::assembly::Assembly",
		Environment:      model.EnvironmentStillness,
		CursorKind:       "sui_object",
		ErrorCount:       1,
		LastErrorSummary: "sui GraphQL returned errors: Request is outside consistent range",
	}); err != nil {
		t.Fatal(err)
	}

	handler := Server{Store: store}.Handler()
	req := httptest.NewRequest(http.MethodGet, "/v1/ops/sui-coverage", nil)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body %s", res.Code, res.Body.String())
	}
	var body struct {
		Data model.SuiCoverageSummary `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Data.TargetCount != 3 || body.Data.EventTargets != 1 || body.Data.ObjectTargets != 2 {
		t.Fatalf("unexpected coverage counts %#v", body.Data)
	}
	if body.Data.ErroredTargets != 1 || body.Data.IndexedTargets != 1 || body.Data.RangeBlockedTargets != 1 {
		t.Fatalf("unexpected health counts %#v", body.Data)
	}
	if body.Data.FullCoverageProven {
		t.Fatal("cursor coverage API must not claim full historical coverage")
	}
}

func TestCollectionRoutesFilterByEntityType(t *testing.T) {
	store := db.NewMemoryStore()
	now := time.Date(2026, 6, 24, 10, 0, 0, 0, time.UTC)
	store.Entities["character:stillness:2112091476"] = model.Entity{
		ID:          "character:stillness:2112091476",
		Slug:        "character-2112091476-stillness",
		Type:        model.EntityTypeCharacter,
		Name:        "Character 2112091476",
		DisplayName: "Character 2112091476",
		Environment: model.EnvironmentStillness,
		UpdatedAt:   now,
	}
	store.Entities["gate:stillness:100"] = model.Entity{
		ID:          "gate:stillness:100",
		Slug:        "gate-100-stillness",
		Type:        model.EntityTypeGate,
		Name:        "Gate 100",
		DisplayName: "Gate 100",
		Environment: model.EnvironmentStillness,
		UpdatedAt:   now.Add(-time.Minute),
	}
	handler := Server{Store: store}.Handler()

	listReq := httptest.NewRequest(http.MethodGet, "/v1/characters?environment=stillness", nil)
	listRes := httptest.NewRecorder()
	handler.ServeHTTP(listRes, listReq)
	if listRes.Code != http.StatusOK {
		t.Fatalf("unexpected list status %d body %s", listRes.Code, listRes.Body.String())
	}
	var listBody struct {
		Data []model.Entity `json:"data"`
	}
	if err := json.Unmarshal(listRes.Body.Bytes(), &listBody); err != nil {
		t.Fatal(err)
	}
	if len(listBody.Data) != 1 || listBody.Data[0].Type != model.EntityTypeCharacter {
		t.Fatalf("character collection leaked other entity types: %#v", listBody.Data)
	}

	detailReq := httptest.NewRequest(http.MethodGet, "/v1/characters/gate-100-stillness", nil)
	detailRes := httptest.NewRecorder()
	handler.ServeHTTP(detailRes, detailReq)
	if detailRes.Code != http.StatusNotFound {
		t.Fatalf("expected type-mismatched detail lookup to 404, got %d body %s", detailRes.Code, detailRes.Body.String())
	}
}

func TestEntityProvenanceRoutesReturnFactsRelationsSourcesAndHistory(t *testing.T) {
	store := db.NewMemoryStore()
	now := time.Date(2026, 6, 24, 10, 0, 0, 0, time.UTC)
	source := model.Source{
		ID:          "source:sui:sui-testnet:graphql",
		Kind:        model.SourceKindSuiEvent,
		Title:       "Sui testnet GraphQL",
		Locator:     "https://graphql.testnet.sui.io/graphql",
		Environment: model.EnvironmentStillness,
	}
	store.Sources[source.ID] = source
	store.Entities["killmail:stillness:310"] = model.Entity{
		ID:          "killmail:stillness:310",
		Slug:        "killmail-310-stillness",
		Type:        model.EntityTypeKillmail,
		Name:        "Killmail 310",
		DisplayName: "Killmail 310",
		Environment: model.EnvironmentStillness,
		UpdatedAt:   now,
	}
	store.Entities["character:stillness:2112000304"] = model.Entity{
		ID:          "character:stillness:2112000304",
		Slug:        "character-2112000304-stillness",
		Type:        model.EntityTypeCharacter,
		Name:        "Character 2112000304",
		DisplayName: "Character 2112000304",
		Environment: model.EnvironmentStillness,
		UpdatedAt:   now,
	}
	if err := store.UpsertEntityFacts(context.Background(), store.Entities["killmail:stillness:310"], []db.EntityFactDraft{{
		Key:          "loss_type",
		Value:        "ship",
		SourceID:     source.ID,
		Confidence:   model.ConfidenceVerified,
		Environment:  model.EnvironmentStillness,
		ReviewStatus: model.ReviewStatusReviewed,
	}}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertRelations(context.Background(), []db.RelationDraft{{
		SubjectEntityID: "killmail:stillness:310",
		Predicate:       "victim",
		ObjectEntityID:  "character:stillness:2112000304",
		SourceID:        source.ID,
		Confidence:      model.ConfidenceVerified,
		Environment:     model.EnvironmentStillness,
	}}); err != nil {
		t.Fatal(err)
	}
	handler := Server{Store: store}.Handler()

	for _, route := range []string{
		"/v1/entities/killmail-310-stillness/facts",
		"/v1/entities/killmail-310-stillness/relations",
		"/v1/entities/killmail-310-stillness/sources",
		"/v1/entities/killmail-310-stillness/history",
	} {
		req := httptest.NewRequest(http.MethodGet, route, nil)
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != http.StatusOK {
			t.Fatalf("%s returned %d body %s", route, res.Code, res.Body.String())
		}
		if !strings.Contains(res.Body.String(), source.ID) {
			t.Fatalf("%s response did not include source provenance: %s", route, res.Body.String())
		}
	}
}

func TestSearchEndpointUsesEntitySearch(t *testing.T) {
	store := db.NewMemoryStore()
	now := time.Date(2026, 6, 24, 10, 0, 0, 0, time.UTC)
	store.Entities["enemy:stillness:type:92096"] = model.Entity{
		ID:          "enemy:stillness:type:92096",
		Slug:        "enemy-caird-92096-stillness",
		Type:        model.EntityTypeEnemy,
		Name:        "Caird",
		DisplayName: "Caird [NPC]",
		Environment: model.EnvironmentStillness,
		UpdatedAt:   now,
	}
	store.Entities["system:stillness:30001001"] = model.Entity{
		ID:          "system:stillness:30001001",
		Slug:        "system-30001001-stillness",
		Type:        model.EntityTypeSystem,
		Name:        "NN0-Y-D5",
		DisplayName: "NN0-Y-D5",
		Environment: model.EnvironmentStillness,
		UpdatedAt:   now.Add(-time.Minute),
	}
	handler := Server{Store: store}.Handler()
	req := httptest.NewRequest(http.MethodGet, "/v1/search?q=caird&environment=stillness", nil)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body %s", res.Code, res.Body.String())
	}
	var body struct {
		Data []model.Entity `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Data) != 1 || body.Data[0].ID != "enemy:stillness:type:92096" {
		t.Fatalf("unexpected search results %#v", body.Data)
	}
}

func TestTypeEndpointsFilterByStaticTypeFacts(t *testing.T) {
	store := db.NewMemoryStore()
	now := time.Date(2026, 6, 25, 16, 0, 0, 0, time.UTC)
	source := model.Source{ID: "source:static-client:types:stillness", Kind: model.SourceKindStaticClientData, Title: "Static types", Locator: "fixture", Environment: model.EnvironmentStillness}
	store.Sources[source.ID] = source
	for _, item := range []struct {
		entity model.Entity
		facts  []db.EntityFactDraft
	}{
		{
			entity: model.Entity{ID: "enemy:stillness:type:92096", Slug: "enemy-caird-92096-stillness", Type: model.EntityTypeEnemy, Name: "Caird", DisplayName: "Caird [NPC]", Environment: model.EnvironmentStillness, UpdatedAt: now},
			facts: []db.EntityFactDraft{
				{Key: "type_id", Value: 92096, SourceID: source.ID, Confidence: model.ConfidenceProbable, Environment: model.EnvironmentStillness, ReviewStatus: model.ReviewStatusReviewed},
				{Key: "group_id", Value: 5033, SourceID: source.ID, Confidence: model.ConfidenceProbable, Environment: model.EnvironmentStillness, ReviewStatus: model.ReviewStatusReviewed},
				{Key: "category_id", Value: 16, SourceID: source.ID, Confidence: model.ConfidenceProbable, Environment: model.EnvironmentStillness, ReviewStatus: model.ReviewStatusReviewed},
				{Key: "market_group_id", Value: 700, SourceID: source.ID, Confidence: model.ConfidenceProbable, Environment: model.EnvironmentStillness, ReviewStatus: model.ReviewStatusReviewed},
				{Key: "wreck_type_id", Value: 81610, SourceID: source.ID, Confidence: model.ConfidenceProbable, Environment: model.EnvironmentStillness, ReviewStatus: model.ReviewStatusReviewed},
				{Key: "static_entity_type", Value: "enemy", SourceID: source.ID, Confidence: model.ConfidenceProbable, Environment: model.EnvironmentStillness, ReviewStatus: model.ReviewStatusReviewed},
				{Key: "source_artefact_id", Value: "artefact:types", SourceID: source.ID, Confidence: model.ConfidenceProbable, Environment: model.EnvironmentStillness, ReviewStatus: model.ReviewStatusReviewed},
			},
		},
		{
			entity: model.Entity{ID: "item:stillness:type:1001", Slug: "item-reflex-1001-stillness", Type: model.EntityTypeItem, Name: "Reflex", DisplayName: "Reflex", Environment: model.EnvironmentStillness, UpdatedAt: now.Add(-time.Minute)},
			facts: []db.EntityFactDraft{
				{Key: "type_id", Value: 1001, SourceID: source.ID, Confidence: model.ConfidenceVerified, Environment: model.EnvironmentStillness, ReviewStatus: model.ReviewStatusReviewed},
				{Key: "group_id", Value: 4000, SourceID: source.ID, Confidence: model.ConfidenceVerified, Environment: model.EnvironmentStillness, ReviewStatus: model.ReviewStatusReviewed},
				{Key: "static_entity_type", Value: "item", SourceID: source.ID, Confidence: model.ConfidenceVerified, Environment: model.EnvironmentStillness, ReviewStatus: model.ReviewStatusReviewed},
				{Key: "source_artefact_id", Value: "artefact:types", SourceID: source.ID, Confidence: model.ConfidenceVerified, Environment: model.EnvironmentStillness, ReviewStatus: model.ReviewStatusReviewed},
			},
		},
	} {
		if err := store.UpsertEntityFacts(context.Background(), item.entity, item.facts); err != nil {
			t.Fatal(err)
		}
	}
	handler := Server{Store: store}.Handler()

	listReq := httptest.NewRequest(http.MethodGet, "/v1/types?environment=stillness&group_id=5033&static_entity_type=enemy", nil)
	listRes := httptest.NewRecorder()
	handler.ServeHTTP(listRes, listReq)
	if listRes.Code != http.StatusOK {
		t.Fatalf("unexpected type list status %d body %s", listRes.Code, listRes.Body.String())
	}
	var listBody struct {
		Data []model.Entity `json:"data"`
	}
	if err := json.Unmarshal(listRes.Body.Bytes(), &listBody); err != nil {
		t.Fatal(err)
	}
	if len(listBody.Data) != 1 || listBody.Data[0].ID != "enemy:stillness:type:92096" {
		t.Fatalf("type filters returned wrong rows: %#v", listBody.Data)
	}

	entityReq := httptest.NewRequest(http.MethodGet, "/v1/entities?environment=stillness&type_id=1001&source_artefact_id=artefact:types", nil)
	entityRes := httptest.NewRecorder()
	handler.ServeHTTP(entityRes, entityReq)
	if entityRes.Code != http.StatusOK {
		t.Fatalf("unexpected entity filter status %d body %s", entityRes.Code, entityRes.Body.String())
	}
	var entityBody struct {
		Data []model.Entity `json:"data"`
	}
	if err := json.Unmarshal(entityRes.Body.Bytes(), &entityBody); err != nil {
		t.Fatal(err)
	}
	if len(entityBody.Data) != 1 || entityBody.Data[0].ID != "item:stillness:type:1001" {
		t.Fatalf("entity fact filters returned wrong rows: %#v", entityBody.Data)
	}

	detailReq := httptest.NewRequest(http.MethodGet, "/v1/types/92096?environment=stillness", nil)
	detailRes := httptest.NewRecorder()
	handler.ServeHTTP(detailRes, detailReq)
	if detailRes.Code != http.StatusOK {
		t.Fatalf("unexpected type detail status %d body %s", detailRes.Code, detailRes.Body.String())
	}
	var detailBody struct {
		Data model.Entity `json:"data"`
	}
	if err := json.Unmarshal(detailRes.Body.Bytes(), &detailBody); err != nil {
		t.Fatal(err)
	}
	if detailBody.Data.ID != "enemy:stillness:type:92096" {
		t.Fatalf("type detail resolved wrong entity: %#v", detailBody.Data)
	}
}

func TestStaticDomainRoutesExposeEnemiesRecipesAndBlueprints(t *testing.T) {
	store := db.NewMemoryStore()
	now := time.Date(2026, 6, 25, 16, 30, 0, 0, time.UTC)
	source := model.Source{ID: "source:static-client:types:stillness", Kind: model.SourceKindStaticClientData, Title: "Static types", Locator: "fixture", Environment: model.EnvironmentStillness}
	store.Sources[source.ID] = source
	for _, item := range []struct {
		entity model.Entity
		facts  []db.EntityFactDraft
	}{
		{
			entity: model.Entity{ID: "enemy:stillness:type:94167", Slug: "enemy-mycena-94167-stillness", Type: model.EntityTypeEnemy, Name: "Mycena", DisplayName: "Mycena [NPC]", Environment: model.EnvironmentStillness, UpdatedAt: now},
			facts: []db.EntityFactDraft{
				{Key: "type_id", Value: 94167, SourceID: source.ID, Confidence: model.ConfidenceProbable, Environment: model.EnvironmentStillness, ReviewStatus: model.ReviewStatusReviewed},
				{Key: "group_id", Value: 5130, SourceID: source.ID, Confidence: model.ConfidenceProbable, Environment: model.EnvironmentStillness, ReviewStatus: model.ReviewStatusReviewed},
				{Key: "wreck_type_id", Value: 81610, SourceID: source.ID, Confidence: model.ConfidenceProbable, Environment: model.EnvironmentStillness, ReviewStatus: model.ReviewStatusReviewed},
				{Key: "static_entity_type", Value: "enemy", SourceID: source.ID, Confidence: model.ConfidenceProbable, Environment: model.EnvironmentStillness, ReviewStatus: model.ReviewStatusReviewed},
			},
		},
		{
			entity: model.Entity{ID: "recipe:stillness:reflex", Slug: "recipe-reflex-stillness", Type: model.EntityTypeRecipe, Name: "Reflex recipe", DisplayName: "Reflex recipe", Environment: model.EnvironmentStillness, UpdatedAt: now.Add(-time.Minute)},
			facts: []db.EntityFactDraft{
				{Key: "recipe_id", Value: "reflex", SourceID: source.ID, Confidence: model.ConfidenceProbable, Environment: model.EnvironmentStillness, ReviewStatus: model.ReviewStatusReviewed},
				{Key: "static_entity_type", Value: "recipe", SourceID: source.ID, Confidence: model.ConfidenceProbable, Environment: model.EnvironmentStillness, ReviewStatus: model.ReviewStatusReviewed},
			},
		},
		{
			entity: model.Entity{ID: "blueprint:stillness:type:75001", Slug: "blueprint-reflex-75001-stillness", Type: model.EntityTypeBlueprint, Name: "Reflex blueprint", DisplayName: "Reflex blueprint", Environment: model.EnvironmentStillness, UpdatedAt: now.Add(-2 * time.Minute)},
			facts: []db.EntityFactDraft{
				{Key: "type_id", Value: 75001, SourceID: source.ID, Confidence: model.ConfidenceProbable, Environment: model.EnvironmentStillness, ReviewStatus: model.ReviewStatusReviewed},
				{Key: "static_entity_type", Value: "blueprint", SourceID: source.ID, Confidence: model.ConfidenceProbable, Environment: model.EnvironmentStillness, ReviewStatus: model.ReviewStatusReviewed},
			},
		},
	} {
		if err := store.UpsertEntityFacts(context.Background(), item.entity, item.facts); err != nil {
			t.Fatal(err)
		}
	}

	handler := Server{Store: store}.Handler()
	for _, tc := range []struct {
		route      string
		entityType model.EntityType
		id         string
	}{
		{route: "/v1/enemies?environment=stillness&group_id=5130&wreck_type_id=81610", entityType: model.EntityTypeEnemy, id: "enemy:stillness:type:94167"},
		{route: "/v1/current/enemies?environment=stillness&q=mycena", entityType: model.EntityTypeEnemy, id: "enemy:stillness:type:94167"},
		{route: "/v1/recipes?environment=stillness", entityType: model.EntityTypeRecipe, id: "recipe:stillness:reflex"},
		{route: "/v1/current/recipes?environment=stillness", entityType: model.EntityTypeRecipe, id: "recipe:stillness:reflex"},
		{route: "/v1/blueprints?environment=stillness&type_id=75001", entityType: model.EntityTypeBlueprint, id: "blueprint:stillness:type:75001"},
		{route: "/v1/current/blueprints?environment=stillness", entityType: model.EntityTypeBlueprint, id: "blueprint:stillness:type:75001"},
	} {
		req := httptest.NewRequest(http.MethodGet, tc.route, nil)
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != http.StatusOK {
			t.Fatalf("%s returned %d body %s", tc.route, res.Code, res.Body.String())
		}
		var body struct {
			Data []model.Entity `json:"data"`
		}
		if strings.Contains(tc.route, "/current/") {
			var currentBody struct {
				Data []model.CurrentEntity `json:"data"`
			}
			if err := json.Unmarshal(res.Body.Bytes(), &currentBody); err != nil {
				t.Fatal(err)
			}
			if len(currentBody.Data) != 1 || currentBody.Data[0].Entity.ID != tc.id || currentBody.Data[0].Entity.Type != tc.entityType {
				t.Fatalf("%s returned wrong current rows: %#v", tc.route, currentBody.Data)
			}
			continue
		}
		if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if len(body.Data) != 1 || body.Data[0].ID != tc.id || body.Data[0].Type != tc.entityType {
			t.Fatalf("%s returned wrong rows: %#v", tc.route, body.Data)
		}
	}
}

func TestOpsSourceGapsReportsEvidenceThatStillNeedsResolvers(t *testing.T) {
	store := db.NewMemoryStore()
	now := time.Date(2026, 6, 25, 17, 0, 0, 0, time.UTC)
	source := model.Source{ID: "source:sui:stillness:objects", Kind: model.SourceKindSuiObject, Title: "Sui objects", Locator: "fixture", Environment: model.EnvironmentStillness}
	store.Sources[source.ID] = source
	assembly := model.Entity{ID: "assembly:stillness:100", Slug: "assembly-100-stillness", Type: model.EntityTypeAssembly, Name: "Assembly 100", DisplayName: "Assembly 100", Environment: model.EnvironmentStillness, UpdatedAt: now}
	if err := store.UpsertEntityFacts(context.Background(), assembly, []db.EntityFactDraft{
		{Key: "owner_cap_id", Value: "0xowner-cap", SourceID: source.ID, Confidence: model.ConfidenceVerified, Environment: model.EnvironmentStillness, ReviewStatus: model.ReviewStatusReviewed},
		{Key: "location_hash", Value: "0xlocation", SourceID: source.ID, Confidence: model.ConfidenceVerified, Environment: model.EnvironmentStillness, ReviewStatus: model.ReviewStatusReviewed},
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertKillmail(context.Background(), model.KillmailRaw{
		ID:          "killmail:stillness:unresolved",
		Environment: model.EnvironmentStillness,
		OccurredAt:  now,
		SourceIDs:   []string{source.ID},
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertEntityFacts(context.Background(), model.Entity{
		ID:          "tribe:stillness:42",
		Slug:        "tribe-42-stillness",
		Type:        model.EntityTypeTribe,
		Name:        "Tribe 42",
		DisplayName: "Tribe 42",
		Environment: model.EnvironmentStillness,
		UpdatedAt:   now,
	}, []db.EntityFactDraft{{
		Key:          "tribe_id",
		Value:        "42",
		SourceID:     source.ID,
		Confidence:   model.ConfidenceVerified,
		Environment:  model.EnvironmentStillness,
		ReviewStatus: model.ReviewStatusPublished,
	}}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveSyncCursor(context.Background(), db.CursorStatus{
		ID:               "cursor:sui:sui-testnet:objects:range-blocked",
		Source:           "sui:sui-testnet:objects:range-blocked",
		Environment:      model.EnvironmentStillness,
		CursorKind:       "sui_object",
		EventsProcessed:  12,
		ErrorCount:       3,
		LastErrorSummary: "sui GraphQL returned errors: Request is outside consistent range",
		UpdatedAt:        now,
	}); err != nil {
		t.Fatal(err)
	}

	handler := Server{Store: store}.Handler()
	req := httptest.NewRequest(http.MethodGet, "/v1/ops/source-gaps?environment=stillness", nil)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("unexpected source gaps status %d body %s", res.Code, res.Body.String())
	}
	var body struct {
		Data []model.SourceGap `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	seen := make(map[string]model.SourceGap)
	for _, gap := range body.Data {
		seen[gap.ID] = gap
	}
	for _, id := range []string{
		"source-gap:stillness:ownership-evidence-only",
		"source-gap:stillness:location-evidence-only",
		"source-gap:stillness:unresolved-killmail-actors",
		"source-gap:stillness:sui-object-provider-range-blocked",
		"source-gap:stillness:tribe-identity-names",
		"source-gap:stillness:tribe-identity-profiles",
		"source-gap:stillness:static-client-full-table-decoder",
	} {
		if seen[id].Count == 0 {
			t.Fatalf("expected source gap %s in %#v", id, body.Data)
		}
	}
	blocked := seen["source-gap:stillness:sui-object-provider-range-blocked"]
	if blocked.Category != "provider_blocked" {
		t.Fatalf("range-blocked gap should expose provider category, got %#v", blocked)
	}
	if blocked.Severity != "info" {
		t.Fatalf("range-blocked gap should be informational, got %#v", blocked)
	}
	if len(blocked.SuggestedCommands) < 2 || !strings.Contains(blocked.SuggestedCommands[0], "derive-events") {
		t.Fatalf("range-blocked gap should point operators at event-first repair, got %#v", blocked)
	}
	if strings.Contains(strings.Join(blocked.SuggestedCommands, " "), "<alternate-public-sui-graphql-url>") {
		t.Fatalf("range-blocked gap should not default to provider escalation, got %#v", blocked)
	}
}

func TestCurrentCharactersEndpointReturnsFactsRelationsAndSources(t *testing.T) {
	store := db.NewMemoryStore()
	now := time.Date(2026, 6, 24, 10, 0, 0, 0, time.UTC)
	source := model.Source{ID: "source:sui:stillness:objects", Kind: model.SourceKindSuiObject, Title: "Sui objects", Locator: "fixture", Environment: model.EnvironmentStillness}
	otherSource := model.Source{ID: "source:static-client:stillness:types", Kind: model.SourceKindStaticClientData, Title: "Static types", Locator: "fixture", Environment: model.EnvironmentStillness}
	store.Sources[source.ID] = source
	store.Sources[otherSource.ID] = otherSource
	character := model.Entity{
		ID:          "character:stillness:2112091476",
		Slug:        "character-2112091476-stillness",
		Type:        model.EntityTypeCharacter,
		Name:        "Tao",
		DisplayName: "Tao",
		Environment: model.EnvironmentStillness,
		UpdatedAt:   now,
	}
	otherCharacter := model.Entity{
		ID:          "character:stillness:2112099999",
		Slug:        "character-2112099999-stillness",
		Type:        model.EntityTypeCharacter,
		Name:        "Other",
		DisplayName: "Other",
		Environment: model.EnvironmentStillness,
		UpdatedAt:   now.Add(time.Minute),
	}
	tribe := model.Entity{
		ID:          "tribe:stillness:42",
		Slug:        "tribe-42-stillness",
		Type:        model.EntityTypeTribe,
		Name:        "Tribe 42",
		DisplayName: "Tribe 42",
		Environment: model.EnvironmentStillness,
		UpdatedAt:   now,
	}
	if err := store.UpsertEntityFacts(context.Background(), character, []db.EntityFactDraft{{
		Key:          "character_address",
		Value:        "0xabc",
		SourceID:     source.ID,
		Confidence:   model.ConfidenceVerified,
		Environment:  model.EnvironmentStillness,
		ReviewStatus: model.ReviewStatusReviewed,
	}, {
		Key:          "metadata_name",
		Value:        "Tao",
		SourceID:     source.ID,
		Confidence:   model.ConfidenceVerified,
		Environment:  model.EnvironmentStillness,
		ReviewStatus: model.ReviewStatusReviewed,
	}, {
		Key:          "metadata_description",
		Value:        "Public pilot profile",
		SourceID:     source.ID,
		Confidence:   model.ConfidenceVerified,
		Environment:  model.EnvironmentStillness,
		ReviewStatus: model.ReviewStatusReviewed,
	}, {
		Key:          "metadata_url",
		Value:        "https://example.invalid/characters/tao",
		SourceID:     source.ID,
		Confidence:   model.ConfidenceVerified,
		Environment:  model.EnvironmentStillness,
		ReviewStatus: model.ReviewStatusReviewed,
	}}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertEntityFacts(context.Background(), otherCharacter, []db.EntityFactDraft{{
		Key:          "character_address",
		Value:        "0xdef",
		SourceID:     otherSource.ID,
		Confidence:   model.ConfidenceVerified,
		Environment:  model.EnvironmentStillness,
		ReviewStatus: model.ReviewStatusReviewed,
	}}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertEntityFacts(context.Background(), tribe, nil); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertRelations(context.Background(), []db.RelationDraft{{
		SubjectEntityID: character.ID,
		Predicate:       "belongs_to",
		ObjectEntityID:  tribe.ID,
		SourceID:        source.ID,
		Confidence:      model.ConfidenceVerified,
		Environment:     model.EnvironmentStillness,
	}}); err != nil {
		t.Fatal(err)
	}

	handler := Server{Store: store}.Handler()
	req := httptest.NewRequest(http.MethodGet, "/v1/current/characters?environment=stillness&source_id="+source.ID, nil)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body %s", res.Code, res.Body.String())
	}
	var body struct {
		Data []model.CurrentEntity `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Data) != 1 {
		t.Fatalf("expected one current character, got %#v", body.Data)
	}
	if body.Data[0].Facts["character_address"] != "0xabc" {
		t.Fatalf("current facts were not included: %#v", body.Data[0].Facts)
	}
	if len(body.Data[0].OutgoingRelations) != 1 || body.Data[0].OutgoingRelations[0].ObjectEntityID != tribe.ID {
		t.Fatalf("current relations were not included: %#v", body.Data[0].OutgoingRelations)
	}
	if len(body.Data[0].SourceIDs) != 1 || body.Data[0].SourceIDs[0] != source.ID {
		t.Fatalf("current source ids were not included: %#v", body.Data[0].SourceIDs)
	}
	if body.Data[0].Derived == nil || body.Data[0].Derived.Profile == nil {
		t.Fatalf("current profile was not derived from metadata facts: %#v", body.Data[0].Derived)
	}
	if body.Data[0].Derived.Profile.MetadataName != "Tao" ||
		body.Data[0].Derived.Profile.MetadataDescription != "Public pilot profile" ||
		body.Data[0].Derived.Profile.MetadataURL != "https://example.invalid/characters/tao" {
		t.Fatalf("current profile metadata was not exposed: %#v", body.Data[0].Derived.Profile)
	}
}

func TestCurrentOwnershipEndpointReturnsOwnedByEdges(t *testing.T) {
	store := db.NewMemoryStore()
	now := time.Date(2026, 6, 24, 10, 0, 0, 0, time.UTC)
	source := model.Source{ID: "source:sui:stillness:events", Kind: model.SourceKindSuiEvent, Title: "Sui events", Locator: "fixture", Environment: model.EnvironmentStillness}
	otherSource := model.Source{ID: "source:static-client:stillness:types", Kind: model.SourceKindStaticClientData, Title: "Static types", Locator: "fixture", Environment: model.EnvironmentStillness}
	store.Sources[source.ID] = source
	store.Sources[otherSource.ID] = otherSource
	store.Entities["assembly:stillness:100"] = model.Entity{ID: "assembly:stillness:100", Slug: "assembly-100-stillness", Type: model.EntityTypeAssembly, Name: "Assembly 100", DisplayName: "Assembly 100", Environment: model.EnvironmentStillness, UpdatedAt: now}
	store.Entities["assembly:stillness:200"] = model.Entity{ID: "assembly:stillness:200", Slug: "assembly-200-stillness", Type: model.EntityTypeAssembly, Name: "Assembly 200", DisplayName: "Assembly 200", Environment: model.EnvironmentStillness, UpdatedAt: now.Add(time.Minute)}
	store.Entities["character:stillness:2112091476"] = model.Entity{ID: "character:stillness:2112091476", Slug: "character-2112091476-stillness", Type: model.EntityTypeCharacter, Name: "Tao", DisplayName: "Tao", Environment: model.EnvironmentStillness, UpdatedAt: now}
	if err := store.UpsertRelations(context.Background(), []db.RelationDraft{{
		SubjectEntityID: "assembly:stillness:100",
		Predicate:       "owned_by",
		ObjectEntityID:  "character:stillness:2112091476",
		SourceID:        source.ID,
		Confidence:      model.ConfidenceVerified,
		Environment:     model.EnvironmentStillness,
	}, {
		SubjectEntityID: "assembly:stillness:200",
		Predicate:       "owned_by",
		ObjectEntityID:  "character:stillness:2112091476",
		SourceID:        otherSource.ID,
		Confidence:      model.ConfidenceVerified,
		Environment:     model.EnvironmentStillness,
	}}); err != nil {
		t.Fatal(err)
	}

	handler := Server{Store: store}.Handler()
	req := httptest.NewRequest(http.MethodGet, "/v1/current/ownership?environment=stillness&source_id="+source.ID, nil)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body %s", res.Code, res.Body.String())
	}
	var body struct {
		Data []model.CurrentRelation `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Data) != 1 || body.Data[0].Predicate != "owned_by" {
		t.Fatalf("unexpected ownership edges %#v", body.Data)
	}
	if body.Data[0].SubjectDisplayName != "Assembly 100" || body.Data[0].ObjectDisplayName != "Tao" {
		t.Fatalf("edge display names were not resolved: %#v", body.Data[0])
	}
}

func TestCurrentDomainEndpointsExposeCoreNormalisedRecords(t *testing.T) {
	store := db.NewMemoryStore()
	now := time.Date(2026, 6, 24, 10, 0, 0, 0, time.UTC)
	source := model.Source{ID: "source:sui:stillness:current", Kind: model.SourceKindSuiObject, Title: "Sui current state", Locator: "fixture", Environment: model.EnvironmentStillness}
	store.Sources[source.ID] = source
	entities := []model.Entity{
		{ID: "character:stillness:2112091476", Slug: "character-2112091476-stillness", Type: model.EntityTypeCharacter, Name: "Tao", DisplayName: "Tao", Environment: model.EnvironmentStillness, UpdatedAt: now.Add(8 * time.Minute)},
		{ID: "tribe:stillness:42", Slug: "tribe-42-stillness", Type: model.EntityTypeTribe, Name: "Black Relay", DisplayName: "Black Relay", Environment: model.EnvironmentStillness, UpdatedAt: now.Add(7 * time.Minute)},
		{ID: "assembly:stillness:100", Slug: "assembly-100-stillness", Type: model.EntityTypeAssembly, Name: "Assembly 100", DisplayName: "Assembly 100", Environment: model.EnvironmentStillness, UpdatedAt: now.Add(6 * time.Minute)},
		{ID: "gate:stillness:200", Slug: "gate-200-stillness", Type: model.EntityTypeGate, Name: "Gate 200", DisplayName: "Gate 200", Environment: model.EnvironmentStillness, UpdatedAt: now.Add(5 * time.Minute)},
		{ID: "storage:stillness:300", Slug: "storage-300-stillness", Type: model.EntityTypeStorage, Name: "Storage 300", DisplayName: "Storage 300", Environment: model.EnvironmentStillness, UpdatedAt: now.Add(4 * time.Minute)},
		{ID: "turret:stillness:400", Slug: "turret-400-stillness", Type: model.EntityTypeTurret, Name: "Turret 400", DisplayName: "Turret 400", Environment: model.EnvironmentStillness, UpdatedAt: now.Add(3 * time.Minute)},
		{ID: "resource_object:stillness:owner-cap:0xcap", Slug: "resource-object-owner-cap-0xcap-stillness", Type: model.EntityTypeResourceObject, Name: "Owner capability 0xcap", DisplayName: "Owner capability 0xcap", Environment: model.EnvironmentStillness, UpdatedAt: now.Add(2800 * time.Millisecond)},
		{ID: "resource_object:stillness:location-hash:loc-1", Slug: "resource-object-location-hash-loc-1-stillness", Type: model.EntityTypeResourceObject, Name: "Location hash loc-1", DisplayName: "Location hash loc-1", Environment: model.EnvironmentStillness, UpdatedAt: now.Add(2775 * time.Millisecond)},
		{ID: "region:stillness:10000001", Slug: "region-10000001-stillness", Type: model.EntityTypeRegion, Name: "Inner Realm", DisplayName: "Inner Realm", Environment: model.EnvironmentStillness, UpdatedAt: now.Add(2750 * time.Millisecond)},
		{ID: "constellation:stillness:20000068", Slug: "constellation-20000068-stillness", Type: model.EntityTypeConstellation, Name: "Inner First", DisplayName: "Inner First", Environment: model.EnvironmentStillness, UpdatedAt: now.Add(2500 * time.Millisecond)},
		{ID: "item:stillness:type:1001", Slug: "item-reflex-1001-stillness", Type: model.EntityTypeItem, Name: "Reflex", DisplayName: "Reflex", Environment: model.EnvironmentStillness, UpdatedAt: now.Add(2250 * time.Millisecond)},
		{ID: "material:stillness:type:1004", Slug: "material-hydrated-sulfide-matrix-1004-stillness", Type: model.EntityTypeMaterial, Name: "Hydrated Sulfide Matrix", DisplayName: "Hydrated Sulfide Matrix", Environment: model.EnvironmentStillness, UpdatedAt: now.Add(2200 * time.Millisecond)},
		{ID: "ship:stillness:type:1002", Slug: "ship-rifter-frame-1002-stillness", Type: model.EntityTypeShip, Name: "Rifter Frame", DisplayName: "Rifter Frame", Environment: model.EnvironmentStillness, UpdatedAt: now.Add(2150 * time.Millisecond)},
		{ID: "structure:stillness:type:1003", Slug: "structure-frontier-gate-structure-1003-stillness", Type: model.EntityTypeStructure, Name: "Frontier Gate Structure", DisplayName: "Frontier Gate Structure", Environment: model.EnvironmentStillness, UpdatedAt: now.Add(2100 * time.Millisecond)},
		{ID: "system:stillness:30001001", Slug: "system-30001001-stillness", Type: model.EntityTypeSystem, Name: "NN0-Y-D5", DisplayName: "NN0-Y-D5", Environment: model.EnvironmentStillness, UpdatedAt: now.Add(2 * time.Minute)},
		{ID: "system:stillness:30001002", Slug: "system-30001002-stillness", Type: model.EntityTypeSystem, Name: "6RG-Y-T4", DisplayName: "6RG-Y-T4", Environment: model.EnvironmentStillness, UpdatedAt: now.Add(time.Minute)},
		{ID: "route:stillness:nn0-y-d5-to-6rg-y-t4", Slug: "route-nn0-y-d5-to-6rg-y-t4-stillness", Type: model.EntityTypeRoute, Name: "NN0-Y-D5 to 6RG-Y-T4", DisplayName: "NN0-Y-D5 to 6RG-Y-T4", Environment: model.EnvironmentStillness, UpdatedAt: now},
	}
	for _, entity := range entities {
		if err := store.UpsertEntityFacts(context.Background(), entity, []db.EntityFactDraft{{
			Key:          "metadata_name",
			Value:        entity.DisplayName,
			SourceID:     source.ID,
			Confidence:   model.ConfidenceVerified,
			Environment:  model.EnvironmentStillness,
			ReviewStatus: model.ReviewStatusReviewed,
		}}); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.UpsertRelations(context.Background(), []db.RelationDraft{
		{SubjectEntityID: "assembly:stillness:100", Predicate: "owned_by", ObjectEntityID: "character:stillness:2112091476", SourceID: source.ID, Confidence: model.ConfidenceVerified, Environment: model.EnvironmentStillness},
		{SubjectEntityID: "assembly:stillness:100", Predicate: "has_owner_cap", ObjectEntityID: "resource_object:stillness:owner-cap:0xcap", SourceID: source.ID, Confidence: model.ConfidenceVerified, Environment: model.EnvironmentStillness},
		{SubjectEntityID: "assembly:stillness:100", Predicate: "has_location_hash", ObjectEntityID: "resource_object:stillness:location-hash:loc-1", SourceID: source.ID, Confidence: model.ConfidenceVerified, Environment: model.EnvironmentStillness},
		{SubjectEntityID: "system:stillness:30001001", Predicate: "links_to", ObjectEntityID: "system:stillness:30001002", SourceID: source.ID, Confidence: model.ConfidenceVerified, Environment: model.EnvironmentStillness},
		{SubjectEntityID: "route:stillness:nn0-y-d5-to-6rg-y-t4", Predicate: "observed_between", ObjectEntityID: "system:stillness:30001001", SourceID: source.ID, Confidence: model.ConfidenceProbable, Environment: model.EnvironmentStillness},
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertEntityFacts(context.Background(), store.Entities["assembly:stillness:100"], []db.EntityFactDraft{
		{Key: "owner_cap_id", Value: "0xcap", SourceID: source.ID, Confidence: model.ConfidenceVerified, Environment: model.EnvironmentStillness, ReviewStatus: model.ReviewStatusReviewed},
		{Key: "location_hash", Value: "loc-1", SourceID: source.ID, Confidence: model.ConfidenceVerified, Environment: model.EnvironmentStillness, ReviewStatus: model.ReviewStatusReviewed},
		{Key: "metadata_description", Value: "Public assembly profile", SourceID: source.ID, Confidence: model.ConfidenceVerified, Environment: model.EnvironmentStillness, ReviewStatus: model.ReviewStatusReviewed},
		{Key: "metadata_url", Value: "https://example.invalid/assemblies/100", SourceID: source.ID, Confidence: model.ConfidenceVerified, Environment: model.EnvironmentStillness, ReviewStatus: model.ReviewStatusReviewed},
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertEntityFacts(context.Background(), store.Entities["tribe:stillness:42"], []db.EntityFactDraft{
		{Key: "tag", Value: "BR", SourceID: source.ID, Confidence: model.ConfidenceReported, Environment: model.EnvironmentStillness, ReviewStatus: model.ReviewStatusReviewed},
		{Key: "aliases", Value: []string{"Relay"}, SourceID: source.ID, Confidence: model.ConfidenceReported, Environment: model.EnvironmentStillness, ReviewStatus: model.ReviewStatusReviewed},
		{Key: "description", Value: "Reviewed public tribe profile", SourceID: source.ID, Confidence: model.ConfidenceReported, Environment: model.EnvironmentStillness, ReviewStatus: model.ReviewStatusReviewed},
		{Key: "url", Value: "https://example.invalid/tribes/black-relay", SourceID: source.ID, Confidence: model.ConfidenceReported, Environment: model.EnvironmentStillness, ReviewStatus: model.ReviewStatusReviewed},
	}); err != nil {
		t.Fatal(err)
	}

	handler := Server{Store: store}.Handler()
	for _, tc := range []struct {
		route      string
		entityType model.EntityType
		id         string
	}{
		{route: "/v1/current/characters?environment=stillness", entityType: model.EntityTypeCharacter, id: "character:stillness:2112091476"},
		{route: "/v1/current/tribes?environment=stillness", entityType: model.EntityTypeTribe, id: "tribe:stillness:42"},
		{route: "/v1/current/assemblies?environment=stillness", entityType: model.EntityTypeAssembly, id: "assembly:stillness:100"},
		{route: "/v1/current/gates?environment=stillness", entityType: model.EntityTypeGate, id: "gate:stillness:200"},
		{route: "/v1/current/storage?environment=stillness", entityType: model.EntityTypeStorage, id: "storage:stillness:300"},
		{route: "/v1/current/turrets?environment=stillness", entityType: model.EntityTypeTurret, id: "turret:stillness:400"},
		{route: "/v1/current/regions?environment=stillness", entityType: model.EntityTypeRegion, id: "region:stillness:10000001"},
		{route: "/v1/current/constellations?environment=stillness", entityType: model.EntityTypeConstellation, id: "constellation:stillness:20000068"},
		{route: "/v1/current/items?environment=stillness", entityType: model.EntityTypeItem, id: "item:stillness:type:1001"},
		{route: "/v1/current/materials?environment=stillness", entityType: model.EntityTypeMaterial, id: "material:stillness:type:1004"},
		{route: "/v1/current/ships?environment=stillness", entityType: model.EntityTypeShip, id: "ship:stillness:type:1002"},
		{route: "/v1/current/structures?environment=stillness", entityType: model.EntityTypeStructure, id: "structure:stillness:type:1003"},
		{route: "/v1/current/routes?environment=stillness", entityType: model.EntityTypeRoute, id: "route:stillness:nn0-y-d5-to-6rg-y-t4"},
	} {
		req := httptest.NewRequest(http.MethodGet, tc.route, nil)
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != http.StatusOK {
			t.Fatalf("%s returned %d body %s", tc.route, res.Code, res.Body.String())
		}
		var body struct {
			Data []model.CurrentEntity `json:"data"`
		}
		if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if len(body.Data) != 1 || body.Data[0].Entity.ID != tc.id || body.Data[0].Entity.Type != tc.entityType {
			t.Fatalf("%s returned wrong current entity set: %#v", tc.route, body.Data)
		}
		if body.Data[0].Facts["metadata_name"] != body.Data[0].Entity.DisplayName {
			t.Fatalf("%s did not include normalised facts: %#v", tc.route, body.Data[0])
		}
		if len(body.Data[0].SourceIDs) == 0 || body.Data[0].SourceIDs[0] != source.ID {
			t.Fatalf("%s did not include provenance: %#v", tc.route, body.Data[0].SourceIDs)
		}
		if tc.id == "tribe:stillness:42" {
			if body.Data[0].Derived == nil || body.Data[0].Derived.Profile == nil {
				t.Fatalf("%s did not expose reviewed tribe profile: %#v", tc.route, body.Data[0].Derived)
			}
			profile := body.Data[0].Derived.Profile
			if profile.Tag != "BR" || profile.Description != "Reviewed public tribe profile" || profile.URL != "https://example.invalid/tribes/black-relay" || len(profile.Aliases) != 1 || profile.Aliases[0] != "Relay" {
				t.Fatalf("%s exposed wrong tribe profile: %#v", tc.route, profile)
			}
		}
		if tc.id == "assembly:stillness:100" {
			if body.Data[0].Derived == nil || body.Data[0].Derived.Profile == nil {
				t.Fatalf("%s did not expose assembly metadata profile: %#v", tc.route, body.Data[0].Derived)
			}
			profile := body.Data[0].Derived.Profile
			if profile.MetadataDescription != "Public assembly profile" || profile.MetadataURL != "https://example.invalid/assemblies/100" {
				t.Fatalf("%s exposed wrong assembly profile: %#v", tc.route, profile)
			}
		}
	}

	systemsReq := httptest.NewRequest(http.MethodGet, "/v1/current/systems?environment=stillness", nil)
	systemsRes := httptest.NewRecorder()
	handler.ServeHTTP(systemsRes, systemsReq)
	if systemsRes.Code != http.StatusOK {
		t.Fatalf("systems endpoint returned %d body %s", systemsRes.Code, systemsRes.Body.String())
	}
	var systemsBody struct {
		Data []model.CurrentEntity `json:"data"`
	}
	if err := json.Unmarshal(systemsRes.Body.Bytes(), &systemsBody); err != nil {
		t.Fatal(err)
	}
	if len(systemsBody.Data) != 2 {
		t.Fatalf("expected both current systems, got %#v", systemsBody.Data)
	}

	evidenceReq := httptest.NewRequest(http.MethodGet, "/v1/current/assemblies?environment=stillness&owner_cap=0xcap&location_hash=loc-1", nil)
	evidenceRes := httptest.NewRecorder()
	handler.ServeHTTP(evidenceRes, evidenceReq)
	if evidenceRes.Code != http.StatusOK {
		t.Fatalf("evidence-filtered assemblies endpoint returned %d body %s", evidenceRes.Code, evidenceRes.Body.String())
	}
	var evidenceBody struct {
		Data []model.CurrentEntity `json:"data"`
	}
	if err := json.Unmarshal(evidenceRes.Body.Bytes(), &evidenceBody); err != nil {
		t.Fatal(err)
	}
	if len(evidenceBody.Data) != 1 || evidenceBody.Data[0].Entity.ID != "assembly:stillness:100" {
		t.Fatalf("evidence filters returned wrong assembly set: %#v", evidenceBody.Data)
	}
	if evidenceBody.Data[0].Derived == nil || evidenceBody.Data[0].Derived.OwnerCap == nil || evidenceBody.Data[0].Derived.LocationHash == nil {
		t.Fatalf("evidence-only derived state was not exposed: %#v", evidenceBody.Data[0].Derived)
	}

	routeEdgesReq := httptest.NewRequest(http.MethodGet, "/v1/current/route-edges?environment=stillness", nil)
	routeEdgesRes := httptest.NewRecorder()
	handler.ServeHTTP(routeEdgesRes, routeEdgesReq)
	if routeEdgesRes.Code != http.StatusOK {
		t.Fatalf("route edges endpoint returned %d body %s", routeEdgesRes.Code, routeEdgesRes.Body.String())
	}
	var edgesBody struct {
		Data []model.CurrentRelation `json:"data"`
	}
	if err := json.Unmarshal(routeEdgesRes.Body.Bytes(), &edgesBody); err != nil {
		t.Fatal(err)
	}
	seenPredicates := make(map[string]model.CurrentRelation)
	for _, edge := range edgesBody.Data {
		seenPredicates[edge.Predicate] = edge
	}
	if len(seenPredicates) != 2 {
		t.Fatalf("expected links_to and observed_between route edges, got %#v", edgesBody.Data)
	}
	if seenPredicates["links_to"].SubjectDisplayName != "NN0-Y-D5" || seenPredicates["links_to"].ObjectDisplayName != "6RG-Y-T4" {
		t.Fatalf("links_to edge names were not resolved: %#v", seenPredicates["links_to"])
	}
	if seenPredicates["observed_between"].SubjectEntityType != model.EntityTypeRoute || seenPredicates["observed_between"].ObjectEntityType != model.EntityTypeSystem {
		t.Fatalf("observed_between edge types were not normalised: %#v", seenPredicates["observed_between"])
	}
}

func TestCurrentEndpointsFilterByProfileAndEvidenceState(t *testing.T) {
	store := db.NewMemoryStore()
	now := time.Date(2026, 6, 26, 16, 0, 0, 0, time.UTC)
	source := model.Source{ID: "source:sui:sui-testnet:graphql", Kind: model.SourceKindSuiObject, Title: "Sui objects", Locator: "fixture", Environment: model.EnvironmentStillness, CreatedAt: now}
	store.Sources[source.ID] = source
	for _, entity := range []model.Entity{
		{ID: "character:stillness:2112091476", Slug: "character-2112091476-stillness", Type: model.EntityTypeCharacter, Name: "FC Jotunn", DisplayName: "FC Jotunn", Environment: model.EnvironmentStillness, UpdatedAt: now.Add(6 * time.Minute)},
		{ID: "character:stillness:42", Slug: "character-42-stillness", Type: model.EntityTypeCharacter, Name: "Character 42", DisplayName: "Character 42", Environment: model.EnvironmentStillness, UpdatedAt: now.Add(5 * time.Minute)},
		{ID: "tribe:stillness:99", Slug: "tribe-99-stillness", Type: model.EntityTypeTribe, Name: "Tribe 99", DisplayName: "Tribe 99", Environment: model.EnvironmentStillness, UpdatedAt: now.Add(4 * time.Minute)},
		{ID: "assembly:stillness:100", Slug: "assembly-100-stillness", Type: model.EntityTypeAssembly, Name: "Assembly 100", DisplayName: "Assembly 100", Environment: model.EnvironmentStillness, UpdatedAt: now.Add(3 * time.Minute)},
		{ID: "assembly:stillness:200", Slug: "assembly-200-stillness", Type: model.EntityTypeAssembly, Name: "Assembly 200", DisplayName: "Assembly 200", Environment: model.EnvironmentStillness, UpdatedAt: now.Add(2 * time.Minute)},
		{ID: "system:stillness:30001001", Slug: "system-30001001-stillness", Type: model.EntityTypeSystem, Name: "NN0-Y-D5", DisplayName: "NN0-Y-D5", Environment: model.EnvironmentStillness, UpdatedAt: now.Add(time.Minute)},
		{ID: "resource_object:stillness:owner-cap:0xcap", Slug: "owner-cap-0xcap", Type: model.EntityTypeResourceObject, Name: "Owner capability 0xcap", DisplayName: "Owner capability 0xcap", Environment: model.EnvironmentStillness, UpdatedAt: now},
		{ID: "resource_object:stillness:location-hash:loc-1", Slug: "location-hash-loc-1", Type: model.EntityTypeResourceObject, Name: "Location hash loc-1", DisplayName: "Location hash loc-1", Environment: model.EnvironmentStillness, UpdatedAt: now},
	} {
		if err := store.UpsertEntityFacts(context.Background(), entity, nil); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.UpsertEntityFacts(context.Background(), store.Entities["character:stillness:2112091476"], []db.EntityFactDraft{
		{Key: "metadata_name", Value: "FC Jotunn", SourceID: source.ID, Confidence: model.ConfidenceVerified, Environment: model.EnvironmentStillness, ReviewStatus: model.ReviewStatusReviewed},
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertEntityFacts(context.Background(), store.Entities["character:stillness:42"], []db.EntityFactDraft{
		{Key: "character_id", Value: "42", SourceID: source.ID, Confidence: model.ConfidenceVerified, Environment: model.EnvironmentStillness, ReviewStatus: model.ReviewStatusReviewed},
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertEntityFacts(context.Background(), store.Entities["assembly:stillness:100"], []db.EntityFactDraft{
		{Key: "owner_cap_id", Value: "0xcap", SourceID: source.ID, Confidence: model.ConfidenceVerified, Environment: model.EnvironmentStillness, ReviewStatus: model.ReviewStatusReviewed},
		{Key: "location_hash", Value: "loc-1", SourceID: source.ID, Confidence: model.ConfidenceVerified, Environment: model.EnvironmentStillness, ReviewStatus: model.ReviewStatusReviewed},
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertRelations(context.Background(), []db.RelationDraft{
		{SubjectEntityID: "character:stillness:2112091476", Predicate: "belongs_to", ObjectEntityID: "tribe:stillness:99", SourceID: source.ID, Confidence: model.ConfidenceVerified, Environment: model.EnvironmentStillness},
		{SubjectEntityID: "assembly:stillness:100", Predicate: "has_owner_cap", ObjectEntityID: "resource_object:stillness:owner-cap:0xcap", SourceID: source.ID, Confidence: model.ConfidenceVerified, Environment: model.EnvironmentStillness},
		{SubjectEntityID: "assembly:stillness:100", Predicate: "has_location_hash", ObjectEntityID: "resource_object:stillness:location-hash:loc-1", SourceID: source.ID, Confidence: model.ConfidenceVerified, Environment: model.EnvironmentStillness},
		{SubjectEntityID: "assembly:stillness:100", Predicate: "owned_by", ObjectEntityID: "character:stillness:2112091476", SourceID: source.ID, Confidence: model.ConfidenceVerified, Environment: model.EnvironmentStillness},
		{SubjectEntityID: "assembly:stillness:100", Predicate: "located_in", ObjectEntityID: "system:stillness:30001001", SourceID: source.ID, Confidence: model.ConfidenceVerified, Environment: model.EnvironmentStillness},
	}); err != nil {
		t.Fatal(err)
	}

	handler := Server{Store: store}.Handler()
	for _, tc := range []struct {
		route string
		want  string
	}{
		{route: "/v1/current/characters?environment=stillness&profile=known", want: "character:stillness:2112091476"},
		{route: "/v1/current/characters?environment=stillness&profile=placeholder", want: "character:stillness:42"},
		{route: "/v1/current/characters?environment=stillness&has_tribe=true", want: "character:stillness:2112091476"},
		{route: "/v1/current/characters?environment=stillness&has_tribe=false", want: "character:stillness:42"},
		{route: "/v1/current/assemblies?environment=stillness&has_owner_cap=true&has_location_hash=true&has_resolved_owner=true&has_resolved_system=true", want: "assembly:stillness:100"},
		{route: "/v1/current/assemblies?environment=stillness&has_owner_cap=false&has_location_hash=false&has_resolved_owner=false&has_resolved_system=false", want: "assembly:stillness:200"},
	} {
		req := httptest.NewRequest(http.MethodGet, tc.route, nil)
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != http.StatusOK {
			t.Fatalf("%s returned %d body %s", tc.route, res.Code, res.Body.String())
		}
		var body struct {
			Data []model.CurrentEntity `json:"data"`
		}
		if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if len(body.Data) != 1 || body.Data[0].Entity.ID != tc.want {
			t.Fatalf("%s returned wrong current rows: %#v", tc.route, body.Data)
		}
	}
}

func TestCurrentTribesEndpointOrdersReviewedProfilesBeforePlaceholders(t *testing.T) {
	store := db.NewMemoryStore()
	ctx := context.Background()
	now := time.Date(2026, 6, 26, 8, 0, 0, 0, time.UTC)
	source := model.Source{ID: "source:tribe-identities:stillness", Kind: model.SourceKindCommunityReport, Title: "Reviewed tribes", Locator: "fixture", Environment: model.EnvironmentStillness}
	store.Sources[source.ID] = source
	reviewed := model.Entity{
		ID:          "tribe:stillness:42",
		Slug:        "tribe-42-stillness",
		Type:        model.EntityTypeTribe,
		Name:        "Black Relay",
		DisplayName: "Black Relay",
		Environment: model.EnvironmentStillness,
		UpdatedAt:   now.Add(-time.Hour),
	}
	placeholder := model.Entity{
		ID:          "tribe:stillness:99",
		Slug:        "tribe-99-stillness",
		Type:        model.EntityTypeTribe,
		Name:        "Tribe 99",
		DisplayName: "Tribe 99",
		Environment: model.EnvironmentStillness,
		UpdatedAt:   now,
	}
	if err := store.UpsertEntityFacts(ctx, reviewed, []db.EntityFactDraft{{
		Key:          "display_name",
		Value:        "Black Relay",
		SourceID:     source.ID,
		Confidence:   model.ConfidenceReported,
		Environment:  model.EnvironmentStillness,
		ReviewStatus: model.ReviewStatusReviewed,
	}}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertEntityFacts(ctx, placeholder, []db.EntityFactDraft{{
		Key:          "tribe_id",
		Value:        "99",
		SourceID:     "source:sui:stillness:events",
		Confidence:   model.ConfidenceVerified,
		Environment:  model.EnvironmentStillness,
		ReviewStatus: model.ReviewStatusPublished,
	}}); err != nil {
		t.Fatal(err)
	}

	handler := Server{Store: store}.Handler()
	req := httptest.NewRequest(http.MethodGet, "/v1/current/tribes?environment=stillness", nil)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body %s", res.Code, res.Body.String())
	}
	var body struct {
		Data []model.CurrentEntity `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Data) != 2 {
		t.Fatalf("expected two tribe rows, got %#v", body.Data)
	}
	if body.Data[0].Entity.ID != reviewed.ID {
		t.Fatalf("reviewed tribe should sort before placeholder tribe: %#v", body.Data)
	}
}
