# Continuous Integration

The Registry CI is a single GitHub Actions workflow at `.github/workflows/ci.yml`. It is intentionally split into separate jobs so failures point at the affected layer.

## Jobs

`CI / Test` runs on Linux x64, Windows x64 and macOS arm64. It checks Go formatting, runs the full Go test suite and builds all commands.

`CI / FreeBSD`, `CI / OpenBSD` and `CI / NetBSD` run the Go test suite and command builds inside BSD virtual machines. These jobs exercise the code on real BSD userlands rather than relying only on cross-compilation.

`CI / Static Analysis` verifies modules, checks that `go mod tidy -diff` is clean, runs `go vet` and runs Staticcheck.

`CI / Docs And Contracts` runs contract/OpenAPI tests, spellchecks README/docs/GitHub prose with British English settings and runs terminology guard tests.

`CI / Container Smoke` runs the scripted smoke path on Linux. It exercises the local proof path against PostgreSQL through Docker Compose, Podman Compose or an existing database when available.

`CI / Vulnerability Scan` runs `govulncheck` against the Go module.

`CI / Workflow Validation` runs `actionlint` against GitHub Actions workflows.

`CI / Cleanup` evaluates all upstream jobs and fails the workflow if any required job failed or was cancelled.

`CodeQL` runs separate Actions and Go analyses on pull requests, pushes to `main`, a weekly schedule and manual dispatch. Go analysis uses manual `go build ./...` extraction and includes tests.

Dependabot is configured for weekly grouped updates to Go modules and GitHub Actions.

## Release Workflow

The release workflow lives at `.github/workflows/release.yml`. It can run as a manual dry run, a manual publish or a tag-push publish.

Manual runs require a SemVer version such as `v0.1.0` or `v0.1.0-rc.1` and a target branch, tag or full commit SHA. Manual runs default to verification only. Set `publish` to true when the workflow should create a tag if needed and publish a GitHub release.

Tag pushes matching `v*` publish a GitHub release from the pushed tag. Tags containing a prerelease suffix such as `-rc.1` are marked as prereleases.

The release workflow:
- checks the target commit is reachable from the default branch;
- runs formatting, module, test, vet, Staticcheck, vulnerability, contract, spelling and workflow checks;
- builds command archives for Linux, Windows, macOS, FreeBSD, OpenBSD and NetBSD;
- writes `SHA2-256SUMS` and `SHA2-512SUMS`;
- signs both checksum manifests with the release PGP key;
- uploads release assets for every run;
- publishes the GitHub release only when requested by a manual input or by a release tag push.

Release signing secrets and verification steps are documented in [Release](release/README.md).

## Local Equivalents

Run the common local checks with:
```sh
export GOFLAGS="-buildvcs=false"
gofmt -w cmd internal openapi
go test -count=1 ./...
go vet ./...
go build -buildvcs=false ./cmd/...
go mod verify
go mod tidy -diff
go run honnef.co/go/tools/cmd/staticcheck@latest ./...
go run golang.org/x/vuln/cmd/govulncheck@v1.5.0 ./...
```

Run documentation checks with:
```sh
npx --yes cspell@10.0.1 lint "README.md" "docs/**/*.md" ".github/**/*.md" ".github/**/*.yml" ".github/**/*.yaml" --config .github/config/cspell.json --no-progress --no-summary --no-must-find-files
```

Run the local smoke path with:
```sh
./scripts/smoke.sh
```

Use an existing PostgreSQL service with:
```sh
export DATABASE_URL="postgres://blackrelay:blackrelay@127.0.0.1:5432/blackrelay_registry?sslmode=disable"
BR_REGISTRY_CONTAINER_RUNTIME=external ./scripts/smoke.sh
```

## CI Boundaries

CI covers committed code buildability, tests, contract fixtures, documentation spelling, workflow syntax, vulnerability checks and the smoke path. Live Sui completeness, World API completeness and R2 publication are operational checks covered by reports, cursor audits and export verification commands.
