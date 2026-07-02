namespace Shell1448.Service;

using System.Net.WebSockets;
using System.Text;
using System.Text.Json;
using Microsoft.Extensions.Hosting;
using Microsoft.Extensions.Logging;
using Microsoft.Extensions.Options;
using Shell1448.Shared;

/// <summary>
/// Ядро сервиса: WS-связь с бэкендом (session_start/end, force_unlock, xp_update),
/// heartbeat, fail-safe (ТЗ 6.3: блокировка при потере связи > 2 мин),
/// трансляция команд в App через Named Pipe.
/// </summary>
public sealed class Worker(
    ILogger<Worker> log,
    IOptions<ServiceConfig> options,
    PipeServer pipe) : BackgroundService
{
    private readonly ServiceConfig _cfg = options.Value;
    private readonly ShellState _state = new()
    {
        FailSafeAfter = TimeSpan.FromSeconds(Math.Max(30, options.Value.FailSafeSeconds)),
    };
    private ClientWebSocket? _ws;
    private bool _lockSent;

    protected override async Task ExecuteAsync(CancellationToken ct)
    {
        pipe.MessageReceived += OnAppMessage;
        _ = pipe.RunAsync(ct);
        _ = FailSafeLoopAsync(ct);

        if (string.IsNullOrWhiteSpace(_cfg.ComputerId))
        {
            log.LogWarning("ComputerId пуст — WS отключён (дев-режим: только pipe)");
            await Task.Delay(Timeout.Infinite, ct).ContinueWith(_ => { }, CancellationToken.None);
            return;
        }

        var url = new Uri($"{_cfg.BackendWs}?computer_id={_cfg.ComputerId}");
        while (!ct.IsCancellationRequested)
        {
            try
            {
                using var ws = new ClientWebSocket();
                await ws.ConnectAsync(url, ct);
                _ws = ws;
                _state.OnConnected(DateTimeOffset.UtcNow);
                log.LogInformation("WS: связь с бэкендом установлена");
                pipe.Broadcast(PipeMessage.Of(PipeMsg.Unlock));
                _lockSent = false;

                using var hb = new CancellationTokenSource();
                var hbTask = HeartbeatAsync(ws, CancellationTokenSource
                    .CreateLinkedTokenSource(ct, hb.Token).Token);

                await ReceiveLoopAsync(ws, ct);
                hb.Cancel();
                try { await hbTask; } catch { /* остановка heartbeat */ }
            }
            catch (OperationCanceledException) { break; }
            catch (Exception ex)
            {
                log.LogWarning("WS: {Error}", ex.Message);
            }
            _ws = null;
            _state.OnDisconnected(DateTimeOffset.UtcNow);
            log.LogInformation("WS: связь потеряна, переподключение через 5с");
            try { await Task.Delay(5000, ct); } catch { break; }
        }
    }

    private async Task ReceiveLoopAsync(ClientWebSocket ws, CancellationToken ct)
    {
        var buf = new byte[16 * 1024];
        var sb = new StringBuilder();
        while (ws.State == WebSocketState.Open && !ct.IsCancellationRequested)
        {
            sb.Clear();
            WebSocketReceiveResult result;
            do
            {
                result = await ws.ReceiveAsync(buf, ct);
                if (result.MessageType == WebSocketMessageType.Close) return;
                sb.Append(Encoding.UTF8.GetString(buf, 0, result.Count));
            } while (!result.EndOfMessage);
            HandleServerMessage(sb.ToString());
        }
    }

    private void HandleServerMessage(string json)
    {
        string type;
        JsonElement payload = default;
        try
        {
            using var doc = JsonDocument.Parse(json);
            type = doc.RootElement.GetProperty("type").GetString() ?? "";
            if (doc.RootElement.TryGetProperty("payload", out var p))
                payload = p.Clone();
        }
        catch (Exception ex)
        {
            log.LogWarning("WS: некорректное сообщение: {Error}", ex.Message);
            return;
        }

        log.LogInformation("WS: команда {Type}", type);
        switch (type)
        {
            case "session_start":
                _state.OnSessionStart();
                pipe.Broadcast(PipeMessage.Of(PipeMsg.SessionStart, payload));
                break;
            case "session_end":
                _state.OnSessionEnd();
                pipe.Broadcast(PipeMessage.Of(PipeMsg.SessionEnd, payload));
                break;
            case "force_unlock":
                _state.OnForceUnlock();
                pipe.Broadcast(PipeMessage.Of(PipeMsg.ForceUnlock));
                break;
            case "xp_update":
                pipe.Broadcast(PipeMessage.Of(PipeMsg.XpUpdate, payload));
                break;
            default:
                log.LogDebug("WS: неизвестный тип {Type}", type);
                break;
        }
    }

    private async Task HeartbeatAsync(ClientWebSocket ws, CancellationToken ct)
    {
        var tick = Encoding.UTF8.GetBytes("""{"type":"session_tick"}""");
        while (!ct.IsCancellationRequested && ws.State == WebSocketState.Open)
        {
            await Task.Delay(TimeSpan.FromSeconds(Math.Max(10, _cfg.HeartbeatSeconds)), ct);
            await ws.SendAsync(tick, WebSocketMessageType.Text, true, ct);
        }
    }

    /// <summary>Fail-safe: раз в 5с проверяем, не пора ли блокировать.</summary>
    private async Task FailSafeLoopAsync(CancellationToken ct)
    {
        while (!ct.IsCancellationRequested)
        {
            try { await Task.Delay(5000, ct); } catch { break; }
            var locked = _state.FailSafeLocked(DateTimeOffset.UtcNow);
            if (locked && !_lockSent)
            {
                log.LogWarning("Fail-safe: связи нет дольше {S}с — команда lock", _cfg.FailSafeSeconds);
                pipe.Broadcast(PipeMessage.Of(PipeMsg.Lock, new { reason = "no_connection" }));
                _lockSent = true;
            }
        }
    }

    private void OnAppMessage(PipeMessage msg)
    {
        switch (msg.Type)
        {
            case PipeMsg.Hello:
                // App просит текущее состояние (после своего рестарта)
                pipe.Broadcast(PipeMessage.Of(PipeMsg.State, new
                {
                    session_active = _state.SessionActive,
                    server_connected = _state.ServerConnected,
                    should_lock = _state.ShouldLock(DateTimeOffset.UtcNow),
                }));
                break;
            case PipeMsg.AdminCall:
                _ = SendToServerAsync("""{"type":"admin_call"}""");
                log.LogInformation("🛎 admin_call переслан на сервер");
                break;
        }
    }

    private async Task SendToServerAsync(string json)
    {
        var ws = _ws;
        if (ws is not { State: WebSocketState.Open }) return;
        try
        {
            await ws.SendAsync(Encoding.UTF8.GetBytes(json),
                WebSocketMessageType.Text, true, CancellationToken.None);
        }
        catch (Exception ex) { log.LogWarning("WS send: {Error}", ex.Message); }
    }
}
