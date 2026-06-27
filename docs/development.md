# Development

## Local Commands
```sh
go test ./...
go vet ./...
go build ./cmd/...
go run ./cmd/br-indexer -mode plan -manifest testdata/fixtures/sui-packages.stillness.json
go run ./cmd/br-indexer -mode objects -manifest testdata/fixtures/sui-packages.stillness.json -max-pages 1
go run ./cmd/br-indexer -mode derive-events -module killmail,character,gate,assembly,storage_unit,turret,rift -derive-batch-size 5000
go run ./cmd/br-indexer -mode derive-objects -derive-batch-size 1000
go run ./cmd/br-indexer -mode resolve-evidence
go run ./cmd/br-indexer -mode audit-tribe-identity-evidence -environment stillness -sample-limit 20
go run ./cmd/br-indexer -mode audit-object-shapes -object-type-name Assembly -shape-limit 1000 -sample-limit 5
go run ./cmd/br-indexer -mode audit-systems -static-universe-path ./tmp/static-client-universe-stillness
go run ./cmd/br-import static-client-inspect-types -client-path "/path/to/eve-frontier/stillness"
go run ./cmd/br-import static-client-decode-types -client-path "/path/to/eve-frontier/stillness" -out ./tmp/static-client-types.native-decode.json
go run ./cmd/br-import static-client-extract-types -client-path "/path/to/eve-frontier/stillness" -out ./tmp/static-client-types.probes.json
go run ./cmd/br-import static-client-extract-types -client-path "/path/to/eve-frontier/stillness" -native-scan -out ./tmp/static-client-types.native-scan.json
go run ./cmd/br-import static-client-extract-types -client-path "/path/to/eve-frontier/stillness" -resolved-json ./tmp/static-client-types-current.json -out ./tmp/static-client-types-all.json
go run ./cmd/br-import static-client-extract-production -client-path "/path/to/eve-frontier/stillness" -out ./tmp/static-client-production-resources.json
go run ./cmd/br-import datahub-types -path testdata/fixtures/datahub-types.json
go run ./cmd/br-import world-systems -path testdata/fixtures/world-systems.json
go run ./cmd/br-export -out exports
go run honnef.co/go/tools/cmd/staticcheck@latest ./...
```

## Script Conventions

Shell scripts under `scripts/*.sh` are the primary operator scripts. PowerShell scripts under `scripts/*.ps1` are Windows compatibility entrypoints and should follow the shell script behaviour where both exist.

PostgreSQL-backed API proof tests use a temporary schema on the configured database:
```sh
go test ./internal/api -run "TestPostgresAPI" -count=1
```

These tests exercise the HTTP layer, migrated PostgreSQL schema, current-state filters, semantic killmail filters and resolver output together.

Smoke test:
```sh
./scripts/smoke.sh
```

The smoke script chooses Docker Compose, Podman Compose or `podman-compose` when one is available. Use an existing PostgreSQL database with:
```sh
export DATABASE_URL="postgres://blackrelay:blackrelay@127.0.0.1:5432/blackrelay_registry?sslmode=disable"
BR_REGISTRY_CONTAINER_RUNTIME=external ./scripts/smoke.sh
```

The same command can be written inline when the shell supports prefix environment assignments:
```sh
BR_REGISTRY_CONTAINER_RUNTIME=external DATABASE_URL='postgres://blackrelay:blackrelay@127.0.0.1:5432/blackrelay_registry?sslmode=disable' ./scripts/smoke.sh
```

## Runtime Configuration
Another operator can run the same binaries with a different database, artefact root and registry instance id.

Common settings:
```text
DATABASE_URL                    PostgreSQL connection string
BR_REGISTRY_ADDR                API listen address, default 127.0.0.1:8080
BR_REGISTRY_ARTEFACT_ROOT       local artefact store root, default artefacts
BR_REGISTRY_ADMIN_TOKEN         local admin bearer token
BR_REGISTRY_ACCESS_TEAM_DOMAIN  Cloudflare Access team domain for production admin auth
BR_REGISTRY_ACCESS_AUD          Cloudflare Access application AUD tag for production admin auth
BR_REGISTRY_ACCESS_CERTS_URL    optional Cloudflare Access JWKS URL override
BR_REGISTRY_INSTANCE_ID         response metadata instance id, default black-relay-registry
BR_REGISTRY_API_VERSION         response metadata API version, default v1
BR_REGISTRY_READY_TIMEOUT       readiness timeout, default 2s
BR_R2_ACCOUNT_ID                Cloudflare account id for publish-r2
BR_R2_ENDPOINT                  optional explicit S3-compatible endpoint
BR_R2_BUCKET                    R2 bucket for published exports
BR_R2_ACCESS_KEY_ID             R2 S3 access key id
BR_R2_SECRET_ACCESS_KEY         R2 S3 secret access key
BR_R2_REGION                    S3 signing region, default auto
```

