# Copyright 2026 The MathWorks, Inc.
#
# Install script for matlab-proxy on Windows.
# Downloads the latest release binary from GitHub and installs it.
#
# Usage:
#   powershell -ExecutionPolicy ByPass -c "irm https://raw.githubusercontent.com/prabhakk-mw/matlab-proxy-go/main/install.ps1 | iex"
#
# Options (via environment variables):
#   VERSION                        Install a specific version (e.g. 0.5.1)
#   INSTALL_DIR                    Install to a custom directory
#   MWI_NO_MODIFY_PATH=1           Don't add the install directory to PATH

<#
.SYNOPSIS
Installer for matlab-proxy.

.DESCRIPTION
This script detects your platform architecture and downloads the appropriate
matlab-proxy binary from GitHub releases, then installs it to the first of:

    $env:INSTALL_DIR (if set)
    $env:XDG_BIN_HOME
    $HOME\.local\bin

It will then add that directory to PATH by editing the user-level
Environment registry key.

.PARAMETER NoModifyPath
Don't add the install directory to PATH

.PARAMETER Help
Print help
#>

param (
    [Parameter(HelpMessage = "Don't add the install directory to PATH")]
    [switch]$NoModifyPath,
    [Parameter(HelpMessage = "Print Help")]
    [switch]$Help
)

$app_name = 'matlab-proxy'
$repo = 'prabhakk-mw/matlab-proxy-go'

if ($env:MWI_NO_MODIFY_PATH) {
    $NoModifyPath = $true
}

function Install-Binary($install_args) {
    if ($Help) {
        Get-Help $PSCommandPath -Detailed
        Exit
    }

    Initialize-Environment

    $arch = Get-Arch
    $version = Get-Version

    $tag = "v$version"
    $artifact = "$app_name-$tag-windows-$arch.zip"
    $url = "https://github.com/$repo/releases/download/$tag/$artifact"

    Write-Information "Installing $app_name $version (windows/$arch)..."
    Write-Information "  From: $url"

    # Download and extract
    $tmp = New-Temp-Dir
    $zip_path = Join-Path $tmp $artifact

    try {
        $wc = New-Object Net.WebClient
        $proxy = Get-WebProxyFromEnvironment
        if ($null -ne $proxy) {
            $wc.Proxy = $proxy
        }
        $wc.DownloadFile($url, $zip_path)
    } catch {
        throw "ERROR: failed to download $url`n$_"
    }

    Expand-Archive -Path $zip_path -DestinationPath $tmp

    # Determine install directory
    $dest_dir = $null
    if ($env:INSTALL_DIR) {
        $dest_dir = $env:INSTALL_DIR
    }
    if (-not $dest_dir) {
        $dest_dir = if ($env:XDG_BIN_HOME) { $env:XDG_BIN_HOME } else { $null }
    }
    if (-not $dest_dir) {
        $dest_dir = if ($HOME) { Join-Path $HOME ".local\bin" } else { $null }
    }
    if (-not $dest_dir) {
        throw "ERROR: could not determine install directory. Set INSTALL_DIR."
    }

    $dest_dir = New-Item -Force -ItemType Directory -Path $dest_dir
    Write-Information "  To:   $dest_dir\$app_name.exe"

    Copy-Item (Join-Path $tmp "$app_name.exe") -Destination $dest_dir -ErrorAction Stop

    # Clean up temp directory
    Remove-Item $tmp -Recurse -Force -ErrorAction SilentlyContinue

    Write-Information ""
    Write-Information "$app_name $version installed successfully!"

    # Verify installation
    $installed_bin = Join-Path $dest_dir "$app_name.exe"
    & $installed_bin --version

    # Update PATH
    if (-not $NoModifyPath) {
        Add-Ci-Path $dest_dir
        if (Add-Path $dest_dir) {
            Write-Information ""
            Write-Information "To add $dest_dir to your PATH, either restart your shell or run:"
            Write-Information ""
            Write-Information "    set Path=$dest_dir;%Path%   (cmd)"
            Write-Information "    `$env:Path = `"$dest_dir;`$env:Path`"   (powershell)"
        }
    }
}

function Get-Version() {
    if ($env:VERSION) {
        return $env:VERSION
    }

    Write-Information "Fetching latest release..."
    try {
        $release = Invoke-RestMethod -Uri "https://api.github.com/repos/$repo/releases/latest" -ErrorAction Stop
        $tag = $release.tag_name
        if ($tag -match '^v(.+)$') {
            return $Matches[1]
        }
        throw "Unexpected tag format: $tag"
    } catch {
        throw "ERROR: could not determine latest version. Set VERSION manually.`n$_"
    }
}

