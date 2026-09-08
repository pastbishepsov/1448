using System.Globalization;
using System.Text;
using System.Text.Json;
using System.Text.RegularExpressions;
using Microsoft.Extensions.Logging;

namespace Shell1448.Service;

/// <summary>
/// К7 «Сенса в игру» (KIOSK.md, Р9) — порт sens.go Go-агента для ПК с C#-киоском.
/// Профиль сенсы гостя приходит в session_start (payload.sens {dpi, games}) и
/// подставляется в конфиги игр: CS2 — "sensitivity" в cs2_user_convars_0_slot0.vcfg,
/// Apex — mouse_sensitivity в settings.cfg. Перед первой записью — бэкап
/// (&lt;файл&gt;.1448bak), на session_end и на старте сервиса бэкапы возвращаются.
/// Valorant не умеем (облако Riot). DryRun=true по умолчанию — только лог.
/// </summary>
public sealed class SensApplier(ILogger log, ServiceConfig cfg)
{
    public const string BackupSuffix = ".1448bak";
    private static readonly Regex VcfgKey = new("^(\\s*)\"([^\"]+)\"(\\s+)\"([^\"]*)\"(\\s*)$");
    private static readonly Regex ApexLine = new("^(\\s*)([A-Za-z0-9_]+)(\\s+)\"([^\"]*)\"(\\s*)$");
    private readonly HashSet<string> _backed = new(StringComparer.OrdinalIgnoreCase);
    private readonly object _lock = new();

    public static string FormatSens(double v)
    {
        var s = v.ToString("0.####", CultureInfo.InvariantCulture);
        return s == "" || s == "-0" ? "0" : s;
    }

    /// <summary>Строки с сохранением своих переводов строки (CRLF/LF как есть).</summary>
    private static List<string> SplitLines(string s)
    {
        var lines = new List<string>();
        int start = 0;
        for (int i = 0; i < s.Length; i++)
        {
            if (s[i] != '\n') continue;
            lines.Add(s[start..(i + 1)]);
            start = i + 1;
        }
        if (start < s.Length) lines.Add(s[start..]);
        return lines;
    }

    private static string LineEnding(List<string> lines)
    {
        foreach (var l in lines)
        {
            if (l.EndsWith("\r\n")) return "\r\n";
            if (l.EndsWith('\n')) return "\n";
        }
        return "\n";
    }

    /// <summary>"key" "value" внутри блока "convars" { … }; остальное байт в байт.</summary>
    public static (string text, bool changed) VcfgSetConvar(string content, string key, string value)
    {
        var lines = SplitLines(content);
        var nl = LineEnding(lines);
        bool inConvars = false; int depth = 0, convarsOpen = -1;
        for (int i = 0; i < lines.Count; i++)
        {
            var l = lines[i].TrimEnd('\r', '\n');
            var t = l.Trim();
            if (!inConvars)
            {
                if (string.Equals(t, "\"convars\"", StringComparison.OrdinalIgnoreCase)) { inConvars = true; depth = 0; }
                continue;
            }
            if (t == "{") { depth++; if (depth == 1) convarsOpen = i; continue; }
            if (t == "}")
            {
                depth--;
                if (depth == 0)
                {
                    if (convarsOpen < 0) break;
                    var indent = "\t\t";
                    if (convarsOpen + 1 < i)
                    {
                        var m0 = VcfgKey.Match(lines[convarsOpen + 1].TrimEnd('\r', '\n'));
                        if (m0.Success) indent = m0.Groups[1].Value;
                    }
                    lines.Insert(convarsOpen + 1, indent + "\"" + key + "\"\t\t\"" + value + "\"" + nl);
                    return (string.Concat(lines), true);
                }
                continue;
            }
            if (depth != 1) continue;
            var m = VcfgKey.Match(l);
            if (!m.Success || !string.Equals(m.Groups[2].Value, key, StringComparison.OrdinalIgnoreCase)) continue;
            if (m.Groups[4].Value == value) return (content, false);
            lines[i] = m.Groups[1].Value + "\"" + m.Groups[2].Value + "\"" + m.Groups[3].Value + "\"" + value + "\"" + m.Groups[5].Value + nl;
            return (string.Concat(lines), true);
        }
        var sb = new StringBuilder(string.IsNullOrWhiteSpace(content) ? "" : content);
        if (sb.Length > 0 && !content.EndsWith('\n')) sb.Append(nl);
        sb.Append("\"config\"").Append(nl).Append('{').Append(nl).Append("\t\"convars\"").Append(nl).Append("\t{").Append(nl)
          .Append("\t\t\"").Append(key).Append("\"\t\t\"").Append(value).Append('"').Append(nl).Append("\t}").Append(nl).Append('}').Append(nl);
        return (sb.ToString(), true);
    }

