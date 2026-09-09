using System.Globalization;
using System.Text;
using System.Text.Json;
using System.Text.RegularExpressions;
using Microsoft.Extensions.Logging;

namespace Shell1448.Service;

/// <summary>
/// К7 «Сенса в игру» (KIOSK.md, Р9) + С2 «Сенса на ПК» (SENS.md, Т2/Т3/Т5) — порт
/// sens.go Go-агента для ПК с C#-киоском. Профиль сенсы гостя приходит в
/// session_start (payload.sens {dpi, games}, уже пересчитанный сервером под DPI
/// мыши клуба) и по команде sens_apply (гость сохранил профиль во время сессии).
/// CS2, режим exec (по умолчанию): пишем game/csgo/cfg/1448.cfg со строкой
/// sensitivity "…", а App запускает CS2 с +exec 1448 (shell.json sens_exec) —
/// работает под любым Steam-аккаунтом и не зависит от момента входа в Steam.
/// Файл живёт всегда: с сенсой после session_start/sens_apply, пустой после
/// session_end. Все vcfg (userdata/*/730) на старте сессии копируются в
/// &lt;файл&gt;.1448bak и возвращаются на session_end — CS2 при выходе пишет сенсу в
/// vcfg текущего аккаунта, клубный аккаунт иначе унаследовал бы её; аккаунт
/// гостя, появившийся во время сессии, в бэкапах не был — его не трогаем.
/// Режим vcfg (К7, запасной): "sensitivity" в cs2_user_convars_0_slot0.vcfg.
/// Apex — mouse_sensitivity в settings.cfg. Valorant не умеем (облако Riot).
/// DryRun=true по умолчанию — только лог.
/// </summary>
public sealed class SensApplier(ILogger log, ServiceConfig cfg)
{
    public const string BackupSuffix = ".1448bak";
    public const string ExecName = "1448.cfg";
    public const string ExecHeader = "// 14:48 — сенса гостя. Файл переписывает агент клуба; править бессмысленно.";
    private const string Cs2Folder = "Counter-Strike Global Offensive";
    private static readonly Regex VcfgKey = new("^(\\s*)\"([^\"]+)\"(\\s+)\"([^\"]*)\"(\\s*)$");
    private static readonly Regex ApexLine = new("^(\\s*)([A-Za-z0-9_]+)(\\s+)\"([^\"]*)\"(\\s*)$");
    private static readonly Regex VdfPath = new("\"path\"\\s+\"([^\"]+)\"", RegexOptions.IgnoreCase);
    private readonly HashSet<string> _backed = new(StringComparer.OrdinalIgnoreCase);
    private bool _vcfgBacked; // С2-и3 (Т3): все vcfg этой сессии уже отложены
    private readonly object _lock = new();

    private string Cs2Mode => string.Equals(cfg.SensCs2Mode?.Trim(), "vcfg", StringComparison.OrdinalIgnoreCase) ? "vcfg" : "exec";

    /// <summary>Содержимое 1448.cfg: пустое value — «пустой» файл (один комментарий).</summary>
    public static string ExecCfgContent(string value) =>
        value == "" ? ExecHeader + "\n" : ExecHeader + "\n" + "sensitivity \"" + value + "\"\n";

    /// <summary>Пути библиотек Steam из steamapps/libraryfolders.vdf ("path" "D:\\Games\\SteamLibrary").</summary>
    public static List<string> ParseLibraryFolders(string content)
    {
        var outp = new List<string>();
        foreach (Match m in VdfPath.Matches(content))
        {
            var p = m.Groups[1].Value.Replace("\\\\", "\\");
            if (p != "" && !outp.Contains(p)) outp.Add(p);
        }
        return outp;
    }

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

    private enum Kind { Vcfg, Apex, Exec }
    private sealed record Target(string Game, string Path, string Key, Kind Kind);

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

    /// <summary>С2-и3: корень CS2 — из конфига; иначе steamapps/common/… у корня Steam и у библиотек.</summary>
    private string FindCs2Dir()
    {
        if (!string.IsNullOrWhiteSpace(cfg.SensCs2Dir)) return cfg.SensCs2Dir;
        foreach (var dir in SteamDirs())
        {
            var p = Path.Combine(dir, "steamapps", "common", Cs2Folder);
            if (Directory.Exists(p)) return p;
            var vdf = Path.Combine(dir, "steamapps", "libraryfolders.vdf");
            if (!File.Exists(vdf)) continue;
            foreach (var lib in ParseLibraryFolders(File.ReadAllText(vdf)))
            {
                var q = Path.Combine(lib, "steamapps", "common", Cs2Folder);
                if (Directory.Exists(q)) return q;
            }
        }
        return "";
    }

