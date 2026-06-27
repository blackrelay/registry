# Operations

The operations surface is JSON-first.
```text
GET /v1/ops/freshness
GET /v1/ops/cursors
GET /v1/ops/sui-coverage
GET /v1/ops/source-gaps
GET /v1/metrics
```

Freshness records expose:
- source
- environment
- last successful ingest
- last checkpoint when known
- processed row count
- error count
- last error summary
- staleness status

Metrics are Prometheus-style text. They currently report build presence plus cursor processed-row/error counters when cursor rows exist.

`/v1/ops/sui-coverage` summarises cursor-table health for Sui event, object and derivation cursors. It is intentionally conservative: `fullCoverageProven` remains false because cursor presence proves resumability, not complete historical chain coverage. Coverage targets include processed row counts, active error state, the last saved checkpoint when a cursor has one and `emptyStream` for a clean stream that returned no rows.

Sui GraphQL object requests can return `Request is outside consistent range` for broad object-by-type scans. Registry reports those object cursors as `range_blocked` with `providerRangeBlocked: true`. That status means the provider range is unavailable through the current endpoint; it is not treated as a retryable hang or a hard Registry failure. `-only-incomplete` skips it unless cursors are reset or the object target is queried explicitly.

`/v1/ops/source-gaps` lists source and resolver gaps that affect semantic completeness. It highlights places where the Registry has public evidence and needs a safe semantic bridge. Each gap includes a stable `kind`, a broader `category`, the human action text and one or more `suggestedCommands` when a command-line repair path exists.

Current gap kinds include:
- `ownership_evidence_only`: public owner capability evidence exists but no `owned_by` relation has been proved. Category: `resolver_missing`.
- `location_evidence_only`: public location hash evidence exists but no `located_in` system relation has been proved. Category: `resolver_missing`.
- `unresolved_killmail_actors`: killmail rows are missing one or more semantic actor or system fields. Category: `resolver_missing`.
- `sui_object_provider_range_blocked`: broad Sui object-by-type cursor targets are outside the public GraphQL provider's consistent-range window. Category: `provider_blocked`.
- `static_client_recipes`: no reviewed recipe rows are present for the requested scope. Category: `static_data_missing`.
- `tribe_identity_names`: tribe entities have only chain-derived placeholder names such as `Tribe 42`; repair with `br-import world-tribes` when the official World API exposes profiles or `br-import tribe-identities` for reviewed public artefacts. Category: `source_missing`.
- `tribe_identity_profiles`: tribe entities are missing reviewed public profile fields such as tag, aliases, description or URL; repair with `br-import world-tribes` when available. Category: `source_missing`.
- `static_client_full_table_decoder`: native Go type and candidate production row decoding exists while canonical recipe promotion still requires reviewed JSON evidence. Category: `decoder_review_required`.

Local public EVE Frontier repositories checked during the Cycle 6 repair expose character `tribe_id` fields and public metadata primitives for characters and assemblies. Tribe name, tag, description and URL fields need a dedicated public source artefact. The public Datacore frontend repository confirms Datacore's API output shape for tribe names, tickers, descriptions and URLs; Registry still requires the backend source evidence or another preserved public artefact before promotion.

Use the tribe identity evidence audit before creating a reviewed profile artefact:
```sh
go run ./cmd/br-indexer -mode audit-tribe-identity-evidence -environment stillness -sample-limit 20
```

The audit separates membership-only `tribe_id` or `corpId` rows from candidate profile rows that contain an explicit tribe name, ticker, description or URL.

`br-indexer -mode report` includes the same source-gap rows in its JSON output under `sourceGaps`, alongside row counts and killmail resolution counts.

`br-indexer -mode status` emits the indexer status contract for freshness consumers. It reads the cursor table and source-gap rows, reports stream lag, range-blocked object cursors and optional export metadata:
```sh
go run ./cmd/br-indexer -mode status -export-manifest ./exports/manifest.json
```

The top-level `status` field is:
- `ok`: cursors are present, fresh and without active errors.
- `stale`: cursors exist but one or more successful streams are older than the configured `-status-stale-after` duration.
- `degraded`: one or more cursors have retryable errors or no successful ingest timestamp.
- `blocked`: reserved for hard-stop conditions. Provider-limited object-by-type scans are reported in `cursorCounts.rangeBlocked`, streams and source gaps instead.

Example:
```sh
curl -fsS "http://127.0.0.1:8080/v1/ops/source-gaps?environment=stillness"
```

## Admin Auth Operations

Production admin routes can sit behind Cloudflare Access. Configure:
```text
BR_REGISTRY_ACCESS_TEAM_DOMAIN
BR_REGISTRY_ACCESS_AUD
```

The API expects the Access JWT in `Cf-Access-Jwt-Assertion`, validates its RS256 signature against the Access JWKS endpoint and checks issuer and audience. `BR_REGISTRY_ACCESS_CERTS_URL` is an optional JWKS override for tests or unusual deployments.

If Access settings are not configured, admin routes use the local development bearer token from `BR_REGISTRY_ADMIN_TOKEN`. Do not expose a token-only development instance to the public internet.

## Sui Cursor Operations

Each Sui event stream stores its own cursor:
```text
sui:<network>:events:<role>:<package-id>:<module-or-*>
```

When a package manifest entry has `startingCheckpoint` or a run passes explicit checkpoint bounds, the event cursor source includes the exclusive checkpoint range:
```text
sui:<network>:events:<role>:<package-id>:<module-or-*>:<after-checkpoint-or-*>:<before-checkpoint-or-*>
```

Each Sui object stream stores its own cursor:
```text
sui:<network>:objects:<role>:<full-move-type>
```

Sui object derivation stores a separate replay cursor:
```text
registry:derive:sui-objects:<network>
```

