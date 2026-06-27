# Run Your Own Registry

Black Relay Registry is built so another operator can run the same data spine with different configuration, source snapshots and public exports. The same source, confidence, review and provenance rules apply to every instance id.

## Prerequisites
- Go matching the module toolchain.
- PostgreSQL.
- A local artefact root.
- Optional Docker Compose, Podman Compose or an existing PostgreSQL service.
- Optional local EVE Frontier static-client files when decoding static resources and importing reviewed static records.

Do not put wallet seeds, private keys, mnemonics, game credentials, Discord exports or private tribe intelligence in the Registry. Import public source evidence only.

## Configuration

Set the database, artefact root and response identity for your deployment:
```sh
export DATABASE_URL="postgres://operator:operator@127.0.0.1:5432/operator_registry?sslmode=disable"
export BR_REGISTRY_ARTEFACT_ROOT="./artefacts"
export BR_REGISTRY_INSTANCE_ID="operator-frontier-registry"
export BR_REGISTRY_API_VERSION="v1"
```

The instance id appears in `meta.registry` and export metadata. It identifies your deployment. Source ids and confidence labels carry source authority.

## Bootstrap

Apply migrations and start with a small proof path:
```sh
go run ./cmd/br-migrate
go run ./cmd/br-import static-enemies -path testdata/fixtures/static-enemies.reviewed.json -environment stillness
go run ./cmd/br-import tribe-identities -path ./local-extract/tribe-identities.reviewed.json -environment stillness
go run ./cmd/br-import killmail-fixture -path testdata/fixtures/killmail.npc-caird.json
go run ./cmd/br-registry -addr 127.0.0.1:8080
```

Skip the tribe identity import until you have a reviewed public artefact or an official World API snapshot that maps stable tribe ids to names, tags, aliases, descriptions or URLs. Sui rows prove `tribe_id` membership, not tribe profile text by themselves.

If the World API exposes tribe profiles, prefer importing that raw public snapshot:
```sh
go run ./cmd/br-import world-tribes -url "https://world-api-stillness.live.pub.evefrontier.com/v2/tribes" -snapshot-path ./local-extract/world-tribes.json -environment stillness
```

In another shell:
```sh
curl -fsS http://127.0.0.1:8080/v1/health
curl -fsS http://127.0.0.1:8080/v1/killmails/killmail:stillness:fixture:caird
```

## Static-Client Decode Evidence

When you have local client files, record deterministic native evidence before importing reviewed rows:
```sh
go run ./cmd/br-import static-client-inspect-types -client-path "/path/to/eve-frontier/stillness"
go run ./cmd/br-import static-client-decode-types -client-path "/path/to/eve-frontier/stillness" -out ./tmp/static-client-types.native-decode.json
go run ./cmd/br-import static-client-compare-types -resolved ./tmp/static-client-types-all.json -native ./tmp/static-client-types.native-decode.json
go run ./cmd/br-import static-client-decode-production -client-path "/path/to/eve-frontier/stillness" -out ./tmp/static-client-production.native-decode.json
```

The native decoder covers localisation-backed static type rows and candidate production rows. Production decode output includes blueprint primary rows, derived recipe candidates and `typematerials` material rows; treat it as review evidence before importing canonical recipe records through the reviewed JSON recipe importer.

## Sui Backfill

Plan before indexing:
```sh
go run ./cmd/br-indexer -mode plan -manifest testdata/fixtures/sui-packages.stillness.json
```

Run bounded proofs first:
```sh
go run ./cmd/br-indexer -mode events -manifest testdata/fixtures/sui-packages.stillness.json -max-pages 5 -concurrency 16
go run ./cmd/br-indexer -mode objects -manifest testdata/fixtures/sui-packages.stillness.json -max-pages 5 -concurrency 16
go run ./cmd/br-indexer -mode derive-events -module killmail,character,gate,assembly,storage_unit,turret,rift -derive-batch-size 5000
go run ./cmd/br-indexer -mode derive-objects -derive-batch-size 5000
go run ./cmd/br-indexer -mode resolve-evidence
```

Use `-max-pages 0` only for a deliberate full append from saved cursors. If the public Sui GraphQL endpoint reports broad object-by-type scans as outside its consistent range, the Registry records that as a provider-limited source gap. Keep the normal repair path event-first and source-backed:
```sh
go run ./cmd/br-indexer -mode derive-events -module killmail,character,gate,assembly,storage_unit,turret,rift -derive-batch-size 5000
go run ./cmd/br-indexer -mode resolve-evidence
go run ./cmd/br-indexer -mode audit-range-blocked-objects -manifest testdata/fixtures/sui-packages.stillness.json
```

Use World API snapshots for tribe and system metadata when available. Use native static-client decoder artefacts for universe data, enemies, type rows and reviewed recipes. Broad Sui object scans are enrichment evidence for deployments that can query the provider window reliably.

## Public Exports

Generate and verify distribution files:
```sh
go run ./cmd/br-export -out exports
go run ./cmd/br-export verify -dir exports
```

The default export scope is the current cycle plus unlabelled rows. Use `-cycles current` for a strict current-cycle bundle, `-cycles all` for an archive bundle or `-cycles 5,6` when you deliberately want both Cycle 5 and Cycle 6 records in one distribution snapshot:
```sh
go run ./cmd/br-export -out exports-current
go run ./cmd/br-export -out exports-all -cycles all
```

Publish locally or to R2 only after verification:
```sh
go run ./cmd/br-export publish-local -dir exports -root published-exports -prefix registry/current
go run ./cmd/br-export publish-local -dir exports-all -root published-exports -prefix registry/archive/all
./scripts/verify-publication.sh --root published-exports --prefix registry/current,registry/archive/all
```

Use separate prefixes for current and archive exports. The current pointer lives at `registry/current/latest/manifest.json`; the archive pointer lives at `registry/archive/all/latest/manifest.json`. Each pointer references immutable bundle files under its own `bundles/<bundle-id>/` directory.

R2 publishing uses S3-compatible credentials. Do not put those credentials in source control.

## Operations Checks

Use these endpoints and commands to decide what still needs work:
```sh
curl -fsS http://127.0.0.1:8080/v1/ops/cursors
curl -fsS http://127.0.0.1:8080/v1/ops/sui-coverage
curl -fsS "http://127.0.0.1:8080/v1/ops/source-gaps?environment=stillness"
go run ./cmd/br-indexer -mode report -exclude-fixtures
go run ./cmd/br-indexer -mode audit-current-state
go run ./cmd/br-indexer -mode audit-killmails -exclude-fixtures -sample-limit 20
go run ./cmd/br-indexer -mode audit-tribe-identity-evidence -environment stillness -sample-limit 20
```

Source gaps include `suggestedCommands` but they are operator hints. Review the source evidence and blast radius before running a broad import or full backfill.
