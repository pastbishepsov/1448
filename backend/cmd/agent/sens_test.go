package main

// К7 «Сенса в игру»: парсер/запись vcfg и settings.cfg, форматирование,
// применение с бэкапом и откат — на временных файлах, без Windows.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func timeAgo() time.Time { return time.Now().Add(-48 * time.Hour) }

const vcfgSample = "\"config\"\n{\n\t\"convars\"\n\t{\n\t\t\"cl_crosshairsize\"\t\t\"3\"\n\t\t\"sensitivity\"\t\t\"1.2\"\n\t\t\"zoom_sensitivity_ratio\"\t\t\"1.0\"\n\t}\n}\n"

func TestFormatSens(t *testing.T) {
	for v, want := range map[float64]string{2.5: "2.5", 2: "2", 1.4: "1.4", 0.8125: "0.8125", 1.23456789: "1.2346", 100: "100"} {
		if got := formatSens(v); got != want {
			t.Errorf("formatSens(%v) = %q, ожидалось %q", v, got, want)
		}
	}
}

func TestVcfgSetConvar(t *testing.T) {
	out, changed := vcfgSetConvar(vcfgSample, "sensitivity", "2.5")
	if !changed || !strings.Contains(out, "\t\t\"sensitivity\"\t\t\"2.5\"\n") {
		t.Fatalf("замена значения: %q", out)
	}
	if !strings.Contains(out, "\"cl_crosshairsize\"\t\t\"3\"") || !strings.Contains(out, "\"zoom_sensitivity_ratio\"\t\t\"1.0\"") {
		t.Error("соседние строки должны остаться нетронутыми")
	}
	if strings.Count(out, "\n") != strings.Count(vcfgSample, "\n") {
		t.Error("число строк не должно меняться при замене")
	}
	// то же значение — без изменений
	if _, changed := vcfgSetConvar(out, "sensitivity", "2.5"); changed {
		t.Error("повторная запись того же значения — не изменение")
	}
	// ключа нет — вставка первой строкой блока convars
	noKey := strings.Replace(vcfgSample, "\t\t\"sensitivity\"\t\t\"1.2\"\n", "", 1)
	out2, changed := vcfgSetConvar(noKey, "sensitivity", "3")
	if !changed || !strings.Contains(out2, "\t{\n\t\t\"sensitivity\"\t\t\"3\"\n\t\t\"cl_crosshairsize\"") {
		t.Fatalf("вставка ключа: %q", out2)
	}
	// CRLF сохраняется
	crlf := strings.ReplaceAll(vcfgSample, "\n", "\r\n")
	out3, _ := vcfgSetConvar(crlf, "sensitivity", "0.9")
	if !strings.Contains(out3, "\"sensitivity\"\t\t\"0.9\"\r\n") || strings.Contains(strings.ReplaceAll(out3, "\r\n", ""), "\n") {
		t.Errorf("CRLF: %q", out3)
	}
	// регистр ключа
	if _, changed := vcfgSetConvar(strings.Replace(vcfgSample, `"sensitivity"`, `"Sensitivity"`, 1), "sensitivity", "1.2"); changed {
		t.Error("ключ сравнивается без учёта регистра")
	}
	// пустой файл — структура целиком
	out4, changed := vcfgSetConvar("", "sensitivity", "2")
	if !changed || !strings.Contains(out4, "\"convars\"") || !strings.Contains(out4, "\"sensitivity\"\t\t\"2\"") {
		t.Errorf("пустой файл: %q", out4)
	}
	// ключ с тем же именем вне блока convars не трогаем
	outside := "\"config\"\n{\n\t\"sensitivity\"\t\t\"9\"\n\t\"convars\"\n\t{\n\t\t\"sensitivity\"\t\t\"1\"\n\t}\n}\n"
	out5, _ := vcfgSetConvar(outside, "sensitivity", "4")
	if !strings.Contains(out5, "\t\"sensitivity\"\t\t\"9\"\n") || !strings.Contains(out5, "\t\t\"sensitivity\"\t\t\"4\"\n") {
		t.Errorf("менять только внутри convars: %q", out5)
	}
}

