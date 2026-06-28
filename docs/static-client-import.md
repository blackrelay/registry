# Static-Client Import

Registry includes native Go decoders for local EVE Frontier static-client resources. The decoders are based on reverse-engineered local static-resource layouts and are treated as evidence extractors, not as a claim that every client resource is fully understood.

Static-client data enters Registry through the artefact-backed import path. Raw decoder outputs are preserved as source evidence; canonical entities and facts are created only by reviewed import commands.

The current decoder scope is:
- universe resources for regions, constellations, systems and jumps
- `types.fsdbinary` rows with localisation-backed names, group ids, type-name ids and wreck type ids
- reviewed enemy candidate groups and individual enemy type ids
- production resources for blueprint, recipe and material-requirement candidates
- deterministic comparison artefacts for patch review

The current reviewed enemy candidate fixture is:
```text
testdata/fixtures/static-enemies.reviewed.json
```

Import command:
```sh
go run ./cmd/br-import static-enemies -path testdata/fixtures/static-enemies.reviewed.json
```

The importer:

1. reads the reviewed candidate file
2. rejects unexpected non-local paths
3. registers a source artefact
4. hashes the artefact with SHA-256
5. records an import
6. creates or updates enemy entities
7. writes resolver-searchable display names such as `Caird [NPC]`

The reviewed set contains 26 enemy rows. Group `27` is not treated as an enemy group. Only `Feral Mooneater`, group `27`, type `85702`, is imported from that group.

`Caird` is the reviewed spelling used by the current enemy fixture.

## Static Universe Import

Use the native static-universe decoder to recover system, constellation, region and jump rows from the installed client:
```sh
go run ./cmd/br-import static-client-decode-universe -client-path "/path/to/eve-frontier/stillness" -out ./tmp/static-client-universe-stillness
```

The decoder resolves these client resources through `resfileindex.txt`:
```text
res:/staticdata/regions.static
res:/staticdata/constellations.static
res:/staticdata/systems.static
res:/staticdata/jumps.static
res:/localizationfsd/localization_fsd_en-us.pickle
```

It writes the import-compatible JSON extraction under `fsd_binary_schema`. System, region and constellation names are resolved from native static-client row fields first, with localisation used only as a fallback. The importer does not copy display-only web map data.

Import the decoded extraction:
```sh
go run ./cmd/br-import static-universe -path ./tmp/static-client-universe-stillness
```

The importer reads these files directly:
```text
fsd_binary_schema/regions.json
fsd_binary_schema/constellations.json
fsd_binary_schema/systems.json
fsd_binary_schema/jumps.json
```

Each raw file is registered as a separate source artefact. The importer creates source-backed region, constellation, system and route entities, and writes verified graph relations for constellation membership and jump links. The Go importer reads the extracted JSON files directly.

## Static Enemy Candidate Import

Import a static-client type JSON artefact then import the reviewed enemy subset from the same evidence:
```sh
go run ./cmd/br-import static-client-inspect-types -client-path "/path/to/eve-frontier/stillness"
go run ./cmd/br-import static-client-decode-types -client-path "/path/to/eve-frontier/stillness" -out ./tmp/static-client-types.native-decode.json
go run ./cmd/br-import static-client-extract-types -client-path "/path/to/eve-frontier/stillness" -out ./tmp/static-client-types.probes.json
go run ./cmd/br-import static-client-extract-types -client-path "/path/to/eve-frontier/stillness" -native-scan -out ./tmp/static-client-types.native-scan.json
go run ./cmd/br-import static-client-extract-types -client-path "/path/to/eve-frontier/stillness" -resolved-json ./tmp/static-client-types-current.json -out ./tmp/static-client-types-all.json
go run ./cmd/br-import static-client-compare-types -resolved ./tmp/static-client-types-all.json -native ./tmp/static-client-types.native-decode.json
go run ./cmd/br-import static-client-types -path ./tmp/static-client-types-all.json
go run ./cmd/br-import static-client-enemies -path ./tmp/static-client-types-all.json
go run ./cmd/br-import static-client-extract-production -client-path "/path/to/eve-frontier/stillness" -out ./tmp/static-client-production-resources.json
go run ./cmd/br-import static-client-decode-production -client-path "/path/to/eve-frontier/stillness" -out ./tmp/static-client-production.native-decode.json
```

`static-client-inspect-types` is the native Go inspection path for local static-client type evidence. It resolves `types.fsdbinary`, records its SHA-256, prints the first header bytes, reports little-endian byte offsets for reviewed enemy/type probes, decodes numeric row probes when the expected type-row shape is present and resolves probed names from the localisation resource when available. This proves the local binary resource and its matching localisation evidence while keeping broad byte matches in review.

`static-client-decode-types` is the native Go decode artefact path. It resolves `types.fsdbinary` and the matching English localisation resource from the client resource index, decodes stable type rows, records row offsets, records the resource hashes used as evidence and writes deterministic JSON with `schemaVersion: registry.static-client-type-decode.v1`. Use this output for patch review and compare runs because it carries concrete decoder evidence, not only an inspection log.

