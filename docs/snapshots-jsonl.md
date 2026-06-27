# JSONL Snapshots

Static-client JSONL dumps are source evidence. They are not normalised Registry records.

The pipeline separates four things:
- raw source artefacts, which preserve evidence
- PostgreSQL Registry tables, which hold the working canonical model
- database dumps, which are backups
- public exports, which are distribution snapshots for consumers

## Patch Workflow
1. Run extraction before a patch.
2. Register the artefact.
3. Promote it when it is meaningful.
4. Run extraction after a patch.
5. Register the candidate artefact.
6. Compare byte by byte.
7. Run the semantic diff.
8. Promote only when the change is meaningful.
9. Supersede the previous snapshot.
10. Append import, export and reindex jobs.

## Byte And Semantic Checks

A byte-identical run has the same SHA-256 hash as an existing artefact. It is recorded as a no-op and leaves the existing canonical artefact in place.

A semantically unchanged run has different bytes but the same normalised rows. Ordering, whitespace and ignored metadata are coalesced into the existing canonical snapshot.

A meaningful change updates the canonical snapshot set. The previous canonical artefact is superseded, not overwritten. Canonical artefacts are never deleted because they are evidence.

## Enemy JSONL Normalisation

Static enemy rows are normalised into deterministic comparison rows:
```text
group_id
type_id
name
is_enemy_group
is_reviewed_individual
source_context
```

Rows are sorted by:
```text
group_id
type_id
name
```

The diff detects:
- new type IDs
- removed type IDs
- changed names for the same type ID
- changed groups for the same type ID
- duplicate names with different type IDs
- groups newly classified as enemy groups
- individual reviewed enemy rows added outside enemy groups
- individual reviewed enemy rows removed outside enemy groups

## Outbox

Meaningful snapshot changes append jobs to `outbox_jobs`. Jobs are appended because later workers must see work in the same order the Registry observed it.

Current job kinds:
```text
static_enemy_import
snapshot_diff_generated
public_export_required
search_reindex_required
resolver_refresh_required
```

No-op runs record the ingest result without appending import, export or reindex jobs.
