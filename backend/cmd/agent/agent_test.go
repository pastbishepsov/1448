package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestResolveApp(t *testing.T) {
	cfg := &Config{Apps: map[string]AppEntry{
		"cs2":   {Target: "steam://rungameid/730"},
		"empty": {Target: ""},
	}}

	if _, err := resolveApp(cfg, "cs2"); err != nil {
		t.Errorf("cs2 из allowlist отклонён: %v", err)
	}
	// не из allowlist — отклоняется, ничего не выполняется
	if _, err := resolveApp(cfg, "cmd.exe"); !errors.Is(err, errUnknownApp) {
		t.Errorf("посторонний id прошёл allowlist: %v", err)
	}
	// пустой target — тоже отказ
	if _, err := resolveApp(cfg, "empty"); !errors.Is(err, errUnknownApp) {
		t.Errorf("пустой target прошёл allowlist: %v", err)
	}
}

func TestLoadConfig(t *testing.T) {
	dir := t.TempDir()

	// нет файла → дефолты + ошибка (не падаем)
	cfg, err := loadConfig(filepath.Join(dir, "нет.json"))
	if err == nil {
		t.Error("ожидалась ошибка отсутствующего файла")
	}
	if cfg.Listen != "127.0.0.1:1448" || cfg.BackendWS == "" {
		t.Errorf("дефолты не применились: %+v", cfg)
	}

	// валидный файл
	p := filepath.Join(dir, "agent.json")
	os.WriteFile(p, []byte(`{"computer_id":"abc","apps":{"steam":{"target":"steam://open/main"}}}`), 0o644)
	cfg, err = loadConfig(p)
	if err != nil {
		t.Fatalf("валидный конфиг не прочитался: %v", err)
	}
	if cfg.ComputerID != "abc" || len(cfg.Apps) != 1 || cfg.Listen != "127.0.0.1:1448" {
		t.Errorf("конфиг прочитан неверно: %+v", cfg)
	}

	// битый JSON → дефолты + ошибка
	os.WriteFile(p, []byte(`{битый`), 0o644)
	cfg, err = loadConfig(p)
	if err == nil {
		t.Error("ожидалась ошибка битого JSON")
	}
	if len(cfg.Apps) != 0 {
		t.Error("после битого JSON allowlist должен быть пуст")
	}
}
