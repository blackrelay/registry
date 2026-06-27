param(
  [ValidateSet("auto", "docker", "podman", "external")]
  [string]$Runtime = "auto",
  [string]$DatabaseUrl = $env:DATABASE_URL,
  [string]$ClientPath = "",
  [string]$StaticUniversePath = ""
)

$ErrorActionPreference = "Stop"

if (-not $DatabaseUrl) {
  $DatabaseUrl = "postgres://blackrelay:blackrelay@127.0.0.1:5432/blackrelay_registry?sslmode=disable"
}

$env:DATABASE_URL = $DatabaseUrl

.\scripts\smoke.ps1 -Runtime $Runtime

if ($ClientPath) {
  New-Item -ItemType Directory -Force -Path ".\tmp" | Out-Null
  go run ./cmd/br-import static-client-extract-production -client-path $ClientPath -out ".\tmp\static-client-production-resources.json"
  if ($LASTEXITCODE -ne 0) {
    throw "static-client production extraction failed"
  }
  go run ./cmd/br-import static-client-extract-types -client-path $ClientPath -out ".\tmp\static-client-types.probes.json"
  if ($LASTEXITCODE -ne 0) {
    throw "static-client type probe extraction failed"
  }
}

if ($StaticUniversePath) {
  go run ./cmd/br-import static-universe -database-url $DatabaseUrl -path $StaticUniversePath
  if ($LASTEXITCODE -ne 0) {
    throw "static universe import failed"
  }
}

.\scripts\api-proof.ps1 -StartServer -DatabaseUrl $DatabaseUrl
