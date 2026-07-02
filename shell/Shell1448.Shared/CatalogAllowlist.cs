namespace Shell1448.Shared;

using System.Text.Json;
using System.Text.Json.Serialization;

/// <summary>Приложение из каталога сервера (GET /catalog).</summary>
public sealed class CatalogItem
{
    [JsonPropertyName("id")]     public string Id { get; set; } = "";
    [JsonPropertyName("target")] public string? Target { get; set; }
    [JsonPropertyName("args")]   public string? Args { get; set; } // JSON-массив строк
}

/// <summary>Ответ /catalog по категориям.</summary>
public sealed class CatalogResponse
{
    [JsonPropertyName("games")]  public List<CatalogItem> Games { get; set; } = [];
    [JsonPropertyName("apps")]   public List<CatalogItem> Apps { get; set; } = [];
    [JsonPropertyName("system")] public List<CatalogItem> System { get; set; } = [];

    public IEnumerable<CatalogItem> All() => Games.Concat(Apps).Concat(System);
}

/// <summary>Одна разрешённая к запуску запись.</summary>
public readonly record struct AllowEntry(string Target, string[] Args);

public static class CatalogAllowlist
{
    /// <summary>
    /// Чистая сборка allowlist из каталога (зеркало remoteAllowlist Go-агента):
    /// пропускаем записи без id/target; битые args → без аргументов.
    /// </summary>
    public static Dictionary<string, AllowEntry> Build(IEnumerable<CatalogItem> items)
    {
        var map = new Dictionary<string, AllowEntry>();
        foreach (var it in items)
        {
            if (string.IsNullOrEmpty(it.Id) || string.IsNullOrEmpty(it.Target)) continue;
            var args = Array.Empty<string>();
            if (!string.IsNullOrEmpty(it.Args))
            {
                try { args = JsonSerializer.Deserialize<string[]>(it.Args!) ?? []; }
                catch (JsonException) { /* битые args — запуск без аргументов */ }
            }
            map[it.Id] = new AllowEntry(it.Target!, args);
        }
        return map;
    }
}
