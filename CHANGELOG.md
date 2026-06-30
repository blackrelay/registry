# Changelog

All notable changes to Black Relay Registry are recorded here.

## [1.1.0] - 2026-06-30

### Changed
- Enforced strict Cycle 6 public scope for current exports, search and typed public reads.
- Removed legacy Cycle 5 and `all` cycle options from the supported public Registry read path.
- Limited current public character rows to event-backed Cycle 6 character evidence.
- Limited current public tribe rows to source-backed Cycle 6 public tribe profiles and the known Clonebank identity.
- Aligned public export filtering with the API current-state filtering rules so R2 and D1 consumers receive the same public scope.

### Fixed
- Repaired event-referenced character objects so Cycle 6 character metadata can be recovered without relying on broad object-by-type scans.
- Preserved current character item IDs while merging repaired object-backed metadata.
- Filtered stale object-only character rows out of public current character counts and search results.
- Filtered placeholder tribe rows, NPC corporation rows and pre-cycle public tribe IDs out of public current tribe results.
- Deduplicated equivalent current tribe identities such as `tribe:stillness:<id>` and raw `<id>` rows.
- Ensured current-cycle exports do not reintroduce older-cycle rows after API or D1 rebuilds.

### Operations
- Added the expected post-upgrade repair path: backfill events, derive current event semantics, replay event-referenced character objects then generate and verify current exports.
- Documented that D1 public API stores should be rebuilt from a verified current export after this upgrade because incremental raw syncs cannot repair earlier loose-scope current rows.

### Known Limits
- World API tribe profile evidence can lag chain events.
- Sui object-by-type scans remain subject to upstream provider range windows.
- Rows without enough public source evidence remain canonical PostgreSQL evidence rather than resolved public current identities.

## [1.0.1] - 2026-06-29

### Fixed
- Corrected release metadata after the first public release flow.

## [1.0.0] - 2026-06-27

### Added
- PostgreSQL-backed Registry schema and migrations for source-aware public EVE Frontier world data.
- Go command set: `br-registry`, `br-migrate`, `br-import`, `br-indexer` and `br-export`.
- Public `/v1` API for health, readiness, search, entities, current state, source events, killmails, operations state and admin review entries.
- OpenAPI 3.1 contract at `openapi/registry.v1.yaml`.
- JSON contract schemas for exported facts, entities, killmails, sources and export manifests.
- Source artefact model with SHA-256 evidence, reviewed static-client imports and JSONL snapshot comparison.
- Static-client import support for reviewed enemy data, universe rows, type rows and recipe evidence.
- Sui GraphQL event and object backfill with resumable cursors, current-cycle defaults and range-blocked coverage reporting.
- Conservative derivation for characters, tribes, systems, assemblies, gates, storage, turrets, killmails and route relations.
- Semantic killmail output for player, enemy and unresolved actors.
- Public export generation, verification and local/R2 publication commands.
- Cloudflare-oriented distribution documentation for API, exports and R2 publication.
- CI coverage for Linux, Windows, macOS arm64, FreeBSD, OpenBSD and NetBSD.
- Release workflow with multi-platform archives, signed checksum manifests and release PGP key export.

### Security
- Release manifests are signed with a dedicated release PGP key.
- Admin routes support Cloudflare Access JWT validation and local bearer-token development mode.
- Private key material is documented as local-only and excluded from the Registry repository.

### Known Limits
- Historical cycle data may include unresolved names when matching public World API or static-client artefacts are unavailable.
- Full Sui object coverage depends on upstream provider range availability.
- Release artefact signing covers checksum manifests, not binary notarisation or platform package signing.
