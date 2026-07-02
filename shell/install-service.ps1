# install-service.ps1 — установка Shell1448.Service как Windows-сервиса.
# Запускать в PowerShell ОТ ИМЕНИ АДМИНИСТРАТОРА, из папки со сборкой.
#
#   powershell -ExecutionPolicy Bypass -File install-service.ps1
#
# Перед запуском: впиши computer_id в appsettings.json (UUID ПК из таблицы computers).

param(
    [string]$ExePath = "$PSScriptRoot\Shell1448.Service.exe",
    [string]$ServiceName = "Shell1448"
)

$ErrorActionPreference = "Stop"

if (-not (Test-Path $ExePath)) {
    Write-Error "Не найден $ExePath. Сначала собери сервис (см. shell/README.md)."
    exit 1
}

# Пересоздать, если уже стоит
if (Get-Service $ServiceName -ErrorAction SilentlyContinue) {
    Write-Host "Останавливаю и удаляю прежний сервис $ServiceName..."
    Stop-Service $ServiceName -Force -ErrorAction SilentlyContinue
    sc.exe delete $ServiceName | Out-Null
    Start-Sleep -Seconds 2
}

Write-Host "Регистрирую сервис $ServiceName..."
New-Service -Name $ServiceName -BinaryPathName "`"$ExePath`"" `
    -DisplayName "14:48 Shell Service" -StartupType Automatic `
    -Description "PC Shell 14:48: связь с сервером, fail-safe блокировка, управление сессиями."

# Автоперезапуск при падении (fail-safe на уровне SCM)
sc.exe failure $ServiceName reset= 86400 actions= restart/5000/restart/5000/restart/5000 | Out-Null

Start-Service $ServiceName
Write-Host "Готово. Статус:" -ForegroundColor Green
Get-Service $ServiceName
