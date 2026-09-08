//go:build windows

package main

// Тост Windows через WinRT (Windows.UI.Notifications) из встроенного
// PowerShell: без BurntToast и прочих модулей, без регистрации приложения.
// Скрипт пишется во временный .ps1 (так не нужно экранировать кавычки в
// -Command), окно PowerShell скрыто, на всё — 10 секунд.

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func showToast(appID, title, text string) error {
	script := toastScript(appID, title, text)
	f, err := os.CreateTemp("", "1448-toast-*.ps1")
	if err != nil {
		return err
	}
	path := f.Name()
	defer os.Remove(path)
	// BOM: без него PowerShell 5.1 читает файл как ANSI и ломает кириллицу.
	if _, err := f.Write(append([]byte{0xEF, 0xBB, 0xBF}, []byte(script)...)); err != nil {
		f.Close()
		return err
	}
	f.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := hidden(exec.CommandContext(ctx, "powershell.exe",
		"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-WindowStyle", "Hidden",
		"-File", filepath.Clean(path)))
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("powershell: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
