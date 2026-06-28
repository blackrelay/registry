package db

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/blackrelay/registry/internal/model"
)

func TestMemoryStorePreservesReviewedTribeDisplayWhenPlaceholderArrives(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()
	reviewed := model.Entity{
		ID:          "tribe:stillness:42",
		Slug:        "tribe-42-stillness",
		Type:        model.EntityTypeTribe,
		Name:        "Example Relay",
		DisplayName: "Example Relay",
		Summary:     "Reviewed public tribe identity.",
		Environment: model.EnvironmentStillness,
	}
	if err := store.UpsertEntityFacts(ctx, reviewed, []EntityFactDraft{{
		Key:          "display_name",
		Value:        "Example Relay",
		SourceID:     "source:tribe-identities:stillness",
		Confidence:   model.ConfidenceReported,
		Environment:  model.EnvironmentStillness,
		ReviewStatus: model.ReviewStatusReviewed,
	}}); err != nil {
		t.Fatalf("insert reviewed tribe identity: %v", err)
	}

	placeholder := model.Entity{
		ID:          "tribe:stillness:42",
		Slug:        "tribe-42-stillness",
		Type:        model.EntityTypeTribe,
		Name:        "Tribe 42",
		DisplayName: "Tribe 42",
		Summary:     "Public on-chain tribe identity observed from Sui event data.",
		Environment: model.EnvironmentStillness,
	}
	if err := store.UpsertEntityFacts(ctx, placeholder, []EntityFactDraft{{
		Key:          "tribe_id",
		Value:        "42",
		SourceID:     "source:sui:stillness:events",
		Confidence:   model.ConfidenceVerified,
		Environment:  model.EnvironmentStillness,
		ReviewStatus: model.ReviewStatusPublished,
	}}); err != nil {
		t.Fatalf("insert chain placeholder tribe identity: %v", err)
	}

	got, ok, err := store.GetEntity(ctx, reviewed.ID)
	if err != nil {
		t.Fatalf("GetEntity returned error: %v", err)
	}
	if !ok {
		t.Fatal("tribe entity missing")
	}
	if got.Name != "Example Relay" || got.DisplayName != "Example Relay" {
		t.Fatalf("placeholder overwrote reviewed display: %#v", got)
	}
}

func TestMemoryStorePreservesImportedSystemDisplayWhenPlaceholderArrives(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()
	imported := model.Entity{
		ID:          "system:stillness:30016868",
		Slug:        "system-30016868-stillness",
		Type:        model.EntityTypeSystem,
		Name:        "I8V-PCH",
		DisplayName: "I8V-PCH",
		Summary:     "Static-client solar system metadata.",
		Environment: model.EnvironmentStillness,
	}
	if err := store.UpsertEntityFacts(ctx, imported, []EntityFactDraft{{
		Key:          "system_id",
		Value:        "30016868",
		SourceID:     "source:static-client:universe:stillness",
		Confidence:   model.ConfidenceVerified,
		Environment:  model.EnvironmentStillness,
		ReviewStatus: model.ReviewStatusReviewed,
	}}); err != nil {
		t.Fatalf("insert imported system identity: %v", err)
	}

	placeholder := model.Entity{
		ID:          "system:stillness:30016868",
		Slug:        "system-30016868-stillness",
		Type:        model.EntityTypeSystem,
		Name:        "System 30016868",
		DisplayName: "System 30016868",
		Summary:     "Public on-chain solar system reference observed from Sui object data.",
		Environment: model.EnvironmentStillness,
	}
	if err := store.UpsertEntityFacts(ctx, placeholder, []EntityFactDraft{{
		Key:          "item_id",
		Value:        "30016868",
		SourceID:     "source:sui:stillness:objects",
		Confidence:   model.ConfidenceVerified,
		Environment:  model.EnvironmentStillness,
		ReviewStatus: model.ReviewStatusReviewed,
	}}); err != nil {
		t.Fatalf("insert chain placeholder system identity: %v", err)
	}

	got, ok, err := store.GetEntity(ctx, imported.ID)
	if err != nil {
		t.Fatalf("GetEntity returned error: %v", err)
	}
	if !ok {
		t.Fatal("system entity missing")
	}
	if got.Name != "I8V-PCH" || got.DisplayName != "I8V-PCH" {
		t.Fatalf("placeholder overwrote imported system display: %#v", got)
	}
}