Equivalent `br-registry` flags:
```sh
go run ./cmd/br-registry `
  -database-url "postgres://operator:operator@127.0.0.1:5432/operator_registry?sslmode=disable" `
  -artefact-root ./artefacts `
  -registry-id operator-frontier-registry `
  -api-version v1 `
  -access-team-domain "https://your-team.cloudflareaccess.com" `
  -access-aud "your-application-aud-tag" `
  -addr 127.0.0.1:8080
```

The instance id appears in `meta.registry` on API responses. It should be stable for a deployment. Source, confidence and provenance fields carry data authority.

When both Access settings are configured, admin routes require a valid Cloudflare Access JWT in the `Cf-Access-Jwt-Assertion` header. The local bearer token remains the development fallback when Access is not configured.

## Sui Backfill Development

The safe default command is a plan-only run:
```sh
go run ./cmd/br-indexer -mode plan -manifest testdata/fixtures/sui-packages.stillness.json
```

Manifest package entries can carry `startingCheckpoint` for newly published Sui packages. The event planner turns that into an exclusive `afterCheckpoint` filter and a checkpoint-scoped cursor source. This is used for newly observed Stillness packages. Registry cycles are not inferred from package names; they are assigned from source timestamps when a source row has enough time evidence.

Cycle boundary normalisation for indexed Sui rows currently emits only cycles present in the Sui-backed data:
```text
Cycle 5 starts 2026-03-11T09:00:00Z
Cycle 6 starts 2026-06-25T09:00:00Z
```

Raw Sui events receive a cycle from `occurred_at` when they fall in Cycle 5 or later. Event-derived entities and facts inherit that event cycle. Object-derived entities and facts use the object observation time. The current indexed Sui data contains Cycle 5 and Cycle 6 rows for this normaliser. Manual, observed and community imports use a null cycle until their own source proves a cycle.

Audit the manifest target set against saved cursors without making Sui GraphQL calls:
```sh
go run ./cmd/br-indexer -mode audit-stillness -manifest testdata/fixtures/sui-packages.stillness.json
```

The event indexer mode writes to PostgreSQL:
```sh
go run ./cmd/br-indexer -mode events -manifest testdata/fixtures/sui-packages.stillness.json -max-pages 1
```

The object indexer mode writes raw current-state Move objects to PostgreSQL:
```sh
go run ./cmd/br-indexer -mode objects -manifest testdata/fixtures/sui-packages.stillness.json -max-pages 1
```

Use `-object-type-name PlayerProfile` for a manifest object type or `-object-type 0x...::character::PlayerProfile` for a full Move type discovered outside the manifest.

Replay stored objects into conservative Registry entities after object backfill:
```sh
go run ./cmd/br-indexer -mode derive-objects -derive-batch-size 1000
```

Replay stored Sui events into conservative Registry entities, killmail records and relations:
```sh
go run ./cmd/br-indexer -mode derive-events -module killmail,character,gate,assembly,storage_unit,turret,rift -derive-batch-size 5000
```

The module-scoped event derivation command is the normal repair/append path for semantic API data. It stores separate cursors for each requested module and avoids replaying high-volume streams such as `network_node` unless a deliberate full replay is required. Omit `-module` only for that full maintenance replay.

Run events and objects in one network pass:
```sh
go run ./cmd/br-indexer -mode all -manifest testdata/fixtures/sui-packages.stillness.json -max-pages 0 -concurrency 16
```

Use `-max-pages 0` only for a deliberate full backfill. Use `-package`, `-module`, `-object-type-name` and `-object-type` to reduce the target set during debugging. Saved cursors are used unless `-reset-cursors` is provided.

For a repair pass after an interrupted run, use `-only-incomplete`. It skips clean saved cursors and only runs streams with no cursor or a retryable cursor error. Object cursors with Sui GraphQL `Request is outside consistent range` are reported as provider-limited `range_blocked` evidence and skipped unless cursors are reset or the object target is queried explicitly:
```sh
go run ./cmd/br-indexer -mode all -manifest testdata/fixtures/sui-packages.stillness.json -max-pages 0 -concurrency 64 -only-incomplete
```

Audit saved manifest object targets that are currently marked as provider-range blocked:
```sh
go run ./cmd/br-indexer -mode audit-range-blocked-objects -manifest testdata/fixtures/sui-packages.stillness.json
```

The primary Cycle 6 repair path is event backfill and module-scoped `derive-events` for chain-derived state. Storage-unit inventory rows include item mint, burn, destroy, deposit and withdraw actions, including the Cycle 6 v2 deposit/withdraw event shapes. These derive conservative item type placeholders, storage evidence and character action relations until static-client type imports provide names and categories. World API tribe/system snapshots provide public profile fields where those endpoints exist. Static-client imports provide systems, constellations, regions, enemies, type rows and reviewed recipes.

Generate fixture-aware local quality reports after derivation:
```sh
go run ./cmd/br-indexer -mode report -exclude-fixtures
go run ./cmd/br-indexer -mode audit-killmails -exclude-fixtures -sample-limit 20
go run ./cmd/br-indexer -mode audit-current-state
go run ./cmd/br-indexer -mode audit-character-profiles -sample-limit 20
go run ./cmd/br-indexer -mode audit-tribe-identity-evidence -environment stillness -sample-limit 20
go run ./cmd/br-indexer -mode audit-evidence-bridges -sample-limit 20
go run ./cmd/br-indexer -mode audit-object-shapes -object-type-name Assembly -shape-limit 1000 -sample-limit 5
go run ./cmd/br-indexer -mode audit-systems -static-universe-path ./tmp/static-client-universe-stillness
```

Generate the status JSON used by freshness/status consumers:
```sh
go run ./cmd/br-indexer -mode status -export-manifest ./exports/manifest.json
```

The status command reads cursor rows, counts current lag, classifies the overall indexer state as `ok`, `stale`, `degraded` or `blocked` and includes optional export bundle metadata when `-export-manifest` is provided. Provider-limited object-by-type scans stay visible in `rangeBlocked` counts and source gaps. Registry status is based on the combined cursor and freshness picture.

The killmail audit lists semantic resolution counts and evidence counts for raw `killer_id` versus explicit `killer_type_id`. It also samples unresolved real rows. The current-state audit counts character tribe links, public activity, ownership, deployment/system placement, cap/hash evidence and route-edge coverage. The character profile audit separates sourced metadata profiles from placeholder display names such as `Character 42`. The evidence bridge audit counts owner-capability and location-hash evidence separately from promoted owner/system relations and samples unresolved bridge values.

Current-state infrastructure routes expose `owner_cap` and `location_hash` filters for evidence-only Sui object fields. A record becomes queryable by `owner` or `system` when the Registry has a separate public bridge that proves an unambiguous `owned_by` or `located_in` relation. Run `br-indexer -mode resolve-evidence` to rerun that bridge without re-deriving object rows. Static-universe relations are traversed both ways for the `system` filter, so a region or constellation can be selected by one of its member systems.

The object-shape audit reads stored raw Sui object rows and reports deterministic JSON key paths for one package, module, type name or exact Move type. Use it after package updates before adding semantic normalisers. Fields such as `owner_cap_id` and `location_hash` are evidence until a public source proves the corresponding owner or solar-system relationship. Object derivation resolves those evidence fields only when the same value maps to exactly one public character or system entity in the same environment; ambiguous values remain evidence-only. The system audit compares Registry system IDs with a static-client universe extraction and reports source-only and Registry-only IDs.

The static-client type inspector is a native Go evidence probe. It resolves and hashes `types.fsdbinary`, reports header bytes, searches for reviewed IDs as little-endian integers, decodes numeric row probes for known type-row evidence and resolves those probed rows through the localisation resource when available. `static-client-decode-types` writes the deterministic native decode artefact with row offsets, resource hashes and localisation-backed type names. `static-client-extract-types -native-scan` performs an opt-in localisation-backed scan of the same binary row shape and writes a review artefact. `static-client-decode-production` writes a separate candidate artefact for production rows with blueprint primaries, derived recipe candidates and `typematerials` material rows. Continue to use `static-client-extract-types -resolved-json` for broad canonical type imports until scan-vs-resolved deltas have been reviewed for the current patch; keep recipe promotion behind the reviewed JSON recipe importer.

For the current Cycle 6 package window, use the wrapper script for a bounded refresh:
```sh
./scripts/refresh-cycle6.sh --max-pages 5 --concurrency 64
```

Use `--full` only for a deliberate complete append from saved package cursors. The script runs event and object backfills, derivation, Stillness package audit, killmail audit, current-state audit and the aggregate report.

By default the script also generates a compact public export in a staging directory, verifies it, promotes it to the configured export path and writes `tmp/cycle6-refresh-summary.json`. Use `--summary-path` to change the summary artefact path. Use `--skip-export` for a repair-only run and `--export-cycles all` for an archive export. Use `--publish-local` or `--publish-r2` only after the verified export should be published to an object-store layout. The default publish prefix is `registry/current`; archive publication should pass `--export-cycles all --publish-prefix registry/archive/all`.

For an operator-friendly local proof path, use the bootstrap script:
```sh
./scripts/bootstrap.sh --runtime external --client-path "/path/to/eve-frontier/stillness" --static-universe-path ./tmp/static-client-universe-stillness
```

Run the local HTTP proof against an existing API or let the script start a temporary local API:
```sh
./scripts/api-proof.sh
./scripts/api-proof.sh --start-server
```

## Metadata Imports

Datahub type metadata and World API system or tribe metadata are imported from local JSON snapshots. The importer records the snapshot as a SHA-256 source artefact before writing item, system or tribe facts.
```sh
go run ./cmd/br-import datahub-types -path ./local-extract/datahub-types.json -source-url "https://datahub.evefrontier.com/types.json"
go run ./cmd/br-import world-systems -path ./local-extract/world-systems.json -source-url "https://world-api.evefrontier.com/systems.json"
go run ./cmd/br-import world-tribes -path ./local-extract/world-tribes.json -source-url "https://world-api-stillness.live.pub.evefrontier.com/v2/tribes"
```

The `-source-url` value is provenance only. These commands import local snapshot files. Private `*.priv.evefrontier.com` hosts are rejected.

To fetch a public URL into a local evidence snapshot before import:
```sh
go run ./cmd/br-import datahub-types -url "https://datahub.evefrontier.com/types.json" -snapshot-path ./local-extract/datahub-types.json
go run ./cmd/br-import world-systems -url "https://world-api.evefrontier.com/systems.json" -snapshot-path ./local-extract/world-systems.json
go run ./cmd/br-import world-tribes -url "https://world-api-stillness.live.pub.evefrontier.com/v2/tribes" -snapshot-path ./local-extract/world-tribes.json
```

The fetch path writes the local file first then imports through normal source-artefact registration. It rejects private `*.priv.evefrontier.com` hosts.

## Public Exports
Generate compact public distribution files with:
```sh
go run ./cmd/br-export -out exports
```

The default export drains current-cycle plus unlabelled rows through keyset pagination. Use `-cycles current` for strict current-cycle rows, `-cycles all` for an archive bundle or `-cycles 5,6` for an explicit multi-cycle bundle. Use `-limit 5000` for a bounded sample export.

Raw indexed evidence exports are opt-in:
```sh
go run ./cmd/br-export -out exports-raw -include-events -include-sui-objects -timeout 30m
go run ./cmd/br-export -out exports-all -cycles all -include-events -include-sui-objects -timeout 30m
```

Verify an export before publishing or syncing it:
```sh
go run ./cmd/br-export verify -dir exports
```

Verification recomputes file SHA-256 values, byte sizes and row counts from `manifest.json`. It also validates the required JSONL row fields documented in `contracts/*.export.v1.schema.json`. It exits non-zero if a file is missing, truncated, tampered with, listed with an unsafe path or missing required row fields.

Publish a verified export into a local object-store-shaped directory:
```sh
go run ./cmd/br-export publish-local -dir exports -root published-exports -prefix registry/current
go run ./cmd/br-export publish-local -dir exports-all -root published-exports -prefix registry/archive/all
./scripts/verify-publication.sh --root published-exports --prefix registry/current,registry/archive/all
```

`publish-local` refuses invalid exports. For a valid export it writes immutable objects under:
```text
registry/current/bundles/<bundle-id>/
registry/archive/all/bundles/<bundle-id>/
```

It writes each prefix's `latest/manifest.json` last. That latest object is a small pointer containing the bundle id, immutable manifest key, manifest SHA-256 and file list. This is the publication order used for R2 or S3-compatible storage.

Publish the same verified export to Cloudflare R2 with S3-compatible credentials:
```sh
export BR_R2_ACCOUNT_ID="your-cloudflare-account-id"
export BR_R2_BUCKET="registry-exports"
export BR_R2_ACCESS_KEY_ID="..."
export BR_R2_SECRET_ACCESS_KEY="..."
go run ./cmd/br-export publish-r2 -dir exports -prefix registry/current
```

Use `-endpoint` instead of `-account-id` for a non-Cloudflare S3-compatible endpoint. `publish-r2` verifies the export first, writes immutable bundle objects with conditional writes and writes the selected prefix's `latest/manifest.json` last.

The exporter writes:
```text
catalog.json
manifest.json
entities.jsonl
killmails.jsonl
sources.jsonl
```

`catalog.json` and `manifest.json` record the configured registry instance id, API version, requested cycle scope, effective cycles and whether unlabelled rows were included. `manifest.json` also records each distribution file's SHA-256 checksum, byte size, row count, source database identity and collection high-water marks. The manifest records `catalog.json` and data files; the manifest file is excluded from its own checksum list.

Use `-registry-id` and `-api-version` or the matching environment variables when generating exports for a non-Black Relay deployment.

When raw evidence is requested, it also writes:
```text
events.jsonl
sui_objects.jsonl
```
