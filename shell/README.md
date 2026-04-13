# 14:48 — PC Shell (C# / WinUI 3)

Кастомная оболочка Windows для игровых ПК в клубах.

## Стек
- **C# / .NET 8** + WinUI 3
- **Windows Shell Launcher** API (замена explorer.exe)
- **WebSocket** клиент (System.Net.WebSockets)
- **WPF Overlay** для XP-виджета поверх игр

## Архитектура
```
Shell (ограниченный пользователь)
  ↕ Named Pipe (TLS)
Windows Service (System/Admin права)
  ↕ WebSocket TLS
Go Backend
```

## Ключевые ссылки
- Shell Launcher: https://learn.microsoft.com/windows/configuration/shell-launcher
- KioskAssistant: github.com/florinDNL/KioskAssistant
- Overlay: github.com/lolp1/Overlay.NET

## ВАЖНО
Shell Launcher требует Windows Enterprise / Education.
Для разработки используй Windows 11 Enterprise (Trial доступен бесплатно).