func TestApexCfgSet(t *testing.T) {
	src := "cl_fovScale \"1.550000\"\nmouse_sensitivity \"2.000000\"\nmouse_zoomed_sensitivity_scalar_0 \"1.000000\"\n"
	out, changed := apexCfgSet(src, "mouse_sensitivity", "1.4")
	if !changed || !strings.Contains(out, "mouse_sensitivity \"1.4\"\n") || !strings.Contains(out, "cl_fovScale \"1.550000\"") {
		t.Fatalf("apex замена: %q", out)
	}
	if _, changed := apexCfgSet(out, "mouse_sensitivity", "1.4"); changed {
		t.Error("то же значение — без изменений")
	}
	out2, changed := apexCfgSet("cl_fovScale \"1.5\"", "mouse_sensitivity", "1.4")
	if !changed || out2 != "cl_fovScale \"1.5\"\nmouse_sensitivity \"1.4\"\n" {
		t.Errorf("apex дописать в конец: %q", out2)
	}
}

func TestSensApplyAndRestore(t *testing.T) {
	dir := t.TempDir()
	cs2 := filepath.Join(dir, "cs2_user_convars_0_slot0.vcfg")
	apex := filepath.Join(dir, "settings.cfg")
	os.WriteFile(cs2, []byte(vcfgSample), 0o644)
	os.WriteFile(apex, []byte("mouse_sensitivity \"2.000000\"\n"), 0o644)
	f := false
	s := newSensApplier(SensConfig{Enabled: true, DryRun: &f, CS2Mode: "vcfg", CS2Cfg: cs2, ApexCfg: apex})
	s.getenv = func(string) string { return "" }

	res := s.apply(sensProfile{DPI: 800, Games: map[string]float64{"cs2": 2.5, "apex": 1.4, "valorant": 0.35, "ow2": 5, "quake": 3}})
	if strings.Join(res.Applied, ",") != "apex,cs2" || res.DryRun {
		t.Fatalf("applied: %+v", res)
	}
	if res.Skipped["valorant"] != "cloud" || res.Skipped["ow2"] != "unsupported" || res.Skipped["quake"] != "unsupported" {
		t.Errorf("skipped: %+v", res.Skipped)
	}
	got, _ := os.ReadFile(cs2)
	if !strings.Contains(string(got), "\"sensitivity\"\t\t\"2.5\"") {
		t.Errorf("cs2 не записан: %q", got)
	}
	bak, err := os.ReadFile(cs2 + sensBackupSuffix)
	if err != nil || string(bak) != vcfgSample {
		t.Fatalf("бэкап оригинала: %v %q", err, bak)
	}
	// вторая запись в той же сессии не перезаписывает бэкап оригиналом-подделкой
	s.apply(sensProfile{Games: map[string]float64{"cs2": 3}})
	bak2, _ := os.ReadFile(cs2 + sensBackupSuffix)
	if string(bak2) != vcfgSample {
		t.Error("бэкап должен хранить ОРИГИНАЛ, а не промежуточное значение")
	}
	got2, _ := os.ReadFile(cs2)
	if !strings.Contains(string(got2), "\"sensitivity\"\t\t\"3\"") {
		t.Error("вторая запись не применилась")
	}
	// откат
	if n := s.restore(); n != 2 {
		t.Fatalf("restore вернул %d, ожидалось 2", n)
	}
	back, _ := os.ReadFile(cs2)
	if string(back) != vcfgSample {
		t.Errorf("после отката оригинал: %q", back)
	}
	if _, err := os.Stat(cs2 + sensBackupSuffix); !os.IsNotExist(err) {
		t.Error("бэкап после отката должен исчезнуть")
	}
	if n := s.restore(); n != 0 {
		t.Error("повторный откат — нечего возвращать")
	}
	// профиль без поддерживаемых игр — ничего не трогаем
	res = s.apply(sensProfile{Games: map[string]float64{"valorant": 0.4}})
	if len(res.Applied) != 0 || res.Skipped["valorant"] != "cloud" {
		t.Errorf("valorant only: %+v", res)
	}
	if _, err := os.Stat(cs2 + sensBackupSuffix); !os.IsNotExist(err) {
		t.Error("без записи — без бэкапа")
	}
}