    /// <summary>key "value" в settings.cfg Apex; нет ключа — в конец.</summary>
    public static (string text, bool changed) ApexCfgSet(string content, string key, string value)
    {
        var lines = SplitLines(content);
        var nl = LineEnding(lines);
        for (int i = 0; i < lines.Count; i++)
        {
            var m = ApexLine.Match(lines[i].TrimEnd('\r', '\n'));
            if (!m.Success || !string.Equals(m.Groups[2].Value, key, StringComparison.OrdinalIgnoreCase)) continue;
            if (m.Groups[4].Value == value) return (content, false);
            lines[i] = m.Groups[1].Value + m.Groups[2].Value + m.Groups[3].Value + "\"" + value + "\"" + m.Groups[5].Value + nl;
            return (string.Concat(lines), true);
        }
        var tail = content.Length > 0 && !content.EndsWith('\n') ? nl : "";
        return (content + tail + key + " \"" + value + "\"" + nl, true);
    }

    private sealed record Target(string Game, string Path, string Key, bool Apex);

    private IEnumerable<string> SteamDirs()
    {
        if (!string.IsNullOrWhiteSpace(cfg.SensSteamDir)) yield return cfg.SensSteamDir;
        var pf86 = Environment.GetEnvironmentVariable("ProgramFiles(x86)");
        if (!string.IsNullOrEmpty(pf86)) yield return Path.Combine(pf86, "Steam");
        var pf = Environment.GetEnvironmentVariable("ProgramFiles");
        if (!string.IsNullOrEmpty(pf)) yield return Path.Combine(pf, "Steam");
        yield return @"C:\Program Files (x86)\Steam";
        yield return @"C:\Program Files\Steam";
    }

    private string FindCs2Cfg()
    {
        if (!string.IsNullOrWhiteSpace(cfg.SensCs2Cfg)) return cfg.SensCs2Cfg;
        string best = ""; DateTime bestMod = DateTime.MinValue;
        foreach (var dir in SteamDirs())
        {
            var userdata = Path.Combine(dir, "userdata");
            if (!Directory.Exists(userdata)) continue;
            foreach (var acc in Directory.GetDirectories(userdata))
            {
                var p = Path.Combine(acc, "730", "local", "cfg", "cs2_user_convars_0_slot0.vcfg");
                if (!File.Exists(p)) continue;
                var mod = File.GetLastWriteTimeUtc(p);
                if (best == "" || mod > bestMod) { best = p; bestMod = mod; }
            }
            if (best != "") break;
        }
        return best;
    }

    private string FindApexCfg()
    {
        if (!string.IsNullOrWhiteSpace(cfg.SensApexCfg)) return cfg.SensApexCfg;
        var up = Environment.GetEnvironmentVariable("USERPROFILE");
        return string.IsNullOrEmpty(up) ? "" : Path.Combine(up, "Saved Games", "Respawn", "Apex", "local", "settings.cfg");
    }

