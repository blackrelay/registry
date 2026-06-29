# API

The public API is versioned under `/v1`.

Response metadata includes the registry instance id configured by the operator. Black Relay uses `black-relay-registry` by default but another deployment should set `BR_REGISTRY_INSTANCE_ID` or `br-registry -registry-id` to its own stable value.

List, search, current-state, event, killmail and source-gap endpoints default to the Stillness environment when `environment` is omitted. Pass `environment=utopia` only when querying explicitly imported archive data for that environment.

Implemented endpoints:
```text
GET /v1/health
GET /v1/ready
GET /v1/metrics
GET /metrics
GET /v1/search
GET /v1/entities
GET /v1/entities/{idOrSlug}
GET /v1/entities/{idOrSlug}/facts
GET /v1/entities/{idOrSlug}/relations
GET /v1/entities/{idOrSlug}/sources
GET /v1/entities/{idOrSlug}/history
GET /v1/types
GET /v1/types/{typeID}
GET /v1/current/characters
GET /v1/current/tribes
GET /v1/current/assemblies
GET /v1/current/gates
GET /v1/current/storage
GET /v1/current/turrets
GET /v1/current/regions
GET /v1/current/constellations
GET /v1/current/items
GET /v1/current/materials
GET /v1/current/enemies
GET /v1/current/recipes
GET /v1/current/blueprints
GET /v1/current/ships
GET /v1/current/structures
GET /v1/current/systems
GET /v1/current/routes
GET /v1/current/ownership
GET /v1/current/route-edges
GET /v1/events
GET /v1/events/{id}
GET /v1/killmails
GET /v1/killmails/{id}
GET /v1/killmails/{id}/raw
GET /v1/systems
GET /v1/systems/{idOrSlug}
GET /v1/characters
GET /v1/characters/{idOrSlug}
GET /v1/tribes
GET /v1/tribes/{idOrSlug}
GET /v1/assemblies
GET /v1/assemblies/{idOrSlug}
GET /v1/gates
GET /v1/gates/{idOrSlug}
GET /v1/regions
GET /v1/regions/{idOrSlug}
GET /v1/constellations
GET /v1/constellations/{idOrSlug}
GET /v1/items
GET /v1/items/{idOrSlug}
GET /v1/materials
GET /v1/materials/{idOrSlug}
GET /v1/enemies
GET /v1/enemies/{idOrSlug}
GET /v1/recipes
GET /v1/recipes/{idOrSlug}
GET /v1/blueprints
GET /v1/blueprints/{idOrSlug}
GET /v1/ships
GET /v1/ships/{idOrSlug}
GET /v1/structures
GET /v1/structures/{idOrSlug}
GET /v1/sources/{id}
GET /v1/artefacts/{id}
GET /v1/ops/freshness
GET /v1/ops/cursors
GET /v1/ops/sui-coverage
GET /v1/ops/source-gaps
POST /v1/admin/imports
GET /v1/admin/reviews
POST /v1/admin/static-enemies/import
POST /v1/admin/reviews/{id}/publish
POST /v1/admin/reviews/{id}/reject
```

Admin endpoints support two auth modes:
- Cloudflare Access in production, configured with `BR_REGISTRY_ACCESS_TEAM_DOMAIN` and `BR_REGISTRY_ACCESS_AUD`. The API validates the `Cf-Access-Jwt-Assertion` JWT signature, issuer and application AUD tag before allowing admin routes.
- Local bearer token auth for development, configured with `BR_REGISTRY_ADMIN_TOKEN` and sent through the protocol `Authorization` header.

Errors use this shape:
```json
{
  "error": {
    "code": "not_found",
    "message": "Entity not found."
  },
  "meta": {
    "registry": "black-relay-registry",
    "apiVersion": "v1"
  }
}
```

`meta.registry` identifies the running registry instance that produced the response.

OpenAPI is maintained at:
```text
openapi/registry.v1.yaml
```

Copyable consumer response examples are maintained under:
```text
testdata/examples
```

Those fixtures cover the common read shapes for current entities, semantic killmails, operations coverage, source gaps and public export publication.

`/v1/ops/sui-coverage` includes `range_blocked` targets for Sui object cursors where the public GraphQL endpoint reports `Request is outside consistent range`. These provider-window limits are reported as source-gap evidence while event derivation, World API imports and static-client imports remain the primary repair paths.

## Cycle Scope
List, search, current-state, event and killmail endpoints default to the current cycle. The current cycle is Cycle 6, which began at `2026-06-25T09:00:00Z`. Unlabelled compatibility rows and older cycles are not part of the public current scope.

Use `cycles=current` or `cycles=6` for strict Cycle 6 only. Other cycle values are rejected. Older cycle support has been removed from public reads because the Registry does not have complete World API source artefacts for those public identity fields.

Examples:
```sh
curl -fsS "http://127.0.0.1:8080/v1/current/tribes?environment=stillness"
curl -fsS "http://127.0.0.1:8080/v1/events?environment=stillness&cycles=6&module=character"
```

