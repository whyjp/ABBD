# Install For My Harness skills into a project for Claude Code.
# Usage: install.ps1 [-Project <dir>]   (default: current directory)
# Re-running replaces only the harness skill directories; other skills are left alone.
param(
    [string]$Project = "."
)
$ErrorActionPreference = "Stop"

$src = Join-Path $PSScriptRoot "..\..\skills"
if (-not (Test-Path -LiteralPath $Project -PathType Container)) {
    Write-Error "project directory not found: $Project"
    exit 1
}

$dest = Join-Path $Project ".claude\skills"
New-Item -ItemType Directory -Force -Path $dest | Out-Null

$count = 0
foreach ($dir in Get-ChildItem -LiteralPath $src -Directory) {
    if (-not (Test-Path -LiteralPath (Join-Path $dir.FullName "SKILL.md"))) { continue }
    $target = Join-Path $dest $dir.Name
    if (Test-Path -LiteralPath $target) {
        Remove-Item -LiteralPath $target -Recurse -Force -Confirm:$false
    }
    Copy-Item -LiteralPath $dir.FullName -Destination $target -Recurse
    Write-Output "installed $target"
    $count++
}

if ($count -eq 0) {
    Write-Error "no skills found under $src"
    exit 1
}

Write-Output @'

Add this server to <project>/.mcp.json (merge with existing "mcpServers" if present):

{
  "mcpServers": {
    "myharness": {
      "command": "myharness",
      "args": ["mcp"]
    }
  }
}
'@
