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
# id здесь = id записи каталога (seed/catalog-pilot.json), файл ляжет в
# web/covers/<id>.jpg и уже прописан там в cover_url.
$steam = @{
  cs2    = 730
  dota2  = 570
  gta5   = 271590
  apex   = 1172470
  pubg   = 578080
  rivals = 2767030   # Marvel Rivals
  ow2    = 2357570   # Overwatch 2

  # — игры пилотного клуба (дамп PC102 31.08.2026, docs/РАЗВЕДКА-ИГРЫ.md) —
  bf6              = 2807960
  cod              = 1938090
  forza5           = 1551360
  forza4           = 1293830
  bg3              = 1086940
  doom_eternal     = 782330
  rust             = 252490
  arc_raiders      = 1808500
  deadlock         = 1422450
  squad            = 393380
  arma3            = 107410
  quake_champions  = 611500
  l4d2             = 550
  human_fall_flat  = 477160
  cod2             = 2630
  wolfteam_reboot  = 2170050
  ut2004           = 13230
  flatout2         = 2990
  sven_coop        = 225840
  rv_there_yet     = 3949040
  trackmania_nf    = 11020
  half_life        = 70
  cs16_steam       = 10
  quake3           = 2200
  serious_sam_tfe  = 41050
  worms_armageddon = 217200
  revolt           = 287310
  soldat           = 638490
}
$missing = @()
foreach ($id in $steam.Keys) {
  $out = Join-Path $dir "$id.jpg"
  if (Test-Path $out) { Write-Host "skip  $id (есть)"; continue }
  $base = "https://cdn.cloudflare.steamstatic.com/steam/apps/$($steam[$id])"
  try {
    try { Invoke-WebRequest -Uri "$base/library_600x900_2x.jpg" -OutFile $out -UseBasicParsing }
    catch { Invoke-WebRequest -Uri "$base/library_600x900.jpg" -OutFile $out -UseBasicParsing }
    Write-Host "ok    $id  <- steam/$($steam[$id])"
  } catch {
    # У старых игр (Quake III, UT2004, Sven Co-op) вертикальной капсулы в Steam
    # может не быть вовсе. Тогда плитка честно покажет градиент с тегом:
    # cover_url в каталоге останется, файл просто не найдётся, и фон-градиент
    # из shell.html отработает как фолбэк. Ничего чинить не нужно.
    if (Test-Path $out) { Remove-Item $out -Force }   # не оставляем битый файл
    $missing += $id
    Write-Warning "fail  $id : нет вертикальной обложки ($($_.Exception.Message))"
  }
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

if ($missing.Count) {
  Write-Host "`nБез обложки осталось ($($missing.Count)): $($missing -join ', ')" -ForegroundColor Yellow
  Write-Host "Это нормально для старых игр — плитка покажет градиент с тегом."
}

Write-Host "`nГотово. Файлы в web\covers\ — закоммить. Каталог уже ссылается на них"
Write-Host "(cover_url = /covers/<id>.jpg в seed\catalog-pilot.json)."
