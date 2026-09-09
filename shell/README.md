# 14:48 — PC Shell (C# / .NET 8)

Кастомная оболочка Windows для игровых ПК клуба. Промышленная замена
Go-агенту (`backend/cmd/agent`): нативный киоск + привилегированный сервис.

## Из чего состоит

```
Shell1448.Shared    протокол Named Pipe, машина состояний, allowlist (чистая логика + тесты)
Shell1448.Service   Windows-сервис: WS к бэкенду, heartbeat, fail-safe, pipe-сервер,
                    сенса гостя в конфиги игр (SensApplier.cs, К7)
Shell1448.App       WPF-киоск: fullscreen WebView2 (shell.html), блокировка клавиш,
                    запуск игр по каталогу, XP-оверлей поверх игр, hard-lock
Shell1448.Tests     xunit — ShellState, PipeProtocol, CatalogAllowlist (кросс-платформенные)
```

Архитектура (ТЗ 6.3): App работает под **ограниченным** пользователем, Service —
под System. Связь между ними — Named Pipe. Fail-safe: если Service теряет связь
с сервером дольше `FailSafeSeconds` (по умолчанию 120), он шлёт App команду `lock`.

## Почему WPF + WebView2, а не WinUI 3

WPF собирается без Visual Studio (`dotnet` CLI), WebView2 предустановлен в Win11,
и главное — внутри киоска переиспользуется **готовый** гостевой экран
`web/shell.html`. Нативная часть добавляет только то, чего браузеру нельзя:
запуск процессов, блокировку системных клавиш, оверлей, связь с сервисом.
Мост: `window.chrome.webview.postMessage({cmd:'launch', id})` — экран сам
определяет, что он внутри киоска (`NATIVE`), и шлёт запуск в C# вместо HTTP-агента.

## Сборка

Кросс-платформенные проекты (Shared, Tests, Service) — собираются где угодно,
где есть .NET 8 SDK. App (WPF) — **только на Windows**.

```bash
# тесты логики (Linux/Mac/Windows или в CI):
dotnet test shell/Shell1448.Tests/Shell1448.Tests.csproj

# сервис — single-file exe (можно и в Docker linux-контейнере с dotnet SDK):
dotnet publish shell/Shell1448.Service -c Release -r win-x64 --self-contained -p:PublishSingleFile=true
```

```powershell
# киоск (WPF) — на Windows:
dotnet publish shell\Shell1448.App -c Release -r win-x64 --self-contained -p:PublishSingleFile=true
# положи web/shell.html и web/aim.html рядом: <вывод>\web\, либо укажи shell_url в shell.json
```

CI (`.github/workflows/ci.yml`, job `shell`) собирает всё на windows-runner и
гоняет тесты — так проверяется компиляция WPF без локального Windows.

## Установка на клубной машине (фаза 1 — киоск)

1. Собери Service и App (выше).
2. Service: впиши `computer_id` и `AgentToken` (тот же секрет, что
   `AGENT_TOKEN` на сервере — К4-и3) в `appsettings.json`, затем
   `install-service.ps1` от администратора.
3. App: скопируй `shell.example.json` → `shell.json`, задай `exit_password` и
   `agent_token` (без него сервер с токеном не отдаст каталог с путями — кнопки
   игр не запустят ничего); положи рядом `web/shell.html` и `web/aim.html`
   (или укажи `shell_url` — тогда оба файла отдаёт сервер из `web/`).
4. Автозапуск App при входе гостя — через реестр/автозагрузку (фаза 1) либо
   Shell Launcher (фаза 2).

Аварийный выход из киоска: **Ctrl+Alt+Shift+Q** + пароль из `shell.json`.

## Дорожная карта

- **Фаза 1 (готово, этот код):** киоск, сервис, fail-safe, запуск по allowlist,
  оверлей, блокировка Win/Alt+Tab/Alt+F4. Работает на Windows 10/11 Home/Pro.