func TestMemoryStorePreservesImportedItemDisplayWhenStaticTypePlaceholderArrives(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()
	imported := model.Entity{
		ID:          "item:stillness:type:3001",
		Slug:        "item-char-3001-stillness",
		Type:        model.EntityTypeItem,
		Name:        "Char",
		DisplayName: "Char",
		Summary:     "Static-client type metadata.",
		Environment: model.EnvironmentStillness,
	}
	if err := store.UpsertEntityFacts(ctx, imported, []EntityFactDraft{{
		Key:          "type_id",
		Value:        3001,
		SourceID:     "source:static-client:types:stillness",
		Confidence:   model.ConfidenceVerified,
		Environment:  model.EnvironmentStillness,
		ReviewStatus: model.ReviewStatusReviewed,
	}}); err != nil {
		t.Fatalf("insert imported item identity: %v", err)
	}

	placeholder := model.Entity{
		ID:          "item:stillness:type:3001",
		Slug:        "item-input-type-3001-stillness",
		Type:        model.EntityTypeItem,
		Name:        "Input type 3001",
		DisplayName: "Input type 3001",
		Summary:     "Static-client type placeholder from recipe metadata, type 3001.",
		Environment: model.EnvironmentStillness,
	}
	if err := store.UpsertEntityFacts(ctx, placeholder, []EntityFactDraft{{
		Key:          "recipe_quantity",
		Value:        20,
		SourceID:     "source:static-client:recipes:stillness",
		Confidence:   model.ConfidenceVerified,
		Environment:  model.EnvironmentStillness,
		ReviewStatus: model.ReviewStatusReviewed,
	}}); err != nil {
		t.Fatalf("insert recipe placeholder item identity: %v", err)
	}

	got, ok, err := store.GetEntity(ctx, imported.ID)
	if err != nil {
		t.Fatalf("GetEntity returned error: %v", err)
	}
	if !ok {
		t.Fatal("item entity missing")
	}
	if got.Name != "Char" || got.DisplayName != "Char" {
		t.Fatalf("placeholder overwrote imported item display: %#v", got)
	}
}

func TestMemoryStorePreservesImportedCharacterDisplayWhenPlaceholderArrives(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()
	imported := model.Entity{
		ID:          "character:stillness:2112091476",
		Slug:        "character-2112091476-stillness",
		Type:        model.EntityTypeCharacter,
		Name:        "FC Jotunn",
		DisplayName: "FC Jotunn",
		Summary:     "Public character metadata imported from chain evidence.",
		Environment: model.EnvironmentStillness,
	}
	if err := store.UpsertEntityFacts(ctx, imported, []EntityFactDraft{{
		Key:          "metadata_name",
		Value:        "FC Jotunn",
		SourceID:     "source:sui:stillness:objects",
		Confidence:   model.ConfidenceVerified,
		Environment:  model.EnvironmentStillness,
		ReviewStatus: model.ReviewStatusReviewed,
	}}); err != nil {
		t.Fatalf("insert imported character identity: %v", err)
	}

	placeholder := model.Entity{
		ID:          "character:stillness:2112091476",
		Slug:        "character-2112091476-stillness",
		Type:        model.EntityTypeCharacter,
		Name:        "Character 2112091476",
		DisplayName: "Character 2112091476",
		Summary:     "Public on-chain character identity observed from Sui event data.",
		Environment: model.EnvironmentStillness,
	}
	if err := store.UpsertEntityFacts(ctx, placeholder, []EntityFactDraft{{
		Key:          "character_id",
		Value:        "2112091476",
		SourceID:     "source:sui:stillness:events",
		Confidence:   model.ConfidenceVerified,
		Environment:  model.EnvironmentStillness,
		ReviewStatus: model.ReviewStatusReviewed,
	}}); err != nil {
		t.Fatalf("insert chain placeholder character identity: %v", err)
	}

	got, ok, err := store.GetEntity(ctx, imported.ID)
	if err != nil {
		t.Fatalf("GetEntity returned error: %v", err)
	}
	if !ok {
		t.Fatal("character entity missing")
	}
	if got.Name != "FC Jotunn" || got.DisplayName != "FC Jotunn" {
		t.Fatalf("placeholder overwrote imported character display: %#v", got)
	}
}

