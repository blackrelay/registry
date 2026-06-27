package sui

import (
	"context"
	"errors"
	"testing"

	"github.com/blackrelay/registry/internal/db"
	"github.com/blackrelay/registry/internal/model"
)

func TestEventStreamTargetsIncludeOriginalPublishedModuleShards(t *testing.T) {
	manifest := testManifest()
	targets, err := EventStreamTargets(manifest, TargetOptions{Environment: model.EnvironmentStillness, Network: "sui-testnet"})
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 4 {
		t.Fatalf("expected 4 targets, got %d: %#v", len(targets), targets)
	}
	found := map[string]bool{}
	for _, target := range targets {
		found[string(target.Role)+":"+target.PackageID+":"+target.ModuleName] = true
	}
	if !found["original:"+testPackageID+":character"] {
		t.Fatal("missing original character shard")
	}
	if !found["published:0xd2fd1224f881e7a705dbc211888af11655c315f2ee0f03fe680fc3176e6e4780:gate"] {
		t.Fatal("missing published gate shard")
	}
}

func TestEventStreamTargetsUseManifestStartingCheckpoint(t *testing.T) {
	manifest := testManifest()
	manifest.Packages[0].StartingCheckpoint = 352596413
	targets, err := EventStreamTargets(manifest, TargetOptions{
		Environment:    model.EnvironmentStillness,
		Network:        "sui-testnet",
		PackageIDs:     []string{testPackageID},
		ModuleNames:    []string{"character"},
		NoModuleShards: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 1 {
		t.Fatalf("expected one target, got %d: %#v", len(targets), targets)
	}
	if targets[0].CheckpointAfter == nil || *targets[0].CheckpointAfter != 352596412 {
		t.Fatalf("manifest starting checkpoint was not applied: %#v", targets[0])
	}
	if got := EventCursorSource(targets[0]); got != "sui:sui-testnet:events:original:"+testPackageID+":*:352596412:*" {
		t.Fatalf("checkpoint-scoped cursor source was not used: %s", got)
	}
}

func TestEventStreamTargetsExplicitCheckpointOverridesManifestStartingCheckpoint(t *testing.T) {
	manifest := testManifest()
	manifest.Packages[0].StartingCheckpoint = 352596413
	targets, err := EventStreamTargets(manifest, TargetOptions{
		Environment:    model.EnvironmentStillness,
		Network:        "sui-testnet",
		PackageIDs:     []string{testPackageID},
		ModuleNames:    []string{"character"},
		CheckpointFrom: 400,
		CheckpointTo:   500,
		NoModuleShards: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 1 {
		t.Fatalf("expected one target, got %d: %#v", len(targets), targets)
	}
	if targets[0].CheckpointAfter == nil || *targets[0].CheckpointAfter != 399 {
		t.Fatalf("explicit checkpoint-from did not override manifest default: %#v", targets[0])
	}
	if targets[0].CheckpointBefore == nil || *targets[0].CheckpointBefore != 501 {
		t.Fatalf("checkpoint-to was not applied to package target: %#v", targets[0])
	}
}

func TestEventStreamTargetsDeduplicateSameOriginalAndPublishedPackage(t *testing.T) {
	manifest := testManifest()
	manifest.Packages[0].PublishedPackageID = manifest.Packages[0].OriginalPackageID
	targets, err := EventStreamTargets(manifest, TargetOptions{
		Environment:    model.EnvironmentStillness,
		Network:        "sui-testnet",
		PackageIDs:     []string{testPackageID},
		NoModuleShards: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 1 {
		t.Fatalf("expected a single package-level target, got %d: %#v", len(targets), targets)
	}
	if targets[0].Role != PackageRoleOriginal {
		t.Fatalf("unexpected role for deduplicated target %#v", targets[0])
	}
}

func TestEventStreamTargetsFilterManifestPackagesByCycle(t *testing.T) {
	cycle5 := 5
	cycle6 := 6
	manifest := Manifest{
		SchemaVersion: "registry.sui-packages.v1",
		Packages: []PackageManifest{
			{
				Name:               "cycle-5-world",
				Network:            "sui-testnet",
				OriginalPackageID:  testPackageID,
				PublishedPackageID: testPackageID,
				Cycle:              &cycle5,
				Modules:            []string{"character"},
			},
			{
				Name:               "cycle-6-world",
				Network:            "sui-testnet",
				OriginalPackageID:  "0x8b8a46ed766fa1358ce7c5c51f6a164b13d627a63e45343f69ed0ba0446c1aa1",
				PublishedPackageID: "0x8b8a46ed766fa1358ce7c5c51f6a164b13d627a63e45343f69ed0ba0446c1aa1",
				Cycle:              &cycle6,
				Modules:            []string{"character"},
			},
		},
	}
	targets, err := EventStreamTargets(manifest, TargetOptions{
		Environment:    model.EnvironmentStillness,
		Network:        "sui-testnet",
		Cycles:         []int{6},
		NoModuleShards: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 1 || targets[0].PackageName != "cycle-6-world" {
		t.Fatalf("unexpected cycle-filtered targets %#v", targets)
	}
	targets, err = EventStreamTargets(manifest, TargetOptions{
		Environment:    model.EnvironmentStillness,
		Network:        "sui-testnet",
		Cycles:         []int{5, 6},
		NoModuleShards: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 2 {
		t.Fatalf("expected both archive and current targets, got %#v", targets)
	}
}

func TestEventStreamTargetsShardExplicitEventTypesByCheckpointRange(t *testing.T) {
	eventType := testPackageID + "::fuel::FuelEvent"
	targets, err := EventStreamTargets(testManifest(), TargetOptions{
		Environment:         model.EnvironmentStillness,
		Network:             "sui-testnet",
		EventTypes:          []string{eventType},
		CheckpointFrom:      0,
		CheckpointTo:        250,
		CheckpointShardSize: 100,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 3 {
		t.Fatalf("expected 3 checkpoint shards, got %d: %#v", len(targets), targets)
	}
	if targets[0].EventType != eventType || targets[0].CheckpointAfter != nil || targets[0].CheckpointBefore == nil || *targets[0].CheckpointBefore != 100 {
		t.Fatalf("unexpected first shard %#v", targets[0])
	}
	if targets[1].CheckpointAfter == nil || *targets[1].CheckpointAfter != 99 || targets[1].CheckpointBefore == nil || *targets[1].CheckpointBefore != 200 {
		t.Fatalf("unexpected middle shard %#v", targets[1])
	}
	if targets[2].CheckpointAfter == nil || *targets[2].CheckpointAfter != 199 || targets[2].CheckpointBefore == nil || *targets[2].CheckpointBefore != 251 {
		t.Fatalf("unexpected last shard %#v", targets[2])
	}
}

func TestObjectTypeTargetsIncludeManifestObjectTypes(t *testing.T) {
	manifest := testManifest()
	targets, err := ObjectTypeTargets(manifest, ObjectTargetOptions{Environment: model.EnvironmentStillness, Network: "sui-testnet"})
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 4 {
		t.Fatalf("expected 4 targets, got %d: %#v", len(targets), targets)
	}
	found := map[string]bool{}
	for _, target := range targets {
		found[string(target.Role)+":"+target.TypeRepr] = true
	}
	if !found["original:"+testPackageID+"::character::PlayerProfile"] {
		t.Fatal("missing original PlayerProfile object target")
	}
	if !found["published:0xd2fd1224f881e7a705dbc211888af11655c315f2ee0f03fe680fc3176e6e4780::gate::Gate"] {
		t.Fatal("missing published Gate object target")
	}
}

func TestObjectTypeTargetsDeduplicateSameOriginalAndPublishedPackage(t *testing.T) {
	manifest := testManifest()
	manifest.Packages[0].PublishedPackageID = manifest.Packages[0].OriginalPackageID
	targets, err := ObjectTypeTargets(manifest, ObjectTargetOptions{
		Environment: model.EnvironmentStillness,
		Network:     "sui-testnet",
		PackageIDs:  []string{testPackageID},
		TypeNames:   []string{"Gate"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 1 {
		t.Fatalf("expected a single object target, got %d: %#v", len(targets), targets)
	}
	if targets[0].Role != PackageRoleOriginal || targets[0].TypeName != "Gate" {
		t.Fatalf("unexpected target %#v", targets[0])
	}
}

func TestRunEventBackfillResumesFromSavedCursor(t *testing.T) {
	store := db.NewMemoryStore()
	target := EventStreamTarget{
		Environment: model.EnvironmentStillness,
		Network:     "sui-testnet",
		PackageName: "world",
		PackageID:   testPackageID,
		Role:        PackageRoleOriginal,
		ModuleName:  "character",
	}
	if err := store.SaveSyncCursor(context.Background(), db.CursorStatus{
		ID:              EventCursorID(target),
		Source:          EventCursorSource(target),
		Environment:     model.EnvironmentStillness,
		CursorValue:     "saved-cursor",
		CursorKind:      "sui_event",
		EventsProcessed: 5,
	}); err != nil {
		t.Fatal(err)
	}
	fetcher := &recordingFetcher{
		pages: []EventsPage{{
			Nodes: []MoveEventNode{testMoveEventNode()},
		}},
	}
	summary, err := RunEventBackfill(context.Background(), store, fetcher, Manifest{
		SchemaVersion: "registry.sui-packages.v1",
		Packages: []PackageManifest{{
			Name:               "world",
			Network:            "sui-testnet",
			OriginalPackageID:  testPackageID,
			PublishedPackageID: "0xd2fd1224f881e7a705dbc211888af11655c315f2ee0f03fe680fc3176e6e4780",
			Modules:            []string{"character"},
		}},
	}, BackfillOptions{
		Environment: model.EnvironmentStillness,
		Network:     "sui-testnet",
		Endpoint:    "https://graphql.testnet.sui.io/graphql",
		First:       50,
		MaxPages:    1,
		Concurrency: 1,
		PackageIDs:  []string{testPackageID},
		ModuleNames: []string{"character"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(fetcher.queries) != 1 {
		t.Fatalf("expected 1 query, got %d", len(fetcher.queries))
	}
	if fetcher.queries[0].After != "saved-cursor" {
		t.Fatalf("expected saved cursor, got %q", fetcher.queries[0].After)
	}
	if fetcher.queries[0].EventType != "" {
		t.Fatalf("unexpected event type query %#v", fetcher.queries[0])
	}
	if summary.EventsProcessed != 1 {
		t.Fatalf("expected 1 processed event, got %d", summary.EventsProcessed)
	}
	if _, ok := store.Events["event:7YCpwNPBFJrd6APq6ModDosVUcCmSqsCSB9X76zzm3ch:2"]; !ok {
		t.Fatal("stored event not found")
	}
	cursor, ok, err := store.GetSyncCursor(context.Background(), EventCursorID(target))
	if err != nil || !ok {
		t.Fatalf("cursor not saved: ok=%v err=%v", ok, err)
	}
	if cursor.EventsProcessed != 6 {
		t.Fatalf("expected cumulative event count 6, got %d", cursor.EventsProcessed)
	}
}

func TestRunEventBackfillOnlyIncompleteSkipsCleanSavedCursors(t *testing.T) {
	store := db.NewMemoryStore()
	cleanTarget := EventStreamTarget{
		Environment: model.EnvironmentStillness,
		Network:     "sui-testnet",
		PackageName: "world",
		PackageID:   testPackageID,
		Role:        PackageRoleOriginal,
		ModuleName:  "character",
	}
	if err := store.SaveSyncCursor(context.Background(), db.CursorStatus{
		ID:              EventCursorID(cleanTarget),
		Source:          EventCursorSource(cleanTarget),
		Environment:     model.EnvironmentStillness,
		CursorValue:     "clean-cursor",
		CursorKind:      "sui_event",
		EventsProcessed: 5,
		ErrorCount:      1,
	}); err != nil {
		t.Fatal(err)
	}
	fetcher := &recordingFetcher{
		pages: []EventsPage{{
			Nodes: []MoveEventNode{testMoveEventNode()},
		}},
	}
	summary, err := RunEventBackfill(context.Background(), store, fetcher, Manifest{
		SchemaVersion: "registry.sui-packages.v1",
		Packages: []PackageManifest{{
			Name:               "world",
			Network:            "sui-testnet",
			OriginalPackageID:  testPackageID,
			PublishedPackageID: "0xd2fd1224f881e7a705dbc211888af11655c315f2ee0f03fe680fc3176e6e4780",
			Modules:            []string{"character", "gate"},
		}},
	}, BackfillOptions{
		Environment:    model.EnvironmentStillness,
		Network:        "sui-testnet",
		Endpoint:       "https://graphql.testnet.sui.io/graphql",
		First:          50,
		MaxPages:       1,
		Concurrency:    1,
		PackageIDs:     []string{testPackageID},
		OnlyIncomplete: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(fetcher.queries) != 1 {
		t.Fatalf("expected only the missing gate stream to be fetched, got %d queries: %#v", len(fetcher.queries), fetcher.queries)
	}
	if fetcher.queries[0].ModuleName != "gate" {
		t.Fatalf("unexpected fetched module %#v", fetcher.queries[0])
	}
	if summary.Targets != 1 || summary.SkippedTargets != 1 {
		t.Fatalf("unexpected target counts %#v", summary)
	}
}

func TestRunObjectBackfillResumesFromSavedCursor(t *testing.T) {
	store := db.NewMemoryStore()
	target := ObjectTypeTarget{
		Environment: model.EnvironmentStillness,
		Network:     "sui-testnet",
		PackageName: "world",
		PackageID:   testPackageID,
		Role:        PackageRoleOriginal,
		ModuleName:  "character",
		TypeName:    "PlayerProfile",
		TypeRepr:    testPackageID + "::character::PlayerProfile",
	}
	if err := store.SaveSyncCursor(context.Background(), db.CursorStatus{
		ID:              ObjectCursorID(target),
		Source:          ObjectCursorSource(target),
		Environment:     model.EnvironmentStillness,
		CursorValue:     "saved-cursor",
		CursorKind:      "sui_object",
		EventsProcessed: 5,
	}); err != nil {
		t.Fatal(err)
	}
	fetcher := &recordingObjectFetcher{
		pages: []ObjectsPage{{
			Nodes: []MoveObjectNode{testMoveObjectNode()},
		}},
	}
	summary, err := RunObjectBackfill(context.Background(), store, fetcher, Manifest{
		SchemaVersion: "registry.sui-packages.v1",
		Packages: []PackageManifest{{
			Name:               "world",
			Network:            "sui-testnet",
			OriginalPackageID:  testPackageID,
			PublishedPackageID: "0xd2fd1224f881e7a705dbc211888af11655c315f2ee0f03fe680fc3176e6e4780",
			Modules:            []string{"character"},
			ObjectTypes:        []ObjectTypeManifest{{ModuleName: "character", TypeName: "PlayerProfile"}},
		}},
	}, ObjectBackfillOptions{
		Environment: model.EnvironmentStillness,
		Network:     "sui-testnet",
		Endpoint:    "https://graphql.testnet.sui.io/graphql",
		First:       50,
		MaxPages:    1,
		Concurrency: 1,
		PackageIDs:  []string{testPackageID},
		ModuleNames: []string{"character"},
		TypeNames:   []string{"PlayerProfile"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(fetcher.queries) != 1 {
		t.Fatalf("expected 1 query, got %d", len(fetcher.queries))
	}
	if fetcher.queries[0].After != "saved-cursor" {
		t.Fatalf("expected saved cursor, got %q", fetcher.queries[0].After)
	}
	if summary.ObjectsProcessed != 1 {
		t.Fatalf("expected 1 processed object, got %d", summary.ObjectsProcessed)
	}
	if _, ok := store.Objects["object:0x59714bcd14f03bd20794bd3b5a2a52a0045e75e1bc9cc78aada8c56847e5731c:7"]; !ok {
		t.Fatal("stored object not found")
	}
	cursor, ok, err := store.GetSyncCursor(context.Background(), ObjectCursorID(target))
	if err != nil || !ok {
		t.Fatalf("cursor not saved: ok=%v err=%v", ok, err)
	}
	if cursor.EventsProcessed != 6 {
		t.Fatalf("expected cumulative object count 6, got %d", cursor.EventsProcessed)
	}
}

func TestRunObjectBackfillOnlyIncompleteSkipsCleanSavedCursors(t *testing.T) {
	store := db.NewMemoryStore()
	cleanTarget := ObjectTypeTarget{
		Environment: model.EnvironmentStillness,
		Network:     "sui-testnet",
		PackageName: "world",
		PackageID:   testPackageID,
		Role:        PackageRoleOriginal,
		ModuleName:  "character",
		TypeName:    "PlayerProfile",
		TypeRepr:    testPackageID + "::character::PlayerProfile",
	}
	if err := store.SaveSyncCursor(context.Background(), db.CursorStatus{
		ID:              ObjectCursorID(cleanTarget),
		Source:          ObjectCursorSource(cleanTarget),
		Environment:     model.EnvironmentStillness,
		CursorValue:     "clean-cursor",
		CursorKind:      "sui_object",
		EventsProcessed: 5,
		ErrorCount:      1,
	}); err != nil {
		t.Fatal(err)
	}
	fetcher := &recordingObjectFetcher{
		pages: []ObjectsPage{{
			Nodes: []MoveObjectNode{testMoveObjectNode()},
		}},
	}
	summary, err := RunObjectBackfill(context.Background(), store, fetcher, Manifest{
		SchemaVersion: "registry.sui-packages.v1",
		Packages: []PackageManifest{{
			Name:               "world",
			Network:            "sui-testnet",
			OriginalPackageID:  testPackageID,
			PublishedPackageID: "0xd2fd1224f881e7a705dbc211888af11655c315f2ee0f03fe680fc3176e6e4780",
			Modules:            []string{"character", "gate"},
			ObjectTypes: []ObjectTypeManifest{
				{ModuleName: "character", TypeName: "PlayerProfile"},
				{ModuleName: "gate", TypeName: "Gate"},
			},
		}},
	}, ObjectBackfillOptions{
		Environment:    model.EnvironmentStillness,
		Network:        "sui-testnet",
		Endpoint:       "https://graphql.testnet.sui.io/graphql",
		First:          50,
		MaxPages:       1,
		Concurrency:    1,
		PackageIDs:     []string{testPackageID},
		OnlyIncomplete: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(fetcher.queries) != 1 {
		t.Fatalf("expected only the missing gate object stream to be fetched, got %d queries: %#v", len(fetcher.queries), fetcher.queries)
	}
	if fetcher.queries[0].Type != testPackageID+"::gate::Gate" {
		t.Fatalf("unexpected fetched object type %#v", fetcher.queries[0])
	}
	if summary.Targets != 1 || summary.SkippedTargets != 1 {
		t.Fatalf("unexpected target counts %#v", summary)
	}
}

func TestRunObjectBackfillOnlyIncompleteSkipsProviderRangeBlockedCursors(t *testing.T) {
	store := db.NewMemoryStore()
	blockedTarget := ObjectTypeTarget{
		Environment: model.EnvironmentStillness,
		Network:     "sui-testnet",
		PackageName: "world",
		PackageID:   testPackageID,
		Role:        PackageRoleOriginal,
		ModuleName:  "character",
		TypeName:    "PlayerProfile",
		TypeRepr:    testPackageID + "::character::PlayerProfile",
	}
	if err := store.SaveSyncCursor(context.Background(), db.CursorStatus{
		ID:               ObjectCursorID(blockedTarget),
		Source:           ObjectCursorSource(blockedTarget),
		Environment:      model.EnvironmentStillness,
		CursorKind:       "sui_object",
		ErrorCount:       1,
		LastErrorSummary: "sui GraphQL returned errors: Request is outside consistent range",
	}); err != nil {
		t.Fatal(err)
	}
	fetcher := &recordingObjectFetcher{
		pages: []ObjectsPage{{
			Nodes: []MoveObjectNode{testMoveObjectNode()},
		}},
	}
	summary, err := RunObjectBackfill(context.Background(), store, fetcher, Manifest{
		SchemaVersion: "registry.sui-packages.v1",
		Packages: []PackageManifest{{
			Name:               "world",
			Network:            "sui-testnet",
			OriginalPackageID:  testPackageID,
			PublishedPackageID: testPackageID,
			Modules:            []string{"character", "gate"},
			ObjectTypes: []ObjectTypeManifest{
				{ModuleName: "character", TypeName: "PlayerProfile"},
				{ModuleName: "gate", TypeName: "Gate"},
			},
		}},
	}, ObjectBackfillOptions{
		Environment:    model.EnvironmentStillness,
		Network:        "sui-testnet",
		Endpoint:       "https://graphql.testnet.sui.io/graphql",
		First:          50,
		MaxPages:       1,
		Concurrency:    1,
		PackageIDs:     []string{testPackageID},
		OnlyIncomplete: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(fetcher.queries) != 1 {
		t.Fatalf("expected only the missing gate object stream to be fetched, got %d queries: %#v", len(fetcher.queries), fetcher.queries)
	}
	if fetcher.queries[0].Type != testPackageID+"::gate::Gate" {
		t.Fatalf("unexpected fetched object type %#v", fetcher.queries[0])
	}
	if summary.Targets != 1 || summary.SkippedTargets != 1 {
		t.Fatalf("unexpected target counts %#v", summary)
	}
}

func TestRunObjectBackfillAcceptsExplicitTargetsForRangeBlockedRetry(t *testing.T) {
	store := db.NewMemoryStore()
	blockedTarget := ObjectTypeTarget{
		Environment: model.EnvironmentStillness,
		Network:     "sui-testnet",
		PackageName: "world",
		PackageID:   testPackageID,
		Role:        PackageRoleOriginal,
		ModuleName:  "character",
		TypeName:    "PlayerProfile",
		TypeRepr:    testPackageID + "::character::PlayerProfile",
	}
	if err := store.SaveSyncCursor(context.Background(), db.CursorStatus{
		ID:               ObjectCursorID(blockedTarget),
		Source:           ObjectCursorSource(blockedTarget),
		Environment:      model.EnvironmentStillness,
		CursorKind:       "sui_object",
		ErrorCount:       1,
		LastErrorSummary: "sui GraphQL returned errors: Request is outside consistent range",
	}); err != nil {
		t.Fatal(err)
	}
	fetcher := &recordingObjectFetcher{
		pages: []ObjectsPage{{
			Nodes: []MoveObjectNode{testMoveObjectNode()},
		}},
	}
	summary, err := RunObjectBackfill(context.Background(), store, fetcher, testManifest(), ObjectBackfillOptions{
		Environment:       model.EnvironmentStillness,
		Network:           "sui-testnet",
		Endpoint:          "https://graphql.testnet.sui.io/graphql",
		First:             50,
		MaxPages:          1,
		Concurrency:       1,
		Targets:           []ObjectTypeTarget{blockedTarget},
		AllowTargetErrors: true,
		ResetCursors:      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(fetcher.queries) != 1 || fetcher.queries[0].Type != blockedTarget.TypeRepr {
		t.Fatalf("expected only the explicit blocked object target to be retried, got %#v", fetcher.queries)
	}
	if summary.Targets != 1 || summary.SkippedTargets != 0 || summary.ObjectsProcessed != 1 {
		t.Fatalf("unexpected explicit target summary %#v", summary)
	}
	cursor, ok, err := store.GetSyncCursor(context.Background(), ObjectCursorID(blockedTarget))
	if err != nil || !ok {
		t.Fatalf("missing original blocked target cursor ok=%v err=%v", ok, err)
	}
	if cursor.LastErrorSummary != "" || cursor.EventsProcessed != 1 {
		t.Fatalf("retry should repair the original cursor, got %#v", cursor)
	}
}

func TestRunObjectBackfillContinuesAfterTargetError(t *testing.T) {
	store := db.NewMemoryStore()
	fetcher := &typeErrorObjectFetcher{
		errorsByType: map[string]error{
			testPackageID + "::aaa::Missing": errors.New("request is outside consistent range"),
		},
		pagesByType: map[string][]ObjectsPage{
			testPackageID + "::character::PlayerProfile": {{
				Nodes: []MoveObjectNode{testMoveObjectNode()},
			}},
		},
	}
	summary, err := RunObjectBackfill(context.Background(), store, fetcher, Manifest{
		SchemaVersion: "registry.sui-packages.v1",
		Packages: []PackageManifest{{
			Name:               "world",
			Network:            "sui-testnet",
			OriginalPackageID:  testPackageID,
			PublishedPackageID: testPackageID,
			Modules:            []string{"aaa", "character"},
			ObjectTypes: []ObjectTypeManifest{
				{ModuleName: "aaa", TypeName: "Missing"},
				{ModuleName: "character", TypeName: "PlayerProfile"},
			},
		}},
	}, ObjectBackfillOptions{
		Environment: model.EnvironmentStillness,
		Network:     "sui-testnet",
		Endpoint:    "https://graphql.testnet.sui.io/graphql",
		First:       50,
		MaxPages:    1,
		Concurrency: 1,
	})
	if err == nil {
		t.Fatal("expected target error")
	}
	if summary.Errors != 1 {
		t.Fatalf("expected one target error, got %#v", summary)
	}
	if summary.ObjectsProcessed != 1 {
		t.Fatalf("expected later target to be processed, got %#v", summary)
	}
	if len(fetcher.queries) != 2 {
		t.Fatalf("expected both object targets to be queried, got %#v", fetcher.queries)
	}
}

func TestRunObjectBackfillCanTolerateTargetErrors(t *testing.T) {
	store := db.NewMemoryStore()
	fetcher := &typeErrorObjectFetcher{
		errorsByType: map[string]error{
			testPackageID + "::aaa::Missing": errors.New("request is outside consistent range"),
		},
		pagesByType: map[string][]ObjectsPage{
			testPackageID + "::character::PlayerProfile": {{
				Nodes: []MoveObjectNode{testMoveObjectNode()},
			}},
		},
	}
	summary, err := RunObjectBackfill(context.Background(), store, fetcher, Manifest{
		SchemaVersion: "registry.sui-packages.v1",
		Packages: []PackageManifest{{
			Name:               "world",
			Network:            "sui-testnet",
			OriginalPackageID:  testPackageID,
			PublishedPackageID: testPackageID,
			Modules:            []string{"aaa", "character"},
			ObjectTypes: []ObjectTypeManifest{
				{ModuleName: "aaa", TypeName: "Missing"},
				{ModuleName: "character", TypeName: "PlayerProfile"},
			},
		}},
	}, ObjectBackfillOptions{
		Environment:       model.EnvironmentStillness,
		Network:           "sui-testnet",
		Endpoint:          "https://graphql.testnet.sui.io/graphql",
		First:             50,
		MaxPages:          1,
		Concurrency:       1,
		AllowTargetErrors: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if summary.Errors != 1 || summary.ObjectsProcessed != 1 {
		t.Fatalf("expected tolerated error and later processed object, got %#v", summary)
	}
	cursor, ok, err := store.GetSyncCursor(context.Background(), "cursor:sui:sui-testnet:objects:original:"+testPackageID+"::aaa::Missing")
	if err != nil || !ok {
		t.Fatalf("missing errored cursor ok=%v err=%v", ok, err)
	}
	if cursor.LastErrorSummary == "" {
		t.Fatalf("target error should still be recorded on its cursor: %#v", cursor)
	}
}

type recordingFetcher struct {
	queries []EventsQuery
	pages   []EventsPage
}

func (f *recordingFetcher) FetchEvents(ctx context.Context, query EventsQuery) (EventsPage, error) {
	_ = ctx
	f.queries = append(f.queries, query)
	if len(f.pages) == 0 {
		return EventsPage{}, nil
	}
	page := f.pages[0]
	f.pages = f.pages[1:]
	return page, nil
}

type recordingObjectFetcher struct {
	queries []ObjectsQuery
	pages   []ObjectsPage
}

func (f *recordingObjectFetcher) FetchObjects(ctx context.Context, query ObjectsQuery) (ObjectsPage, error) {
	_ = ctx
	f.queries = append(f.queries, query)
	if len(f.pages) == 0 {
		return ObjectsPage{}, nil
	}
	page := f.pages[0]
	f.pages = f.pages[1:]
	return page, nil
}

type typeErrorObjectFetcher struct {
	queries      []ObjectsQuery
	errorsByType map[string]error
	pagesByType  map[string][]ObjectsPage
}

func (f *typeErrorObjectFetcher) FetchObjects(ctx context.Context, query ObjectsQuery) (ObjectsPage, error) {
	_ = ctx
	f.queries = append(f.queries, query)
	if err := f.errorsByType[query.Type]; err != nil {
		return ObjectsPage{}, err
	}
	pages := f.pagesByType[query.Type]
	if len(pages) == 0 {
		return ObjectsPage{}, nil
	}
	page := pages[0]
	f.pagesByType[query.Type] = pages[1:]
	return page, nil
}

func testManifest() Manifest {
	return Manifest{
		SchemaVersion: "registry.sui-packages.v1",
		Packages: []PackageManifest{{
			Name:               "world",
			Network:            "sui-testnet",
			OriginalPackageID:  testPackageID,
			PublishedPackageID: "0xd2fd1224f881e7a705dbc211888af11655c315f2ee0f03fe680fc3176e6e4780",
			Modules:            []string{"gate", "character"},
			ObjectTypes: []ObjectTypeManifest{
				{ModuleName: "character", TypeName: "PlayerProfile"},
				{ModuleName: "gate", TypeName: "Gate"},
			},
		}},
	}
}

func testMoveEventNode() MoveEventNode {
	return MoveEventNode{
		SequenceNumber: 2,
		Timestamp:      "2026-05-27T18:12:12.879Z",
		Transaction:    &TransactionNode{Digest: "7YCpwNPBFJrd6APq6ModDosVUcCmSqsCSB9X76zzm3ch"},
		TransactionModule: &TransactionModuleNode{
			Name:    "character",
			Package: &AddressNode{Address: testPackageID},
		},
		Contents: &MoveEventContents{
			Type: &TypeNode{Repr: testPackageID + "::character::CharacterCreatedEvent"},
			JSON: map[string]any{
				"key": map[string]any{
					"item_id": "2112091476",
					"tenant":  "stillness",
				},
			},
		},
	}
}

func testMoveObjectNode() MoveObjectNode {
	return MoveObjectNode{
		Address: "0x59714bcd14f03bd20794bd3b5a2a52a0045e75e1bc9cc78aada8c56847e5731c",
		Digest:  "AbcDigest",
		Version: GraphQLScalar("7"),
		AsMoveObject: &MoveObjectData{Contents: &MoveEventContents{
			Type: &TypeNode{Repr: testPackageID + "::character::PlayerProfile"},
			JSON: map[string]any{
				"character_id": "2112091476",
			},
		}},
	}
}