- **Фаза 2:** [Shell Launcher](https://learn.microsoft.com/windows/configuration/shell-launcher)
  (замена explorer.exe) — требует Windows **Enterprise/Education**. Отключает
  Ctrl+Alt+Del/Task Manager на уровне политики.
- **Фаза 3:** совместимость с античитами (VAC, Vanguard, EAC, BattlEye),
  восстановление образа ПК до baseline после сессии, Named Pipe с проверкой подписи.

## Что закрыто софтом, а что ждёт фазы 2 (Г9-и5)

Го-агент (`backend/cmd/agent`) на старте сессии программно гасит залипание/
фильтрацию/озвучивание клавиш (`SystemParametersInfo`, `keys_windows.go`), а
комплект реестра `backend/cmd/agent/kiosk-policies.reg` (импорт установщиком от
администратора; откат — `kiosk-policies-undo.reg`) добивает системные пути.
Полное покрытие клавиатуры даёт C#-киоск (`Shell1448.App`, хук фазы 1).

| Что | Как закрыто в фазе 1 | Статус |
|-----|----------------------|--------|
| Залипание/фильтрация клавиш (5×Shift и т.п.) | агент `SystemParametersInfo` + `.reg` Accessibility Flags | ✅ софтом |
| Win, Win+D, Win+R, Alt+Tab, Alt+F4 | клавиатурный хук `Shell1448.App` + `NoRun`/`NoWinKeys` в `.reg` | ✅ софтом |
| Диспетчер задач | `DisableTaskMgr` в `.reg` | ✅ политикой |
| Блокировка ПК / смена пароля / автозапуск носителей | `.reg` (System/Explorer policies) | ✅ политикой |
| Автозапуск киоска при входе гостя | реестр/автозагрузка | ✅ фаза 1 (Shell Launcher — чище, фаза 2) |
| **Ctrl+Alt+Del (Secure Attention Sequence)** | перехватить нельзя без политики Enterprise | ⏳ фаза 2 (Shell Launcher, Windows Enterprise/Education) |
| Explorer.exe как оболочка (жёсткий киоск) | — | ⏳ фаза 2 |
| Восстановление образа ПК после сессии | — | ⏳ фаза 3 (железо) |

Очистку между гостями (закрыть Steam/Discord/Riot/Epic, вычистить Downloads/
Desktop/%TEMP%/корзину/буфер) делает агент по блоку `cleanup` в `agent.json` на
`session_end`; по умолчанию `dry_run: true` — только пишет план в лог, боевой
режим включается после сухого прогона на живом ПК. Пути очистки проходят
проверку безопасности (`safeCleanupDir`): системные каталоги и мелкие пути не
трогаются даже при опечатке в конфиге.

## Сенса гостя в конфиги игр (К7, KIOSK.md Р9 → С2, SENS.md)

Сервер кладёт профиль сенсы гостя в `session_start` (`payload.sens {dpi,
games, rescaled_from?}`, уже пересчитанный под DPI мыши клуба — настройка
`mouse_dpi` в админке) и шлёт `sens_apply {dpi, games}`, когда гость сохранил
профиль во время сессии; `SensApplier.cs` в Service — порт `sens.go`
Go-агента. CS2 в режиме `exec` (по умолчанию): пишем
`<CS2>\game\csgo\cfg\1448.cfg` со строкой `sensitivity "…"`, а App запускает
CS2 с `+exec 1448` (`shell.json` `sens_exec: true`, `Shell1448.Shared/
SensLaunch.cs`: `steam://rungameid/730` → `steam://run/730//+exec%201448/`) —
работает под любым Steam-аккаунтом и не зависит от момента входа в Steam.
Файл живёт всегда: с сенсой после `session_start`/`sens_apply`, пустой после
`session_end`. Все vcfg (`userdata\*\730`) на первой записи в сессии
копируются в `<файл>.1448bak` и возвращаются на `session_end` и на старте
сервиса (клубный аккаунт как был; аккаунт гостя, появившийся в сессии, не
трогаем). Режим `vcfg` (К7) — запись `"sensitivity"` в
`cs2_user_convars_0_slot0.vcfg` самого свежего аккаунта. Apex —
`mouse_sensitivity` в `settings.cfg`; Valorant не умеем (облако Riot). На
`sens_apply` сервис отвечает `sens_result {applied, skipped, dry_run,
running: []}` (запущенные игры знает только App) — сервер делает из него тост
гостю. Ключи в `appsettings.json` (`Shell:`):

| Ключ | По умолчанию | Что |
|------|--------------|-----|
| `SensEnabled` | `false` | подставлять ли вообще |
| `SensDryRun` | `true` | только лог «записал бы …», файлы не трогаем — боевой режим после сухого прогона на пилотном ПК |
| `SensSteamDir` | `""` | корень Steam; пусто — `%ProgramFiles(x86)%\Steam` и стандартные пути, vcfg — самый свежий из `userdata\*\730\local\cfg` |
| `SensCs2Cfg` | `""` | явный путь к vcfg (главнее поиска) |
| `SensApexCfg` | `""` | явный путь к `settings.cfg`; пусто — `Saved Games\Respawn\Apex\local` пользователя, под которым идёт сервис (под System это НЕ профиль гостя — на пилоте задать явно) |
| `SensCs2Mode` | `"exec"` | `exec` — 1448.cfg + запуск с `+exec 1448` (App: `sens_exec`); `vcfg` — запись в vcfg (К7) |
| `SensCs2Dir` | `""` | корень CS2 (`…\steamapps\common\Counter-Strike Global Offensive`); пусто — по Steam и `libraryfolders.vdf` |

`shell.json` киоска: `"sens_exec": true` — CS2 стартует с `+exec 1448`
(по умолчанию `false`, как и вся сенса; без файла `exec` в консоли CS2 лишь
ругнётся). Кнопки «Применить в игре» в студии больше нет — «Сохранить в
аккаунт» при активной сессии сам доходит до ПК через сервер (`sens_apply`).
На пилотном ПК с Go-агентом рядом сенсу ведёт агент (`sens.enabled`), а
`sens_exec` в `shell.json` нужен всё равно — игры запускает App. Подробности
— `docs/АГЕНТ-ЗАПУСК-ИГР.md`, раздел «Сенса в игру».

## AI-калибратор сенсы внутри киоска (С0, SENS.md)

`window.open` в киоске запрещён (`NewWindowRequested → Handled`), поэтому
калибратор `web/aim.html` шелл открывает не окном, а оверлеем: `<iframe
allow="pointer-lock" src="aim.html?embed=1&dpi=&sens=&lang=">` поверх
экрана (z-index ниже лок-экрана и тостов — предупреждения о нуле/AFK видны,
конец сессии накрывает калибровку и убирает iframe). Результат идёт
`postMessage`'ом `{type:'aim_result', sens, dpi, cm360, verdict:
rec|cur|tie|none, …}`, кнопки «Подставить и сохранить в аккаунт» /
«Закрыть» — `aim_save` / `aim_close`; localStorage-моста больше нет, чужой
результат следующему гостю не достаётся (шелл забывает его на выходе).
Нативной части делать ничего не нужно, кроме одного: `aim.html` должен
лежать рядом с `shell.html` (или отдаваться сервером при `shell_url` =
https). На пилотном ПК проверить: клик по арене захватывает курсор в
iframe (WebView2), Esc отпускает; если захват не даётся — запасной путь в
SENS.md (С0-и5).

## ВАЖНО

Клавиатурный хук глотает Win/Alt+Tab/Alt+F4, но **Ctrl+Alt+Del перехватить нельзя**
(Secure Attention Sequence) — это делается только политикой в фазе 2 на Enterprise.
Поэтому фаза 1 — рабочий пилот для одного-двух клубов, а не финальная защита.
