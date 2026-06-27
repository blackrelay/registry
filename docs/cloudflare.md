# Cloudflare Distribution

Cloudflare is a distribution layer for Registry exports, not the canonical database.

PostgreSQL remains the working Registry. Export publication should follow this order:
```text
PostgreSQL
-> br-export
-> br-export verify
-> br-export publish-local or br-export publish-r2
-> immutable object bundle
-> latest pointer
-> Worker or static client reads complete bundles
```

The normal Cycle 6 operator wrapper follows that order by default:
```sh
./scripts/refresh-cycle6.sh --max-pages 5 --concurrency 64
```

It runs migrations, Sui append/repair work, derivation, evidence resolution, audits, the aggregate report, compact export generation into a staging directory, export verification, promotion to the configured export path and a summary write. The default export scope is the current cycle plus unlabelled rows and the default publish prefix is `registry/current`. Archive publication uses `-ExportCycles all -PublishPrefix registry/archive/all`. Repair passes can use `-SkipExport`. R2 publication uses `-PublishR2` with `BR_R2_ACCOUNT_ID`, `BR_R2_BUCKET`, `BR_R2_ACCESS_KEY_ID` and `BR_R2_SECRET_ACCESS_KEY` configured for the target bucket.

The local publisher writes the object-key shape intended for R2:
```text
registry/bundles/<bundle-id>/catalog.json
registry/bundles/<bundle-id>/entities.jsonl
registry/bundles/<bundle-id>/killmails.jsonl
registry/bundles/<bundle-id>/sources.jsonl
registry/bundles/<bundle-id>/facts.jsonl
registry/bundles/<bundle-id>/relations.jsonl
registry/bundles/<bundle-id>/entity_sources.jsonl
registry/bundles/<bundle-id>/source_artefacts.jsonl
registry/bundles/<bundle-id>/current_entities.jsonl
registry/bundles/<bundle-id>/current_relations.jsonl
registry/bundles/<bundle-id>/ops_freshness.json
registry/bundles/<bundle-id>/ops_cursors.json
registry/bundles/<bundle-id>/ops_sui_coverage.json
registry/bundles/<bundle-id>/ops_source_gaps.json
registry/bundles/<bundle-id>/manifest.json
registry/latest/manifest.json
```

Use separate prefixes for current and archive distribution:
```text
registry/current/latest/manifest.json
registry/current/bundles/<bundle-id>/manifest.json
registry/current/bundles/<bundle-id>/entities.jsonl
registry/archive/all/latest/manifest.json
registry/archive/all/bundles/<bundle-id>/manifest.json
registry/archive/all/bundles/<bundle-id>/entities.jsonl
```

`latest/manifest.json` is a pointer object under each prefix. It is written last so readers see a new latest bundle after all immutable objects have been uploaded. Bundle objects can be cached as immutable; latest pointer objects should use a short TTL or revalidation policy.

R2 integration should use either:
- `br-export publish-r2` with the S3-compatible R2 API from local or CI publishing tools
- a Worker with an R2 bucket binding for read-only public serving

`publish-r2` accepts `BR_R2_ACCOUNT_ID`, `BR_R2_ENDPOINT`, `BR_R2_BUCKET`, `BR_R2_ACCESS_KEY_ID`, `BR_R2_SECRET_ACCESS_KEY` and `BR_R2_REGION`. The default endpoint is `https://<account-id>.r2.cloudflarestorage.com` and the default signing region is `auto`.

The R2 publisher applies conditional writes to immutable bundle objects. `registry/latest/manifest.json` is allowed to overwrite because it is the current-pointer object.

Do not use D1 as the canonical Registry database. D1 can be added as a generated read model only if there is a concrete operational reason.
