# Install lo0 from its GitHub releases.
#
#   irm https://lo0.ai/install.ps1 | iex
#
# $env:LO0_VERSION      a release tag such as v0.3.0; the newest release by default
# $env:LO0_INSTALL_DIR  where to put lo0.exe; %LOCALAPPDATA%\Programs\lo0 by default
#
# The binary is checked against the release's checksums.txt before it is
# installed. The install folder is added to your user PATH.

# irm | iex runs in the caller's session, so the script block keeps these
# settings and variables out of it.
& {
  $ErrorActionPreference = 'Stop'
  $ProgressPreference = 'SilentlyContinue'
  [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12

  $repo = 'https://github.com/daveearley/lo0'
  $dir = if ($env:LO0_INSTALL_DIR) { $env:LO0_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA 'Programs\lo0' }

  $arch = switch ($env:PROCESSOR_ARCHITECTURE) {
    'AMD64' { 'amd64' }
    'ARM64' { 'arm64' }
    default { throw "lo0 install: $($env:PROCESSOR_ARCHITECTURE) processors are not supported. lo0 runs on amd64 and arm64." }
  }

  $base = if ($env:LO0_VERSION) { "$repo/releases/download/$($env:LO0_VERSION)" } else { "$repo/releases/latest/download" }
  $name = "lo0-windows-$arch.exe"
  $tmp = Join-Path ([IO.Path]::GetTempPath()) ([IO.Path]::GetRandomFileName())
  New-Item -ItemType Directory -Path $tmp | Out-Null

  try {
    Write-Host "Downloading $name"
    Invoke-WebRequest -UseBasicParsing -Uri "$base/$name" -OutFile (Join-Path $tmp 'lo0.exe')
    Invoke-WebRequest -UseBasicParsing -Uri "$base/checksums.txt" -OutFile (Join-Path $tmp 'checksums.txt')

    $want = Get-Content (Join-Path $tmp 'checksums.txt') |
      ForEach-Object { $f = $_ -split '\s+'; if ($f[1] -eq $name -or $f[1] -eq "*$name") { $f[0] } } |
      Select-Object -First 1
    if (-not $want) { throw "lo0 install: checksums.txt has no entry for $name" }
    $got = (Get-FileHash -Algorithm SHA256 (Join-Path $tmp 'lo0.exe')).Hash.ToLower()
    if ($want.ToLower() -ne $got) { throw "lo0 install: checksum mismatch for ${name}: expected $want, got $got" }

    New-Item -ItemType Directory -Force -Path $dir | Out-Null
    Move-Item -Force (Join-Path $tmp 'lo0.exe') (Join-Path $dir 'lo0.exe')
  } finally {
    Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
  }

  $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
  if (-not (($userPath -split ';') -contains $dir)) {
    [Environment]::SetEnvironmentVariable('Path', ($userPath.TrimEnd(';') + ";$dir").TrimStart(';'), 'User')
    $env:Path = "$env:Path;$dir"
    Write-Host "Added $dir to your PATH. Open a new terminal for other windows to see it."
  }

  Write-Host "Installed lo0 to $dir\lo0.exe"
  Write-Host ''
  Write-Host 'Run lo0 to see what your machine can run.'
}
