param(
  [string]$BaseUrl = "http://127.0.0.1:8080",
  [switch]$StartServer,
  [string]$Addr = "127.0.0.1:8080",
  [string]$DatabaseUrl = $env:DATABASE_URL,
  [int]$TimeoutSeconds = 30
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

if (-not $DatabaseUrl) {
  $DatabaseUrl = "postgres://blackrelay:blackrelay@127.0.0.1:5432/blackrelay_registry?sslmode=disable"
}

$serverProcess = $null

function Invoke-RegistryJson {
  param([string]$Path)
  $uri = "$BaseUrl$Path"
  Invoke-RestMethod -Method Get -Uri $uri -TimeoutSec 10
}

function Get-ObjectPropertyValue {
  param(
    [object]$Object,
    [string]$Name,
    [object]$Default = $null
  )
  if ($null -eq $Object) {
    return $Default
  }
  $property = $Object.PSObject.Properties[$Name]
  if ($null -eq $property) {
    return $Default
  }
  $property.Value
}

function Get-RangeBlockedTargetCount {
  param([object]$CoverageData)
  $direct = Get-ObjectPropertyValue -Object $CoverageData -Name "rangeBlockedTargets"
  if ($null -ne $direct) {
    return [int]$direct
  }
  $targets = @(Get-ObjectPropertyValue -Object $CoverageData -Name "targets" -Default @())
  @($targets | Where-Object { (Get-ObjectPropertyValue -Object $_ -Name "status" -Default "") -eq "range_blocked" }).Count
}

try {
  if ($StartServer) {
    if ($BaseUrl -eq "http://127.0.0.1:8080" -and $Addr -ne "127.0.0.1:8080") {
      $BaseUrl = "http://$Addr"
    }
    $arguments = @(
      "run",
      "./cmd/br-registry",
      "-addr",
      $Addr,
      "-database-url",
      $DatabaseUrl
    )
    $serverProcess = Start-Process -FilePath "go" -ArgumentList $arguments -PassThru -WindowStyle Hidden
  }

  $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
  do {
    try {
      $ready = Invoke-RegistryJson "/v1/ready"
      if ($ready.data.status -eq "ready") {
        break
      }
    } catch {
      if ((Get-Date) -ge $deadline) {
        throw
      }
      Start-Sleep -Milliseconds 500
    }
  } while ((Get-Date) -lt $deadline)

  $health = Invoke-RegistryJson "/v1/health"
  if ($health.data.status -ne "ok") {
    throw "health endpoint returned unexpected status"
  }

  $killmails = Invoke-RegistryJson "/v1/killmails?environment=stillness&exclude_fixtures=true&limit=1"
  $characters = Invoke-RegistryJson "/v1/current/characters?environment=stillness&has_activity=true&limit=1"
  $systems = Invoke-RegistryJson "/v1/current/systems?environment=stillness&has_activity=true&limit=1"
  $routeEdges = Invoke-RegistryJson "/v1/current/route-edges?environment=stillness&limit=1"
  $enemies = Invoke-RegistryJson "/v1/current/enemies?environment=stillness&limit=1"
  $materials = Invoke-RegistryJson "/v1/current/materials?environment=stillness&limit=1"
  $recipes = Invoke-RegistryJson "/v1/current/recipes?environment=stillness&limit=1"
  $blueprints = Invoke-RegistryJson "/v1/current/blueprints?environment=stillness&limit=1"
  $coverage = Invoke-RegistryJson "/v1/ops/sui-coverage"
  $sourceGaps = Invoke-RegistryJson "/v1/ops/source-gaps?environment=stillness"

  [pscustomobject]@{
    baseUrl = $BaseUrl
    health = $health.data.status
    ready = $ready.data.status
    killmailRows = @($killmails.data).Count
    activeCharacterRows = @($characters.data).Count
    activeSystemRows = @($systems.data).Count
    routeEdgeRows = @($routeEdges.data).Count
    enemyRows = @($enemies.data).Count
    materialRows = @($materials.data).Count
    recipeRows = @($recipes.data).Count
    blueprintRows = @($blueprints.data).Count
    suiCoverageTargets = $coverage.data.targetCount
    suiRangeBlockedTargets = Get-RangeBlockedTargetCount -CoverageData $coverage.data
    sourceGapRows = @($sourceGaps.data).Count
    registry = $health.meta.registry
    apiVersion = $health.meta.apiVersion
  } | ConvertTo-Json -Depth 4
} finally {
  if ($null -ne $serverProcess -and -not $serverProcess.HasExited) {
    Stop-Process -Id $serverProcess.Id -Force
  }
}