function Get-Arch() {
    try {
        # Use .NET to detect OS architecture
        # Works correctly on PowerShell Core 7.3+ and Windows PowerShell on Win 11 22H2+
        $a = [System.Reflection.Assembly]::LoadWithPartialName("System.Runtime.InteropServices.RuntimeInformation")
        $t = $a.GetType("System.Runtime.InteropServices.RuntimeInformation")
        $p = $t.GetProperty("OSArchitecture")
        switch ($p.GetValue($null).ToString()) {
            "X64"   { return "amd64" }
            "Arm64" { return "arm64" }
        }
    } catch {
        Write-Verbose "Could not detect architecture via RuntimeInformation: $_"
    }

    # Fallback for older .NET
    if ([System.Environment]::Is64BitOperatingSystem) {
        return "amd64"
    }

    throw "ERROR: unsupported architecture. Only 64-bit Windows is supported."
}

function Get-WebProxyFromEnvironment() {
    $proxy_url = if ($env:HTTPS_PROXY) { $env:HTTPS_PROXY } else { $env:ALL_PROXY }
    if ([string]::IsNullOrWhiteSpace($proxy_url)) {
        return $null
    }
    try {
        $uri = [System.Uri]$proxy_url
        $proxy = New-Object System.Net.WebProxy($uri)
        if (-not [string]::IsNullOrEmpty($uri.UserInfo)) {
            $parts = $uri.UserInfo.Split(':')
            $user = [System.Uri]::UnescapeDataString($parts[0])
            $pass = if ($null -eq $parts[1]) { "" } else { [System.Uri]::UnescapeDataString($parts[1]) }
            $proxy.Credentials = New-Object System.Net.NetworkCredential($user, $pass)
        }
        return $proxy
    } catch {
        Write-Verbose "Failed to parse proxy URL: $_"
        return $null
    }
}

function Initialize-Environment() {
    if (($PSVersionTable.PSVersion.Major) -lt 5) {
        throw @"
Error: PowerShell 5 or later is required to install $app_name.
Upgrade PowerShell:

    https://docs.microsoft.com/en-us/powershell/scripting/setup/installing-windows-powershell

"@
    }

    $allowedExecutionPolicy = @('Unrestricted', 'RemoteSigned', 'Bypass')
    if ((Get-ExecutionPolicy).ToString() -notin $allowedExecutionPolicy) {
        throw @"
Error: PowerShell requires an execution policy in [$($allowedExecutionPolicy -join ", ")] to run $app_name. For example, to set the execution policy to 'RemoteSigned' please run:

    Set-ExecutionPolicy RemoteSigned -scope CurrentUser

"@
    }

    if ([System.Enum]::GetNames([System.Net.SecurityProtocolType]) -notcontains 'Tls12') {
        throw @"
Error: Installing $app_name requires at least .NET Framework 4.5
Please download and install it first:

    https://www.microsoft.com/net/download

"@
    }
}

# Add to PATH in GitHub Actions
function Add-Ci-Path($PathToAdd) {
    if (($gh_path = $env:GITHUB_PATH)) {
        Write-Output "$PathToAdd" | Out-File -FilePath "$gh_path" -Encoding utf8 -Append
    }
}

# Permanently add to user-level PATH via the registry
# Returns $true if the registry was modified, $false if already on PATH
function Add-Path($LiteralPath) {
    $RegistryPath = 'registry::HKEY_CURRENT_USER\Environment'
    $CurrentDirectories = (Get-Item -LiteralPath $RegistryPath).GetValue('Path', '', 'DoNotExpandEnvironmentNames') -split ';' -ne ''

    if ($LiteralPath -in $CurrentDirectories) {
        return $false
    }

    $NewPath = (,$LiteralPath + $CurrentDirectories) -join ';'
    Set-ItemProperty -Type ExpandString -LiteralPath $RegistryPath Path $NewPath

    # Broadcast WM_SETTINGCHANGE so the shell reloads the updated PATH
    $DummyName = 'matlab-proxy-' + [guid]::NewGuid().ToString()
    [Environment]::SetEnvironmentVariable($DummyName, 'dummy', 'User')
    [Environment]::SetEnvironmentVariable($DummyName, [NullString]::value, 'User')

    return $true
}

function New-Temp-Dir() {
    [CmdletBinding(SupportsShouldProcess)]
    param()
    $parent = [System.IO.Path]::GetTempPath()
    [string] $name = [System.Guid]::NewGuid()
    New-Item -ItemType Directory -Path (Join-Path $parent $name)
}

# Suppress PSScriptAnalyzer warnings for globals
$Null = $NoModifyPath, $Help
$InformationPreference = "Continue"

try {
    Install-Binary "$Args"
} catch {
    Write-Information $_
    exit 1
}
