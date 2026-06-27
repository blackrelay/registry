# Source Policy

Registry data is public-game data only. Accepted records are public, source-backed and reviewable. Sensitive material stays outside the Registry. This includes wallet seeds, mnemonics, private keys, game credentials, private Discord messages, private tribe intelligence, real-world identity data, doxxing material and live sensitive route intelligence.

Allowed sources:
- public Sui chain data
- public World API data
- public Datahub data
- official documentation
- reviewed static-client evidence
- labelled observed gameplay
- labelled community reports

Sui character rows prove public tribe membership by `tribe_id`. Human-readable tribe names, tags, descriptions and URLs require a reviewed public tribe-identity artefact or another source that explicitly contains that identity claim. Chain-derived placeholders such as `Tribe 42` are temporary ids that yield to reviewed names.

Other community sites resolving tribe names can help identify possible sources. Registry promotion still requires a public artefact, a reviewed local extraction or another source that can be preserved as evidence. Datacore, EF-map and similar private APIs remain product references and source-discovery context only.

The public Datacore frontend repository confirms that tribe API responses can contain names, tickers, descriptions, URLs and membership counts. The inspected code is frontend/generated-client code, so Registry treats it as source-discovery context. See [Tribe Identity Sources](tribe-identity-sources.md).

The contract fixture at `testdata/fixtures/tribe-identities.reviewed.json` shows the reviewed artefact shape with example data.

The public EVE Frontier contracts and SDK expose `name`, `description` and `url` inside character and assembly-style metadata. Those fields may be recorded from public Sui object or event evidence for the object they belong to. Tribe profile metadata requires a source that explicitly maps those fields to a tribe id.

Reviewed tribe identity artefacts use this shape:
```json
{
  "schemaVersion": "registry.tribe-identities.v1",
  "environment": "stillness",
  "source": {
    "kind": "community_report",
    "confidence": "reported",
    "title": "Reviewed public tribe identity list",
    "locator": "operator-reviewed-public-list",
    "checkedAt": "2026-06-26T00:00:00Z",
    "reviewStatus": "reviewed"
  },
  "tribes": [
    {
      "tribeId": "42",
      "name": "Example Relay",
      "tag": "ER",
      "aliases": ["Relay Example"],
      "description": "Example public tribe profile",
      "url": "https://example.invalid/tribes/example-relay",
      "confidence": "reported",
      "sourceContext": "reviewed public profile"
    }
  ]
}
```

Import with:
```sh
go run ./cmd/br-import tribe-identities -path ./local-extract/tribe-identities.reviewed.json -environment stillness
```

When the official public World API exposes tribe metadata, use the direct World API importer instead of a hand-written reviewed envelope:
```sh
go run ./cmd/br-import world-tribes -url "https://world-api-stillness.live.pub.evefrontier.com/v2/tribes" -snapshot-path ./local-extract/world-tribes.json -environment stillness
```

`world-tribes` accepts raw arrays or envelopes containing `tribes`, `items`, `data` or `rows`. It records the snapshot as a `world_api` source artefact and imports rows with fields such as `tribeId`, `name`, `ticker`, `description`, `url`, `memberCount` and `foundedAt`.

Before writing a reviewed identity artefact, inspect stored public Sui payloads for native tribe profile evidence:
```sh
go run ./cmd/br-indexer -mode audit-tribe-identity-evidence -environment stillness -sample-limit 20
```

Rejected source categories:
- Datacore API scraping
- private hosts
- authenticated private endpoints
- Discord session data
- unsourced claims promoted as fact

Timestamped public Sui rows can prove a cycle through the Registry cycle boundary normaliser. Manual, observed and community records stay labelled and reviewed. Cycle fields remain null until a source proves the cycle.
