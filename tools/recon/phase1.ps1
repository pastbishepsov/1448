<#
  14:48 — Фаза 1: добор разведки за игровым ПК (docs/ФАЗА-1-ЗАХОД.md).

  Что снимает:
    1. ИМЕНА ПРОЦЕССОВ игр — главное. Без них агент не закроет игру по концу
       сессии: для steam:// и лаунчеров PID запуска игре не принадлежит.
    2. Запись на D: (тест-файл создаётся и тут же удаляется) и свободное место.
    3. Куда игра пишет сейвы и кэш — папки в профиле, изменившиеся за сессию.
    4. Точные exe лаунчеров, которых не было в первом дампе
       (Ubisoft Connect, Battlestate/Tarkov, HoYoPlay).
    5. Обложки старой оболочки SENET, если найдутся.

  ЧТО НЕ ДЕЛАЕТ: ничего не ставит, не удаляет, не меняет настройки, не трогает
  SENET и сервер. Единственная запись — тест-файл на D:, он удаляется сразу.

  Запуск (обычный PowerShell, права админа не нужны):
      powershell -ExecutionPolicy Bypass -File phase1.ps1

  Результат — папка «14-48-фаза1-<ПК>-<дата>» на рабочем столе: report.txt,
  process-map.json и готовый catalog-process-patch.json. Забрать её целиком.
#>

param([string]$OutDir = "")

$ErrorActionPreference = 'Continue'
$ProgressPreference    = 'SilentlyContinue'

# ─────────────────────────── куда пишем ───────────────────────────
if (-not $OutDir) {
  $stamp  = Get-Date -Format 'yyyy-MM-dd-HHmm'
  $OutDir = Join-Path ([Environment]::GetFolderPath('Desktop')) "14-48-фаза1-$env:COMPUTERNAME-$stamp"
}
New-Item -ItemType Directory -Force -Path $OutDir | Out-Null
$report = Join-Path $OutDir 'report.txt'
$result = [ordered]@{
  pc = $env:COMPUTERNAME; user = "$env:USERDOMAIN\$env:USERNAME"
  at = (Get-Date).ToString('s'); processes = @{}; candidates = @{}
  disk_d = $null; save_dirs = @(); launchers = @{}; senet_covers = @()
}

function Say([string]$t, [string]$color = 'Gray') { Write-Host $t -ForegroundColor $color }
function Log([string]$t) { Add-Content -Path $report -Value $t -Encoding UTF8 }
function Save-Result {
  ($result | ConvertTo-Json -Depth 6) | Set-Content (Join-Path $OutDir 'process-map.json') -Encoding UTF8
}

Log "14:48 — Фаза 1, $env:COMPUTERNAME, $(Get-Date)"
Log ("=" * 70)

# ─────────────────────────── 1. Запись на D: ───────────────────────────
Say "`n[1/5] Проверяю запись на D: ..." Cyan
$d = Get-PSDrive -Name D -ErrorAction SilentlyContinue
if (-not $d) {
  $result.disk_d = @{ exists = $false }
  Say "  D: не найден" Yellow; Log "D: не найден"
} else {
  $free = [math]::Round($d.Free / 1GB, 1)
  $test = 'D:\_1448_write_test.txt'
  $can  = $false
  try {
    Set-Content -Path $test -Value '14:48 write test' -Encoding UTF8 -ErrorAction Stop
    $can = (Get-Content $test -ErrorAction Stop) -match '14:48'
    Remove-Item $test -Force -ErrorAction SilentlyContinue
  } catch { $can = $false }
  $result.disk_d = @{ exists = $true; free_gb = $free; writable = $can }
  Say ("  D: свободно {0} ГБ, запись: {1}" -f $free, $(if ($can) { 'ДА' } else { 'НЕТ' })) Green
  Log "D: свободно $free ГБ, запись=$can (тест-файл удалён)"
}