func TestSensDryRunAndMissing(t *testing.T) {
	dir := t.TempDir()
	cs2 := filepath.Join(dir, "cs2.vcfg")
	os.WriteFile(cs2, []byte(vcfgSample), 0o644)
	s := newSensApplier(SensConfig{Enabled: true, CS2Mode: "vcfg", CS2Cfg: cs2, ApexCfg: filepath.Join(dir, "nope", "settings.cfg")}) // dry_run по умолчанию
	s.getenv = func(string) string { return "" }
	res := s.apply(sensProfile{Games: map[string]float64{"cs2": 2.5, "apex": 1.4}})
	if !res.DryRun || strings.Join(res.Applied, ",") != "cs2" || res.Skipped["apex"] != "no_config" {
		t.Fatalf("dry run: %+v", res)
	}
	// игру не знаем вовсе — unsupported, а не «нет конфига»
	if r := s.apply(sensProfile{Games: map[string]float64{"fortnite": 3}}); r.Skipped["fortnite"] != "unsupported" {
		t.Errorf("неизвестная игра: %+v", r)
	}
	got, _ := os.ReadFile(cs2)
	if string(got) != vcfgSample {
		t.Error("в пробном режиме файл не трогаем")
	}
	if _, err := os.Stat(cs2 + sensBackupSuffix); !os.IsNotExist(err) {
		t.Error("в пробном режиме бэкапа нет")
	}
}

func TestSensTargetsDiscovery(t *testing.T) {
	dir := t.TempDir()
	// два аккаунта Steam — берём самый свежий конфиг
	old := filepath.Join(dir, "userdata", "111", "730", "local", "cfg")
	fresh := filepath.Join(dir, "userdata", "222", "730", "local", "cfg")
	os.MkdirAll(old, 0o755)
	os.MkdirAll(fresh, 0o755)
	os.WriteFile(filepath.Join(old, "cs2_user_convars_0_slot0.vcfg"), []byte(vcfgSample), 0o644)
	os.WriteFile(filepath.Join(fresh, "cs2_user_convars_0_slot0.vcfg"), []byte(vcfgSample), 0o644)
	os.Chtimes(filepath.Join(old, "cs2_user_convars_0_slot0.vcfg"), timeAgo(), timeAgo())
	env := map[string]string{"USERPROFILE": filepath.Join(dir, "home")}
	getenv := func(k string) string { return env[k] }
	targets := sensTargets(SensConfig{SteamDir: dir, CS2Mode: "vcfg"}, getenv)
	if targets["cs2"].Path != filepath.Join(fresh, "cs2_user_convars_0_slot0.vcfg") {
		t.Errorf("cs2: %+v", targets["cs2"])
	}
	if targets["apex"].Path != filepath.Join(dir, "home", "Saved Games", "Respawn", "Apex", "local", "settings.cfg") {
		t.Errorf("apex: %+v", targets["apex"])
	}
	// явный путь главнее поиска
	tg := sensTargets(SensConfig{SteamDir: dir, CS2Mode: "vcfg", CS2Cfg: "/x/y.vcfg"}, getenv)
	if tg["cs2"].Path != "/x/y.vcfg" {
		t.Error("cs2_cfg из конфига должен побеждать")
	}
	// без Steam и USERPROFILE — cs2 нет, apex нет
	if tg := sensTargets(SensConfig{CS2Mode: "vcfg"}, func(string) string { return "" }); len(tg) != 0 {
		t.Errorf("пусто ожидалось: %+v", tg)
	}
}