The same inspection command also prints static resource candidates discovered from the client resource index. It classifies likely type, blueprint, recipe and material-requirement resources and records their SHA-256 hashes when the files can be resolved locally. Recipe promotion still goes through reviewed artefacts.

`static-client-extract-types` is the Registry-owned wrapper for the import source: it reads the local client resource index, hashes `types.fsdbinary` and related static resources then writes a deterministic Registry JSON artefact from reviewed resolved JSON rows, configured native probe rows or an opt-in `-native-scan` output with localisation-backed names. It repairs known mojibake such as `Host�s`, sorts rows by stable numeric keys, removes duplicate type IDs and preserves source-resource hashes in the output.

Native Go binary row probes recover numeric fields such as group id, type-name id and wreck type id for reviewed type ids then use the client localisation resource to resolve the corresponding name. `static-client-decode-types` and `-native-scan` apply the same reviewed row-shape decoder across `types.fsdbinary` and are useful for patch review and delta checks. `static-client-compare-types` compares native and reviewed/resolved artefacts by stable type ID, not display name. It reports native-only rows, resolved-only rows, changed names, changed groups, changed wreck type ids and duplicate names without collapsing rows that share a name. A reviewed resolved JSON export is still the preferred path for broad canonical type imports until the native scan's false-positive and group-zero deltas have been reviewed for the current patch. The import path after that point is Go-only: the Registry treats the JSON artefact as raw source evidence, stores it by hash then normalises rows in Go. The general type importer conservatively creates item, material, ship, structure or enemy entities. The enemy subset importer accepts reviewed NPC groups `5033`, `4963`, `4770` and `5130`, plus reviewed individual enemy type IDs `85702` and `88089`. It also requires wreck type `81610` by default; broad player-ship groups that happen to have the same wreck type are not imported unless the operator deliberately changes the flags.

Current group `5130` NPC candidates found in the local Stillness client are:
```text
95291  Chrysalis
95283  Dermestid
94167  Mycena
95504  Mycena
```

## Static Recipe and Blueprint Import

The production decoder records blueprint, recipe and material-requirement candidates from local client resources. The recipe importer promotes only reviewed JSON rows. It validates the `contracts/static-client-recipes.v1.schema.json` shape and rejects malformed reviewed rows before registering an artefact.

Record the local production binary evidence before a patch or import review:
```sh
go run ./cmd/br-import static-client-extract-production -client-path "/path/to/eve-frontier/stillness" -out ./tmp/static-client-production-resources.json
go run ./cmd/br-import static-client-decode-production -client-path "/path/to/eve-frontier/stillness" -out ./tmp/static-client-production.native-decode.json
go run ./cmd/br-import static-client-summarise-production -path ./tmp/static-client-production-resources.json
go run ./cmd/br-import static-client-compare-production -before ./tmp/static-client-production-resources.before.json -after ./tmp/static-client-production-resources.json
```

The extraction command writes a deterministic manifest for blueprint, recipe and material-requirement resources discovered in the client resource index. It stores hashes, sizes and source resource paths so patch changes can be compared. `static-client-decode-production` writes a deterministic candidate artefact with blueprint primary rows, derived recipe candidates and `typematerials` material rows. It validates every decoded type ID through the native type decoder and keeps row offsets as evidence. These rows are not promoted by the decode command; review the candidate artefact before importing canonical recipe records. The summary command reports resource counts by kind for quick patch triage. The comparison command reports added, removed and changed production resources by resource path and kind.

Command:
```sh
go run ./cmd/br-import static-client-recipes -path ./tmp/static-client-recipes.reviewed.json -environment stillness
```

Accepted top-level arrays are `recipes`, `data`, `items`, `rows` or a direct array. The preferred reviewed artefact shape uses `schemaVersion: registry.static-client-recipes.v1`. Each recipe row should include stable type IDs rather than display-name-only matching:
```json
{
  "recipes": [
    {
      "recipeId": "reflex",
      "name": "Reflex",
      "outputTypeId": 1001,
      "outputQuantity": 1,
      "blueprintTypeId": 75001,
      "facilityTypeId": 70001,
      "inputs": [
        { "typeId": 3001, "quantity": 3 },
        { "typeId": 3002, "quantity": 1 }
      ],
      "sourceContext": "reviewed extraction row"
    }
  ]
}
```

The importer registers the JSON artefact, creates a recipe entity, creates a blueprint entity when `blueprintTypeId` is present and creates placeholder item or facility entities for missing referenced type records. It writes `produces`, `requires_input`, `uses_blueprint` and `uses_facility` relations. Placeholder entities are source-backed and should be replaced by richer type imports when the full type metadata is available.

Recipe entities also carry deterministic `output` and `inputs` facts so clients can render a bill of materials without guessing quantities from relation edges.

## JSONL Snapshot Path
```sh
go run ./cmd/br-import static-enemies-jsonl -path testdata/fixtures/static-enemies.before.jsonl -environment stillness
```

This path stores content-addressed JSONL artefacts, compares each candidate against the current canonical snapshot and appends outbox jobs only when the normalised rows meaningfully change.

See [JSONL Snapshots](snapshots-jsonl.md).
