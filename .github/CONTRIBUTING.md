# Contributing

Black Relay Registry is a source-aware public EVE Frontier data registry. Contributions should keep the indexer, importers, API, snapshots and exports reviewable and honest about source quality.

## Before Opening An Issue

Before opening an issue, please read:
- [README.md](../README.md)
- [docs/README.md](../docs/README.md)
- [docs/architecture.md](../docs/architecture.md)
- [docs/source-policy.md](../docs/source-policy.md)
- [docs/api.md](../docs/api.md)

Issues should be specific enough to act on. Include the command, endpoint, source artefact, snapshot, fixture or exported file that demonstrates the problem when possible.

## Bug Reports

Bug reports should include:
- what happened
- what you expected to happen
- exact commands or API requests
- operating system, Go version and PostgreSQL version when relevant
- whether the problem affects ingestion, derivation, resolver output, API responses, exports or documentation
- relevant logs or command output in plain text

Keep reports limited to public game data and reproducible technical details. Redact wallet seeds, private keys, credentials, private Discord exports, private tribe intelligence, access tokens and unpublished operational information.

## Data Corrections

Data corrections must be source-backed. A useful correction includes:
- the entity or endpoint affected
- the current value and proposed value
- source kind, source URL or source artefact path
- environment and cycle when known
- confidence level and review state
- whether the change affects public exports or resolver output

Canonical data corrections require source evidence. Placeholders such as `Character 42`, `Tribe 42` and `System 30001001` are temporary labels until a public source proves a better name.

## Feature Requests

Feature requests should describe:
- the operator or consumer problem
- the source evidence required
- API, CLI, schema or export compatibility impact
- privacy and security risks
- expected test coverage

Accepted feature requests use public sources, preserved artefacts and reviewable provenance.

## Pull Request Expectations

Pull requests should:
- be scoped to one coherent change
- include tests when behaviour changes
- update docs when commands, schemas, API responses, source policy, CI or operator workflows change
- explain source/provenance impact when imports, resolver rules, confidence labels, cycle scope or exports change
- keep public game identity separate from real-world identity and private tribe information
- preserve local-safe defaults such as `127.0.0.1` binding

Required boundaries:
- canonical records are source-backed
- completeness claims are backed by fixtures, live-source evidence or explicit source-gap reporting
- Sui object provider gaps remain visible as coverage gaps
- Datacore is used only as product/source-discovery context
- public docs and runtime paths avoid telemetry, analytics, remote fonts and remote scripts
- repository files stay free of secrets, credentials, wallet seeds and private keys
- admin authentication, publication checks and source validation stay at least as strict as the current implementation

## Compatibility Changes

These are compatibility-sensitive surfaces:
- API routes and JSON response fields
- OpenAPI schema
- contracts under `contracts/`
- PostgreSQL migrations
- CLI command names, flags and output used by scripts
- public export file names, manifest fields and JSONL row contracts
- source, confidence, review state, cycle and entity-type vocabularies

Compatibility-impacting changes need tests and documentation. The change notes should explain what changed, who is affected and whether old input or exports still work.

## Development

Run the relevant checks before proposing a change:
```sh
export GOFLAGS="-buildvcs=false"
go test -count=1 ./...
go vet ./...
go build -buildvcs=false ./cmd/...
go mod verify
go mod tidy -diff
go run honnef.co/go/tools/cmd/staticcheck@latest ./...
go run golang.org/x/vuln/cmd/govulncheck@v1.5.0 ./...
npx --yes cspell@10.0.1 lint "README.md" "docs/**/*.md" ".github/**/*.md" ".github/**/*.yml" ".github/**/*.yaml" --config .github/config/cspell.json --no-progress --no-summary --no-must-find-files
```

If a check cannot be run, state that clearly in the pull request.

## Documentation Style

Public documentation should:
- use British English
- keep claims tied to implemented behaviour
- distinguish raw source evidence, canonical Registry records and public export artefacts
- state source gaps and provider limitations directly
- keep examples copyable
- use precise claims backed by implemented behaviour

Use American spelling only for source-code identifiers, JSON fields, command flags, protocol terms and upstream file names that require it.

## Security Issues

Report suspected vulnerabilities through the private process in [SECURITY.md](SECURITY.md).
