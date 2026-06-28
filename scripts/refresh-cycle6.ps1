param(
  [string]$DatabaseUrl = $env:DATABASE_URL,
  [string]$Manifest = "testdata/fixtures/sui-packages.stillness.json",
  [string]$WorldPackage = "0x8b8a46ed766fa1358ce7c5c51f6a164b13d627a63e45343f69ed0ba0446c1aa1",
  [string]$TokenPackage = "0xac361aa5ceb726bd974f885c9dea9e55dc9bc98fa1f5731c5965a810707bf0b8",
  [int]$Concurrency = 64,
  [int]$MaxPages = 5,
  [switch]$Full,
  [switch]$IncludeTokenPackage,
  [switch]$OnlyIncomplete,
  [string]$ExportOut = "exports/cycle6-latest",
  [int]$ExportLimit = 0,
  [string]$ExportCycles = "current",
  [switch]$IncludeRawExports,
  [switch]$SkipExport,
  [switch]$PublishLocal,
  [string]$PublishRoot = "published-exports",
  [string]$PublishPrefix = "registry/current",
  [switch]$PublishR2,
  [string]$StatusPath = "tmp/indexer-status.json",
  [string]$SummaryPath = "tmp/cycle6-refresh-summary.json"
)

$ErrorActionPreference = "Stop"

if (-not $DatabaseUrl) {
  $DatabaseUrl = "postgres://blackrelay:blackrelay@127.0.0.1:5432/blackrelay_registry?sslmode=disable"
}

if ($Full) {
  $MaxPages = 0
}

$normalisedPublishPrefix = ($PublishPrefix -replace "\\", "/").Trim("/")
if ($ExportCycles.Trim().ToLowerInvariant() -eq "all") {
  throw "Unsupported export cycle scope 'all'. Use current or 6."
}

function Invoke-Go {
  param([string[]]$Arguments)
  Write-Host "+ go $(Format-CommandArguments $Arguments)"
  & go @Arguments
  if ($LASTEXITCODE -ne 0) {
    throw "go $(Format-CommandArguments $Arguments) failed with exit code $LASTEXITCODE"
  }
}

function Invoke-GoJson {
  param([string[]]$Arguments)
  Write-Host "+ go $(Format-CommandArguments $Arguments)"
  $output = & go @Arguments
  if ($LASTEXITCODE -ne 0) {
    throw "go $(Format-CommandArguments $Arguments) failed with exit code $LASTEXITCODE"
  }
  $text = ($output | Out-String).Trim()
  if ($text) {
    Write-Host $text
    return ($text | ConvertFrom-Json)
  }
  return $null
}

function New-StagedExportPath {
  param([string]$Path)
  $parent = Split-Path -Parent $Path
  $leaf = Split-Path -Leaf $Path
  if (-not $parent) {
    $parent = "."
  }
  $stamp = (Get-Date).ToUniversalTime().ToString("yyyyMMdd-HHmmss")
  return (Join-Path $parent "$leaf.staging-$stamp")
}

function Promote-VerifiedExport {
  param(
    [string]$StagePath,
    [string]$FinalPath
  )
  $finalParent = Split-Path -Parent $FinalPath
  if ($finalParent) {
    New-Item -ItemType Directory -Force -Path $finalParent | Out-Null
  }
  if (-not (Test-Path -LiteralPath $FinalPath)) {
    Move-Item -LiteralPath $StagePath -Destination $FinalPath
    return $null
  }
  $backupPath = "$FinalPath.superseded-$((Get-Date).ToUniversalTime().ToString("yyyyMMdd-HHmmss"))"
  Move-Item -LiteralPath $FinalPath -Destination $backupPath
  try {
    Move-Item -LiteralPath $StagePath -Destination $FinalPath
  } catch {
    Move-Item -LiteralPath $backupPath -Destination $FinalPath
    throw
  }
  return $backupPath
}

function Format-CommandArguments {
  param([string[]]$Arguments)
  $redacted = New-Object System.Collections.Generic.List[string]
  $secretFlags = @("-database-url", "-secret-access-key", "-access-key-id")
  for ($i = 0; $i -lt $Arguments.Count; $i++) {
    $arg = $Arguments[$i]
    $redacted.Add($arg)
    if ($secretFlags -contains $arg -and $i + 1 -lt $Arguments.Count) {
      $redacted.Add("<redacted>")
      $i++
    }
  }
  return ($redacted -join " ")
}

$packages = @($WorldPackage)
if ($IncludeTokenPackage) {
  $packages += $TokenPackage
}

Write-Host "Cycle 6 refresh mode: max-pages=$MaxPages concurrency=$Concurrency packages=$($packages -join ',')"
$startedAt = (Get-Date).ToUniversalTime()

Invoke-Go @("run", "./cmd/br-migrate", "-database-url", $DatabaseUrl)

foreach ($package in $packages) {
  Invoke-Go @("run", "./cmd/br-indexer", "-mode", "plan", "-database-url", $DatabaseUrl, "-manifest", $Manifest, "-package", $package, "-cycles", "current", "-max-pages", "$MaxPages", "-concurrency", "$Concurrency")

  $common = @("run", "./cmd/br-indexer", "-database-url", $DatabaseUrl, "-manifest", $Manifest, "-package", $package, "-cycles", "current", "-max-pages", "$MaxPages", "-concurrency", "$Concurrency", "-retries", "12")
  if ($OnlyIncomplete) {
    $common += "-only-incomplete"
  }

  Invoke-Go ($common + @("-mode", "events"))
  if ($package -eq $TokenPackage) {
    Write-Host "Skipping object backfill for token package $package; no object types are configured."
    continue
  }
  Invoke-Go ($common + @("-mode", "objects", "-allow-object-target-errors"))
}

