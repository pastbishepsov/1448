package main

// Г9-и4: очистка гостевого ПК между гостями — по конфигу agent.json, на
// команде session_end. Что умеет: закрыть лаунчеры (Steam/Discord/Riot/Epic —
// список в конфиге), вычистить СОДЕРЖИМОЕ заданных папок (сами папки
// остаются), почистить %TEMP%, корзину и буфер обмена.
//
// Безопасность — прежде всего: путь из конфига проходит safeCleanupDir
// (абсолютный, без «..», не корень диска, не Windows/Program Files, глубина
// от 3 сегментов) — опечатка в конфиге не смоет системный диск. dry_run=true
// (дефолт) только пишет план в лог — включать боевой режим владелец будет
// после сухого прогона на живом ПК. Фаза 3 «образ ПК» — отдельная история
// про железо, этот механизм её не заменяет.

import (
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// CleanupConfig — блок "cleanup" в agent.json.
type CleanupConfig struct {
	Enabled         bool     `json:"enabled"`           // выкл по умолчанию
	DryRun          bool     `json:"dry_run"`           // true = только лог, ничего не трогаем
	Kill            []string `json:"kill"`              // имена процессов (steam.exe, Discord.exe…)
	ClearDirs       []string `json:"clear_dirs"`        // папки, содержимое которых вычищается
	ClearTemp       bool     `json:"clear_temp"`        // %TEMP% текущего пользователя
	EmptyRecycleBin bool     `json:"empty_recycle_bin"` // корзина
	ClearClipboard  bool     `json:"clear_clipboard"`   // буфер обмена
}

// safeCleanupDir — можно ли чистить такой путь (чистая функция, тест).
// Правила: абсолютный, без «..», минимум 3 сегмента после корня диска,
// не системные каталоги Windows.
func safeCleanupDir(path string) bool {
	p := strings.TrimSpace(path)
	if p == "" || strings.Contains(p, "..") {
		return false
	}
	// нормализуем слэши, срезаем букву диска для подсчёта глубины
	norm := strings.ReplaceAll(p, "\\", "/")
	if !(strings.HasPrefix(norm, "/") || (len(norm) > 2 && norm[1] == ':' && norm[2] == '/')) {
		return false // относительный путь
	}
	low := strings.ToLower(norm)
	for _, bad := range []string{"/windows", "/program files", "/programdata", "/system32"} {
		if strings.Contains(low, bad) {
			return false
		}
	}
	rest := norm
	if len(norm) > 2 && norm[1] == ':' {
		rest = norm[2:] // C:/Users/... → /Users/...
	}
	segs := 0
	for _, s := range strings.Split(rest, "/") {
		if strings.TrimSpace(s) != "" {
			segs++
		}
	}
	return segs >= 3 // C:/Users/guest/Downloads — да; C:/Users — нет
}

// cleanupPlan — человекочитаемый план действий (чистая функция, тест).
// Небезопасные пути попадают в план с пометкой «ПРОПУЩЕН» — видно в логе.
func cleanupPlan(cfg CleanupConfig) []string {
	var plan []string
	for _, p := range cfg.Kill {
		if strings.TrimSpace(p) != "" {
			plan = append(plan, "закрыть процесс: "+p)
		}
	}
	for _, d := range cfg.ClearDirs {
		if safeCleanupDir(d) {
			plan = append(plan, "вычистить содержимое: "+d)
		} else {
			plan = append(plan, "ПРОПУЩЕН небезопасный путь: "+d)
		}
	}
	if cfg.ClearTemp {
		plan = append(plan, "вычистить %TEMP%")
	}
	if cfg.EmptyRecycleBin {
		plan = append(plan, "опустошить корзину")
	}
	if cfg.ClearClipboard {
		plan = append(plan, "очистить буфер обмена")
	}
	return plan
}

// clearDirContents — удалить содержимое папки, оставив её саму.
func clearDirContents(dir string) (removed, failed int) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, 1
	}
	for _, e := range entries {
		if err := os.RemoveAll(filepath.Join(dir, e.Name())); err != nil {
			failed++
		} else {
			removed++
		}
	}
	return removed, failed
}

// runCleanup — исполнить очистку по конфигу (вызывается на session_end).
func runCleanup(cfg CleanupConfig) {
	if !cfg.Enabled {
		return
	}
	plan := cleanupPlan(cfg)
	if len(plan) == 0 {
		return
	}
	mode := ""
	if cfg.DryRun {
		mode = " [СУХОЙ ПРОГОН — ничего не трогаю]"
	}
	log.Printf("🧹 Очистка между гостями%s: %d шагов", mode, len(plan))
	for _, step := range plan {
		log.Println("🧹  · " + step)
	}
	if cfg.DryRun {
		return
	}

	for _, p := range cfg.Kill {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if runtime.GOOS == "windows" {
			_ = exec.Command("taskkill", "/IM", p, "/F", "/T").Run()
		} else {
			_ = exec.Command("pkill", "-f", p).Run()
		}
	}
	for _, d := range cfg.ClearDirs {
		if !safeCleanupDir(d) {
			continue
		}
		rm, fail := clearDirContents(d)
		log.Printf("🧹 %s: удалено %d, не удалось %d", d, rm, fail)
	}
	if cfg.ClearTemp {
		rm, fail := clearDirContents(os.TempDir())
		log.Printf("🧹 %%TEMP%%: удалено %d, не удалось %d (занятые файлы — норма)", rm, fail)
	}
	if cfg.EmptyRecycleBin && runtime.GOOS == "windows" {
		_ = exec.Command("powershell", "-NoProfile", "-Command", "Clear-RecycleBin -Force -ErrorAction SilentlyContinue").Run()
		log.Println("🧹 Корзина опустошена")
	}
	if cfg.ClearClipboard && runtime.GOOS == "windows" {
		_ = exec.Command("cmd", "/C", "echo off | clip").Run()
		log.Println("🧹 Буфер обмена очищен")
	}
	log.Println("🧹 Очистка завершена")
}
