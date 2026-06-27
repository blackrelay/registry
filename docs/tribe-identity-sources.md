# Tribe Identity Sources

Registry treats tribe membership and tribe profile text as separate claims.

Sui character rows can prove a public `tribe_id` or `corpId` membership value. Human-readable tribe name, tag, description and URL fields need their own source evidence.

## Datacore Repo Audit

`beaukode/evedatacore` is useful as a public source-discovery lead. Registry promotion still requires preserved source evidence.

The public repository contains a React frontend, generated API clients and local dApp helpers. It shows that Datacore's private API responses contain tribe fields such as:
- `name`
- `ticker`
- `description`
- `url`
- `memberCount`
- `foundedAt`
- logbook `tribeName` and `tribeTicker` fields

The inspected public repository contains the frontend and generated clients. Tribe-name promotion needs the source that derives those values.

The public MUD helper in that repo reads the `evefrontier__Characters` table and returns `tribeId`. The generated Stillness-style smart-character type includes `corpId`; tribe/corporation name, ticker, description and URL fields need a separate profile source. `EntityRecordMeta` contains `name`, `dappURL` and `description` for entity metadata, and the inspected frontend code uses it for assembly-style metadata rather than as a canonical tribe profile table.

Datacore confirms the shape a good tribe API can expose. Registry uses that as source-discovery context and promotes tribe identity data from preserved public artefacts or reviewed local extraction evidence.

## Acceptable Promotion Paths

Tribe profile text may be promoted when one of these exists:
- a public chain event or object payload that explicitly maps a stable tribe id to a name, tag, description or URL;
- an official public World API or Datahub snapshot with those fields;
- a reviewed local extraction artefact that preserves source evidence;
- a reviewed public community artefact imported with `br-import tribe-identities`.

If the official World API exposes tribe metadata, prefer the direct importer:
```sh
go run ./cmd/br-import world-tribes -url "https://world-api-stillness.live.pub.evefrontier.com/v2/tribes" -snapshot-path ./local-extract/world-tribes.json -environment stillness
```

The current public Stillness tenant host is `world-api-stillness.live.pub.evefrontier.com`. Older documentation may mention `world-api-stillness.live.tech.evefrontier.com`; that host was not resolvable during the 2026-06-27 local import check.

`world-tribes` records the raw JSON snapshot as evidence and imports source-backed tribe facts from rows containing fields such as `tribeId`, `name`, `ticker`, `description`, `url`, `memberCount` and `foundedAt`. It also accepts corporation-style aliases such as `corpId`, `corpName` and `corpTicker` because some public tools and SDK types use that vocabulary for tribe membership.

Membership-only rows stay useful. They can power `belongs_to` relations and current-state filtering, while tribe display names remain placeholders such as `Tribe 42` until profile evidence is imported.

## Audit Command

Use the tribe identity evidence audit to inspect stored public Sui payloads before creating a reviewed tribe identity artefact:
```sh
go run ./cmd/br-indexer -mode audit-tribe-identity-evidence -environment stillness -sample-limit 20
```

Optional filters:
```sh
go run ./cmd/br-indexer -mode audit-tribe-identity-evidence -environment stillness -module character
go run ./cmd/br-indexer -mode audit-tribe-identity-evidence -environment stillness -object-type-name PlayerProfile
```

The audit reports:
- rows scanned from stored Sui events and objects;
- rows with only membership/id evidence;
- rows with candidate tribe name, ticker, description or URL evidence;
- sample source rows and matched payload keys.

Candidate rows still need review before promotion. If the audit only finds `tribe_id` or `corpId`, keep tribe names review-gated and use:
```sh
go run ./cmd/br-import tribe-identities -path ./local-extract/tribe-identities.reviewed.json -environment stillness
```
