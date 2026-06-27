package sui

import (
	"context"
	"testing"
	"time"

	"github.com/blackrelay/registry/internal/db"
	"github.com/blackrelay/registry/internal/model"
)

func TestAuditCoverageMarksMissingErroredLimitedAndIndexedTargets(t *testing.T) {
	store := db.NewMemoryStore()
	manifest := Manifest{
		SchemaVersion: "registry.sui-packages.v1",
		Packages: []PackageManifest{{
			Name:               "world",
			Network:            "sui-testnet",
			OriginalPackageID:  testPackageID,
			PublishedPackageID: "0xd2fd1224f881e7a705dbc211888af11655c315f2ee0f03fe680fc3176e6e4780",
			Modules:            []string{"character", "gate"},
			ObjectTypes: []ObjectTypeManifest{
				{ModuleName: "character", TypeName: "PlayerProfile"},
			},
		}},
	}
	now := time.Date(2026, 6, 25, 1, 2, 3, 0, time.UTC)
	indexedTarget := EventStreamTarget{
		Environment: model.EnvironmentStillness,
		Network:     "sui-testnet",
		PackageName: "world",
		PackageID:   testPackageID,
		Role:        PackageRoleOriginal,
		ModuleName:  "character",
	}
	erroredObjectTarget := ObjectTypeTarget{
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
		ID:                   EventCursorID(indexedTarget),
		Source:               EventCursorSource(indexedTarget),
		Environment:          model.EnvironmentStillness,
		CursorValue:          "event-cursor",
		CursorKind:           "sui_event",
		LastSuccessfulIngest: &now,
		EventsProcessed:      10,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveSyncCursor(context.Background(), db.CursorStatus{
		ID:                   ObjectCursorID(erroredObjectTarget),
		Source:               ObjectCursorSource(erroredObjectTarget),
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

	audit, err := AuditCoverage(context.Background(), store, manifest, CoverageAuditOptions{
		Environment: model.EnvironmentStillness,
		Network:     "sui-testnet",
		MaxPages:    1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if audit.EventTargets != 4 || audit.ObjectTargets != 2 || audit.TargetCount != 6 {
		t.Fatalf("unexpected target counts %#v", audit)
	}
	if audit.IndexedTargets != 0 {
		t.Fatalf("max-page audit should mark clean cursors as limited, got %#v", audit)
	}
	if audit.LimitedTargets != 1 || audit.ErroredTargets != 1 || audit.NotSeenTargets != 4 {
		t.Fatalf("unexpected coverage status counts %#v", audit)
	}
	if audit.FullCoverageProven {
		t.Fatal("cursor audit must not claim full coverage proof")
	}
	statusByCursor := map[string]model.CoverageStatus{}
	for _, target := range audit.Targets {
		statusByCursor[target.CursorID] = target.Status
	}
	if statusByCursor[EventCursorID(indexedTarget)] != model.CoverageStatusLimited {
		t.Fatalf("event target was not marked limited: %#v", statusByCursor)
	}
	if statusByCursor[ObjectCursorID(erroredObjectTarget)] != model.CoverageStatusErrored {
		t.Fatalf("object target was not marked errored: %#v", statusByCursor)
	}
}

func TestAuditCoverageTreatsHistoricalErrorCountAsIndexedAfterSuccessfulIngest(t *testing.T) {
	store := db.NewMemoryStore()
	manifest := Manifest{
		SchemaVersion: "registry.sui-packages.v1",
		Packages: []PackageManifest{{
			Name:               "world",
			Network:            "sui-testnet",
			OriginalPackageID:  testPackageID,
			PublishedPackageID: "0xd2fd1224f881e7a705dbc211888af11655c315f2ee0f03fe680fc3176e6e4780",
			Modules:            []string{"character"},
			ObjectTypes: []ObjectTypeManifest{
				{ModuleName: "character", TypeName: "PlayerProfile"},
			},
		}},
	}
	now := time.Date(2026, 6, 25, 2, 3, 4, 0, time.UTC)
	repairedTarget := EventStreamTarget{
		Environment: model.EnvironmentStillness,
		Network:     "sui-testnet",
		PackageName: "world",
		PackageID:   testPackageID,
		Role:        PackageRoleOriginal,
		ModuleName:  "character",
	}
	if err := store.SaveSyncCursor(context.Background(), db.CursorStatus{
		ID:                   EventCursorID(repairedTarget),
		Source:               EventCursorSource(repairedTarget),
		Environment:          model.EnvironmentStillness,
		CursorValue:          "repaired-cursor",
		CursorKind:           "sui_event",
		LastSuccessfulIngest: &now,
		EventsProcessed:      99,
		ErrorCount:           1,
		LastErrorSummary:     "",
	}); err != nil {
		t.Fatal(err)
	}

	audit, err := AuditCoverage(context.Background(), store, manifest, CoverageAuditOptions{
		Environment: model.EnvironmentStillness,
		Network:     "sui-testnet",
	})
	if err != nil {
		t.Fatal(err)
	}
	if audit.ErroredTargets != 0 {
		t.Fatalf("historical error count should not be active error coverage: %#v", audit)
	}
	statusByCursor := map[string]model.CoverageStatus{}
	for _, target := range audit.Targets {
		statusByCursor[target.CursorID] = target.Status
	}
	if statusByCursor[EventCursorID(repairedTarget)] != model.CoverageStatusIndexed {
		t.Fatalf("repaired event target was not marked indexed: %#v", statusByCursor)
	}
}

func TestAuditCoverageClassifiesConsistentRangeObjectErrorsAsRangeBlocked(t *testing.T) {
	store := db.NewMemoryStore()
	manifest := Manifest{
		SchemaVersion: "registry.sui-packages.v1",
		Packages: []PackageManifest{{
			Name:               "world",
			Network:            "sui-testnet",
			OriginalPackageID:  testPackageID,
			PublishedPackageID: testPackageID,
			Modules:            []string{"character"},
			ObjectTypes: []ObjectTypeManifest{
				{ModuleName: "character", TypeName: "PlayerProfile"},
			},
		}},
	}
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
		ID:               ObjectCursorID(target),
		Source:           ObjectCursorSource(target),
		Environment:      model.EnvironmentStillness,
		CursorKind:       "sui_object",
		ErrorCount:       1,
		LastErrorSummary: "sui GraphQL returned errors: Request is outside consistent range",
	}); err != nil {
		t.Fatal(err)
	}

	audit, err := AuditCoverage(context.Background(), store, manifest, CoverageAuditOptions{
		Environment: model.EnvironmentStillness,
		Network:     "sui-testnet",
	})
	if err != nil {
		t.Fatal(err)
	}
	if audit.ErroredTargets != 0 || audit.RangeBlockedTargets != 1 {
		t.Fatalf("provider range blocks should not be reported as retryable errors: %#v", audit)
	}
	var objectCoverage model.SuiCoverageTarget
	for _, item := range audit.Targets {
		if item.CursorID == ObjectCursorID(target) {
			objectCoverage = item
			break
		}
	}
	if objectCoverage.CursorID == "" {
		t.Fatalf("object coverage target was not found: %#v", audit.Targets)
	}
	if objectCoverage.Status != model.CoverageStatusRangeBlocked {
		t.Fatalf("unexpected target status %#v", objectCoverage)
	}
	if !objectCoverage.ProviderRangeBlocked {
		t.Fatalf("target did not expose provider range block flag: %#v", objectCoverage)
	}
}

func TestProviderRangeBlockedObjectTargetsSelectsOnlyManifestRangeBlockedCursors(t *testing.T) {
	store := db.NewMemoryStore()
	manifest := Manifest{
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
	}
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
	cleanTarget := ObjectTypeTarget{
		Environment: model.EnvironmentStillness,
		Network:     "sui-testnet",
		PackageName: "world",
		PackageID:   testPackageID,
		Role:        PackageRoleOriginal,
		ModuleName:  "gate",
		TypeName:    "Gate",
		TypeRepr:    testPackageID + "::gate::Gate",
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
	if err := store.SaveSyncCursor(context.Background(), db.CursorStatus{
		ID:              ObjectCursorID(cleanTarget),
		Source:          ObjectCursorSource(cleanTarget),
		Environment:     model.EnvironmentStillness,
		CursorKind:      "sui_object",
		EventsProcessed: 4,
	}); err != nil {
		t.Fatal(err)
	}

	targets, err := ProviderRangeBlockedObjectTargets(context.Background(), store, manifest, CoverageAuditOptions{
		Environment: model.EnvironmentStillness,
		Network:     "sui-testnet",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 1 {
		t.Fatalf("expected one range-blocked target, got %#v", targets)
	}
	if targets[0] != blockedTarget {
		t.Fatalf("range-blocked target identity was not preserved: %#v", targets[0])
	}
}

func TestCursorCoverageSummaryTreatsHistoricalErrorCountAsIndexed(t *testing.T) {
	now := time.Date(2026, 6, 25, 3, 4, 5, 0, time.UTC)
	summary := CursorCoverageSummary([]db.CursorStatus{{
		ID:                   "cursor:sui:sui-testnet:events:original:pkg:character",
		Source:               "sui:sui-testnet:events:original:pkg:character",
		Environment:          model.EnvironmentStillness,
		CursorKind:           "sui_event",
		LastSuccessfulIngest: &now,
		EventsProcessed:      100,
		ErrorCount:           2,
		LastErrorSummary:     "",
	}})
	if summary.IndexedTargets != 1 || summary.ErroredTargets != 0 {
		t.Fatalf("historical error count should be reported as indexed, got %#v", summary)
	}
	if summary.Targets[0].Status != model.CoverageStatusIndexed {
		t.Fatalf("unexpected target status %#v", summary.Targets[0])
	}
	if summary.Targets[0].ErrorCount != 2 {
		t.Fatalf("historical error count was not preserved: %#v", summary.Targets[0])
	}
}

func TestCursorCoverageSummaryReportsLastCheckpointAndEmptyStreams(t *testing.T) {
	now := time.Date(2026, 6, 25, 4, 5, 6, 0, time.UTC)
	summary := CursorCoverageSummary([]db.CursorStatus{{
		ID:                   "cursor:sui:sui-testnet:events:original:pkg:rift",
		Source:               "sui:sui-testnet:events:original:pkg:rift",
		Environment:          model.EnvironmentStillness,
		CursorKind:           "sui_event",
		LastSuccessfulIngest: &now,
		LastCheckpoint:       "352596413",
		EventsProcessed:      0,
	}})
	if len(summary.Targets) != 1 {
		t.Fatalf("expected one target, got %#v", summary.Targets)
	}
	if summary.Targets[0].LastCheckpoint != "352596413" {
		t.Fatalf("last checkpoint was not reported: %#v", summary.Targets[0])
	}
	if !summary.Targets[0].EmptyStream {
		t.Fatalf("zero-row successful stream should be marked empty: %#v", summary.Targets[0])
	}
}