The cursor value is the opaque `endCursor` returned by Sui GraphQL. Do not edit or manufacture cursor values. Use `br-indexer -mode events -reset-cursors` only when intentionally replaying a stream.

`br-indexer` defaults to the current cycle package scope. At the moment that means Cycle 6 package entries from the manifest. Use `-cycles all` or an explicit list such as `-cycles 5,6` for archive work. Cycle 5 remains useful for historical comparison, with tribe names, descriptions and URLs limited by the available Cycle 5 World API evidence.

Useful smoke commands:
```sh
go run ./cmd/br-indexer -mode audit-stillness -manifest testdata/fixtures/sui-packages.stillness.json
go run ./cmd/br-indexer -mode events -manifest testdata/fixtures/sui-packages.stillness.json -max-pages 1
go run ./cmd/br-indexer -mode objects -manifest testdata/fixtures/sui-packages.stillness.json -max-pages 1
go run ./cmd/br-indexer -mode derive-events -module killmail,character,gate,assembly,storage_unit,turret -derive-batch-size 5000 -max-batches 1
go run ./cmd/br-indexer -mode derive-objects -derive-batch-size 1000 -max-batches 1
go run ./cmd/br-indexer -mode resolve-evidence
go run ./cmd/br-indexer -mode audit-killmails -exclude-fixtures -sample-limit 20
go run ./cmd/br-indexer -mode audit-current-state
go run ./cmd/br-indexer -mode audit-character-profiles -sample-limit 20
go run ./cmd/br-indexer -mode audit-tribe-identity-evidence -environment stillness -sample-limit 20
go run ./cmd/br-indexer -mode audit-evidence-bridges -sample-limit 20
go run ./cmd/br-indexer -mode audit-object-shapes -object-type-name Assembly -shape-limit 1000 -sample-limit 5
go run ./cmd/br-indexer -mode status -export-manifest ./exports/manifest.json
```

For a new package window such as Cycle 6, use the package manifest's `startingCheckpoint` first and keep the proof bounded before a full append:
```sh
go run ./cmd/br-indexer -mode plan -manifest testdata/fixtures/sui-packages.stillness.json -package 0x8b8a46ed766fa1358ce7c5c51f6a164b13d627a63e45343f69ed0ba0446c1aa1
go run ./cmd/br-indexer -mode events -manifest testdata/fixtures/sui-packages.stillness.json -package 0x8b8a46ed766fa1358ce7c5c51f6a164b13d627a63e45343f69ed0ba0446c1aa1 -max-pages 5 -concurrency 16
go run ./cmd/br-indexer -mode objects -manifest testdata/fixtures/sui-packages.stillness.json -package 0x8b8a46ed766fa1358ce7c5c51f6a164b13d627a63e45343f69ed0ba0446c1aa1 -max-pages 5 -concurrency 16
go run ./cmd/br-indexer -mode audit-stillness -manifest testdata/fixtures/sui-packages.stillness.json -package 0x8b8a46ed766fa1358ce7c5c51f6a164b13d627a63e45343f69ed0ba0446c1aa1 -max-pages 5
```

For semantic event repair, prefer the module-scoped `derive-events` command above without `-max-batches`. It appends from saved module cursors. Omit `-module` only when intentionally replaying every stored event module including high-volume modules that are not usually needed for killmail and current-state repair.

Use object-shape audits before adding or changing object normalisers. They inspect stored Sui object JSON keys for one package, module, type name or exact Move type at a time. This is useful after a package update because it exposes new fields without promoting them into current-state semantics.

Use `-only-incomplete` when you want a repair pass over missing or errored streams without touching clean saved cursors:
```sh
go run ./cmd/br-indexer -mode events -manifest testdata/fixtures/sui-packages.stillness.json -only-incomplete -max-pages 0 -concurrency 64
go run ./cmd/br-indexer -mode objects -manifest testdata/fixtures/sui-packages.stillness.json -only-incomplete -max-pages 0 -concurrency 64
```

Use a normal run without `-only-incomplete` for a quick append pass across every stream. Saved cursors are still used. A genesis replay requires `-reset-cursors`.

Archive examples:
```sh
go run ./cmd/br-indexer -mode plan -manifest testdata/fixtures/sui-packages.stillness.json -cycles all
go run ./cmd/br-indexer -mode events -manifest testdata/fixtures/sui-packages.stillness.json -cycles 5,6 -only-incomplete -max-pages 0 -concurrency 64
go run ./cmd/br-indexer -mode derive-events -cycles all -module killmail,character,gate,assembly,storage_unit,turret -derive-batch-size 5000
```

Audit Sui object targets that are currently marked as provider-range blocked when you need to show the exact provider-limited object types:
```sh
go run ./cmd/br-indexer -mode audit-range-blocked-objects -manifest testdata/fixtures/sui-packages.stillness.json
```

The normal Cycle 6 repair path is event backfill and module-scoped `derive-events` for public chain state, followed by World API snapshots for tribe names, descriptions and URLs when available and static-client evidence for systems, constellations, regions, enemies, item/type rows and reviewed recipes. Targeted object retry mode remains available when the provider window changes. Complete Sui object coverage still requires explicit coverage evidence.

Use `br-indexer -mode resolve-evidence` when you want to rerun only the owner-capability and location-hash bridge after importing another public source. It promotes `owner_cap_id` and `location_hash` evidence into `owned_by` and `located_in` relations only when the value maps to exactly one public character or system in the same environment.

Docker is optional for the Go test suite. The smoke scripts can use Docker Compose, Podman Compose, `podman-compose` or an existing PostgreSQL database selected with `BR_REGISTRY_CONTAINER_RUNTIME=external` and `DATABASE_URL`.