    private Dictionary<string, Target> Targets()
    {
        var t = new Dictionary<string, Target>();
        var cs2 = FindCs2Cfg();
        if (cs2 != "") t["cs2"] = new Target("cs2", cs2, "sensitivity", false);
        var apex = FindApexCfg();
        if (apex != "") t["apex"] = new Target("apex", apex, "mouse_sensitivity", true);
        return t;
    }

    /// <summary>session_start: payload.sens {dpi, games} → конфиги игр.</summary>
    public void ApplyFromPayload(JsonElement payload)
    {
        if (!cfg.SensEnabled) return;
        if (payload.ValueKind != JsonValueKind.Object || !payload.TryGetProperty("sens", out var sens) || sens.ValueKind != JsonValueKind.Object) return;
        if (!sens.TryGetProperty("games", out var games) || games.ValueKind != JsonValueKind.Object) return;
        var profile = new Dictionary<string, double>();
        foreach (var g in games.EnumerateObject())
            if (g.Value.TryGetDouble(out var v) && v > 0) profile[g.Name] = v;
        var (applied, skipped) = Apply(profile);
        log.LogInformation("сенса на старте сессии: подставлено [{Applied}], пропущено [{Skipped}] (пробный режим: {Dry})",
            string.Join(",", applied), string.Join(",", skipped.Select(kv => kv.Key + ":" + kv.Value)), cfg.SensDryRun);
    }

    public (List<string> applied, Dictionary<string, string> skipped) Apply(Dictionary<string, double> games)
    {
        var applied = new List<string>();
        var skipped = new Dictionary<string, string>();
        lock (_lock)
        {
            var targets = Targets();
            foreach (var game in games.Keys.OrderBy(k => k, StringComparer.Ordinal))
            {
                if (game == "valorant") { skipped[game] = "cloud"; continue; }
                if (!targets.TryGetValue(game, out var t)) { skipped[game] = game is "cs2" or "apex" ? "no_config" : "unsupported"; continue; }
                if (!File.Exists(t.Path)) { skipped[game] = "no_config"; continue; }
                try
                {
                    ApplyOne(t, FormatSens(games[game]));
                    applied.Add(game);
                }
                catch (Exception ex)
                {
                    log.LogWarning("сенса: {Game} — {Error}", game, ex.Message);
                    skipped[game] = "error";
                }
            }
        }
        return (applied, skipped);
    }

    private void ApplyOne(Target t, string value)
    {
        var data = File.ReadAllText(t.Path);
        var (next, changed) = t.Apex ? ApexCfgSet(data, t.Key, value) : VcfgSetConvar(data, t.Key, value);
        if (!changed) { log.LogInformation("сенса: {Game} уже {Key} = {Value}", t.Game, t.Key, value); return; }
        if (cfg.SensDryRun) { log.LogInformation("сенса [пробный режим]: {Game} — записал бы {Key} = {Value} в {Path}", t.Game, t.Key, value, t.Path); return; }
        if (!_backed.Contains(t.Path))
        {
            var bak = t.Path + BackupSuffix;
            if (!File.Exists(bak)) File.WriteAllText(bak, data);
            _backed.Add(t.Path);
        }
        File.WriteAllText(t.Path, next);
        log.LogInformation("сенса: {Game} — {Key} = {Value} записано в {Path}", t.Game, t.Key, value, t.Path);
    }

    /// <summary>session_end / старт сервиса: вернуть оригиналы из бэкапов.</summary>
    public int Restore()
    {
        int n = 0;
        lock (_lock)
        {
            foreach (var t in Targets().Values)
            {
                var bak = t.Path + BackupSuffix;
                if (!File.Exists(bak)) continue;
                try
                {
                    File.WriteAllText(t.Path, File.ReadAllText(bak));
                    File.Delete(bak);
                    log.LogInformation("сенса: {Path} возвращён из бэкапа", t.Path);
                    n++;
                }
                catch (Exception ex) { log.LogWarning("сенса: откат {Path} не удался: {Error}", t.Path, ex.Message); }
            }
            _backed.Clear();
        }
        return n;
    }
}
