# 14:48 — PC Shell (C# / .NET 8)

Кастомная оболочка Windows для игровых ПК клуба. Промышленная замена
Go-агенту (`backend/cmd/agent`): нативный киоск + привилегированный сервис.

## Из чего состоит

```
Shell1448.Shared    протокол Named Pipe, машина состояний, allowlist (чистая логика + тесты)
Shell1448.Service   Windows-сервис: WS к бэкенду, heartbeat, fail-safe, pipe-сервер
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
# положи web/shell.html рядом: <вывод>\web\shell.html, либо укажи shell_url в shell.json
```

CI (`.github/workflows/ci.yml`, job `shell`) собирает всё на windows-runner и
гоняет тесты — так проверяется компиляция WPF без локального Windows.

## Установка на клубной машине (фаза 1 — киоск)

1. Собери Service и App (выше).
2. Service: впиши `computer_id` в `appsettings.json`, затем
   `install-service.ps1` от администратора.
3. App: скопируй `shell.example.json` → `shell.json`, задай `exit_password`;
   положи рядом `web/shell.html` (или укажи `shell_url`).
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

## ВАЖНО

Клавиатурный хук глотает Win/Alt+Tab/Alt+F4, но **Ctrl+Alt+Del перехватить нельзя**
(Secure Attention Sequence) — это делается только политикой в фазе 2 на Enterprise.
Поэтому фаза 1 — рабочий пилот для одного-двух клубов, а не финальная защита.
