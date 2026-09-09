namespace Shell1448.Service;

/// <summary>Конфигурация сервиса (appsettings.json, секция "Shell").</summary>
public sealed class ServiceConfig
{
    /// <summary>WS бэкенда, как у Go-агента.</summary>
    public string BackendWs { get; set; } = "ws://localhost:8080/api/v1/ws/shell";

    /// <summary>UUID ПК из таблицы computers. Пусто = WS не подключаем (дев-режим).</summary>
    public string ComputerId { get; set; } = "";

    /// <summary>К4-и3: тот же секрет, что AGENT_TOKEN на сервере. Пусто — только пока сервер без токена.</summary>
    public string AgentToken { get; set; } = "";

    /// <summary>Fail-safe: блокировка при потере связи дольше этого числа секунд (ТЗ 6.3).</summary>
    public int FailSafeSeconds { get; set; } = 120;

    /// <summary>Интервал heartbeat session_tick, сек.</summary>
    public int HeartbeatSeconds { get; set; } = 60;

    /// <summary>К7: подставлять сенсу гостя (payload.sens в session_start) в конфиги CS2/Apex. Выкл по умолчанию.</summary>
    public bool SensEnabled { get; set; } = false;
    /// <summary>К7: true — только лог, файлы не трогаем (боевой режим — после сухого прогона на пилотном ПК).</summary>
    public bool SensDryRun { get; set; } = true;
    /// <summary>К7: корень Steam; пусто — стандартные пути.</summary>
    public string SensSteamDir { get; set; } = "";
    /// <summary>К7: явный путь к cs2_user_convars_0_slot0.vcfg; пусто — самый свежий в userdata.</summary>
    public string SensCs2Cfg { get; set; } = "";
    /// <summary>К7: явный путь к settings.cfg Apex; пусто — Saved Games текущего пользователя.</summary>
    public string SensApexCfg { get; set; } = "";
    /// <summary>С2-и3 (SENS.md, Т2): "exec" (по умолчанию) — CS2 через game/csgo/cfg/1448.cfg + запуск с +exec 1448
    /// (App: shell.json sens_exec), работает под любым Steam-аккаунтом; "vcfg" — запись в vcfg самого свежего аккаунта (К7).</summary>
    public string SensCs2Mode { get; set; } = "exec";
    /// <summary>С2-и3: корень CS2 (…\steamapps\common\Counter-Strike Global Offensive); пусто — ищем по Steam и libraryfolders.vdf.</summary>
    public string SensCs2Dir { get; set; } = "";
}