// ── С2 (SENS.md) ──

const loginUsersSample = `"users"
{
	"76561198000000001"
	{
		"AccountName"		"club1448"
		"PersonaName"		"Club"
		"RememberPassword"		"1"
		"MostRecent"		"0"
	}
	"76561198000000042"
	{
		"AccountName"		"guest"
		"MostRecent"		"1"
	}
}
`

func TestParseLoginUsersAndAccountID(t *testing.T) {
	if id := parseLoginUsers(loginUsersSample); id != "76561198000000042" {
		t.Fatalf("MostRecent: %q", id)
	}
	if acc := steamAccountID("76561198000000042"); acc != "39734314" {
		t.Fatalf("accountid: %q", acc)
	}
	if parseLoginUsers("") != "" || steamAccountID("x") != "" || steamAccountID("1") != "" {
		t.Error("пусто/мусор → пусто")
	}
}

func TestVcfgGetConvarAndApexGet(t *testing.T) {
	if v, ok := vcfgGetConvar(vcfgSample, "sensitivity"); !ok || v != "1.2" {
		t.Fatalf("sensitivity из образца: %q %v", v, ok)
	}
	if _, ok := vcfgGetConvar(vcfgSample, "nope"); ok {
		t.Error("нет ключа — нет значения")
	}
	if v, ok := apexCfgGet("mouse_sensitivity \"2.000000\"\r\nfov \"90\"\r\n", "mouse_sensitivity"); !ok || v != "2.000000" {
		t.Fatalf("apex: %q %v", v, ok)
	}
}

func TestParseCS2VideoAndAspect(t *testing.T) {
	video := "\"video.cfg\"\n{\n\t\"setting.defaultres\"\t\t\"1280\"\n\t\"setting.defaultresheight\"\t\t\"1024\"\n\t\"setting.aspectratiomode\"\t\t\"0\"\n\t\"setting.fullscreen\"\t\t\"1\"\n}\n"
	w, h, mode := parseCS2Video(video)
	if w != 1280 || h != 1024 || mode != 0 {
		t.Fatalf("video: %d×%d mode %d", w, h, mode)
	}
	for _, c := range []struct {
		w, h int
		want string
	}{{1280, 1024, "5:4"}, {1024, 768, "4:3"}, {1680, 1050, "16:10"}, {1920, 1080, "16:9"}, {0, 0, ""}} {
		if got := aspectLabel(c.w, c.h); got != c.want {
			t.Errorf("%d×%d: %q, ждал %q", c.w, c.h, got, c.want)
		}
	}
}

func TestParseLibraryFolders(t *testing.T) {
	vdf := "\"libraryfolders\"\n{\n\t\"0\"\n\t{\n\t\t\"path\"\t\t\"C:\\\\Program Files (x86)\\\\Steam\"\n\t}\n\t\"1\"\n\t{\n\t\t\"path\"\t\t\"D:\\\\Games\\\\SteamLibrary\"\n\t}\n}\n"
	got := parseLibraryFolders(vdf)
	if len(got) != 2 || got[0] != `C:\Program Files (x86)\Steam` || got[1] != `D:\Games\SteamLibrary` {
		t.Fatalf("библиотеки: %q", got)
	}
}