# ─────────────────────────── 2. Точные exe лаунчеров ───────────────────────────
Say "`n[2/5] Ищу exe лаунчеров, которых не было в первом дампе ..." Cyan
$hunt = @{
  'Ubisoft Connect' = @('D:\online\Ubisoft Game Launcher', 'C:\Program Files (x86)\Ubisoft\Ubisoft Game Launcher')
  'Battlestate (Tarkov)' = @('D:\online\Battlestate Games\BsgLauncher', 'D:\online\Battlestate Games')
  'HoYoPlay' = @('D:\online\HoYoPlay')
  'Lesta Game Center' = @('D:\online\Lesta\GameCenter')
  'Arena Breakout' = @('D:\online\Arena Breakout Infinite')
}
foreach ($k in $hunt.Keys) {
  $found = @()
  foreach ($p in $hunt[$k]) {
    if (Test-Path $p) {
      Get-ChildItem $p -Filter *.exe -Recurse -Depth 2 -ErrorAction SilentlyContinue |
        Sort-Object Length -Descending | Select-Object -First 6 | ForEach-Object {
          $found += [ordered]@{ path = $_.FullName; mb = [math]::Round($_.Length / 1MB, 1) }
        }
    }
  }
  $result.launchers[$k] = $found
  if ($found.Count) {
    Say ("  {0}: найдено {1} exe" -f $k, $found.Count) Green
    Log "`n$k :"; $found | ForEach-Object { Log ("  {0}  ({1} МБ)" -f $_.path, $_.mb) }
  } else {
    Say ("  {0}: папка не найдена" -f $k) DarkGray
    Log "`n$k : папка не найдена"
  }
}
# ключи игр HoYoPlay (--game=...) из конфигов лаунчера
$hoyo = Get-ChildItem 'D:\online\HoYoPlay' -Include *.ini,*.json,*.cfg -Recurse -Depth 3 -ErrorAction SilentlyContinue
if ($hoyo) {
  $keys = $hoyo | Select-String -Pattern '(hk4e|hkrpg|nap|bh3)[_a-z0-9]*' -AllMatches -ErrorAction SilentlyContinue |
          ForEach-Object { $_.Matches.Value } | Sort-Object -Unique
  if ($keys) { Log "`nHoYoPlay, найденные ключи игр: $($keys -join ', ')"; Say "  HoYoPlay: ключи $($keys -join ', ')" Green }
}
Save-Result

# ─────────────────────────── 3. Обложки SENET ───────────────────────────
Say "`n[3/5] Ищу картинки игр в старой оболочке ..." Cyan
$senetRoots = @(
  "$env:ProgramFiles\Enestech", "${env:ProgramFiles(x86)}\Enestech",
  "$env:ProgramData\Enestech", "$env:ProgramData\SENET",
  "$env:LOCALAPPDATA\Enestech", "$env:APPDATA\Enestech"
) | Where-Object { $_ -and (Test-Path $_) }
foreach ($r in $senetRoots) {
  $imgs = Get-ChildItem $r -Include *.jpg,*.jpeg,*.png,*.webp -Recurse -Depth 5 -ErrorAction SilentlyContinue
  if ($imgs.Count -ge 10) {
    $dirs = $imgs | Group-Object DirectoryName | Sort-Object Count -Descending | Select-Object -First 5
    foreach ($g in $dirs) {
      $mb = [math]::Round((($g.Group | Measure-Object Length -Sum).Sum / 1MB), 1)
      $result.senet_covers += [ordered]@{ dir = $g.Name; files = $g.Count; mb = $mb }
      Say ("  {0} — {1} картинок, {2} МБ" -f $g.Name, $g.Count, $mb) Green
      Log "Картинки SENET: $($g.Name) — $($g.Count) шт., $mb МБ"
    }
  }
}
if (-not $result.senet_covers.Count) { Say "  Похожих папок не нашёл" DarkGray; Log "`nКартинки SENET: не найдены" }
Save-Result

# ─────────────────────────── 4. Имена процессов игр ───────────────────────────
# Что игрой быть не может: сам Steam и его спутники, лаунчеры, оверлеи,
# античиты (их вообще нельзя трогать), драйверы периферии.
$skip = @(
  'steam','steamwebhelper','steamerrorreporter','gameoverlayui','steamservice',
  'EpicGamesLauncher','EpicWebHelper','RiotClientServices','RiotClientUx','RiotClientUxRender',
  'Battle.net','Agent','Battle.net Launcher','EADesktop','EALauncher','EABackgroundService',
  'UbisoftConnect','upc','UplayWebCore','lgc','launcher','HYP','BsgLauncher',
  'vgc','vgtray','EasyAntiCheat','EasyAntiCheat_EOS','BEService','BattlEye','FACEIT',
  'anticheatexpert','ACE-BASE','GameGuard','EAAntiCheat',
  'RazerSynapse','RazerCentral','LGHUB','LGHUB_Updater','NVIDIA Share','nvcontainer',
  'explorer','powershell','pwsh','conhost','cmd','ServiceApp','SenetInstaller','winvnc',
  'MeshAgent','RustDesk','hiddify-next','zabbix_agent2','msedgewebview2','crashpad_handler'
)
function Snapshot { Get-Process -ErrorAction SilentlyContinue | Select-Object -ExpandProperty Id }
function New-Candidates([int[]]$before) {
  $cmd = @{}
  Get-CimInstance Win32_Process -ErrorAction SilentlyContinue | ForEach-Object { $cmd[[int]$_.ProcessId] = $_.CommandLine }
  Get-Process -ErrorAction SilentlyContinue |
    Where-Object { $before -notcontains $_.Id } |
    Where-Object { $skip -notcontains $_.ProcessName } |
    Where-Object { $_.MainWindowHandle -ne 0 -or $_.WorkingSet64 -gt 300MB } |
    Sort-Object WorkingSet64 -Descending |
    Select-Object -First 5 | ForEach-Object {
      $path = ''
      try { $path = $_.Path } catch { }
      [ordered]@{
        process = $_.ProcessName + '.exe'
        ram_mb  = [math]::Round($_.WorkingSet64 / 1MB)
        window  = ($_.MainWindowHandle -ne 0)
        path    = $path
        cmdline = $cmd[[int]$_.Id]
      }
    }
}

