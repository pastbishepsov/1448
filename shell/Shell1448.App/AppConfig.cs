namespace Shell1448.App;

using System.IO;
using System.Text.Json;
using System.Text.Json.Serialization;

/// <summary>Конфиг киоска: shell.json рядом с exe.</summary>
public sealed class AppConfig
{
    // К4-и3 (попутно): ключи shell.json — snake_case, как в shell.example.json и README;
    // без JsonPropertyName System.Text.Json их не связывал, и конфиг молча оставался
    // дефолтным (в т.ч. exit_password).
    /// <summary>URL гостевого экрана (file:///C:/.../web/shell.html или https).</summary>
    [JsonPropertyName("shell_url")]
    public string ShellUrl { get; set; } = "";

    /// <summary>Каталог приложений (allowlist запуска).</summary>
    [JsonPropertyName("catalog_url")]
    public string CatalogUrl { get; set; } = "http://localhost:8080/api/v1/catalog";

    /// <summary>К4-и3: токен агента (AGENT_TOKEN сервера) — без него полный каталог с путями не отдаётся.</summary>
    [JsonPropertyName("agent_token")]
    public string AgentToken { get; set; } = "";

    /// <summary>Киоск: блокировать системные клавиши (Win, Alt+Tab, Alt+F4). Выкл для дева.</summary>
    [JsonPropertyName("kiosk_keys")]
    public bool KioskKeys { get; set; } = true;

    /// <summary>Аварийный выход из киоска: Ctrl+Alt+Shift+Q + этот пароль.</summary>
    [JsonPropertyName("exit_password")]
    public string ExitPassword { get; set; } = "1448";

    /// <summary>С2-и3 (SENS.md): запускать CS2 с +exec 1448 — сенса гостя из game/csgo/cfg/1448.cfg,
    /// который пишет Service (SensEnabled, SensCs2Mode=exec) или Go-агент (sens.enabled, cs2_mode exec).
    /// Выкл по умолчанию, как и вся сенса; без файла exec в консоли CS2 лишь ругнётся.</summary>
    [JsonPropertyName("sens_exec")]
    public bool SensExec { get; set; } = false;

    public static AppConfig Load()
    {
        var path = Path.Combine(AppContext.BaseDirectory, "shell.json");
        try
        {
            if (File.Exists(path))
                return JsonSerializer.Deserialize<AppConfig>(File.ReadAllText(path)) ?? new AppConfig();
        }
        catch { /* дефолты ниже */ }
        return new AppConfig();
    }
}
