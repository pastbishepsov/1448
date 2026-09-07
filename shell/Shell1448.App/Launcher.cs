namespace Shell1448.App;

using System.Diagnostics;
using System.Net.Http;
using System.Text.Json;
using Shell1448.Shared;

/// <summary>
/// Запуск приложений строго по allowlist из каталога сервера (GET /catalog).
/// Нативная замена Go-агенту на клубном ПК: exe-пути, протоколы, ms-settings.
/// Чистая логика allowlist — в Shell1448.Shared.CatalogAllowlist (тестируется).
/// </summary>
public sealed class Launcher(string catalogUrl, string agentToken = "")
{
    private static readonly HttpClient Http = new() { Timeout = TimeSpan.FromSeconds(5) };
    private Dictionary<string, AllowEntry> _allow = new();

    /// <summary>К4-и3: полный каталог (пути, аргументы) — только по ?full=1 с токеном агента.</summary>
    public static string FullCatalogUrl(string url) => url.Contains('?') ? url + "&full=1" : url + "?full=1";

    public async Task RefreshAsync()
    {
        try
        {
            using var req = new HttpRequestMessage(HttpMethod.Get, FullCatalogUrl(catalogUrl));
            if (!string.IsNullOrWhiteSpace(agentToken))
                req.Headers.TryAddWithoutValidation("X-Agent-Token", agentToken.Trim());
            using var resp = await Http.SendAsync(req);
            if (!resp.IsSuccessStatusCode) return; // 401: токен не совпал — работаем со старым списком
            var json = await resp.Content.ReadAsStringAsync();
            var cr = JsonSerializer.Deserialize<CatalogResponse>(json);
            if (cr is null) return;
            _allow = CatalogAllowlist.Build(cr.All());
        }
        catch { /* каталог недоступен — работаем со старым списком */ }
    }

    public bool Launch(string appId)
    {
        if (!_allow.TryGetValue(appId, out var app)) return false;
        try
        {
            // cmd start понимает и exe, и протоколы (steam://), и ms-settings:
            var psi = new ProcessStartInfo(app.Target) { UseShellExecute = true };
            foreach (var a in app.Args) psi.ArgumentList.Add(a);
            Process.Start(psi);
            return true;
        }
        catch { return false; }
    }
}
