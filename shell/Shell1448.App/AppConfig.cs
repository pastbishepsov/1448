namespace Shell1448.App;

using System.IO;
using System.Text.Json;

/// <summary>Конфиг киоска: shell.json рядом с exe.</summary>
public sealed class AppConfig
{
    /// <summary>URL гостевого экрана (file:///C:/.../web/shell.html или https).</summary>
    public string ShellUrl { get; set; } = "";

    /// <summary>Каталог приложений (allowlist запуска).</summary>
    public string CatalogUrl { get; set; } = "http://localhost:8080/api/v1/catalog";

    /// <summary>Киоск: блокировать системные клавиши (Win, Alt+Tab, Alt+F4). Выкл для дева.</summary>
    public bool KioskKeys { get; set; } = true;

    /// <summary>Аварийный выход из киоска: Ctrl+Alt+Shift+Q + этот пароль.</summary>
    public string ExitPassword { get; set; } = "1448";

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
