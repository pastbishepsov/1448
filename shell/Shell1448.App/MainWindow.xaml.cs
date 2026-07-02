namespace Shell1448.App;

using System.IO;
using System.Text.Json;
using System.Windows;
using System.Windows.Threading;
using Microsoft.Web.WebView2.Core;
using Shell1448.Shared;

/// <summary>
/// Киоск: fullscreen WebView2 с гостевым экраном (web/shell.html).
/// Нативная часть добавляет то, чего браузеру нельзя: запуск приложений,
/// блокировку клавиш, оверлей поверх игр, связь с сервисом (pipe).
/// </summary>
public partial class MainWindow : Window
{
    private readonly AppConfig _cfg = AppConfig.Load();
    private readonly KeyboardHook _hook = new();
    private readonly PipeClient _pipe = new();
    private readonly Launcher _launcher;
    private readonly OverlayWindow _overlay = new();

    public MainWindow()
    {
        InitializeComponent();
        _launcher = new Launcher(_cfg.CatalogUrl);

        if (_cfg.KioskKeys)
        {
            _hook.EmergencyExitRequested += OnEmergencyExit;
            _hook.Install();
        }
        Deactivated += (_, _) => // потеря фокуса: вернуть киоск наверх (кроме запущенных игр — они fullscreen поверх)
            Dispatcher.BeginInvoke(() => { if (!_overlay.IsVisible) Activate(); }, DispatcherPriority.ApplicationIdle);
        Closing += (_, e) => e.Cancel = !_allowClose; // Alt+F4 и прочее — нет
        Loaded += async (_, _) => await InitAsync();
    }

    private bool _allowClose;

    private async Task InitAsync()
    {
        await _launcher.RefreshAsync();
        _ = PeriodicCatalogRefreshAsync();

        // WebView2: профиль в LocalAppData, чтобы работал под ограниченным юзером
        var dataDir = Path.Combine(
            Environment.GetFolderPath(Environment.SpecialFolder.LocalApplicationData),
            "Shell1448", "WebView2");
        var env = await CoreWebView2Environment.CreateAsync(null, dataDir);
        await Web.EnsureCoreWebView2Async(env);

        var wv = Web.CoreWebView2;
        wv.Settings.AreDefaultContextMenusEnabled = false;
        wv.Settings.AreDevToolsEnabled = false;
        wv.Settings.IsStatusBarEnabled = false;
        wv.Settings.IsZoomControlEnabled = false;
        wv.WebMessageReceived += OnWebMessage;
        wv.NewWindowRequested += (_, e) => e.Handled = true; // попапы запрещены

        var url = string.IsNullOrWhiteSpace(_cfg.ShellUrl)
            ? new Uri(Path.Combine(AppContext.BaseDirectory, "web", "shell.html")).AbsoluteUri
            : _cfg.ShellUrl;
        wv.Navigate(url);

        _pipe.MessageReceived += msg => Dispatcher.BeginInvoke(() => OnPipeMessage(msg));
        _pipe.Start();
    }

    private async Task PeriodicCatalogRefreshAsync()
    {
        while (true)
        {
            await Task.Delay(TimeSpan.FromMinutes(5));
            await _launcher.RefreshAsync();
        }
    }

    // ── Мост из shell.html: window.chrome.webview.postMessage({cmd, ...}) ──
    private void OnWebMessage(object? sender, CoreWebView2WebMessageReceivedEventArgs e)
    {
        try
        {
            using var doc = JsonDocument.Parse(e.WebMessageAsJson);
            var root = doc.RootElement;
            var cmd = root.TryGetProperty("cmd", out var c) ? c.GetString() : null;
            switch (cmd)
            {
                case "launch":
                    var id = root.GetProperty("id").GetString() ?? "";
                    if (!_launcher.Launch(id))
                        PostToWeb(new { evt = "launch_failed", id });
                    break;
                case "admin_call":
                    _pipe.Send(PipeMessage.Of(PipeMsg.AdminCall));
                    break;
            }
        }
        catch { /* некорректное сообщение страницы — игнор */ }
    }

    private void PostToWeb(object payload) =>
        Web.CoreWebView2?.PostWebMessageAsJson(JsonSerializer.Serialize(payload));

    // ── Команды сервиса ──
    private void OnPipeMessage(PipeMessage msg)
    {
        switch (msg.Type)
        {
            case PipeMsg.SessionStart:
                HardLock.Visibility = Visibility.Collapsed;
                _overlay.SessionStarted(DateTimeOffset.UtcNow);
                PostToWeb(new { evt = "session_start" });
                break;
            case PipeMsg.SessionEnd:
                _overlay.SessionEnded();
                PostToWeb(new { evt = "session_end" });
                break;
            case PipeMsg.XpUpdate:
                if (msg.Payload is { } p)
                {
                    long xp = p.TryGetProperty("xp_total", out var x) ? x.GetInt64() : 0;
                    int lvl = p.TryGetProperty("level", out var l) ? l.GetInt32() : 0;
                    long granted = p.TryGetProperty("granted", out var g) ? g.GetInt64() : 0;
                    _overlay.UpdateXp(xp, lvl, granted);
                    PostToWeb(new { evt = "xp_update", xp_total = xp, level = lvl, granted });
                }
                break;
            case PipeMsg.Lock:
                HardLockReason.Text = "Потеряна связь с сервером клуба";
                HardLock.Visibility = Visibility.Visible;
                break;
            case PipeMsg.Unlock:
            case PipeMsg.ForceUnlock:
                HardLock.Visibility = Visibility.Collapsed;
                break;
            case PipeMsg.State:
                if (msg.Payload is { } st &&
                    st.TryGetProperty("should_lock", out var sl) && sl.GetBoolean() &&
                    st.TryGetProperty("server_connected", out var sc) && !sc.GetBoolean())
                {
                    HardLockReason.Text = "Потеряна связь с сервером клуба";
                    HardLock.Visibility = Visibility.Visible;
                }
                break;
        }
    }

    // ── Аварийный выход: Ctrl+Alt+Shift+Q + пароль ──
    private void OnEmergencyExit()
    {
        Dispatcher.BeginInvoke(() =>
        {
            if (PasswordPrompt.Ask(this) == _cfg.ExitPassword)
            {
                _allowClose = true;
                _hook.Dispose();
                _overlay.Close();
                Close();
            }
        });
    }
}
