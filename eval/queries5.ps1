# Newcomer questions asked of the Orca blueprint with abbd explain and abbd path.
#   powershell -File eval/queries5.ps1 <orca checkout> <abbd exe> <out dir>
param($orca, $abbd, $out)
New-Item -ItemType Directory -Force $out | Out-Null
$qs = @(
  @('q1-explain-mobile', 'explain', 'mobile app (Expo)'),
  @('q2-path-mobile-main', 'path', 'mobile app (Expo)', 'desktop main process'),
  @('q3-path-mobile-relay', 'path', 'mobile app (Expo)', '@orca-cloud/relay'),
  @('q4-path-macos-helper-main', 'path', 'computer-use-macos', 'desktop main process'),
  @('q5-explain-runtime-rpc', 'explain', 'Orca runtime RPC methods'),
  @('q6-path-renderer-main', 'path', 'desktop renderer UI', 'desktop main process'),
  @('q7-explain-push-gateway', 'explain', 'push gateway HTTP API'),
  @('q8-path-cli-mobile', 'path', 'orca CLI', 'mobile app (Expo)')
)
Push-Location $orca
foreach ($q in $qs) {
  $name = $q[0]; $args2 = $q[1..($q.Length - 1)]
  $t0 = Get-Date
  $o = & $abbd @args2 2>&1 | Out-String
  $ms = [int]((Get-Date) - $t0).TotalMilliseconds
  Set-Content -Encoding utf8 -Path (Join-Path $out "$name.txt") -Value ("$ abbd " + ($args2 -join ' | ') + "`n# exit $LASTEXITCODE, $ms ms, $($o.Length) chars`n" + $o)
  "$name exit=$LASTEXITCODE ${ms}ms $($o.Length)chars lines=$(($o -split "`n").Count)"
}
Pop-Location
