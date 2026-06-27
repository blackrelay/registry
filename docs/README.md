# Registry Documentation

This directory contains the working documentation for Black Relay Registry.

## Start Here
- [Architecture](architecture.md): canonical data model, ingestion flow, source artefacts and current derivation boundaries.
- [API](api.md): implemented `/v1` routes, filters, response contracts and cycle-scope behaviour.
- [Development](development.md): local commands, PostgreSQL setup, Sui backfill, imports, reports and exports.
- [Operations](operations.md): freshness checks, source gaps, publication checks and operational repair paths.
- [Run Your Own Registry](run-your-own-registry.md): operator bootstrap with custom database, artefact root and registry identity.

## Source And Import Guides
- [Source Policy](source-policy.md): source kinds, confidence labels, review rules and privacy boundaries.
- [Static Client Import](static-client-import.md): native static-client decode flow, artefact evidence and reviewed imports.
- [Snapshot JSONL](snapshots-jsonl.md): content-addressed JSONL artefacts, semantic diffs and snapshot promotion.
- [Tribe Identity Sources](tribe-identity-sources.md): tribe name, tag, description and URL evidence rules.
- [Resolver](resolver.md): semantic killmail and identity-resolution behaviour.

## Distribution And Integration
- [Cloudflare Distribution](cloudflare.md): export publication and Cloudflare-facing deployment notes.
- [CI](ci.md): continuous-integration gates and matching local commands.
- [Release](release/README.md): release workflow, signed checksum manifests and local signing key setup.

## Documentation Rules

Documentation should use British English except where code, protocol, JSON field names, upstream file names or command flags require exact spelling. Claims should be tied to implemented behaviour, tests, fixtures, source artefacts or documented operator commands.

When a behaviour depends on source completeness, say so directly. Source gaps are part of the Registry model and should be visible in prose, reports and API output.