Invoke-Go @("run", "./cmd/br-indexer", "-mode", "derive-events", "-database-url", $DatabaseUrl, "-cycles", "current", "-module", "killmail,character,gate,assembly,storage_unit,turret,rift", "-derive-batch-size", "5000")
Invoke-Go @("run", "./cmd/br-indexer", "-mode", "derive-objects", "-database-url", $DatabaseUrl, "-cycles", "current", "-derive-batch-size", "5000")
Invoke-Go @("run", "./cmd/br-indexer", "-mode", "resolve-evidence", "-database-url", $DatabaseUrl)
$coverageAudit = Invoke-GoJson @("run", "./cmd/br-indexer", "-mode", "audit-stillness", "-database-url", $DatabaseUrl, "-manifest", $Manifest, "-package", $WorldPackage, "-cycles", "current")
$killmailAudit = Invoke-GoJson @("run", "./cmd/br-indexer", "-mode", "audit-killmails", "-database-url", $DatabaseUrl, "-exclude-fixtures", "-sample-limit", "20")
$currentStateAudit = Invoke-GoJson @("run", "./cmd/br-indexer", "-mode", "audit-current-state", "-database-url", $DatabaseUrl)
$report = Invoke-GoJson @("run", "./cmd/br-indexer", "-mode", "report", "-database-url", $DatabaseUrl, "-exclude-fixtures")

$exportResult = $null
$verifyResult = $null
$publishLocalResult = $null
$publishR2Result = $null
$status = $null
$exportStage = $null
$exportSupersededPath = $null
if (-not $SkipExport) {
  $exportStage = New-StagedExportPath -Path $ExportOut
  $exportArgs = @("run", "./cmd/br-export", "-database-url", $DatabaseUrl, "-out", $exportStage, "-limit", "$ExportLimit", "-cycles", $ExportCycles)
  if ($IncludeRawExports) {
    $exportArgs += @("-include-events", "-include-sui-objects", "-timeout", "30m")
  }
  $exportResult = Invoke-GoJson $exportArgs
  $verifyResult = Invoke-GoJson @("run", "./cmd/br-export", "verify", "-dir", $exportStage)
  $exportSupersededPath = Promote-VerifiedExport -StagePath $exportStage -FinalPath $ExportOut
  Write-Host "Verified export promoted from $exportStage to $ExportOut"
  if ($PublishLocal) {
    $publishLocalResult = Invoke-GoJson @("run", "./cmd/br-export", "publish-local", "-dir", $ExportOut, "-root", $PublishRoot, "-prefix", $PublishPrefix)
  }
  if ($PublishR2) {
    $publishR2Result = Invoke-GoJson @("run", "./cmd/br-export", "publish-r2", "-dir", $ExportOut, "-prefix", $PublishPrefix)
  }
}

$statusArgs = @("run", "./cmd/br-indexer", "-mode", "status", "-database-url", $DatabaseUrl, "-environment", "stillness")
$exportManifestPath = Join-Path $ExportOut "manifest.json"
if ((-not $SkipExport) -and (Test-Path -LiteralPath $exportManifestPath)) {
  $statusArgs += @("-export-manifest", $exportManifestPath)
}
$status = Invoke-GoJson $statusArgs
$statusDir = Split-Path -Parent $StatusPath
if ($statusDir) {
  New-Item -ItemType Directory -Force -Path $statusDir | Out-Null
}
$statusJson = $status | ConvertTo-Json -Depth 50
Set-Content -Path $StatusPath -Value $statusJson -Encoding utf8
Write-Host "Indexer status written to $StatusPath"

$summary = [ordered]@{
  schemaVersion = "registry.cycle6_refresh.v1"
  startedAt = $startedAt
  finishedAt = (Get-Date).ToUniversalTime()
  manifest = $Manifest
  worldPackage = $WorldPackage
  tokenPackage = $TokenPackage
  packages = $packages
  maxPages = $MaxPages
  concurrency = $Concurrency
  full = [bool]$Full
  onlyIncomplete = [bool]$OnlyIncomplete
  includeTokenPackage = [bool]$IncludeTokenPackage
  coverage = $coverageAudit
  killmails = $killmailAudit
  currentState = $currentStateAudit
  report = $report
  status = $status
  export = [ordered]@{
    skipped = [bool]$SkipExport
    out = $ExportOut
    cycles = $ExportCycles
    limit = $ExportLimit
    includeRaw = [bool]$IncludeRawExports
    stagedOut = $exportStage
    supersededPath = $exportSupersededPath
    result = $exportResult
    verification = $verifyResult
    publishLocal = $publishLocalResult
    publishR2 = $publishR2Result
  }
}

$summaryDir = Split-Path -Parent $SummaryPath
if ($summaryDir) {
  New-Item -ItemType Directory -Force -Path $summaryDir | Out-Null
}
$summaryJson = $summary | ConvertTo-Json -Depth 50
Set-Content -Path $SummaryPath -Value $summaryJson -Encoding utf8
Write-Host "Cycle 6 refresh summary written to $SummaryPath"
Write-Output $summaryJson
