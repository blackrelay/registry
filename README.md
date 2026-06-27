<div align="center">

<img src=".github/banner.png" alt="Black Relay Registry" width="960">

[![Latest Release](https://img.shields.io/github/v/release/blackrelay/registry?label=Release&style=for-the-badge&labelColor=1a0b05&color=fe4700)](https://github.com/blackrelay/registry/releases/latest "Latest Release")
[![Download](https://img.shields.io/badge/Download-latest-fe4700.svg?style=for-the-badge&labelColor=1a0b05)](https://github.com/blackrelay/registry/releases/latest "Download Latest Release")
[![CI](https://img.shields.io/github/actions/workflow/status/blackrelay/registry/ci.yml?branch=main&label=CI&style=for-the-badge&labelColor=1a0b05&color=fe4700)](https://github.com/blackrelay/registry/actions/workflows/ci.yml "CI")
[![CodeQL](https://img.shields.io/github/actions/workflow/status/blackrelay/registry/codeql.yml?branch=main&label=CodeQL&style=for-the-badge&labelColor=1a0b05&color=fe4700)](https://github.com/blackrelay/registry/actions/workflows/codeql.yml "CodeQL")
[![Go](https://img.shields.io/github/go-mod/go-version/blackrelay/registry?style=for-the-badge&labelColor=1a0b05&color=fe4700)](go.mod "Go Version")
[![Licence](https://img.shields.io/badge/Licence-Apache--2.0-fe4700.svg?style=for-the-badge&labelColor=1a0b05)](LICENSE "Apache-2.0 Licence")
[![OpenAPI](https://img.shields.io/badge/OpenAPI-v1-fe4700.svg?style=for-the-badge&labelColor=1a0b05)](openapi/registry.v1.yaml "OpenAPI Contract")
[![Release Signing](https://img.shields.io/badge/Checksums-signed-fe4700.svg?style=for-the-badge&labelColor=1a0b05)](docs/release/README.md "Release Verification")

</div>

Black Relay Registry is a source-aware public EVE Frontier world-data registry. It ingests public chain data, public metadata and Registry-owned static-client decoder artefacts then exposes normalised entities, semantic killmails, source artefacts, sync cursors and freshness state through a Go API backed by PostgreSQL.

Black Relay Registry is an unofficial EVE Frontier community project. EVE Frontier and related marks belong to their respective owners.
Sections:
- [Repository Role](#repository-role)
- [Release Files](#release-files)
- [Quick Proof Path](#quick-proof-path)
- [Common Workflows](#common-workflows)
- [API Examples](#api-examples)
- [Runtime Configuration](#runtime-configuration)
- [Development](#development)
- [Documentation](#documentation)
- [Licence](#licence)

## Repository Role

`registry` owns canonical public world data for Black Relay tools. Other services and sites should consume Registry data instead of inventing item, recipe, site, system, character, tribe, assembly or killmail truth.

Current implemented surfaces:
- Go API, importer and indexer commands.
- PostgreSQL canonical registry schema and migrations.
- Source artefact registry with SHA-256 evidence.
- Native Go static-client decoders for universe, type and production resources with reviewed promotion paths for enemies, types and recipes.
- Sui GraphQL event and object backfill with cursor resume.
- Conservative event/object derivation for characters, tribes, systems, assemblies, gates, storage, turrets, killmails and route relations.
- Resolver and semantic killmail output for player, enemy and unresolved actors.
- Public export generation, verification and local/R2 publication.
- OpenAPI 3.1 contract at [openapi/registry.v1.yaml](openapi/registry.v1.yaml).

PostgreSQL is the working Registry database. Raw source artefacts preserve evidence. Public exports distribute reviewed Registry data to consumers.

## Release Files

Published releases are available from [GitHub Releases](https://github.com/blackrelay/registry/releases/latest). Platform archives include versioned file names, so the README links to the latest release page rather than pretending there are stable archive URLs.

Verification files keep stable names on each release:
- [SHA2-256SUMS](https://github.com/blackrelay/registry/releases/latest/download/SHA2-256SUMS)
- [SHA2-256SUMS.sig](https://github.com/blackrelay/registry/releases/latest/download/SHA2-256SUMS.sig)
- [SHA2-512SUMS](https://github.com/blackrelay/registry/releases/latest/download/SHA2-512SUMS)
- [SHA2-512SUMS.sig](https://github.com/blackrelay/registry/releases/latest/download/SHA2-512SUMS.sig)
- [public.key](https://github.com/blackrelay/registry/releases/latest/download/public.key)

Archive names use this pattern:

Platform | Archive name
:--- | :---
Linux amd64 | `blackrelay-registry_<version>_linux_amd64.tar.gz`
Linux arm64 | `blackrelay-registry_<version>_linux_arm64.tar.gz`
Windows amd64 | `blackrelay-registry_<version>_windows_amd64.zip`
Windows arm64 | `blackrelay-registry_<version>_windows_arm64.zip`
macOS amd64 | `blackrelay-registry_<version>_darwin_amd64.tar.gz`
macOS arm64 | `blackrelay-registry_<version>_darwin_arm64.tar.gz`
FreeBSD amd64 | `blackrelay-registry_<version>_freebsd_amd64.tar.gz`
OpenBSD amd64 | `blackrelay-registry_<version>_openbsd_amd64.tar.gz`
NetBSD amd64 | `blackrelay-registry_<version>_netbsd_amd64.tar.gz`

## Quick Proof Path

Start PostgreSQL:
```sh
docker compose up -d postgres
```

Or use Podman:
```sh
podman compose up -d postgres
```

Or point the tools at an existing PostgreSQL instance:
```sh
export DATABASE_URL="postgres://blackrelay:blackrelay@127.0.0.1:5432/blackrelay_registry?sslmode=disable"
```

Windows:
```powershell
$env:DATABASE_URL = "postgres://blackrelay:blackrelay@127.0.0.1:5432/blackrelay_registry?sslmode=disable"
```

Apply migrations:
```sh
go run ./cmd/br-migrate
```

Import reviewed fixture data:
```sh
go run ./cmd/br-import static-enemies -path testdata/fixtures/static-enemies.reviewed.json
go run ./cmd/br-import tribe-identities -path testdata/fixtures/tribe-identities.reviewed.json
go run ./cmd/br-import world-tribes -path testdata/fixtures/world-tribes.json
go run ./cmd/br-import datahub-types -path testdata/fixtures/datahub-types.json
go run ./cmd/br-import world-systems -path testdata/fixtures/world-systems.json
go run ./cmd/br-import killmail-fixture -path testdata/fixtures/killmail.npc-caird.json
```

Start the API:
```sh
go run ./cmd/br-registry -addr 127.0.0.1:8080
```

Query the semantic NPC killmail fixture:
```sh
curl -fsS "http://127.0.0.1:8080/v1/killmails/killmail:stillness:fixture:caird"
```

Windows:
```powershell
Invoke-RestMethod "http://127.0.0.1:8080/v1/killmails/killmail:stillness:fixture:caird"
```

Expected core result:
```json
{
  "data": {
    "killer": {
      "entityType": "enemy",
      "displayName": "Caird [NPC]",
      "typeId": "92096",
      "confidence": "probable"
    }
  }
}
```

The scripted smoke path runs the local proof path with Docker Compose, Podman Compose or an existing database:
```sh
./scripts/smoke.sh
```

Windows:
```powershell
.\scripts\smoke.ps1
```

## Common Workflows

Most read APIs and indexer/importer commands default to the current cycle. As of this revision, that is Cycle 6. Use `-cycles all` or `-cycles 5,6` only when you deliberately want archive data; Cycle 5 tribe names and profile fields may remain unresolved without a Cycle 5 World API source artefact.

Preview Sui package streams without network writes:
```sh
go run ./cmd/br-indexer -mode plan -manifest testdata/fixtures/sui-packages.stillness.json
```

Append current-cycle events and objects:
```sh
go run ./cmd/br-indexer -mode events -manifest testdata/fixtures/sui-packages.stillness.json -max-pages 1
go run ./cmd/br-indexer -mode objects -manifest testdata/fixtures/sui-packages.stillness.json -max-pages 1
```

Run a deliberate full current-cycle backfill by setting `-max-pages 0`:
```sh
go run ./cmd/br-indexer -mode all -manifest testdata/fixtures/sui-packages.stillness.json -max-pages 0 -concurrency 16
```

Repair missing or retryable interrupted streams:
```sh
go run ./cmd/br-indexer -mode all -manifest testdata/fixtures/sui-packages.stillness.json -max-pages 0 -concurrency 64 -only-incomplete
```

Derive semantic records from stored chain rows:
```sh
go run ./cmd/br-indexer -mode derive-events -module killmail,character,gate,assembly,storage_unit,turret -derive-batch-size 5000
go run ./cmd/br-indexer -mode derive-objects -derive-batch-size 5000
go run ./cmd/br-indexer -mode resolve-evidence
```

Decode and import local static-client evidence.

Registry includes native Go decoders for local EVE Frontier static-client resources. The decode commands read the local client resource index, hash the source resources, write deterministic evidence artefacts and leave promotion to reviewed import commands.
```sh
go run ./cmd/br-import static-client-inspect-types -client-path "/path/to/eve-frontier/stillness"
go run ./cmd/br-import static-client-decode-types -client-path "/path/to/eve-frontier/stillness" -out ./tmp/static-client-types.native-decode.json
go run ./cmd/br-import static-client-extract-types -client-path "/path/to/eve-frontier/stillness" -resolved-json ./tmp/static-client-types-current.json -out ./tmp/static-client-types-all.json
go run ./cmd/br-import static-client-types -path ./tmp/static-client-types-all.json
go run ./cmd/br-import static-client-enemies -path ./tmp/static-client-types-all.json
go run ./cmd/br-import static-client-decode-production -client-path "/path/to/eve-frontier/stillness" -out ./tmp/static-client-production.native-decode.json
go run ./cmd/br-import static-client-recipes -path ./tmp/static-client-recipes.reviewed.json
```

Generate reports and freshness status:
```sh
go run ./cmd/br-indexer -mode report -exclude-fixtures
go run ./cmd/br-indexer -mode audit-killmails -exclude-fixtures -sample-limit 20
go run ./cmd/br-indexer -mode audit-current-state
go run ./cmd/br-indexer -mode audit-range-blocked-objects -manifest testdata/fixtures/sui-packages.stillness.json
go run ./cmd/br-indexer -mode status -export-manifest ./exports/manifest.json
```

Provider-limited Sui object scans can return `Request is outside consistent range`. Registry records that as `range_blocked` coverage rather than treating it as a retryable hard failure. Use event backfill and `derive-events` as the primary chain-derived state path then use World API and static-client imports for public names and static data.

Generate and verify public exports:
```sh
go run ./cmd/br-export -out exports
go run ./cmd/br-export verify -dir exports
```

Publish a verified export into a local object-store-shaped directory:
```sh
go run ./cmd/br-export publish-local -dir exports -root published-exports -prefix registry/current
```

Publish to Cloudflare R2 after configuring R2 credentials:
```sh
go run ./cmd/br-export publish-r2 -dir exports -prefix registry/current
```

## API Examples
Common API calls:
```sh
curl -fsS "http://127.0.0.1:8080/v1/health"
curl -fsS "http://127.0.0.1:8080/v1/entities?type=enemy&environment=stillness"
curl -fsS "http://127.0.0.1:8080/v1/search?q=caird&environment=stillness"
curl -fsS "http://127.0.0.1:8080/v1/current/characters?environment=stillness"
curl -fsS "http://127.0.0.1:8080/v1/current/enemies?environment=stillness&q=mycena"
curl -fsS "http://127.0.0.1:8080/v1/current/recipes?environment=stillness&q=reflex"
curl -fsS "http://127.0.0.1:8080/v1/killmails?environment=stillness&exclude_fixtures=true&limit=20"
curl -fsS "http://127.0.0.1:8080/v1/events?environment=stillness&module=character&limit=10"
curl -fsS "http://127.0.0.1:8080/v1/ops/cursors"
curl -fsS "http://127.0.0.1:8080/v1/ops/sui-coverage"
curl -fsS "http://127.0.0.1:8080/v1/ops/source-gaps?environment=stillness"
curl -fsS "http://127.0.0.1:8080/v1/metrics"
```

Admin routes use Cloudflare Access when `BR_REGISTRY_ACCESS_TEAM_DOMAIN` and `BR_REGISTRY_ACCESS_AUD` are configured. For local development, set `BR_REGISTRY_ADMIN_TOKEN` and send it as a bearer token:
```sh
export BR_REGISTRY_ADMIN_TOKEN="local-dev-token"
curl -fsS -X POST \
  -H "Authorization: Bearer local-dev-token" \
  -H "Content-Type: application/json" \
  -d '{"targetKind":"source_artefact","targetId":"artefact:fixture","notes":"needs review"}' \
  "http://127.0.0.1:8080/v1/admin/imports"
```

Windows:
```powershell
$env:BR_REGISTRY_ADMIN_TOKEN = "local-dev-token"
Invoke-RestMethod `
  -Method Post `
  -Headers @{ Authorization = "Bearer local-dev-token" } `
  -ContentType application/json `
  -Body '{"targetKind":"source_artefact","targetId":"artefact:fixture","notes":"needs review"}' `
  "http://127.0.0.1:8080/v1/admin/imports"
```

## Runtime Configuration

Common environment variables:
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

Equivalent command flags:
```sh
go run ./cmd/br-registry \
  -database-url "postgres://operator:operator@127.0.0.1:5432/operator_registry?sslmode=disable" \
  -artefact-root ./artefacts \
  -registry-id operator-frontier-registry \
  -api-version v1 \
  -access-team-domain "https://your-team.cloudflareaccess.com" \
  -access-aud "your-application-aud-tag" \
  -addr 127.0.0.1:8080
```

Windows:
```powershell
go run ./cmd/br-registry `
  -database-url "postgres://operator:operator@127.0.0.1:5432/operator_registry?sslmode=disable" `
  -artefact-root .\artefacts `
  -registry-id operator-frontier-registry `
  -api-version v1 `
  -access-team-domain "https://your-team.cloudflareaccess.com" `
  -access-aud "your-application-aud-tag" `
  -addr 127.0.0.1:8080
```

## Development

Useful local checks:
```sh
export GOFLAGS="-buildvcs=false"
go test ./...
go vet ./...
go build -buildvcs=false ./cmd/...
go mod verify
go run honnef.co/go/tools/cmd/staticcheck@latest ./...
npx --yes cspell@10.0.1 lint "README.md" "docs/**/*.md" ".github/**/*.md" ".github/**/*.yml" ".github/**/*.yaml" --config .github/config/cspell.json --no-progress --no-summary --no-must-find-files
```

Windows:
```powershell
$env:GOFLAGS = "-buildvcs=false"
go test ./...
go vet ./...
go build -buildvcs=false ./cmd/...
go mod verify
go run honnef.co/go/tools/cmd/staticcheck@latest ./...
npx --yes cspell@10.0.1 lint "README.md" "docs/**/*.md" ".github/**/*.md" ".github/**/*.yml" ".github/**/*.yaml" --config .github/config/cspell.json --no-progress --no-summary --no-must-find-files
```

Format Go files with:
```sh
gofmt -w cmd internal openapi
```

## Documentation
Main references:
- [Documentation Index](docs/README.md): short guide to the flat documentation set.
- [Development](docs/development.md): local commands, PostgreSQL setup, Sui backfill, cycle scope and report commands.
- [API](docs/api.md): endpoint behaviour and response shapes.
- [Architecture](docs/architecture.md): data model and ingestion flow.
- [Source Policy](docs/source-policy.md): source, confidence, privacy and review rules.
- [Static Client Import](docs/static-client-import.md): native static-client decode flow, artefact evidence and reviewed imports.
- [Snapshot JSONL](docs/snapshots-jsonl.md): content-addressed JSONL artefact handling.
- [Cloudflare Distribution](docs/cloudflare.md): R2/export publication model.
- [Run Your Own Registry](docs/run-your-own-registry.md): operator bootstrap with custom database and artefact paths.
- [CI](docs/ci.md): continuous-integration gates and matching local commands.
- [Release](docs/release/README.md): release workflow, signed checksum manifests and signing key setup.
- [Changelog](CHANGELOG.md): notable release changes.

## Licence

Black Relay Registry is available under [Apache-2.0](LICENSE).
