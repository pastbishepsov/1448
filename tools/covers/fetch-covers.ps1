# 14:48 — скачивание обложек игр в web/covers/ (постеры 600x900 под плитку 2:3).
# Запуск из корня репо:  powershell -ExecutionPolicy Bypass -File tools\covers\fetch-covers.ps1
# С SteamGridDB-ключом (для Valorant/LoL/Fortnite):  ... fetch-covers.ps1 -SgdbKey "КЛЮЧ"
# Ключ бесплатно: https://www.steamgriddb.com/profile/preferences/api
# Уже скачанные файлы пропускаются. Итог кладётся в web/covers/{id}.jpg — закоммить.

param([string]$SgdbKey = '')

$ErrorActionPreference = 'Stop'
$root = Join-Path $PSScriptRoot '..\..'
$dir  = Join-Path $root 'web\covers'
New-Item -ItemType Directory -Force -Path $dir | Out-Null

# — Steam: официальные вертикальные капсулы библиотеки (публичный CDN, без ключа) —
$steam = @{
  cs2    = 730
  dota2  = 570
  gta5   = 271590
  apex   = 1172470
  pubg   = 578080
  rivals = 2767030   # Marvel Rivals
  ow2    = 2357570   # Overwatch 2
}
foreach ($id in $steam.Keys) {
  $out = Join-Path $dir "$id.jpg"
  if (Test-Path $out) { Write-Host "skip  $id (есть)"; continue }
  $base = "https://cdn.cloudflare.steamstatic.com/steam/apps/$($steam[$id])"
  try {
    try { Invoke-WebRequest -Uri "$base/library_600x900_2x.jpg" -OutFile $out -UseBasicParsing }
    catch { Invoke-WebRequest -Uri "$base/library_600x900.jpg" -OutFile $out -UseBasicParsing }
    Write-Host "ok    $id  <- steam/$($steam[$id])"
  } catch { Write-Warning "fail  $id : $($_.Exception.Message)" }
}

# — Вне Steam: SteamGridDB (нужен API-ключ) —
$sgdb = @{ valorant = 'Valorant'; lol = 'League of Legends'; fortnite = 'Fortnite' }
if ($SgdbKey) {
  $H = @{ Authorization = "Bearer $SgdbKey" }
  foreach ($id in $sgdb.Keys) {
    $out = Join-Path $dir "$id.jpg"
    if (Test-Path $out) { Write-Host "skip  $id (есть)"; continue }
    try {
      $q = [uri]::EscapeDataString($sgdb[$id])
      $game = (Invoke-RestMethod "https://www.steamgriddb.com/api/v2/search/autocomplete/$q" -Headers $H).data | Select-Object -First 1
      if (-not $game) { Write-Warning "fail  $id : не найдено в SGDB"; continue }
      $grid = (Invoke-RestMethod "https://www.steamgriddb.com/api/v2/grids/game/$($game.id)?dimensions=600x900&styles=alternate,official&nsfw=false" -Headers $H).data | Select-Object -First 1
      if (-not $grid) { Write-Warning "fail  $id : нет обложек 600x900"; continue }
      Invoke-WebRequest -Uri $grid.url -OutFile $out -UseBasicParsing
      Write-Host "ok    $id  <- sgdb/$($game.id) (автор: $($grid.author.name))"
    } catch { Write-Warning "fail  $id : $($_.Exception.Message)" }
  }
} else {
  Write-Host "`nБез -SgdbKey пропущены: $($sgdb.Keys -join ', ') (Steam-версий нет)." -ForegroundColor Yellow
  Write-Host "Либо положи свои файлы вручную: web\covers\valorant.jpg, lol.jpg, fortnite.jpg (600x900)."
}

Write-Host "`nГотово. Плитки подхватят файлы автоматически (COVERS в web/shell.html)."