func TestMemoryStoreSourceGapsCountsPlaceholderTribeNames(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()
	if err := store.UpsertEntityFacts(ctx, model.Entity{
		ID:          "tribe:stillness:42",
		Slug:        "tribe-42-stillness",
		Type:        model.EntityTypeTribe,
		Name:        "Tribe 42",
		DisplayName: "Tribe 42",
		Environment: model.EnvironmentStillness,
	}, []EntityFactDraft{{
		Key:          "tribe_id",
		Value:        "42",
		SourceID:     "source:sui:stillness:events",
		Confidence:   model.ConfidenceVerified,
		Environment:  model.EnvironmentStillness,
		ReviewStatus: model.ReviewStatusPublished,
	}}); err != nil {
		t.Fatal(err)
	}

	gaps, err := store.ListSourceGaps(ctx, model.EnvironmentStillness)
	if err != nil {
		t.Fatal(err)
	}
	for _, gap := range gaps {
		if gap.ID == "source-gap:stillness:tribe-identity-names" {
			if gap.Count != 1 {
				t.Fatalf("expected one placeholder tribe name gap, got %#v", gap)
			}
			return
		}
	}
	t.Fatalf("missing tribe identity name source gap in %#v", gaps)
}

func TestSourceGapsSuggestLivePublicWorldAPITribeHost(t *testing.T) {
	gaps := sourceGapRows(model.EnvironmentStillness, 0, 0, 0, 1, 0, 1, 1)
	for _, gap := range gaps {
		if gap.Kind != "tribe_identity_names" && gap.Kind != "tribe_identity_profiles" {
			continue
		}
		for _, command := range gap.SuggestedCommands {
			if strings.Contains(command, "world-api-stillness.live.tech.evefrontier.com") {
				t.Fatalf("tribe source gap suggests dead World API host: %s", command)
			}
			if strings.Contains(command, "world-tribes") && !strings.Contains(command, "world-api-stillness.live.pub.evefrontier.com") {
				t.Fatalf("tribe source gap does not suggest the verified public World API host: %s", command)
			}
		}
	}
}

func TestMemoryStoreSourceGapsCountsTribeProfileGaps(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()
	if err := store.UpsertEntityFacts(ctx, model.Entity{
		ID:          "tribe:stillness:42",
		Slug:        "tribe-42-stillness",
		Type:        model.EntityTypeTribe,
		Name:        "Example Relay",
		DisplayName: "Example Relay",
		Environment: model.EnvironmentStillness,
	}, []EntityFactDraft{{
		Key:          "display_name",
		Value:        "Example Relay",
		SourceID:     "source:tribe-identities:stillness",
		Confidence:   model.ConfidenceReported,
		Environment:  model.EnvironmentStillness,
		ReviewStatus: model.ReviewStatusReviewed,
	}}); err != nil {
		t.Fatal(err)
	}

	gaps, err := store.ListSourceGaps(ctx, model.EnvironmentStillness)
	if err != nil {
		t.Fatal(err)
	}
	for _, gap := range gaps {
		if gap.ID == "source-gap:stillness:tribe-identity-profiles" {
			if gap.Count != 1 {
				t.Fatalf("expected one tribe profile gap, got %#v", gap)
			}
			return
		}
	}
	t.Fatalf("missing tribe identity profile source gap in %#v", gaps)
}

func TestMemoryStoreListSuiObjectsFiltersByObservedCycle(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()
	for _, object := range []SuiObjectRecord{
		{
			ID:          "object:cycle5",
			ObjectID:    "0x5",
			Environment: model.EnvironmentStillness,
			TypeRepr:    "0xabc::character::PlayerProfile",
			ObservedAt:  time.Date(2026, 6, 24, 10, 0, 0, 0, time.UTC),
		},
		{
			ID:          "object:cycle6",
			ObjectID:    "0x6",
			Environment: model.EnvironmentStillness,
			TypeRepr:    "0xabc::character::PlayerProfile",
			ObservedAt:  time.Date(2026, 6, 25, 10, 0, 0, 0, time.UTC),
		},
		{
			ID:          "object:unlabelled",
			ObjectID:    "0xnil",
			Environment: model.EnvironmentStillness,
			TypeRepr:    "0xabc::character::PlayerProfile",
		},
	} {
		if err := store.UpsertSuiObject(ctx, object); err != nil {
			t.Fatal(err)
		}
	}

	page, err := store.ListSuiObjects(ctx, SuiObjectQuery{
		Environment:     model.EnvironmentStillness,
		Cycles:          []int{6},
		IncludeUncycled: true,
		Limit:           10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := objectIDs(page.Items); strings.Join(got, ",") != "object:unlabelled,object:cycle6" {
		t.Fatalf("unexpected default-compatible object scope %v", got)
	}

	page, err = store.ListSuiObjects(ctx, SuiObjectQuery{
		Environment: model.EnvironmentStillness,
		Cycles:      []int{6},
		Limit:       10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := objectIDs(page.Items); strings.Join(got, ",") != "object:cycle6" {
		t.Fatalf("unexpected strict object scope %v", got)
	}
}

func objectIDs(items []SuiObjectRecord) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.ID)
	}
	return out
}
