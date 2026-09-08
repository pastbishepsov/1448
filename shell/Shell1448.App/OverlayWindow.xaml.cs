namespace Shell1448.App;

using System.Windows;
using System.Windows.Threading;

/// <summary>XP-оверлей поверх игр: уровень, свежий XP, таймер сессии.</summary>
public partial class OverlayWindow : Window
{
    private readonly DispatcherTimer _timer = new() { Interval = TimeSpan.FromSeconds(1) };
    private readonly DispatcherTimer _noteTimer = new() { Interval = TimeSpan.FromSeconds(8) };
    private DateTimeOffset? _sessionStart;

    public OverlayWindow()
    {
        InitializeComponent();
        _timer.Tick += (_, _) =>
        {
            if (_sessionStart is not { } t0) return;
            var t = DateTimeOffset.UtcNow - t0;
            TimerText.Text = t.TotalHours >= 1
                ? $"{(int)t.TotalHours}:{t.Minutes:D2}:{t.Seconds:D2}"
                : $"{t.Minutes:D2}:{t.Seconds:D2}";
        };
        _noteTimer.Tick += (_, _) =>
        {
            _noteTimer.Stop();
            NoteRow.Visibility = Visibility.Collapsed;
            if (_sessionStart is null) Hide(); // без сессии оверлей жил только ради строки
        };
    }

    /// <summary>К4-и2: строка уведомления поверх игры на 8 секунд (шелл шлёт только критичное).</summary>
    public void Notify(string text)
    {
        NoteText.Text = text;
        NoteRow.Visibility = Visibility.Visible;
        _noteTimer.Stop();
        _noteTimer.Start();
        Show();
    }

    public void SessionStarted(DateTimeOffset startedAt)
    {
        _sessionStart = startedAt;
        _timer.Start();
        Show();
    }

    public void SessionEnded()
    {
        _sessionStart = null;
        _timer.Stop();
        Hide();
    }

    public void UpdateXp(long xpTotal, int level, long granted)
    {
        LvlText.Text = $"LVL {level}";
        XpText.Text = granted > 0 ? $"+{granted} XP" : $"{xpTotal:N0} XP";
    }
}