func TestWithSensArgs(t *testing.T) {
	tg, args := withSensArgs("cs2", "steam://rungameid/730", nil, true)
	if tg != "steam://run/730//+exec%201448/" || len(args) != 0 {
		t.Fatalf("steam url: %q %v", tg, args)
	}
	if tg, _ := withSensArgs("cs2", "steam://run/730/", nil, true); tg != "steam://run/730//+exec%201448/" {
		t.Errorf("run с хвостовым слэшем: %q", tg)
	}
	tg, args = withSensArgs("cs2", `D:\Games\cs2\game\bin\win64\cs2.exe`, []string{"-novid"}, true)
	if tg != `D:\Games\cs2\game\bin\win64\cs2.exe` || strings.Join(args, " ") != "-novid +exec 1448" {
		t.Errorf("exe: %q %v", tg, args)
	}
	if tg, args := withSensArgs("apex", "steam://rungameid/1172470", nil, true); tg != "steam://rungameid/1172470" || len(args) != 0 {
		t.Errorf("не cs2 — как есть: %q %v", tg, args)
	}
	if tg, _ := withSensArgs("cs2", "steam://rungameid/730", nil, false); tg != "steam://rungameid/730" {
		t.Errorf("выключено — как есть: %q", tg)
	}
	if tg, _ := withSensArgs("cs2", "riot://play", nil, true); tg != "riot://play" {
		t.Errorf("чужой протокол — как есть: %q", tg)
	}
}

// exec-режим: 1448.cfg с сенсой, бэкап ВСЕХ vcfg, откат → vcfg как были, файл пустой.
func TestSensExecModeApplyAndRestore(t *testing.T) {
	steam := t.TempDir()
	cs2dir := filepath.Join(steam, "steamapps", "common", cs2Folder)
	os.MkdirAll(filepath.Join(cs2dir, "game", "csgo", "cfg"), 0o755)
	v1 := filepath.Join(steam, "userdata", "1", "730", "local", "cfg", "cs2_user_convars_0_slot0.vcfg")
	v2 := filepath.Join(steam, "userdata", "2", "730", "local", "cfg", "cs2_user_convars_0_slot0.vcfg")
	os.MkdirAll(filepath.Dir(v1), 0o755)
	os.MkdirAll(filepath.Dir(v2), 0o755)
	os.WriteFile(v1, []byte(vcfgSample), 0o644)
	os.WriteFile(v2, []byte("other"), 0o644)
	f := false
	s := newSensApplier(SensConfig{Enabled: true, DryRun: &f, SteamDir: steam})
	s.getenv = func(string) string { return "" }

	exec := cs2ExecPath(cs2dir)
	if tg := sensTargets(s.cfg, s.getenv); tg["cs2"].Kind != sensKindExec || tg["cs2"].Path != exec {
		t.Fatalf("цель cs2 в exec-режиме: %+v", tg["cs2"])
	}
	res := s.apply(sensProfile{DPI: 800, Games: map[string]float64{"cs2": 2.5, "valorant": 0.4}})
	if strings.Join(res.Applied, ",") != "cs2" || res.Skipped["valorant"] != "cloud" || res.DryRun {
		t.Fatalf("applied: %+v", res)
	}
	got, _ := os.ReadFile(exec)
	if string(got) != sensExecHeader+"\nsensitivity \"2.5\"\n" {
		t.Fatalf("1448.cfg: %q", got)
	}
	for _, v := range []string{v1, v2} { // Т3: все vcfg отложены, сами не тронуты
		if _, err := os.Stat(v + sensBackupSuffix); err != nil {
			t.Errorf("нет бэкапа %s", v)
		}
	}
	if b, _ := os.ReadFile(v1); string(b) != vcfgSample {
		t.Error("vcfg в exec-режиме не пишем")
	}
	// вторая запись в сессии: файл переписан, бэкапы не затираются
	os.WriteFile(v1, []byte("changed by game"), 0o644) // игра сохранила сенсу в vcfg
	s.apply(sensProfile{Games: map[string]float64{"cs2": 3}})
	got2, _ := os.ReadFile(exec)
	if !strings.Contains(string(got2), `sensitivity "3"`) {
		t.Error("вторая запись не применилась")
	}
	if b, _ := os.ReadFile(v1 + sensBackupSuffix); string(b) != vcfgSample {
		t.Error("бэкап должен хранить оригинал")
	}
	// откат: vcfg как были, 1448.cfg пустой
	if n := s.restore(); n != 2 {
		t.Fatalf("restore вернул %d, ожидалось 2 (два vcfg)", n)
	}
	if b, _ := os.ReadFile(v1); string(b) != vcfgSample {
		t.Errorf("v1 после отката: %q", b)
	}
	if b, _ := os.ReadFile(v2); string(b) != "other" {
		t.Errorf("v2 после отката: %q", b)
	}
	if b, _ := os.ReadFile(exec); string(b) != execCfgContent("") {
		t.Errorf("1448.cfg после отката должен быть пустым: %q", b)
	}
	if _, err := os.Stat(v1 + sensBackupSuffix); !os.IsNotExist(err) {
		t.Error("бэкап после отката должен исчезнуть")
	}
	// пробный режим: ничего не пишем
	s2 := newSensApplier(SensConfig{Enabled: true, SteamDir: steam})
	s2.getenv = s.getenv
	os.Remove(exec)
	if res := s2.apply(sensProfile{Games: map[string]float64{"cs2": 2}}); !res.DryRun || strings.Join(res.Applied, ",") != "cs2" {
		t.Errorf("dry run: %+v", res)
	}
	if _, err := os.Stat(exec); !os.IsNotExist(err) {
		t.Error("в пробном режиме 1448.cfg не создаётся")
	}
	// выключено — skipped disabled
	s3 := newSensApplier(SensConfig{Enabled: false, SteamDir: steam})
	if res := s3.apply(sensProfile{Games: map[string]float64{"cs2": 2}}); res.Skipped["cs2"] != "disabled" {
		t.Errorf("disabled: %+v", res)
	}
}

