namespace Shell1448.App;

using System.Windows;
using System.Windows.Threading;

/// <summary>XP-оверлей поверх игр: уровень, свежий XP, таймер сессии.</summary>
public partial class OverlayWindow : Window
{
    private readonly DispatcherTimer _timer = new() { Interval = TimeSpan.FromSeconds(1) };
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