## Search
`GET /v1/search` is a public search view over source-backed entities. It accepts:
```text
q
type
environment
cycles
type_id
group_id
source_artefact_id
static_entity_type
limit
cursor
```

The implementation uses the same keyset pagination contract as `/v1/entities`.

Default public search and entity-list reads suppress raw tribe placeholder rows, `NPC Corp <id>` rows and pre-cycle public tribe ids. Those source rows remain in the Registry evidence graph but they are not part of the public Cycle 6 discovery surface.

The exact fact filters are intended for static-client type metadata. `type_id` and `group_id` match numeric static-client facts as strings, `source_artefact_id` narrows results to one registered artefact and `static_entity_type` matches the normalised entity type derived from static rows.

## Entity Provenance
Entity detail records are intentionally not the whole story. Use the provenance routes to inspect what supports a record:
```text
GET /v1/entities/{idOrSlug}/facts
GET /v1/entities/{idOrSlug}/relations
GET /v1/entities/{idOrSlug}/sources
GET /v1/entities/{idOrSlug}/history
```

`history` returns the entity, current facts, connected relations and contributing sources in one response.

## Static Type API
`GET /v1/types` is a convenience view over static-client-backed type entities. It accepts the same entity filters as `/v1/search` and `/v1/entities` including exact static-client fact filters:
```text
q
type
environment
cycles
type_id
group_id
category_id
market_group_id
wreck_type_id
source_artefact_id
static_entity_type
limit
cursor
```

`GET /v1/types/{typeID}` resolves one static-client `type_id` with optional `environment`, `type`, `group_id`, `category_id`, `market_group_id`, `wreck_type_id`, `source_artefact_id` and `static_entity_type` filters.

Examples:
```sh
curl -fsS "http://127.0.0.1:8080/v1/types?environment=stillness&group_id=5130&wreck_type_id=81610&static_entity_type=enemy"
curl -fsS "http://127.0.0.1:8080/v1/types/94167?environment=stillness&static_entity_type=enemy"
```

## Current State
The current-state routes expose the current normalised read model without changing the compact entity-list routes.
```text
GET /v1/current/characters
GET /v1/current/tribes
GET /v1/current/assemblies
GET /v1/current/gates
GET /v1/current/storage
GET /v1/current/turrets
GET /v1/current/regions
GET /v1/current/constellations
GET /v1/current/items
GET /v1/current/materials
GET /v1/current/enemies
GET /v1/current/recipes
GET /v1/current/blueprints
GET /v1/current/ships
GET /v1/current/structures
GET /v1/current/systems
GET /v1/current/routes
```

Each current entity includes the compact entity record, latest facts, outgoing relations, incoming relations, source ids and a small `derived` summary when the graph has enough public data. The derived summary can include a `profile` object, tribe, owner, system, evidence-only owner capability, evidence-only location hash, member count, owned-object count, connected-system count, route-edge count, killmail count and public-activity count. `profile` exposes public metadata facts such as `metadataName`, `metadataDescription` and `metadataUrl`, plus reviewed tribe profile facts such as `tag`, `aliases`, `description` and `url`. Embedded relations include subject/object entity ids, entity types and display names so clients can render current state without immediate follow-up entity lookups. The relation feeds expose graph edges directly:
```text
GET /v1/current/ownership
GET /v1/current/route-edges
```

`ownership` returns `owned_by` edges. `route-edges` returns `links_to` and `observed_between` edges. All current-state routes accept `environment`, `cycles`, `limit` and `cursor`. Cycle-scoped `/v1/current/characters` returns event-backed current characters; use `profile=known` to narrow the response to rows with public profile metadata or `profile=placeholder` for diagnostics. Cycle-scoped `/v1/current/tribes` returns public Cycle 6 player tribe profiles and excludes raw `Tribe <id>` placeholders, `NPC Corp <id>` rows and pre-cycle player tribe ids.

Additional current-state filters:
```text
q              text search over current entity id, slug, name and summary
cycles         current or 6
source_id      contributing source id recorded on current facts or relation edges
profile        known or placeholder, based on sourced profile facts versus generated placeholder names
tribe          character `belongs_to` tribe entity id
owner          assembly/gate/storage/turret `owned_by` entity id
owner_cap      assembly/gate/storage/turret `owner_cap_id` evidence value
location_hash  assembly/gate/storage/turret `location_hash` evidence value
system         entity, route edge or static-universe parent connected to a system entity id
connected_to   system, gate or route connected to another system entity id
has_activity   true or false, based on public activity relations
has_tribe      true or false, based on a derived `belongs_to` tribe relation
has_owner_cap  true or false, based on owner-capability evidence
has_location_hash true or false, based on location-hash evidence
has_resolved_owner true or false, based on promoted `owned_by` relations
has_resolved_system true or false, based on promoted system-location relations
```

