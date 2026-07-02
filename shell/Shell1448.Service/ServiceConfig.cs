namespace Shell1448.Service;

/// <summary>Конфигурация сервиса (appsettings.json, секция "Shell").</summary>
public sealed class ServiceConfig
{
    /// <summary>WS бэкенда, как у Go-агента.</summary>
    public string BackendWs { get; set; } = "ws://localhost:8080/api/v1/ws/shell";

    /// <summary>UUID ПК из таблицы computers. Пусто = WS не подключаем (дев-режим).</summary>
    public string ComputerId { get; set; } = "";

    /// <summary>Fail-safe: блокировка при потере связи дольше этого числа секунд (ТЗ 6.3).</summary>
    public int FailSafeSeconds { get; set; } = 120;

    /// <summary>Интервал heartbeat session_tick, сек.</summary>
    public int HeartbeatSeconds { get; set; } = 60;
}
