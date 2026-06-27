# Changelog

All notable changes to Black Relay Registry are recorded here.

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
