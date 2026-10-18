# Install the cccv skills for a host that has no plugin install from a GitHub subfolder.
# Usage: powershell -ExecutionPolicy Bypass -File plugins/install.ps1 -HostName codex|antigravity|claude-code [-Scope project|user] [-Project DIR]
# Same behavior as install.sh: copies plugins/cccv/skills/*, replaces only cccv skill directories,
# writes the MCP config only when the file does not exist, otherwise prints the snippet to merge.
param(
    [Parameter(Mandatory = $true)][ValidateSet('codex', 'antigravity', 'claude-code')][string]$HostName,
    [ValidateSet('project', 'user')][string]$Scope = 'project',
    [string]$Project = '.'
)
$ErrorActionPreference = 'Stop'
$src = Join-Path $PSScriptRoot 'cccv/skills'
$home_ = [Environment]::GetFolderPath('UserProfile')

$targets = @{
    'codex:project'       = @((Join-Path $Project '.agents/skills'), (Join-Path $Project '.codex/config.toml'), 'toml')
    'codex:user'          = @((Join-Path $home_ '.agents/skills'), (Join-Path $home_ '.codex/config.toml'), 'toml')
    'antigravity:project' = @((Join-Path $Project '.agents/skills'), (Join-Path $Project '.agents/mcp_config.json'), 'json')
    'antigravity:user'    = @((Join-Path $home_ '.gemini/config/skills'), (Join-Path $home_ '.gemini/config/mcp_config.json'), 'json')
    'claude-code:project' = @((Join-Path $Project '.claude/skills'), (Join-Path $Project '.mcp.json'), 'json')
    'claude-code:user'    = @((Join-Path $home_ '.claude/skills'), '', 'none')
}
if ($Scope -eq 'project' -and -not (Test-Path $Project -PathType Container)) {
    Write-Error "project directory not found: $Project"
}
$skills, $mcp, $fmt = $targets["${HostName}:$Scope"]

New-Item -ItemType Directory -Force $skills | Out-Null
$count = 0
foreach ($dir in Get-ChildItem $src -Directory) {
    if (-not (Test-Path (Join-Path $dir.FullName 'SKILL.md'))) { continue }
    $dest = Join-Path $skills $dir.Name
    if (Test-Path $dest) { Remove-Item -Recurse -Force $dest }
    Copy-Item -Recurse $dir.FullName $dest
    $count++
}
if ($count -eq 0) { Write-Error "no skills under $src" }
Write-Output "installed $count cccv skills into $skills"

$snippet = switch ($fmt) {
    'toml' { "[mcp_servers.cccv]`ncommand = `"cccv`"`nargs = [`"mcp`"]" }
    'json' { "{`n  `"mcpServers`": {`n    `"cccv`": { `"command`": `"cccv`", `"args`": [`"mcp`"] }`n  }`n}" }
    default { '' }
}
if ($mcp) {
    if (Test-Path $mcp) {
        if (Select-String -Path $mcp -Pattern 'cccv' -Quiet) {
            Write-Output "MCP: $mcp already mentions cccv; left unchanged"
        } else {
            Write-Output "MCP: $mcp exists; merge this entry yourself:`n`n$snippet"
        }
    } else {
        New-Item -ItemType Directory -Force (Split-Path $mcp) | Out-Null
        # WriteAllText writes UTF-8 without a BOM; a BOM breaks TOML parsers.
        [IO.File]::WriteAllText((Join-Path (Resolve-Path (Split-Path $mcp)) (Split-Path $mcp -Leaf)), $snippet + "`n")
        Write-Output "MCP: wrote $mcp"
    }
}
if (-not (Get-Command cccv -ErrorAction SilentlyContinue)) {
    Write-Output 'next: go install github.com/whyjp/cccv/cmd/cccv@latest (the skills and the MCP server call it)'
}