Examples:
```sh
curl -fsS "http://127.0.0.1:8080/v1/current/characters?environment=stillness&tribe=tribe:stillness:black-relay&q=tao&has_activity=true"
curl -fsS "http://127.0.0.1:8080/v1/current/characters?environment=stillness&profile=placeholder"
curl -fsS "http://127.0.0.1:8080/v1/current/characters?environment=stillness&source_id=source:sui:stillness:character"
curl -fsS "http://127.0.0.1:8080/v1/current/assemblies?environment=stillness&owner=character:stillness:2112091476&system=system:stillness:30001001"
curl -fsS "http://127.0.0.1:8080/v1/current/assemblies?environment=stillness&owner_cap=0xcap&location_hash=loc-1"
curl -fsS "http://127.0.0.1:8080/v1/current/assemblies?environment=stillness&has_owner_cap=true&has_resolved_owner=false"
curl -fsS "http://127.0.0.1:8080/v1/current/regions?environment=stillness&q=inner"
curl -fsS "http://127.0.0.1:8080/v1/current/constellations?environment=stillness&q=inner"
curl -fsS "http://127.0.0.1:8080/v1/current/items?environment=stillness&q=reflex"
curl -fsS "http://127.0.0.1:8080/v1/current/enemies?environment=stillness&q=mycena"
curl -fsS "http://127.0.0.1:8080/v1/current/recipes?environment=stillness&q=reflex"
curl -fsS "http://127.0.0.1:8080/v1/current/systems?environment=stillness&connected_to=system:stillness:30001002&has_activity=true"
curl -fsS "http://127.0.0.1:8080/v1/current/route-edges?environment=stillness&system=system:stillness:30001001&source_id=source:static-universe:stillness"
```

## Killmail Filters

`GET /v1/killmails` returns semantic killmail records ordered by event time. Each semantic killmail includes `summaryText`; enemy killers are marked with `killer.isNpc`.
```text
environment
cycles
system
victim
killer
killer_type_id
reporter
npc
from
to
exclude_fixtures
limit
cursor
```

`cycles` scopes killmails by the event timestamp-derived cycle. `from` and `to` are inclusive RFC3339 timestamps. `npc=true` returns NPC/enemy killmails when an explicit `killer_type_id` or sourced enemy `killer` relation is present; `npc=false` returns player-character killmails. A raw on-chain `killer_id` is treated as a tenant/item character or object identity. `exclude_fixtures=true` removes local proof fixtures from list results while leaving raw fixture records available by id.

Examples:
```sh
curl -fsS "http://127.0.0.1:8080/v1/killmails?environment=stillness&system=system:stillness:30001001&npc=true"
curl -fsS "http://127.0.0.1:8080/v1/killmails?environment=stillness&killer=character:stillness:2112091476&npc=false"
curl -fsS "http://127.0.0.1:8080/v1/killmails?environment=stillness&exclude_fixtures=true&limit=20"
```

## Event Filters

`GET /v1/events` is ordered by event time and uses keyset pagination. It supports:
```text
kind
environment
cycles
cycle
package_id
module
transaction_digest
source_id
limit
cursor
```

Example:
```sh
curl -fsS "http://127.0.0.1:8080/v1/events?environment=stillness&module=character&limit=10"
curl -fsS "http://127.0.0.1:8080/v1/events?environment=stillness&cycles=6&limit=10"
```

## Operations Source Gaps

`GET /v1/ops/source-gaps` returns source and resolver gaps that affect semantic completeness. Each row includes:
```text
kind
category
severity
count
summary
recommendedAction
suggestedCommands
```

`category` groups the repair path. Current categories are `resolver_missing`, `provider_blocked`, `static_data_missing`, `source_missing` and `decoder_review_required`. `suggestedCommands` is an operator hint only; clients should key on `kind` and `category`, not on exact command text.

Example:
```sh
curl -fsS "http://127.0.0.1:8080/v1/ops/source-gaps?environment=stillness"
```

## Typed Collections

The typed collection routes are thin, type-safe views over `/v1/entities`.
```text
/v1/characters
/v1/tribes
/v1/systems
/v1/assemblies
/v1/gates
/v1/regions
/v1/constellations
/v1/items
/v1/materials
/v1/enemies
/v1/recipes
/v1/blueprints
/v1/ships
/v1/structures
```

They accept `q`, `environment`, `limit` and `cursor`. Static-backed collections also accept the exact fact filters that make sense for the domain, such as `type_id`, `group_id`, `wreck_type_id` and `source_artefact_id`. Detail routes under these prefixes return `404` when the entity exists but has the wrong type.

Examples:
```sh
curl -fsS "http://127.0.0.1:8080/v1/regions?environment=stillness&q=inner"
curl -fsS "http://127.0.0.1:8080/v1/items?environment=stillness&q=reflex"
curl -fsS "http://127.0.0.1:8080/v1/enemies?environment=stillness&group_id=5130&wreck_type_id=81610"
curl -fsS "http://127.0.0.1:8080/v1/blueprints?environment=stillness&type_id=75001"
curl -fsS "http://127.0.0.1:8080/v1/ships?environment=stillness&limit=20"
```
