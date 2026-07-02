namespace Shell1448.App;

using System.IO;
using System.IO.Pipes;
using System.Text;
using Shell1448.Shared;

/// <summary>
/// Клиент Named Pipe к Shell1448.Service: получает команды
/// (session_start/end, lock/unlock, xp_update, force_unlock, state),
/// отправляет события (hello, admin_call). Автопереподключение.
/// </summary>
public sealed class PipeClient : IDisposable
{
    private readonly CancellationTokenSource _cts = new();
    private StreamWriter? _writer;

    public event Action<PipeMessage>? MessageReceived;
    public event Action<bool>? ConnectedChanged;
    public bool Connected { get; private set; }

    public void Start() => _ = RunAsync(_cts.Token);

    private async Task RunAsync(CancellationToken ct)
    {
        while (!ct.IsCancellationRequested)
        {
            try
            {
                using var pipe = new NamedPipeClientStream(
                    ".", PipeProtocol.PipeName, PipeDirection.InOut, PipeOptions.Asynchronous);
                await pipe.ConnectAsync(3000, ct);
                var reader = new StreamReader(pipe, Encoding.UTF8);
                _writer = new StreamWriter(pipe, new UTF8Encoding(false)) { AutoFlush = true };
                Connected = true;
                ConnectedChanged?.Invoke(true);
                Send(PipeMessage.Of(PipeMsg.Hello)); // просим текущее состояние

                while (pipe.IsConnected && !ct.IsCancellationRequested)
                {
                    var line = await reader.ReadLineAsync(ct);
                    if (line is null) break;
                    if (PipeProtocol.Deserialize(line) is { } msg)
                        MessageReceived?.Invoke(msg);
                }
            }
            catch (OperationCanceledException) { break; }
            catch { /* сервис не запущен — дев-режим без него */ }

            if (Connected) { Connected = false; ConnectedChanged?.Invoke(false); }
            _writer = null;
            try { await Task.Delay(3000, ct); } catch { break; }
        }
    }

    public void Send(PipeMessage msg)
    {
        try { _writer?.WriteLine(PipeProtocol.Serialize(msg)); }
        catch { /* переподключимся */ }
    }

    public void Dispose() => _cts.Cancel();
}