    /// <summary>С2-и3 (Т3): ВСЕ vcfg CS2 на этом ПК (все аккаунты Steam).</summary>
    private List<string> AllCs2Vcfg()
    {
        var outp = new List<string>();
        void Add(string p) { if (p != "" && File.Exists(p) && !outp.Contains(p, StringComparer.OrdinalIgnoreCase)) outp.Add(p); }
        Add(cfg.SensCs2Cfg ?? "");
        foreach (var dir in SteamDirs())
        {
            var userdata = Path.Combine(dir, "userdata");
            if (!Directory.Exists(userdata)) continue;
            foreach (var acc in Directory.GetDirectories(userdata))
                Add(Path.Combine(acc, "730", "local", "cfg", "cs2_user_convars_0_slot0.vcfg"));
        }
        outp.Sort(StringComparer.Ordinal);
        return outp;
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
        if (Cs2Mode == "exec")
        {
            var dir = FindCs2Dir();
            if (dir != "") t["cs2"] = new Target("cs2", Path.Combine(dir, "game", "csgo", "cfg", ExecName), "sensitivity", Kind.Exec);
        }
        else
        {
            var cs2 = FindCs2Cfg();
            if (cs2 != "") t["cs2"] = new Target("cs2", cs2, "sensitivity", Kind.Vcfg);
        }
        var apex = FindApexCfg();
        if (apex != "") t["apex"] = new Target("apex", apex, "mouse_sensitivity", Kind.Apex);
        return t;
    }

    /// <summary>С2-и4: sens_apply от сервера — payload {dpi, games} (без обёртки sens).</summary>
    public (List<string> applied, Dictionary<string, string> skipped) ApplyProfile(JsonElement payload)
    {
        var profile = new Dictionary<string, double>();
        if (payload.ValueKind == JsonValueKind.Object && payload.TryGetProperty("games", out var games) && games.ValueKind == JsonValueKind.Object)
            foreach (var g in games.EnumerateObject())
                if (g.Value.TryGetDouble(out var v) && v > 0) profile[g.Name] = v;
        return Apply(profile);
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
                if (!cfg.SensEnabled) { skipped[game] = "disabled"; continue; }
                if (game == "valorant") { skipped[game] = "cloud"; continue; }
                if (!targets.TryGetValue(game, out var t)) { skipped[game] = game is "cs2" or "apex" ? "no_config" : "unsupported"; continue; }
                if (t.Kind != Kind.Exec && !File.Exists(t.Path)) { skipped[game] = "no_config"; continue; } // exec-файл наш и может ещё не существовать
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
        if (t.Kind == Kind.Exec) { ApplyExec(t, value); return; }
        var data = File.ReadAllText(t.Path);
        var (next, changed) = t.Kind == Kind.Apex ? ApexCfgSet(data, t.Key, value) : VcfgSetConvar(data, t.Key, value);
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

    /// <summary>С2-и3: 1448.cfg с сенсой + бэкап всех vcfg (Т3) перед первой записью в сессии.</summary>
    private void ApplyExec(Target t, string value)
    {
        if (cfg.SensDryRun)
        {
            log.LogInformation("сенса [пробный режим]: cs2 — записал бы sensitivity {Value} в {Path} (+ бэкап {N} vcfg)", value, t.Path, AllCs2Vcfg().Count);
            return;
        }
        if (!_vcfgBacked)
        {
            foreach (var p in AllCs2Vcfg())
            {
                var bak = p + BackupSuffix;
                if (File.Exists(bak)) continue;
                File.WriteAllText(bak, File.ReadAllText(p));
                log.LogInformation("сенса: отложен {Path}", p);
            }
            _vcfgBacked = true;
        }
        Directory.CreateDirectory(Path.GetDirectoryName(t.Path)!);
        File.WriteAllText(t.Path, ExecCfgContent(value));
        log.LogInformation("сенса: cs2 — sensitivity {Value} записано в {Path} (игра читает через +exec 1448)", value, t.Path);
    }

    /// <summary>session_end / старт сервиса: вернуть оригиналы из бэкапов — все vcfg CS2 (Т3), Apex; 1448.cfg — пустой.</summary>
    public int Restore()
    {
        int n = 0;
        lock (_lock)
        {
            var paths = AllCs2Vcfg();
            foreach (var t in Targets().Values)
            {
                if (t.Kind == Kind.Exec)
                {
                    if (cfg.SensDryRun || !cfg.SensEnabled) continue;
                    try
                    {
                        if (!File.Exists(t.Path) || File.ReadAllText(t.Path) != ExecCfgContent(""))
                        {
                            Directory.CreateDirectory(Path.GetDirectoryName(t.Path)!);
                            File.WriteAllText(t.Path, ExecCfgContent(""));
                        }
                    }
                    catch (Exception ex) { log.LogWarning("сенса: 1448.cfg не опустошился: {Error}", ex.Message); }
                    continue;
                }
                if (!paths.Contains(t.Path, StringComparer.OrdinalIgnoreCase)) paths.Add(t.Path);
            }
            foreach (var p in paths)
            {
                var bak = p + BackupSuffix;
                if (!File.Exists(bak)) continue;
                try
                {
                    File.WriteAllText(p, File.ReadAllText(bak));
                    File.Delete(bak);
                    log.LogInformation("сенса: {Path} возвращён из бэкапа", p);
                    n++;
                }
                catch (Exception ex) { log.LogWarning("сенса: откат {Path} не удался: {Error}", p, ex.Message); }
            }
            _backed.Clear();
            _vcfgBacked = false;
        }
        return n;
    }
}