$games = @(
  @{ id = 'cs2';      name = 'Counter-Strike 2' }
  @{ id = 'dota2';    name = 'Dota 2' }
  @{ id = 'valorant'; name = 'Valorant' }
  @{ id = 'gta5';     name = 'GTA V' }
  @{ id = 'apex';     name = 'Apex Legends' }
  @{ id = 'pubg';     name = 'PUBG' }
  @{ id = 'rivals';   name = 'Marvel Rivals' }
  @{ id = 'bf6';      name = 'Battlefield 6' }
  @{ id = 'cod';      name = 'Call of Duty' }
  @{ id = 'rust';     name = 'Rust' }
  @{ id = 'forza5';   name = 'Forza Horizon 5' }
  @{ id = 'genshin';  name = 'Genshin Impact' }
)

Say "`n[4/5] Имена процессов игр — самое важное" Cyan
Say "  Порядок на каждую игру: запусти её как обычно → дождись меню (не заставки)" Gray
Say "  → вернись сюда → Enter. Потом закрой игру и снова Enter." Gray
Say "  S + Enter — пропустить игру, Q + Enter — закончить и сохранить.`n" Gray
Log "`n$('=' * 70)`nИМЕНА ПРОЦЕССОВ"

$profileRoots = @("$env:USERPROFILE\Documents", "$env:USERPROFILE\Saved Games", $env:LOCALAPPDATA, $env:APPDATA) |
                Where-Object { $_ -and (Test-Path $_) }
$saveSeen = @{}

foreach ($g in $games) {
  Say ("── {0} ({1})" -f $g.name, $g.id) White
  $a = Read-Host "   Запусти игру, дождись меню, затем Enter (S=пропустить, Q=выход)"
  if ($a -match '^[QqЙй]') { Say "  Останавливаюсь." Yellow; break }
  if ($a -match '^[SsЫы]') { Say "  Пропущено.`n" DarkGray; Log "`n$($g.id): пропущено"; continue }

  $before = Snapshot
  $t0 = Get-Date
  Read-Host "   Игра открыта? Enter — снимаю процессы"
  $cands = @(New-Candidates $before)

  if (-not $cands.Count) {
    Say "  Новых процессов не видно. Игра точно запущена и в меню?" Yellow
    $again = Read-Host "   Enter — снять ещё раз, S — пропустить"
    if ($again -notmatch '^[SsЫы]') { $cands = @(New-Candidates $before) }
  }

  if ($cands.Count) {
    $result.processes[$g.id]  = $cands[0].process
    $result.candidates[$g.id] = $cands
    Say ("  → {0}  ({1} МБ)" -f $cands[0].process, $cands[0].ram_mb) Green
    Log "`n$($g.id) = $($cands[0].process)"
    $cands | ForEach-Object { Log ("    {0}  {1} МБ  окно={2}  {3}" -f $_.process, $_.ram_mb, $_.window, $_.path) }
    if ($cands.Count -gt 1) { Say ("  (ещё кандидаты: {0})" -f (($cands | Select-Object -Skip 1).process -join ', ')) DarkGray }
  } else {
    Say "  Не удалось определить — запиши имя из Диспетчера задач руками" Yellow
    Log "`n$($g.id): не определено автоматически"
  }

  # куда игра писала за это время
  foreach ($root in $profileRoots) {
    Get-ChildItem $root -Directory -ErrorAction SilentlyContinue | Where-Object { $_.LastWriteTime -gt $t0 } |
      ForEach-Object {
        if (-not $saveSeen[$_.FullName]) {
          $saveSeen[$_.FullName] = $true
          $result.save_dirs += [ordered]@{ dir = $_.FullName; after = $g.id }
          Log "    писала в: $($_.FullName)"
        }
      }
  }
  Save-Result
  Read-Host "   Закрой игру полностью, затем Enter"
  Say ""
}

# ─────────────────────────── 5. Итог ───────────────────────────
Say "`n[5/5] Собираю итог ..." Cyan
Log "`n$('=' * 70)`nПАПКИ, В КОТОРЫЕ ПИСАЛИ ИГРЫ (кандидаты в очистку между гостями)"
if ($result.save_dirs.Count) { $result.save_dirs | ForEach-Object { Log ("  {0}   (после {1})" -f $_.dir, $_.after) } }
else { Log "  (ничего не заметил)" }

# готовый патч каталога — доедет, когда поле process появится в БД
$patch = @{ apps = @($result.processes.Keys | ForEach-Object { [ordered]@{ id = $_; process = $result.processes[$_] } }) }
($patch | ConvertTo-Json -Depth 5) | Set-Content (Join-Path $OutDir 'catalog-process-patch.json') -Encoding UTF8
Save-Result

Say ("`nГотово. Снято процессов: {0}" -f $result.processes.Count) Green
Say ("Папка с результатом: {0}" -f $OutDir) Green
Say "Забери её целиком (отчёт + два JSON) и пришли." Gray
Log "`nСнято процессов: $($result.processes.Count)"
