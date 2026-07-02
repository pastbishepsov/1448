namespace Shell1448.Service;

using System.Collections.Concurrent;
using System.IO.Pipes;
using System.Text;
using Microsoft.Extensions.Logging;
using Shell1448.Shared;

/// <summary>
/// Named Pipe сервер: раздаёт команды подключённым App'ам (обычно один)
/// и принимает от них события (hello, admin_call).
/// </summary>
public sealed class PipeServer(ILogger<PipeServer> log)
{
    private readonly ConcurrentDictionary<Guid, StreamWriter> _clients = new();

    /// <summary>Событие от App (admin_call и т.п.) — обрабатывает Worker.</summary>
    public event Action<PipeMessage>? MessageReceived;

    public async Task RunAsync(CancellationToken ct)
    {
        while (!ct.IsCancellationRequested)
        {
            NamedPipeServerStream? server = null;
            try
            {
                server = new NamedPipeServerStream(
                    PipeProtocol.PipeName, PipeDirection.InOut,
                    NamedPipeServerStream.MaxAllowedServerInstances,
                    PipeTransmissionMode.Byte, PipeOptions.Asynchronous);
                await server.WaitForConnectionAsync(ct);
                _ = HandleClientAsync(server, ct); // fire-and-forget на клиента
            }
            catch (OperationCanceledException) { server?.Dispose(); break; }
            catch (Exception ex)
            {
                server?.Dispose();
                log.LogWarning("Pipe: ошибка ожидания клиента: {Error}", ex.Message);
                await Task.Delay(1000, ct);
            }
        }
    }

    private async Task HandleClientAsync(NamedPipeServerStream stream, CancellationToken ct)
    {
        var id = Guid.NewGuid();
        var writer = new StreamWriter(stream, new UTF8Encoding(false)) { AutoFlush = true };
        var reader = new StreamReader(stream, Encoding.UTF8);
        _clients[id] = writer;
        log.LogInformation("Pipe: App подключился ({Id})", id);
        try
        {
            while (!ct.IsCancellationRequested && stream.IsConnected)
            {
                var line = await reader.ReadLineAsync(ct);
                if (line is null) break;
                if (PipeProtocol.Deserialize(line) is { } msg)
                    MessageReceived?.Invoke(msg);
            }
        }
        catch (Exception ex) when (ex is not OperationCanceledException)
        {
            log.LogDebug("Pipe: клиент {Id} отвалился: {Error}", id, ex.Message);
        }
        finally
        {
            _clients.TryRemove(id, out _);
            stream.Dispose();
            log.LogInformation("Pipe: App отключился ({Id})", id);
        }
    }

    /// <summary>Разослать команду всем подключённым App.</summary>
    public void Broadcast(PipeMessage msg)
    {
        var line = PipeProtocol.Serialize(msg);
        foreach (var (id, w) in _clients)
        {
            try { w.WriteLine(line); }
            catch { _clients.TryRemove(id, out _); }
        }
    }
}
