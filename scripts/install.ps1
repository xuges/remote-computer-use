# Offline per-user installer. No administrator rights or symlinks required.
param(
    [string]$Prefix = (Join-Path $HOME '.remote-computer-use'),
    [ValidateSet('', 'codex', 'opencode', 'claude-code')][string]$Agent = ''
)
$ErrorActionPreference = 'Stop'
if (![System.IO.Path]::IsPathRooted($Prefix)) { throw 'Prefix must be an absolute path.' }
if (![Environment]::Is64BitOperatingSystem -or $env:PROCESSOR_ARCHITECTURE -eq 'ARM64') { throw 'This package requires Windows x64.' }
$release = $PSScriptRoot
$ctl = Join-Path $release 'bin\remote-computer-ctl.exe'
$use = Join-Path $release 'bin\remote-computer-use.exe'
$sid = [System.Security.Principal.WindowsIdentity]::GetCurrent().User
function Protect-Private([string]$Path, [bool]$Directory) {
    $acl = if ($Directory) { New-Object System.Security.AccessControl.DirectorySecurity } else { New-Object System.Security.AccessControl.FileSecurity }
    $acl.SetOwner($sid)
    $acl.SetAccessRuleProtection($true, $false)
    $inherit = if ($Directory) { [System.Security.AccessControl.InheritanceFlags]'ContainerInherit,ObjectInherit' } else { [System.Security.AccessControl.InheritanceFlags]::None }
    foreach ($principal in @($sid.Value, 'S-1-5-18', 'S-1-5-32-544')) {
        $identity = New-Object System.Security.Principal.SecurityIdentifier($principal)
        $rule = New-Object System.Security.AccessControl.FileSystemAccessRule($identity, 'FullControl', $inherit, 'None', 'Allow')
        $acl.AddAccessRule($rule)
    }
    Set-Acl -LiteralPath $Path -AclObject $acl
}
$oldOverride = $env:REMOTE_COMPUTER_LIB_DIR
try {
    $env:REMOTE_COMPUTER_LIB_DIR = ''
    & $ctl doctor
    if ($LASTEXITCODE) { throw 'Bundled runtime check failed.' }
} finally { $env:REMOTE_COMPUTER_LIB_DIR = $oldOverride }
$version = & $ctl --version
if ($LASTEXITCODE -or ($version -ne (& $use --version))) { throw 'CLI versions differ.' }
$hashes = (Get-FileHash -Algorithm SHA256 $ctl).Hash + (Get-FileHash -Algorithm SHA256 $use).Hash
$sha = [System.Security.Cryptography.SHA256]::Create()
try { $id = ([BitConverter]::ToString($sha.ComputeHash([Text.Encoding]::ASCII.GetBytes($hashes)))).Replace('-', '').ToLowerInvariant() } finally { $sha.Dispose() }
$bin = Join-Path $Prefix 'bin'
$versions = Join-Path $Prefix 'releases'
$destination = Join-Path $versions $id
$skill = switch ($Agent) {
    'codex' { Join-Path $HOME '.agents\skills\remote-computer-use' }
    'opencode' { Join-Path $HOME '.config\opencode\skills\remote-computer-use' }
    'claude-code' { Join-Path $HOME '.claude\skills\remote-computer-use' }
}
if ($skill -and (Test-Path -LiteralPath $skill)) {
    $existing = Join-Path $skill 'SKILL.md'
    if (!(Test-Path -LiteralPath $existing) -or (Get-FileHash $existing).Hash -ne (Get-FileHash "$release\skills\remote-computer-use\SKILL.md").Hash) { throw 'Existing Skill differs; preserve it before installing.' }
}
foreach ($path in @($Prefix, $versions, $bin)) {
    if (!(Test-Path -LiteralPath $path)) { New-Item -ItemType Directory -Path $path | Out-Null; Protect-Private $path $true }
    if ((Get-Item -LiteralPath $path).Attributes -band [IO.FileAttributes]::ReparsePoint) { throw 'Installation directories cannot be reparse points.' }
}
foreach ($name in @('remote-computer-ctl', 'remote-computer-use')) {
    $entry = Join-Path $bin "$name.cmd"
    if ((Test-Path -LiteralPath $entry) -and !(Get-Content -LiteralPath $entry -TotalCount 2 | Select-String -SimpleMatch 'rem remote-computer-use launcher')) { throw "Refusing to replace existing command: $entry" }
}
$stage = Join-Path $versions ('.install-' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $stage | Out-Null
try {
    Copy-Item -LiteralPath "$release\bin", "$release\THIRD_PARTY_NOTICES" -Destination $stage -Recurse
    Copy-Item -LiteralPath "$release\config.example.toml" -Destination $stage
    if (Test-Path -LiteralPath $destination) {
        foreach ($name in @('remote-computer-ctl', 'remote-computer-use')) {
            if ((Get-FileHash "$stage\bin\$name.exe").Hash -ne (Get-FileHash "$destination\bin\$name.exe").Hash) { throw 'Existing release is incomplete or modified.' }
        }
    } else { Move-Item -LiteralPath $stage -Destination $destination }
    foreach ($name in @('remote-computer-ctl', 'remote-computer-use')) {
        $entry = Join-Path $bin "$name.cmd"
        $temp = Join-Path $bin ('.entry-' + [guid]::NewGuid().ToString('N'))
        $content = "@echo off`r`nrem remote-computer-use launcher`r`n`"%~dp0..\releases\$id\bin\$name.exe`" %*`r`nexit /b %errorlevel%`r`n"
        [IO.File]::WriteAllText($temp, $content, [Text.Encoding]::ASCII)
        if (Test-Path -LiteralPath $entry) { [IO.File]::Replace($temp, $entry, ($temp + '.previous')); Remove-Item -LiteralPath ($temp + '.previous') } else { [IO.File]::Move($temp, $entry) }
    }
} finally { if (Test-Path -LiteralPath $stage) { Remove-Item -LiteralPath $stage -Recurse -Force } }
$configDir = Join-Path $HOME '.remote-computer-use'
if (!(Test-Path -LiteralPath $configDir)) { New-Item -ItemType Directory -Path $configDir | Out-Null; Protect-Private $configDir $true }
$config = Join-Path $configDir 'config.toml'
if (!(Test-Path -LiteralPath $config)) {
    [IO.File]::Copy("$release\config.example.toml", $config, $false)
    Protect-Private $config $false
    Write-Output "Created configuration template: $config"
} else { Write-Output 'Existing configuration preserved.' }
if ($skill) {
    New-Item -ItemType Directory -Path $skill -Force | Out-Null
    if (!(Test-Path -LiteralPath "$skill\SKILL.md")) { Copy-Item -LiteralPath "$release\skills\remote-computer-use\SKILL.md" -Destination $skill }
}
Write-Output "Installed $version in $Prefix"
Write-Output "Add this directory to your terminal/Agent PATH: $bin"
