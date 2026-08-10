$ErrorActionPreference = "Stop"

function Get-Arch {
    $procArch = $env:PROCESSOR_ARCHITECTURE
    switch ($procArch) {
        "AMD64" { return "amd64" }
        "ARM64" { return "arm64" }
        default {
            Write-Error "Unsupported architecture: $procArch"
            exit 1
        }
    }
}

function Resolve-PluginVersion {
    param([string]$PluginDir)

    # HELM_PLUGIN_VERSION is set by Helm from plugin.yaml during hook execution.
    # Use it when it's a real release version (not the dev placeholder).
    $helmVer = $env:HELM_PLUGIN_VERSION
    if ($helmVer -and $helmVer -ne "0.0.0" -and $helmVer -ne "dev") {
        return $helmVer
    }

    # Fallback: read the tag from git. Reliable when Helm checks out a specific
    # tag via `helm plugin install URL --version vX.Y.Z`.
    $versionTag = & git -C $PluginDir describe --tags --abbrev=0 2>$null
    if ($versionTag) {
        return $versionTag -replace '^v', ''
    }

    Write-Error "Could not determine plugin version. Install from a tagged release or ensure plugin.yaml is up to date."
    exit 1
}

function Confirm-Checksum {
    param(
        [string]$FilePath,
        [string]$ArchiveName,
        [string]$ChecksumsPath
    )
    $checksumContent = Get-Content $ChecksumsPath
    $expectedLine = $checksumContent | Where-Object { $_ -match [regex]::Escape($ArchiveName) } | Select-Object -First 1
    if (-not $expectedLine) {
        Write-Error "Checksum entry for $ArchiveName not found in checksums.txt"
        exit 1
    }
    $expectedHash = ($expectedLine -split '\s+')[0].ToUpper()

    $actualHash = (Get-FileHash -Path $FilePath -Algorithm SHA256).Hash.ToUpper()

    if ($actualHash -ne $expectedHash) {
        Write-Error "Checksum mismatch for $ArchiveName`n  expected: $expectedHash`n  actual:   $actualHash"
        exit 1
    }
}

function Main {
    $arch = Get-Arch
    $version = Resolve-PluginVersion -PluginDir $env:HELM_PLUGIN_DIR

    $archiveName = "vigie_windows_${arch}.zip"
    $baseUrl = "https://github.com/fregateops/vigie/releases/download/v${version}"
    $archiveUrl = "${baseUrl}/${archiveName}"
    $checksumsUrl = "${baseUrl}/checksums.txt"

    Write-Host "Installing vigie $version (windows/$arch)..."

    $tmpDir = Join-Path ([System.IO.Path]::GetTempPath()) ([System.IO.Path]::GetRandomFileName())
    New-Item -ItemType Directory -Path $tmpDir | Out-Null

    try {
        $archivePath = Join-Path $tmpDir $archiveName
        $checksumsPath = Join-Path $tmpDir "checksums.txt"

        Write-Host "Downloading $archiveUrl..."
        Invoke-WebRequest -Uri $archiveUrl -OutFile $archivePath -UseBasicParsing

        Write-Host "Downloading checksums..."
        Invoke-WebRequest -Uri $checksumsUrl -OutFile $checksumsPath -UseBasicParsing

        Write-Host "Verifying checksum..."
        Confirm-Checksum -FilePath $archivePath -ArchiveName $archiveName -ChecksumsPath $checksumsPath
        Write-Host "Checksum OK"

        Write-Host "Extracting binary..."
        Expand-Archive -Path $archivePath -DestinationPath $tmpDir -Force

        $binDir = Join-Path $env:HELM_PLUGIN_DIR "bin"
        if (-not (Test-Path $binDir)) {
            New-Item -ItemType Directory -Path $binDir | Out-Null
        }

        $destName = "vigie_windows_${arch}.exe"
        $destPath = Join-Path $binDir $destName

        $extractedExe = Join-Path $tmpDir "vigie.exe"
        Copy-Item -Path $extractedExe -Destination $destPath -Force

        Write-Host "vigie $version installed to $destPath"
    }
    finally {
        Remove-Item -Recurse -Force $tmpDir -ErrorAction SilentlyContinue
    }
}

Main
