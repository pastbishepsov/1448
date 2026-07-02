namespace Shell1448.Shared;

/// <summary>
/// Машина состояний гостевого ПК. Чистая логика без I/O — покрыта тестами.
/// Правила (ТЗ 6.3): при потере связи с сервером дольше FailSafe — блокировка;
/// force_unlock от администратора снимает блокировку до следующей команды.
/// </summary>
public sealed class ShellState
{
    public bool SessionActive { get; private set; }
    public bool ServerConnected { get; private set; }
    public bool ForceUnlocked { get; private set; }
    public DateTimeOffset? DisconnectedAt { get; private set; }

    public TimeSpan FailSafeAfter { get; init; } = TimeSpan.FromMinutes(2);

    public void OnConnected(DateTimeOffset now)
    {
        ServerConnected = true;
        DisconnectedAt = null;
    }

    public void OnDisconnected(DateTimeOffset now)
    {
        if (ServerConnected || DisconnectedAt is null)
            DisconnectedAt = now;
        ServerConnected = false;
    }

    public void OnSessionStart()
    {
        SessionActive = true;
        ForceUnlocked = false;
    }

    public void OnSessionEnd()
    {
        SessionActive = false;
        ForceUnlocked = false;
    }

    public void OnForceUnlock() => ForceUnlocked = true;

    /// <summary>Fail-safe: без связи дольше порога и без force_unlock — экран блокируется.</summary>
    public bool FailSafeLocked(DateTimeOffset now) =>
        !ServerConnected
        && !ForceUnlocked
        && DisconnectedAt is { } t
        && now - t >= FailSafeAfter;

    /// <summary>Итог: должен ли App показывать блокировку прямо сейчас.</summary>
    public bool ShouldLock(DateTimeOffset now) =>
        FailSafeLocked(now) || (!SessionActive && !ForceUnlocked);
}