func TestSensCurrentReadsActiveAccount(t *testing.T) {
	steam := t.TempDir()
	os.MkdirAll(filepath.Join(steam, "config"), 0o755)
	os.WriteFile(filepath.Join(steam, "config", "loginusers.vdf"), []byte(loginUsersSample), 0o644)
	// активный аккаунт 39734314 — его vcfg старее, но именно его надо брать
	act := filepath.Join(steam, "userdata", "39734314", "730", "local", "cfg")
	old := filepath.Join(steam, "userdata", "7", "730", "local", "cfg")
	os.MkdirAll(act, 0o755)
	os.MkdirAll(old, 0o755)
	os.WriteFile(filepath.Join(act, "cs2_user_convars_0_slot0.vcfg"), []byte(strings.Replace(vcfgSample, `"1.2"`, `"2.75"`, 1)), 0o644)
	os.WriteFile(filepath.Join(act, "cs2_video.txt"), []byte("\"video.cfg\"\n{\n\t\"setting.defaultres\"\t\t\"1024\"\n\t\"setting.defaultresheight\"\t\t\"768\"\n}\n"), 0o644)
	os.WriteFile(filepath.Join(old, "cs2_user_convars_0_slot0.vcfg"), []byte(vcfgSample), 0o644)
	apex := filepath.Join(steam, "settings.cfg")
	os.WriteFile(apex, []byte("mouse_sensitivity \"1.400000\"\n"), 0o644)
	getenv := func(string) string { return "" }
	cur := sensCurrent(SensConfig{SteamDir: steam, ApexCfg: apex}, getenv)
	cs2, _ := cur["cs2"].(map[string]any)
	if cs2 == nil || cs2["sens"] != 2.75 || cs2["res_w"] != 1024 || cs2["aspect"] != "4:3" {
		t.Fatalf("cs2 current: %+v", cur)
	}
	if ap, _ := cur["apex"].(map[string]any); ap == nil || ap["sens"] != 1.4 {
		t.Fatalf("apex current: %+v", cur)
	}
	if cur := sensCurrent(SensConfig{}, getenv); len(cur) != 0 {
		t.Errorf("без файлов — пусто: %+v", cur)
	}
}
