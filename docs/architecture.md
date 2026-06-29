# Architecture

Black Relay Registry is Go-first and PostgreSQL-backed.
```text
Go API / importer / indexer
        -> PostgreSQL canonical registry database
        -> local source artefact store
        -> export/read models
```

PostgreSQL is the canonical backend for the rewrite. D1, KV and R2 are not part of the canonical write model. R2 is used only as an export and source-artefact distribution target.

The current implementation focus is the indexer and canonical data pipeline. Public web surfaces are consumers, not separate sources of truth. They must consume Registry data rather than re-declaring item, recipe, system, tribe, character, assembly or killmail truth.

## Main Packages
- `cmd/br-registry`: HTTP API.
- `cmd/br-import`: static-client and fixture importer.
- `cmd/br-indexer`: Sui package manifest planning, event backfill, object backfill and replay derivation.
- `cmd/br-export`: compact public JSONL export, with opt-in raw event/object JSONL exports and checksum manifests.
- `internal/api`: HTTP routing and response envelopes.
- `internal/artefacts`: local artefact registration and SHA-256 hashing.
- `internal/cycles`: UTC cycle boundary normalisation for timestamped source rows.
- `internal/db`: PostgreSQL repository, migrations and in-memory test store.
- `internal/importer`: contract-first import flows.
- `internal/killmail`: semantic killmail assembly.
- `internal/resolver`: ID/name/type resolver.
- `internal/snapshots`: JSONL artefact normalisation, semantic diffs and snapshot promotion.
- `internal/staticdata`: reviewed enemy candidate knowledge.
- `internal/sui`: Sui package manifest model, GraphQL client, event/object normalisation, cursor-backed backfill runners and conservative object derivation.

## Database

`migrations/0001_init.sql` creates:
- source, artefact, import, review and ingest tables
- entities, aliases, facts and relations
- raw events and killmails
- snapshot diffs
- sync cursors
- current-state views
- PostgreSQL full-text search terms

`migrations/0002_snapshots_jsonl.sql` adds:
- snapshot sets and snapshot-to-artefact links
- normalised snapshot rows
- source artefact patch metadata and supersession links
- append-only outbox jobs
- semantic snapshot diff fields

`migrations/0003_sui_indexer.sql` adds event lookup indexes for package, module, transaction and source filtering.

`migrations/0004_sui_objects.sql` adds raw Sui object storage keyed by observed object version. Raw object rows are source evidence and are not deleted when derived entity records change.

`migrations/0008_cycle_normalisation.sql` adds nullable cycle storage for raw events and indexes environment/cycle/event-time queries.

`migrations/0009_backfill_event_cycles.sql` originally backfilled raw event cycles from `occurred_at`. Current public normalisation only emits supported Cycle 6 labels.

`migrations/0010_remove_pre_cycle5_labels.sql` removes pre-Cycle-5 labels from raw events, entities and facts. The current Sui-backed corpus has no Cycle 4 event rows, so earlier cycle labels are left null until a separate historical source model needs them.

List endpoints use keyset cursors. Offset pagination is deliberately not part of the public API.

## Sui Indexing

`br-indexer -mode plan` expands package manifests into event stream and object-type targets without making network calls.

`br-indexer -mode events` reads Sui GraphQL event pages, normalises each Move event into the `events` table and saves one opaque cursor per `network + role + package + module` stream. Cursors are stored in `sync_cursors` and are used exactly as returned by Sui GraphQL.

`br-indexer -mode objects` reads Sui GraphQL object pages by full Move type, normalises each object into the `sui_objects` table and saves one opaque cursor per `network + role + full type` stream. The manifest carries known world object types for `character`, `assembly`, `gate`, `storage_unit`, `turret`, `network_node` and `killmail`. Use `-object-type` for full Move types that are discovered outside the manifest.

`br-indexer -mode all` runs event and object backfill in the same process. Domain-record derivation remains an explicit replay step.

`br-indexer -mode derive-objects` replays stored Sui object rows into conservative source-backed entity records. It currently derives character, assembly, gate, storage, turret and killmail entities where the Move type clearly maps to a Registry domain. Unsupported object types remain raw evidence in `sui_objects` rather than becoming invented Registry truth.

Infrastructure objects can expose `owner_cap_id` and `location_hash` without exposing a direct owner character, tribe or solar-system id. Registry stores those as `resource_object` evidence and `has_owner_cap` / `has_location_hash` relations. It only promotes them to `owned_by` or `located_in` when the evidence value also appears on exactly one public character or system entity in the same environment. Ambiguous values stay as evidence-only relations.

Current Sui killmail objects expose `killer_id`, `victim_id`, `reported_by_character_id` and `solar_system_id` as tenant/item keys. Broad NPC killer labels require a public mapping source that links those chain values to static-client enemy type ids.

`br-indexer -mode derive-events` replays stored Sui event rows into conservative entities, relations and raw killmail records. Use `-module killmail,character,gate,assembly,storage_unit,turret,rift` for the normal semantic repair path. Module-scoped derivation saves a separate cursor for each module and batches page writes into PostgreSQL so append runs avoid replaying high-volume streams such as `network_node`.

`br-indexer -mode repair-character-objects` fetches character objects directly by object ID when those IDs are already referenced by Cycle 6 character events. This is the normal character metadata repair path when broad Sui object-by-type scans are outside the provider's consistent range.

`br-indexer -mode audit-tribe-identity-evidence` scans stored Sui event and object payloads for explicit tribe profile fields. It separates membership-only `tribe_id` or `corpId` evidence from rows that actually contain tribe names, tickers, descriptions or URLs. Candidate profile rows still require review before promotion through `br-import tribe-identities`.

Character event derivation records public metadata `name`, `description` and `url` facts when the event payload exposes them. The metadata name is used as the character display name for that derivation row. Placeholder identities such as `Character <id>`, `Tribe <id>` and `System <id>` are temporary labels that yield to imported or reviewed public display names for the same entity.

The indexer records raw event/object JSON and chain metadata first. Domain-specific derivation can be replayed from stored rows rather than hidden inside network fetching.

Cycle values on raw Sui events are derived from the event timestamp for supported public boundaries. Event-derived entities and facts inherit that cycle. Object-derived entities and facts use the object observation time. Cycle values remain null until the source row proves a supported boundary.
