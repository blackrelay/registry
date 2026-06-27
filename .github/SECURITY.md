# Security Policy

Report suspected security vulnerabilities privately.

## Private Reporting

Use GitHub private vulnerability reporting:
```text
https://github.com/blackrelay/registry/security/advisories/new
```

## Current Security Boundary

Registry stores sourced public EVE Frontier world data, public game identity and public source evidence. It may ingest public chain data, public World API snapshots, public DataHub metadata, native static-client decoder artefacts, reviewed static-client records and manually reviewed public reports. Accepted records are public, source-backed and reviewable.

Sensitive material stays outside the Registry. This includes wallet seeds, private keys, mnemonics, game credentials, access tokens, private Discord messages, private tribe intelligence, real-world identity data, unverified alt accusations, doxxing material, private operational routes and live hostile tracking.

The local API server:
- defaults to `127.0.0.1:8080`
- uses PostgreSQL as the working Registry database
- stores local raw artefacts under the configured artefact root
- requires Cloudflare Access JWT validation for production admin routes when Access settings are configured
- supports a local bearer token only as a development fallback

The export publisher:
- verifies export manifests before publication
- writes immutable bundle objects before updating the latest pointer
- uses R2/S3 credentials only from operator-provided environment variables or flags
- keeps published bundles, credentials and local secrets outside source control

## Security-Sensitive Changes

Changes need explicit review when they affect:
- admin authentication or Cloudflare Access validation
- bearer token handling
- source URL validation or private-host rejection
- PostgreSQL migrations or SQL access patterns
- artefact path handling
- export verification or publication order
- R2/S3 credential handling
- local bind addresses, CORS, Host handling or request limits
- source confidence, review state or public/private data boundaries

Security-sensitive changes should include negative tests where practical.
