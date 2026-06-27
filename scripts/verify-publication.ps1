param(
  [string]$Root = "published-exports",
  [string[]]$Prefix = @("registry/current", "registry/archive/all")
)

$ErrorActionPreference = "Stop"

function Normalize-ObjectKey {
  param([string]$Key)
  $normalised = ($Key -replace "\\", "/").Trim("/")
  if (-not $normalised) {
    throw "object key is required"
  }
  $parts = @($normalised -split "/")
  foreach ($part in $parts) {
    if (-not $part -or $part -eq "." -or $part -eq "..") {
      throw "unsafe object key '$Key'"
    }
  }
  return ($parts -join "/")
}

function Resolve-ObjectPath {
  param(
    [string]$RootPath,
    [string]$Key
  )
  $normalised = Normalize-ObjectKey -Key $Key
  if ([System.IO.Path]::IsPathRooted($normalised)) {
    throw "unsafe object key '$Key': absolute paths are not allowed"
  }
  $path = $RootPath
  foreach ($part in @($normalised -split "/")) {
    $path = Join-Path $path $part
  }
  return $path
}

function Get-Sha256 {
  param([string]$Path)
  return (Get-FileHash -Algorithm SHA256 -LiteralPath $Path).Hash.ToLowerInvariant()
}

function Add-Error {
  param(
    [System.Collections.Generic.List[string]]$Errors,
    [string]$Message
  )
  $Errors.Add($Message) | Out-Null
}

$prefixResults = New-Object System.Collections.Generic.List[object]
$allValid = $true

foreach ($rawPrefix in $Prefix) {
  $errors = New-Object System.Collections.Generic.List[string]
  $prefixKey = ""
  try {
    $prefixKey = Normalize-ObjectKey -Key $rawPrefix
    $pointerKey = "$prefixKey/latest/manifest.json"
    $pointerPath = Resolve-ObjectPath -RootPath $Root -Key $pointerKey
    if (-not (Test-Path -LiteralPath $pointerPath -PathType Leaf)) {
      Add-Error -Errors $errors -Message "missing latest pointer $pointerKey"
      throw "missing latest pointer"
    }

    $pointer = Get-Content -Raw -LiteralPath $pointerPath | ConvertFrom-Json
    if ($pointer.schemaVersion -ne "registry.export_publish_pointer.v1") {
      Add-Error -Errors $errors -Message "unexpected pointer schemaVersion '$($pointer.schemaVersion)'"
    }
    if (-not $pointer.bundleId) {
      Add-Error -Errors $errors -Message "pointer missing bundleId"
    }
    if (-not $pointer.manifestKey) {
      Add-Error -Errors $errors -Message "pointer missing manifestKey"
    }
    if (-not $pointer.manifestSha256) {
      Add-Error -Errors $errors -Message "pointer missing manifestSha256"
    }

    $expectedManifestKey = "$prefixKey/bundles/$($pointer.bundleId)/manifest.json"
    $actualManifestKey = Normalize-ObjectKey -Key $pointer.manifestKey
    if ($pointer.bundleId -and $actualManifestKey -ne $expectedManifestKey) {
      Add-Error -Errors $errors -Message "manifestKey '$actualManifestKey' did not match expected '$expectedManifestKey'"
    }

    $manifestPath = Resolve-ObjectPath -RootPath $Root -Key $actualManifestKey
    if (-not (Test-Path -LiteralPath $manifestPath -PathType Leaf)) {
      Add-Error -Errors $errors -Message "missing bundle manifest $actualManifestKey"
    } else {
      $actualManifestHash = Get-Sha256 -Path $manifestPath
      if ($actualManifestHash -ne $pointer.manifestSha256) {
        Add-Error -Errors $errors -Message "bundle manifest sha256 mismatch: expected $($pointer.manifestSha256), got $actualManifestHash"
      }
      $manifest = Get-Content -Raw -LiteralPath $manifestPath | ConvertFrom-Json
      if ($manifest.schemaVersion -ne "registry.export_manifest.v1") {
        Add-Error -Errors $errors -Message "unexpected bundle manifest schemaVersion '$($manifest.schemaVersion)'"
      }
    }

    $files = @($pointer.files)
    if ($files.Count -eq 0) {
      Add-Error -Errors $errors -Message "pointer listed no files"
    }
    foreach ($file in $files) {
      if (-not $file.objectKey) {
        Add-Error -Errors $errors -Message "published file '$($file.path)' missing objectKey"
        continue
      }
      $objectKey = Normalize-ObjectKey -Key $file.objectKey
      $objectPath = Resolve-ObjectPath -RootPath $Root -Key $objectKey
      if (-not (Test-Path -LiteralPath $objectPath -PathType Leaf)) {
        Add-Error -Errors $errors -Message "missing published object $objectKey"
        continue
      }
      $hash = Get-Sha256 -Path $objectPath
      if ($file.sha256 -and $hash -ne $file.sha256) {
        Add-Error -Errors $errors -Message "sha256 mismatch for ${objectKey}: expected $($file.sha256), got $hash"
      }
      $size = (Get-Item -LiteralPath $objectPath).Length
      if ($null -ne $file.sizeBytes -and $size -ne [int64]$file.sizeBytes) {
        Add-Error -Errors $errors -Message "size mismatch for ${objectKey}: expected $($file.sizeBytes), got $size"
      }
    }
  } catch {
    if ($errors.Count -eq 0) {
      Add-Error -Errors $errors -Message $_.Exception.Message
    }
  }

  $valid = $errors.Count -eq 0
  if (-not $valid) {
    $allValid = $false
  }
  $prefixResults.Add([ordered]@{
    prefix = $rawPrefix
    normalisedPrefix = $prefixKey
    valid = $valid
    errors = @($errors)
  }) | Out-Null
}

$result = [ordered]@{
  schemaVersion = "registry.publication_proof.v1"
  verifiedAt = (Get-Date).ToUniversalTime().ToString("o")
  root = $Root
  valid = $allValid
  prefixes = $prefixResults.ToArray()
}

$result | ConvertTo-Json -Depth 20
if (-not $allValid) {
  exit 1
}
