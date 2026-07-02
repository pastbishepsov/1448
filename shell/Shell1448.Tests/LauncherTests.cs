namespace Shell1448.Tests;

using Shell1448.Shared;
using Xunit;

public class CatalogAllowlistTests
{
    [Fact]
    public void СобираетТолькоЗаписиСTarget()
    {
        var items = new[]
        {
            new CatalogItem { Id = "cs2", Target = "steam://rungameid/730" },
            new CatalogItem { Id = "noTarget", Target = null },
            new CatalogItem { Id = "", Target = "steam://x" },
            new CatalogItem { Id = "val", Target = "C:\\Riot\\x.exe", Args = "[\"--a\",\"--b\"]" },
            new CatalogItem { Id = "bad", Target = "x.exe", Args = "не json" },
        };
        var map = CatalogAllowlist.Build(items);

        Assert.Equal(3, map.Count);                 // cs2, val, bad
        Assert.False(map.ContainsKey("noTarget"));
        Assert.False(map.ContainsKey(""));
        Assert.Equal(2, map["val"].Args.Length);    // args распарсились
        Assert.Empty(map["bad"].Args);              // битые args → без аргументов
        Assert.Equal("steam://rungameid/730", map["cs2"].Target);
    }

    [Fact]
    public void ПустойКаталог_ПустойAllowlist()
    {
        Assert.Empty(CatalogAllowlist.Build([]));
    }
}
