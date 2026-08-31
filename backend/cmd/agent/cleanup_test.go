package main

import (
	"strings"
	"testing"
)

// Г9-и4: правила безопасности путей очистки — опечатка в конфиге не должна
// смывать системный диск.
func TestSafeCleanupDir(t *testing.T) {
	ok := []string{
		`C:\Users\guest\Downloads`,
		`C:\Users\guest\Desktop\_guest`,
		"C:/Users/guest/Downloads",
		"/home/guest/Downloads",
	}
	for _, p := range ok {
		if !safeCleanupDir(p) {
			t.Errorf("%q должен быть разрешён", p)
		}
	}
	bad := []string{
		"", "Downloads", `..\Users\guest\Downloads`,
		`C:\`, `C:\Users`, `C:\Users\guest`, // мало сегментов
		`C:\Windows\Temp`, `C:\Program Files\Steam`, `C:\ProgramData\x\y`,
		`C:\Users\guest\..\..\Windows`,
		"/home/guest", // мало сегментов и для юникс-формы
	}
	for _, p := range bad {
		if safeCleanupDir(p) {
			t.Errorf("%q должен быть отвергнут", p)
		}
	}
}

func TestCleanupPlan(t *testing.T) {
	cfg := CleanupConfig{
		Enabled: true, DryRun: true,
		Kill:            []string{"steam.exe", " ", "Discord.exe"},
		ClearDirs:       []string{`C:\Users\guest\Downloads`, `C:\Windows\Temp`},
		ClearTemp:       true,
		EmptyRecycleBin: true,
		ClearClipboard:  true,
	}
	plan := cleanupPlan(cfg)
	joined := strings.Join(plan, "\n")
	for _, want := range []string{
		"закрыть процесс: steam.exe", "закрыть процесс: Discord.exe",
		"вычистить содержимое: C:\\Users\\guest\\Downloads",
		"ПРОПУЩЕН небезопасный путь: C:\\Windows\\Temp",
		"%TEMP%", "корзину", "буфер",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("в плане нет %q:\n%s", want, joined)
		}
	}
	if len(plan) != 7 {
		t.Errorf("ожидалось 7 шагов, got %d", len(plan))
	}
	if len(cleanupPlan(CleanupConfig{})) != 0 {
		t.Error("пустой конфиг — пустой план")
	}
}

// Порядок на session_end: сперва закрыть игру, которую запустил агент, и
// только потом общая очистка по списку имён (иначе игра переживала сессию —
// ровно та дыра, ради которой делался трекинг).
func TestSessionEndOrder(t *testing.T) {
	a := newTestAgent(&Config{Cleanup: CleanupConfig{Enabled: true, DryRun: true,
		Kill: []string{"steam.exe"}}})
	a.track.put(procRecord{appID: "cs2", pid: 0, state: stateLaunched})

	a.onSessionEnd() // сухой прогон очистки: ничего не удаляет, только логирует

	if len(a.track.snapshot()) != 0 {
		t.Errorf("реестр запущенного должен быть очищен: %+v", a.track.snapshot())
	}

	// выключенное завершение оставляет реестр в покое (клубу может понадобиться
	// оставить игру открытой между гостями — решается конфигом)
	no := false
	b := newTestAgent(&Config{Launch: LaunchConfig{StopOnSessionEnd: &no}})
	b.track.put(procRecord{appID: "cs2", pid: 0, state: stateLaunched})
	b.onSessionEnd()
	if len(b.track.snapshot()) != 1 {
		t.Error("при stop_on_session_end=false трекинг не трогаем")
	}
}

// ── %ПЕРЕМЕННЫЕ% в путях очистки (Фаза 0) ──────────────────────────────────
//
// Имя гостевой учётки на каждом ПК своё (в пилотном клубе support), поэтому
// сейвы и кэш адресуются через %LOCALAPPDATA% / %USERPROFILE%, а не буквой.

func TestExpandEnvPath(t *testing.T) {
	env := map[string]string{
		"LOCALAPPDATA": `C:\Users\support\AppData\Local`,
		"USERPROFILE":  `C:\Users\support`,
		"EMPTY":        "",
	}
	getenv := func(k string) string { return env[k] }
	cases := []struct{ in, want string }{
		{`%LOCALAPPDATA%\Steam\htmlcache`, `C:\Users\support\AppData\Local\Steam\htmlcache`},
		{`%USERPROFILE%\Saved Games`, `C:\Users\support\Saved Games`},
		{`C:\Users\guest\Downloads`, `C:\Users\guest\Downloads`}, // без переменных — как есть
		{`%EMPTY%\Steam`, ``}, // пустая переменная → путь непригоден
		{`%NOPE%\Steam`, ``},  // неизвестная переменная → тоже
	}
	for _, c := range cases {
		if got := expandEnvPath(c.in, getenv); got != c.want {
			t.Errorf("expandEnvPath(%q) = %q, ожидалось %q", c.in, got, c.want)
		}
	}
}

func TestCleanupPlanExpandsEnv(t *testing.T) {
	env := map[string]string{
		"LOCALAPPDATA": `C:\Users\support\AppData\Local`,
		"USERPROFILE":  `C:\Users\support`,
	}
	getenv := func(k string) string { return env[k] }
	cfg := CleanupConfig{ClearDirs: []string{
		`%LOCALAPPDATA%\Steam\htmlcache`, // раскроется и пройдёт проверку
		`%NOPE%\Steam`,                   // переменной нет — в план как ПРОПУЩЕН
		`%USERPROFILE%`,                  // раскроется, но глубина < 3 сегментов
	}}
	plan := cleanupPlanEnv(cfg, getenv)
	want := []string{
		`вычистить содержимое: C:\Users\support\AppData\Local\Steam\htmlcache`,
		`ПРОПУЩЕН небезопасный путь: %NOPE%\Steam`,
		`ПРОПУЩЕН небезопасный путь: %USERPROFILE%`,
	}
	if len(plan) != len(want) {
		t.Fatalf("в плане %d шагов, ожидалось %d: %+v", len(plan), len(want), plan)
	}
	for i := range want {
		if plan[i] != want[i] {
			t.Errorf("шаг %d: %q, ожидалось %q", i+1, plan[i], want[i])
		}
	}
}
