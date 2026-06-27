param(
  [ValidateSet("auto", "docker", "podman", "external")]
  [string]$Runtime = "auto"
)

$ErrorActionPreference = "Stop"

$databaseUrl = $env:DATABASE_URL
if (-not $databaseUrl) {
  $databaseUrl = "postgres://blackrelay:blackrelay@127.0.0.1:5432/blackrelay_registry?sslmode=disable"
}

if ($env:BR_REGISTRY_CONTAINER_RUNTIME) {
  $allowedRuntimes = @("auto", "docker", "podman", "external")
  if ($allowedRuntimes -notcontains $env:BR_REGISTRY_CONTAINER_RUNTIME) {
    throw "Unsupported BR_REGISTRY_CONTAINER_RUNTIME: $env:BR_REGISTRY_CONTAINER_RUNTIME"
  }
  $Runtime = $env:BR_REGISTRY_CONTAINER_RUNTIME
}

function Test-Command {
  param([string]$Name)
  return [bool](Get-Command $Name -ErrorAction SilentlyContinue)
}

$script:ComposeCommand = @()
$runtimeKind = $Runtime

function Use-DockerCompose {
  if (-not (Test-Command "docker")) {
    return $false
  }
  docker compose version *> $null
  if ($LASTEXITCODE -ne 0) {
    return $false
  }
  $script:ComposeCommand = @("docker", "compose")
  return $true
}

function Use-PodmanCompose {
  if ((Test-Command "podman")) {
    podman compose version *> $null
    if ($LASTEXITCODE -eq 0) {
      $script:ComposeCommand = @("podman", "compose")
      return $true
    }
  }
  if ((Test-Command "podman-compose")) {
    podman-compose version *> $null
    if ($LASTEXITCODE -eq 0) {
      $script:ComposeCommand = @("podman-compose")
      return $true
    }
  }
  return $false
}

function Invoke-Compose {
  param([string[]]$Arguments)
  $command = $script:ComposeCommand[0]
  $prefix = @()
  if ($script:ComposeCommand.Count -gt 1) {
    $prefix = $script:ComposeCommand[1..($script:ComposeCommand.Count - 1)]
  }
  & $command @prefix @Arguments
}

function Invoke-Go {
  param([string[]]$Arguments)
  & go @Arguments
  if ($LASTEXITCODE -ne 0) {
    throw "go $($Arguments -join ' ') failed with exit code $LASTEXITCODE"
  }
}

if ($Runtime -eq "auto") {
  if (Use-DockerCompose) {
    $runtimeKind = "docker"
  } elseif (Use-PodmanCompose) {
    $runtimeKind = "podman"
  } else {
    $runtimeKind = "external"
  }
} elseif ($Runtime -eq "docker") {
  if (-not (Use-DockerCompose)) {
    throw "Docker Compose is not available. Install Docker or rerun with -Runtime podman or -Runtime external."
  }
} elseif ($Runtime -eq "podman") {
  if (-not (Use-PodmanCompose)) {
    throw "Podman Compose is not available. Install Podman/podman-compose or rerun with -Runtime docker or -Runtime external."
  }
}

if ($runtimeKind -eq "external") {
  Write-Host "Using external PostgreSQL via DATABASE_URL."
} else {
  Write-Host "Using $runtimeKind Compose for local PostgreSQL."
  Invoke-Compose @("up", "-d", "postgres")

  $ready = $false
  for ($i = 0; $i -lt 60; $i++) {
    Invoke-Compose @("exec", "-T", "postgres", "pg_isready", "-U", "blackrelay", "-d", "blackrelay_registry") | Out-Null
    if ($LASTEXITCODE -eq 0) {
      $ready = $true
      break
    }
    Start-Sleep -Seconds 1
  }
  if (-not $ready) {
    throw "PostgreSQL did not become ready."
  }
}

Invoke-Go @("run", "./cmd/br-migrate", "-database-url", $databaseUrl)
Invoke-Go @("run", "./cmd/br-indexer", "-mode", "audit-stillness", "-database-url", $databaseUrl, "-manifest", "testdata/fixtures/sui-packages.stillness.json")
Invoke-Go @("run", "./cmd/br-import", "static-enemies", "-database-url", $databaseUrl, "-path", "testdata/fixtures/static-enemies.reviewed.json")
Invoke-Go @("run", "./cmd/br-import", "static-client-recipes", "-database-url", $databaseUrl, "-path", "testdata/fixtures/static-client-recipes.reviewed.json")
Invoke-Go @("run", "./cmd/br-import", "killmail-fixture", "-database-url", $databaseUrl, "-path", "testdata/fixtures/killmail.npc-caird.json")

$exportDir = Join-Path ([System.IO.Path]::GetTempPath()) ("blackrelay-registry-export-" + [guid]::NewGuid())
$publishRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("blackrelay-registry-publish-" + [guid]::NewGuid())
$workDir = (Get-Location).Path
$job = $null

try {
  Invoke-Go @("run", "./cmd/br-export", "-database-url", $databaseUrl, "-out", $exportDir)
  Invoke-Go @("run", "./cmd/br-export", "verify", "-dir", $exportDir)
  Invoke-Go @("run", "./cmd/br-export", "publish-local", "-dir", $exportDir, "-root", $publishRoot, "-prefix", "registry")

  $job = Start-Job -ScriptBlock {
    param($db, $wd)
    Set-Location $wd
    go run ./cmd/br-registry -database-url $db -addr 127.0.0.1:8080
  } -ArgumentList $databaseUrl, $workDir

  $apiReady = $false
  for ($i = 0; $i -lt 60; $i++) {
    try {
      Invoke-RestMethod http://127.0.0.1:8080/v1/ready | Out-Null
      $apiReady = $true
      break
    } catch {
      Start-Sleep -Seconds 1
    }
  }
  if (-not $apiReady) {
    throw "Registry API did not become ready."
  }
  Invoke-RestMethod http://127.0.0.1:8080/v1/health | Out-Null
  $killmail = Invoke-RestMethod http://127.0.0.1:8080/v1/killmails/killmail:stillness:fixture:caird
  if ($killmail.data.killer.displayName -ne "Caird [NPC]") {
    throw "semantic killmail did not resolve Caird NPC"
  }
  Invoke-RestMethod "http://127.0.0.1:8080/v1/current/characters?environment=stillness" | Out-Null
  Invoke-RestMethod "http://127.0.0.1:8080/v1/current/route-edges?environment=stillness" | Out-Null
  Invoke-RestMethod "http://127.0.0.1:8080/v1/current/recipes?environment=stillness" | Out-Null
  Invoke-RestMethod "http://127.0.0.1:8080/v1/current/blueprints?environment=stillness" | Out-Null
  Invoke-RestMethod http://127.0.0.1:8080/v1/ops/freshness | Out-Null
  Invoke-RestMethod http://127.0.0.1:8080/v1/ops/cursors | Out-Null
  Invoke-RestMethod http://127.0.0.1:8080/v1/ops/sui-coverage | Out-Null
  Invoke-RestMethod "http://127.0.0.1:8080/v1/ops/source-gaps?environment=stillness" | Out-Null
} finally {
  if ($job) {
    Stop-Job $job -ErrorAction SilentlyContinue | Out-Null
    Remove-Job $job -Force -ErrorAction SilentlyContinue | Out-Null
  }
  Remove-Item -Recurse -Force $exportDir, $publishRoot -ErrorAction SilentlyContinue
}
