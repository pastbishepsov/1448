namespace Shell1448.Shared;

using System.Text.RegularExpressions;

/// <summary>
/// С2-и3 (SENS.md, Т2): запуск CS2 с +exec 1448 — игра читает
/// game/csgo/cfg/1448.cfg с сенсой гостя (файл пишет Service/Go-агент).
/// steam://rungameid/730 и steam://run/730 → steam://run/730//+exec%201448/
/// (аргументы после // Steam отдаёт игре; пробел кодируем), exe-цель —
/// аргументы командной строки. Другие игры и выключенный режим — как есть.
/// Чистая функция (тестируется), зеркало withSensArgs в Go-агенте.
/// </summary>
public static class SensLaunch
{
    private static readonly Regex SteamRun = new("^steam://(rungameid|run)/(\\d+)/?$", RegexOptions.IgnoreCase);
    private static readonly Dictionary<string, string> LaunchArgs = new() { ["cs2"] = "+exec 1448" };

    public static (string target, string[] args) WithExec(string appId, string target, string[] args, bool enabled)
    {
        if (!enabled || !LaunchArgs.TryGetValue(appId, out var extra)) return (target, args);
        var t = target.Trim();
        var m = SteamRun.Match(t);
        if (m.Success) return ("steam://run/" + m.Groups[2].Value + "//" + Uri.EscapeDataString(extra) + "/", args);
        if (t.Contains("://", StringComparison.Ordinal)) return (target, args); // чужой протокол — не знаем, как передать
        return (target, [.. args, .. extra.Split(' ', StringSplitOptions.RemoveEmptyEntries)]);
    }
}
