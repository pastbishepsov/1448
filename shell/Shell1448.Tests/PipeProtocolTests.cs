namespace Shell1448.Tests;

using System.Text.Json;
using Shell1448.Shared;
using Xunit;

public class PipeProtocolTests
{
    [Fact]
    public void Сериализация_Круговая()
    {
        var msg = PipeMessage.Of(PipeMsg.XpUpdate, new { granted = 500, level = 5 });
        var line = PipeProtocol.Serialize(msg);
        Assert.DoesNotContain('\n', line); // line-delimited протокол

        var back = PipeProtocol.Deserialize(line);
        Assert.NotNull(back);
        Assert.Equal(PipeMsg.XpUpdate, back!.Type);
        Assert.Equal(500, back.Payload!.Value.GetProperty("granted").GetInt32());
    }

    [Fact]
    public void БезPayload_НетПоляВJson()
    {
        var line = PipeProtocol.Serialize(PipeMessage.Of(PipeMsg.Lock));
        Assert.Contains("\"type\":\"lock\"", line);
        Assert.DoesNotContain("payload", line);
    }

    [Theory]
    [InlineData("")]
    [InlineData("   ")]
    [InlineData("{битый json")]
    public void Мусор_НеПадает(string line)
    {
        Assert.Null(PipeProtocol.Deserialize(line));
    }

    [Fact]
    public void СовместимостьТипов_СБэкендом()
    {
        // контракт WS-хаба: те же строки типов
        Assert.Equal("session_start", PipeMsg.SessionStart);
        Assert.Equal("session_end", PipeMsg.SessionEnd);
        Assert.Equal("force_unlock", PipeMsg.ForceUnlock);
        Assert.Equal("xp_update", PipeMsg.XpUpdate);
        Assert.Equal("admin_call", PipeMsg.AdminCall);
    }
}
