namespace Shell1448.Tests;

using Shell1448.Shared;
using Xunit;

public class ShellStateTests
{
    private static readonly DateTimeOffset T0 = new(2026, 7, 2, 12, 0, 0, TimeSpan.Zero);

    [Fact]
    public void БезСессии_Заблокирован()
    {
        var s = new ShellState();
        s.OnConnected(T0);
        Assert.True(s.ShouldLock(T0)); // сессии нет — ПК заблокирован
    }

    [Fact]
    public void Сессия_Разблокирует_КонецСессии_Блокирует()
    {
        var s = new ShellState();
        s.OnConnected(T0);
        s.OnSessionStart();
        Assert.False(s.ShouldLock(T0));
        s.OnSessionEnd();
        Assert.True(s.ShouldLock(T0));
    }

    [Fact]
    public void FailSafe_СрабатываетТолькоПослеПорога()
    {
        var s = new ShellState { FailSafeAfter = TimeSpan.FromMinutes(2) };
        s.OnConnected(T0);
        s.OnSessionStart();

        s.OnDisconnected(T0);
        Assert.False(s.FailSafeLocked(T0.AddSeconds(119))); // ещё рано
        Assert.True(s.FailSafeLocked(T0.AddSeconds(121)));  // порог пройден
        Assert.True(s.ShouldLock(T0.AddSeconds(121)));      // даже при активной сессии
    }

    [Fact]
    public void Переподключение_СбрасываетFailSafe()
    {
        var s = new ShellState { FailSafeAfter = TimeSpan.FromMinutes(2) };
        s.OnConnected(T0);
        s.OnSessionStart();
        s.OnDisconnected(T0);
        s.OnConnected(T0.AddSeconds(60)); // связь вернулась до порога
        Assert.False(s.FailSafeLocked(T0.AddMinutes(10)));
        Assert.False(s.ShouldLock(T0.AddMinutes(10)));
    }

    [Fact]
    public void ПовторныйДисконнект_НеСдвигаетТаймер()
    {
        var s = new ShellState { FailSafeAfter = TimeSpan.FromMinutes(2) };
        s.OnConnected(T0);
        s.OnDisconnected(T0);
        s.OnDisconnected(T0.AddSeconds(110)); // дубль события не должен сбросить отсчёт
        Assert.True(s.FailSafeLocked(T0.AddSeconds(121)));
    }

    [Fact]
    public void ForceUnlock_СнимаетЛюбуюБлокировку()
    {
        var s = new ShellState { FailSafeAfter = TimeSpan.FromMinutes(2) };
        s.OnConnected(T0);
        s.OnDisconnected(T0);
        var later = T0.AddMinutes(5);
        Assert.True(s.ShouldLock(later));
        s.OnForceUnlock();
        Assert.False(s.ShouldLock(later)); // админ разблокировал — работаем
        s.OnSessionEnd();
        Assert.True(s.ShouldLock(later));  // следующая команда возвращает обычные правила
    }
}
