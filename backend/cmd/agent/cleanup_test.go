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
