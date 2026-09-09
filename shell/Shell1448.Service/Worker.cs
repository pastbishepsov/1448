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
    private readonly SensApplier _sens = new(log, options.Value); // К7: сенса гостя в конфиги игр

    protected override async Task ExecuteAsync(CancellationToken ct)
    {
        pipe.MessageReceived += OnAppMessage;
        _ = pipe.RunAsync(ct);
        _ = FailSafeLoopAsync(ct);
        var restored = _sens.Restore(); // К7: сервис упал посреди сессии — конфиги игр как были
        if (restored > 0) log.LogInformation("сенса: на старте возвращено конфигов из бэкапа: {N}", restored);

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
                if (!string.IsNullOrWhiteSpace(_cfg.AgentToken))
                    ws.Options.SetRequestHeader("X-Agent-Token", _cfg.AgentToken.Trim()); // К4-и3
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
                if (ex.Message.Contains("401"))
                    log.LogWarning("WS: сервер не принял токен агента (401) — проверь Shell:AgentToken в appsettings.json и AGENT_TOKEN на сервере");
                else
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
                _sens.ApplyFromPayload(payload); // К7: профиль сенсы гостя → CS2/Apex до первого запуска
                pipe.Broadcast(PipeMessage.Of(PipeMsg.SessionStart, payload));
                break;
            case "session_end":
                _state.OnSessionEnd();
                pipe.Broadcast(PipeMessage.Of(PipeMsg.SessionEnd, payload));
                _sens.Restore(); // К7: конфиги игр — как были до гостя
                break;
            // С2-и4 (SENS.md, Т5): гость сохранил профиль во время сессии — сервер прислал
            // sens_apply {dpi, games}; применяем и отвечаем sens_result, сервер делает из
            // него уведомление гостю. running сервис не знает (игры запускает App) — пусто.
            case "sens_apply":
            {
                var (applied, skipped) = _sens.ApplyProfile(payload);
                var result = JsonSerializer.Serialize(new
                {
                    type = "sens_result",
                    payload = new { applied, skipped, dry_run = _cfg.SensDryRun, running = Array.Empty<string>() },
                });
                _ = SendAsync(result);
                break;
            }
            case "force_unlock":
                _state.OnForceUnlock();
                pipe.Broadcast(PipeMessage.Of(PipeMsg.ForceUnlock));
                break;
            // К0-и1 (KIOSK.md, Р1): пауза/AFK — запереть экран, сессию и игру не трогать.
            // Раньше сервер слал на паузе session_end, и киоск вёл себя как при конце сессии.
            case "lock":
                pipe.Broadcast(PipeMessage.Of(PipeMsg.Lock, payload));
                break;
            case "unlock":
                pipe.Broadcast(PipeMessage.Of(PipeMsg.Unlock, payload));
                break;
            case "xp_update":
                pipe.Broadcast(PipeMessage.Of(PipeMsg.XpUpdate, payload));
                break;
            // К4-и1 (KIOSK.md, №13): ответ сервера на session_tick — истина о сессии
            // этого ПК. session_start/session_end/lock/unlock могли не дойти (WS лежал
            // в момент завершения) — по state догоняем: сессия/её отсутствие → те же
            // события в App, пауза → Lock с причиной, не пауза → Unlock.
            case "state":
                ApplyState(payload, pipe);
                break;
            default:
                log.LogDebug("WS: неизвестный тип {Type}", type);
                break;
        }
    }

    private void ApplyState(JsonElement payload, PipeServer pipe)
    {
        if (payload.ValueKind != JsonValueKind.Object) return;
        bool active = payload.TryGetProperty("session_active", out var sa) && sa.ValueKind == JsonValueKind.True;
        bool paused = payload.TryGetProperty("paused", out var pp) && pp.ValueKind == JsonValueKind.True;
        if (active && !_state.SessionActive)
        {
            log.LogInformation("WS: state — сессия идёт, а мы не знали: догоняю session_start");
            _state.OnSessionStart();
            pipe.Broadcast(PipeMessage.Of(PipeMsg.SessionStart, payload));
        }
        else if (!active && _state.SessionActive)
        {
            log.LogInformation("WS: state — сессии нет, а мы думали идёт: догоняю session_end");
            _state.OnSessionEnd();
            pipe.Broadcast(PipeMessage.Of(PipeMsg.SessionEnd, payload));
        }
        if (active)
            pipe.Broadcast(PipeMessage.Of(paused ? PipeMsg.Lock : PipeMsg.Unlock, payload));
    }

    /// <summary>С2-и4: ответ серверу по живому WS (fire-and-forget, обрыв — в лог).
    /// ClientWebSocket не терпит параллельных SendAsync — heartbeat и ответы идут через один семафор.</summary>
    private readonly SemaphoreSlim _sendLock = new(1, 1);
    private async Task SendAsync(string json)
    {
        var ws = _ws;
        if (ws is null || ws.State != WebSocketState.Open) return;
        try { await SendRawAsync(ws, Encoding.UTF8.GetBytes(json), CancellationToken.None); }
        catch (Exception ex) { log.LogWarning("WS: ответ не ушёл: {Error}", ex.Message); }
    }
    private async Task SendRawAsync(ClientWebSocket ws, byte[] bytes, CancellationToken ct)
    {
        await _sendLock.WaitAsync(ct);
        try { await ws.SendAsync(bytes, WebSocketMessageType.Text, true, ct); }
        finally { _sendLock.Release(); }
    }

    private async Task HeartbeatAsync(ClientWebSocket ws, CancellationToken ct)
    {
        var tick = Encoding.UTF8.GetBytes("""{"type":"session_tick"}""");
        while (!ct.IsCancellationRequested && ws.State == WebSocketState.Open)
        {
            await Task.Delay(TimeSpan.FromSeconds(Math.Max(10, _cfg.HeartbeatSeconds)), ct);
            await SendRawAsync(ws, tick, ct);
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
