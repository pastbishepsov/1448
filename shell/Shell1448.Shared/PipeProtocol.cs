namespace Shell1448.Shared;

using System.Text.Json;
using System.Text.Json.Serialization;

/// <summary>
/// Протокол Named Pipe между Service (привилегированный) и App (ограниченный юзер).
/// Формат: одна JSON-строка на сообщение (line-delimited), UTF-8.
/// Совпадает по духу с WS-контрактом бэкенда (type + payload).
/// </summary>
public static class PipeProtocol
{
    public const string PipeName = "shell1448";

    private static readonly JsonSerializerOptions Options = new()
    {
        PropertyNamingPolicy = JsonNamingPolicy.SnakeCaseLower,
        DefaultIgnoreCondition = JsonIgnoreCondition.WhenWritingNull,
    };

    public static string Serialize(PipeMessage msg) => JsonSerializer.Serialize(msg, Options);

    public static PipeMessage? Deserialize(string line)
    {
        if (string.IsNullOrWhiteSpace(line)) return null;
        try { return JsonSerializer.Deserialize<PipeMessage>(line, Options); }
        catch (JsonException) { return null; }
    }
}

/// <summary>Типы сообщений pipe. Service → App: команды; App → Service: события.</summary>
public static class PipeMsg
{
    // Service → App
    public const string SessionStart = "session_start";
    public const string SessionEnd   = "session_end";
    public const string XpUpdate     = "xp_update";
    public const string ForceUnlock  = "force_unlock";
    public const string Lock         = "lock";   // fail-safe: связь с сервером потеряна
    public const string Unlock       = "unlock"; // связь восстановлена
    public const string State        = "state";  // ответ на hello: текущее состояние

    // App → Service
    public const string Hello     = "hello";      // App подключился, просит state
    public const string AdminCall = "admin_call"; // кнопка вызова администратора
}

public sealed class PipeMessage
{
    [JsonPropertyName("type")]    public string Type { get; set; } = "";
    [JsonPropertyName("payload")] public JsonElement? Payload { get; set; }

    public static PipeMessage Of(string type) => new() { Type = type };

    public static PipeMessage Of(string type, object payload) => new()
    {
        Type = type,
        Payload = JsonSerializer.SerializeToElement(payload),
    };
}
