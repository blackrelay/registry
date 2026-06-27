package db

import (
	"strings"

	"github.com/blackrelay/registry/internal/model"
)

func sourceGapRows(environment model.Environment, ownershipEvidenceOnly, locationEvidenceOnly, unresolvedKillmails, recipeCount, suiObjectRangeBlocked, placeholderTribeNames, tribeProfileGaps int64) []model.SourceGap {
	env := string(environment)
	if env == "" {
		env = "all"
	}
	rows := []model.SourceGap{
		{
			ID:                "source-gap:" + env + ":ownership-evidence-only",
			Kind:              "ownership_evidence_only",
			Category:          "resolver_missing",
			Severity:          severityForCount(ownershipEvidenceOnly),
			Environment:       environment,
			Count:             ownershipEvidenceOnly,
			Summary:           "Infrastructure records have public owner capability evidence but no resolved public owner relation.",
			RecommendedAction: "Add or derive a public owner-capability to character mapping, then rerun evidence relation resolution.",
			SuggestedCommands: []string{"go run ./cmd/br-indexer -mode resolve-evidence -environment " + env},
		},
		{
			ID:                "source-gap:" + env + ":location-evidence-only",
			Kind:              "location_evidence_only",
			Category:          "resolver_missing",
			Severity:          severityForCount(locationEvidenceOnly),
			Environment:       environment,
			Count:             locationEvidenceOnly,
			Summary:           "Infrastructure records have public location hash evidence but no resolved system relation.",
			RecommendedAction: "Add or derive a public location-hash to system mapping, then rerun evidence relation resolution.",
			SuggestedCommands: []string{"go run ./cmd/br-indexer -mode resolve-evidence -environment " + env},
		},
		{
			ID:                "source-gap:" + env + ":unresolved-killmail-actors",
			Kind:              "unresolved_killmail_actors",
			Category:          "resolver_missing",
			Severity:          severityForCount(unresolvedKillmails),
			Environment:       environment,
			Count:             unresolvedKillmails,
			Summary:           "Killmail rows are missing one or more semantic actor or system resolutions.",
			RecommendedAction: "Import additional public character, system or NPC type mapping evidence and rerun killmail derivation.",
			SuggestedCommands: []string{
				"go run ./cmd/br-import static-client-decode-types -client-path <client-root> -out ./tmp/static-client-types.native-decode.json",
				"go run ./cmd/br-import static-client-enemies -path ./tmp/static-client-types.native-decode.json -environment " + env,
				"go run ./cmd/br-indexer -mode derive-events -environment " + env + " -module killmail",
			},
		},
		{
			ID:                "source-gap:" + env + ":sui-object-provider-range-blocked",
			Kind:              "sui_object_provider_range_blocked",
			Category:          "provider_blocked",
			Severity:          severityForProviderRangeBlocked(suiObjectRangeBlocked),
			Environment:       environment,
			Count:             suiObjectRangeBlocked,
			Summary:           "Broad Sui object-by-type cursor targets are outside the public GraphQL provider's consistent-range window.",
			RecommendedAction: "Keep the raw cursor error as evidence. Treat events as the primary chain source, repair semantic state from event derivation and use World API or static-client imports for names and static data.",
			SuggestedCommands: []string{
				"go run ./cmd/br-indexer -mode derive-events -environment " + env + " -module killmail,character,gate,assembly,storage_unit,turret -derive-batch-size 5000",
				"go run ./cmd/br-indexer -mode resolve-evidence -environment " + env,
				"go run ./cmd/br-indexer -mode audit-range-blocked-objects -environment " + env,
			},
		},
		{
			ID:                "source-gap:" + env + ":static-client-recipes",
			Kind:              "static_client_recipes",
			Category:          "static_data_missing",
			Severity:          severityForMissingRecipeData(recipeCount),
			Environment:       environment,
			Count:             missingDataCount(recipeCount),
			Summary:           "No static-client recipe records are present for this query scope.",
			RecommendedAction: "Decode native recipe, blueprint and material-requirement candidates, review the candidate artefact, then import reviewed rows with br-import static-client-recipes.",
			SuggestedCommands: []string{
				"go run ./cmd/br-import static-client-extract-production -client-path <client-root> -out ./tmp/static-client-production-resources.json",
				"go run ./cmd/br-import static-client-decode-production -client-path <client-root> -out ./tmp/static-client-production.native-decode.json",
				"go run ./cmd/br-import static-client-recipes -path ./tmp/static-client-recipes.reviewed.json -environment " + env,
			},
		},
		{
			ID:                "source-gap:" + env + ":tribe-identity-names",
			Kind:              "tribe_identity_names",
			Category:          "source_missing",
			Severity:          severityForCount(placeholderTribeNames),
			Environment:       environment,
			Count:             placeholderTribeNames,
			Summary:           "Tribe entities have only chain-derived placeholder names such as Tribe 42.",
			RecommendedAction: "Import a public World API tribe metadata snapshot or a reviewed public tribe identity artefact that maps stable tribe ids to names, tags, aliases, descriptions and URLs.",
			SuggestedCommands: []string{
				"go run ./cmd/br-import world-tribes -url \"https://world-api-stillness.live.pub.evefrontier.com/v2/tribes\" -snapshot-path ./local-extract/world-tribes.json -environment " + env,
				"go run ./cmd/br-import tribe-identities -path ./local-extract/tribe-identities.reviewed.json -environment " + env,
			},
		},
		{
			ID:                "source-gap:" + env + ":tribe-identity-profiles",
			Kind:              "tribe_identity_profiles",
			Category:          "source_missing",
			Severity:          severityForCount(tribeProfileGaps),
			Environment:       environment,
			Count:             tribeProfileGaps,
			Summary:           "Tribe entities are missing reviewed public profile fields such as tag, aliases, description or URL.",
			RecommendedAction: "Import a public World API tribe metadata snapshot or a reviewed public tribe identity artefact that maps stable tribe ids to public profile fields.",
			SuggestedCommands: []string{
				"go run ./cmd/br-import world-tribes -url \"https://world-api-stillness.live.pub.evefrontier.com/v2/tribes\" -snapshot-path ./local-extract/world-tribes.json -environment " + env,
				"go run ./cmd/br-import tribe-identities -path ./local-extract/tribe-identities.reviewed.json -environment " + env,
			},
		},
		{
			ID:                "source-gap:" + env + ":static-client-full-table-decoder",
			Kind:              "static_client_full_table_decoder",
			Category:          "decoder_review_required",
			Severity:          "info",
			Environment:       environment,
			Count:             1,
			Summary:           "Native Go static-client decoding can resolve localisation-backed type rows and candidate production rows; canonical recipe promotion remains review-gated.",
			RecommendedAction: "Decode types and production rows natively, compare/review the output, then promote only reviewed recipe rows through the recipe importer.",
			SuggestedCommands: []string{
				"go run ./cmd/br-import static-client-decode-types -client-path <client-root> -out ./tmp/static-client-types.native-decode.json",
				"go run ./cmd/br-import static-client-compare-types -resolved ./tmp/static-client-types.reviewed.json -native ./tmp/static-client-types.native-decode.json",
				"go run ./cmd/br-import static-client-decode-production -client-path <client-root> -out ./tmp/static-client-production.native-decode.json",
			},
		},
	}
	out := make([]model.SourceGap, 0, len(rows))
	for _, row := range rows {
		if row.Count > 0 || row.Kind == "static_client_full_table_decoder" {
			out = append(out, row)
		}
	}
	return out
}

func severityForCount(count int64) string {
	if count == 0 {
		return "none"
	}
	return "actionable"
}

func severityForProviderRangeBlocked(count int64) string {
	if count == 0 {
		return "none"
	}
	return "info"
}

func severityForMissingRecipeData(recipeCount int64) string {
	if recipeCount == 0 {
		return "actionable"
	}
	return "none"
}

func missingDataCount(rowCount int64) int64 {
	if rowCount == 0 {
		return 1
	}
	return 0
}

func isProviderRangeBlockedCursor(cursor CursorStatus) bool {
	return cursor.CursorKind == "sui_object" && strings.Contains(strings.ToLower(cursor.LastErrorSummary), "outside consistent range")
}
