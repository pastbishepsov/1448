namespace Shell1448.Tests;

using Shell1448.Shared;
using Xunit;

public class SensLaunchTests
{
    [Fact]
    public void SteamUrl_ПолучаетExecВнутриАдреса()
    {
        var (t, a) = SensLaunch.WithExec("cs2", "steam://rungameid/730", [], true);
        Assert.Equal("steam://run/730//+exec%201448/", t);
        Assert.Empty(a);
        (t, _) = SensLaunch.WithExec("cs2", "steam://run/730/", [], true);
        Assert.Equal("steam://run/730//+exec%201448/", t);
    }

    [Fact]
    public void Exe_ПолучаетАргументы()
    {
        var (t, a) = SensLaunch.WithExec("cs2", @"D:\Games\cs2\game\bin\win64\cs2.exe", ["-novid"], true);
        Assert.Equal(@"D:\Games\cs2\game\bin\win64\cs2.exe", t);
        Assert.Equal(new[] { "-novid", "+exec", "1448" }, a);
    }

    [Fact]
    public void ЧужаяИгра_Выключено_ЧужойПротокол_КакЕсть()
    {
        Assert.Equal("steam://rungameid/1172470", SensLaunch.WithExec("apex", "steam://rungameid/1172470", [], true).target);
        Assert.Equal("steam://rungameid/730", SensLaunch.WithExec("cs2", "steam://rungameid/730", [], false).target);
        Assert.Equal("riot://play", SensLaunch.WithExec("cs2", "riot://play", [], true).target);
    }
}
