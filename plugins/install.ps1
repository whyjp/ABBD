# Install the abbd skills for a host that has no plugin install from a GitHub subfolder.
# Usage: powershell -ExecutionPolicy Bypass -File plugins/install.ps1 -HostName codex|antigravity|claude-code [-Scope project|user] [-Project DIR]
# Same behavior as install.sh: copies plugins/abbd/skills/*, replaces only abbd skill directories,
# writes the MCP config only when the file does not exist, otherwise prints the snippet to merge.
param(
    [Parameter(Mandatory = $true)][ValidateSet('codex', 'antigravity', 'claude-code')][string]$HostName,
    [ValidateSet('project', 'user')][string]$Scope = 'project',
    [string]$Project = '.'
)
$ErrorActionPreference = 'Stop'
$src = Join-Path $PSScriptRoot 'abbd/skills'
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
    # Without a plugin namespace the skill names carry the abbd prefix (PRD v1.5 CR-13).
    $name = "abbd-$($dir.Name)"
    $dest = Join-Path $skills $name
    if (Test-Path $dest) { Remove-Item -Recurse -Force $dest }
    Copy-Item -Recurse $dir.FullName $dest
    $skillFile = Join-Path $dest 'SKILL.md'
    $text = [IO.File]::ReadAllText($skillFile) -replace '(?m)^name: .*$', "name: $name"
    [IO.File]::WriteAllText($skillFile, $text)
    $count++
}
if ($count -eq 0) { Write-Error "no skills under $src" }
Write-Output "installed $count abbd skills into $skills"

$snippet = switch ($fmt) {
    'toml' { "[mcp_servers.abbd]`ncommand = `"abbd`"`nargs = [`"mcp`"]" }
    'json' { "{`n  `"mcpServers`": {`n    `"abbd`": { `"command`": `"abbd`", `"args`": [`"mcp`"] }`n  }`n}" }
    default { '' }
}
if ($mcp) {
    if (Test-Path $mcp) {
        if (Select-String -Path $mcp -Pattern 'abbd' -Quiet) {
            Write-Output "MCP: $mcp already mentions abbd; left unchanged"
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
if (-not (Get-Command abbd -ErrorAction SilentlyContinue)) {
    Write-Output 'next: go install github.com/whyjp/abbd/cmd/abbd@latest (the skills and the MCP server call it)'
}
