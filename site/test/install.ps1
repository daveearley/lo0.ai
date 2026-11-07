# Tests site/public/install.ps1 against a release built on this machine.
#
#   site/test/install.ps1 -Dist DIST -Version VERSION
#
# DIST holds lo0-windows-<arch>.exe and checksums.txt, as make dist writes
# them and a GitHub release publishes them; VERSION is what the binaries
# report. The installer runs unchanged through Invoke-Expression, as the
# quick start's irm | iex does, with an Invoke-WebRequest that serves DIST
# in place of GitHub.
#
# install.ps1 adds its folder to the user PATH in the registry, so this
# runs only in CI, on a machine that is thrown away afterwards.
param(
  [Parameter(Mandatory)][string]$Dist,
  [Parameter(Mandatory)][string]$Version
)
$ErrorActionPreference = 'Stop'

if (-not $env:CI) { throw 'install.ps1 changes the user PATH; run this test only in CI.' }

$Dist = (Resolve-Path $Dist).Path
$installer = Get-Content -Raw (Join-Path $PSScriptRoot '..\public\install.ps1')
$repo = 'https://github.com/daveearley/lo0.ai/releases'
$arch = if ($env:PROCESSOR_ARCHITECTURE -eq 'ARM64') { 'arm64' } else { 'amd64' }
$name = "lo0-windows-$arch.exe"
$work = Join-Path ([IO.Path]::GetTempPath()) ([IO.Path]::GetRandomFileName())
New-Item -ItemType Directory -Path $work | Out-Null
$script:failures = 0

function Fail([string]$what) { Write-Host "FAIL $what"; $script:failures++ }
function Pass([string]$what) { Write-Host "ok   $what" }

# Answers the installer's downloads from $global:Lo0Serve and logs each URL
# in $global:Lo0Urls. A function takes precedence over the cmdlet of the
# same name. The installer runs in the caller's scope and sets variables of
# its own, so everything the shim reads is global and named apart.
$global:Lo0Repo = $repo
$global:Lo0Serve = $Dist
$global:Lo0Urls = @()
function global:Invoke-WebRequest {
  param([switch]$UseBasicParsing, [string]$Uri, [string]$OutFile)
  $global:Lo0Urls += $Uri
  if (-not $Uri.StartsWith("$global:Lo0Repo/")) { throw "Invoke-WebRequest shim: unexpected URL $Uri" }
  $file = Join-Path $global:Lo0Serve ($Uri -split '/')[-1]
  if (-not (Test-Path $file)) { throw "The remote server returned an error: (404) Not Found. $Uri" }
  Copy-Item $file $OutFile
}

# Runs the installer as irm | iex would, with the given environment, and
# returns the error it threw, if any.
function Install([hashtable]$envVars) {
  $global:Lo0Urls = @()
  $saved = @{}
  foreach ($k in $envVars.Keys) {
    $saved[$k] = [Environment]::GetEnvironmentVariable($k)
    [Environment]::SetEnvironmentVariable($k, $envVars[$k])
  }
  try {
    Invoke-Expression $installer | Out-Host
    return $null
  } catch {
    return $_.Exception.Message
  } finally {
    foreach ($k in $saved.Keys) { [Environment]::SetEnvironmentVariable($k, $saved[$k]) }
  }
}

try {
  # 1. The default: newest release into %LOCALAPPDATA%\Programs\lo0, added
  # to the user PATH.
  $local = Join-Path $work 'fresh\AppData\Local'
  $dir = Join-Path $local 'Programs\lo0'
  $err = Install @{ LOCALAPPDATA = $local; LO0_INSTALL_DIR = $null; LO0_VERSION = $null }
  $exe = Join-Path $dir 'lo0.exe'
  if ($err) { Fail "default install threw: $err" }
  elseif (-not (Test-Path $exe)) { Fail "default install did not write $exe" }
  elseif ((& $exe version) -ne "lo0 $Version") { Fail "installed binary reports $(& $exe version), want lo0 $Version" }
  else { Pass "installs $name to %LOCALAPPDATA%\Programs\lo0" }
  if ($global:Lo0Urls -notcontains "$repo/latest/download/$name") { Fail "default install did not fetch the latest release: $($global:Lo0Urls)" }
  $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
  if (($userPath -split ';') -notcontains $dir) { Fail 'default install did not add its folder to the user PATH' }
  else { Pass 'adds the folder to the user PATH' }

  # 2. Installing again adds the folder to PATH once.
  $err = Install @{ LOCALAPPDATA = $local; LO0_INSTALL_DIR = $null; LO0_VERSION = $null }
  $count = @([Environment]::GetEnvironmentVariable('Path', 'User') -split ';' | Where-Object { $_ -eq $dir }).Count
  if ($err -or $count -ne 1) { Fail "a second install threw '$err' or listed the folder $count times" }
  else { Pass 'a second install leaves PATH as it was' }

  # 3. The quick start's next step: run lo0 on a machine with no history.
  if (Test-Path $exe) {
    $saved = $env:USERPROFILE
    $env:USERPROFILE = Join-Path $work 'fresh'; $env:LO0_OFFLINE = '1'
    # Windows PowerShell turns a native command's stderr into errors, which
    # Stop would make fatal.
    $ErrorActionPreference = 'Continue'
    $out = & $exe match 2>&1 | Out-String
    $code = $LASTEXITCODE
    $ErrorActionPreference = 'Stop'
    $env:USERPROFILE = $saved; $env:LO0_OFFLINE = $null
    if ($code -ne 0 -or $out -notmatch 'Verdict') { Fail "lo0 match after install exited $code`n$out" }
    else { Pass 'lo0 match runs straight after install' }
  }

  # 4. A pinned version into a chosen folder.
  $dir = Join-Path $work 'pinned'
  $err = Install @{ LO0_INSTALL_DIR = $dir; LO0_VERSION = 'v9.9.9' }
  if ($err) { Fail "pinned install threw: $err" }
  elseif ($global:Lo0Urls -notcontains "$repo/download/v9.9.9/$name") { Fail "LO0_VERSION did not pick the release: $($global:Lo0Urls)" }
  elseif (-not (Test-Path (Join-Path $dir 'lo0.exe'))) { Fail 'LO0_INSTALL_DIR was not used' }
  else { Pass 'honours LO0_VERSION and LO0_INSTALL_DIR' }

  # 5. A binary that does not match checksums.txt is never installed.
  $global:Lo0Serve = Join-Path $work 'tampered-dist'
  New-Item -ItemType Directory -Path $global:Lo0Serve | Out-Null
  Copy-Item (Join-Path $Dist 'checksums.txt') $global:Lo0Serve
  Set-Content -Path (Join-Path $global:Lo0Serve $name) -Value 'not lo0'
  $dir = Join-Path $work 'tampered'
  $err = Install @{ LO0_INSTALL_DIR = $dir; LO0_VERSION = $null }
  if (-not $err) { Fail 'a tampered binary was accepted' }
  elseif (Test-Path (Join-Path $dir 'lo0.exe')) { Fail 'a tampered binary was installed' }
  elseif ($err -notmatch 'checksum mismatch') { Fail "tampered install failed without saying why: $err" }
  else { Pass 'refuses a binary that does not match checksums.txt' }
  $global:Lo0Serve = $Dist
} finally {
  Remove-Item -Recurse -Force $work -ErrorAction SilentlyContinue
}

if ($script:failures -gt 0) { Write-Host "$($script:failures) installer checks failed"; exit 1 }
