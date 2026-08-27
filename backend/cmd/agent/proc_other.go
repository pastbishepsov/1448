//go:build !windows

package main

// Заглушки-аналоги для Linux/macOS (Г9: агент собирается и тестируется в CI
// без Windows). Поведение осмысленное — на дев-машине запуск работает.

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
)

func startFile(target string, args []string, dir string) (*exec.Cmd, error) {
	target = strings.TrimSpace(target)
	cmd := exec.Command(target, args...)
	if dir == "" {
		dir = filepath.Dir(target)
	}
	cmd.Dir = dir
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return cmd, nil
}

func startProtocol(target string, args []string) error {
	opener := "xdg-open"
	if runtime.GOOS == "darwin" {
		opener = "open"
	}
	return exec.Command(opener, append([]string{strings.TrimSpace(target)}, args...)...).Start()
}

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return p.Signal(syscall.Signal(0)) == nil
}

// findProcessByName — самый свежий процесс с таким именем (pgrep -n -x).
// Расширение .exe в именах из agent.json на юниксе отбрасывается.
func findProcessByName(name string) (int, bool) {
	name = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(name), ".exe"))
	if name == "" {
		return 0, false
	}
	out, err := exec.Command("pgrep", "-n", "-x", name).Output()
	if err != nil {
		return 0, false
	}
	return parsePgrepPID(string(out))
}

func terminateGentle(pid int) error {
	if pid <= 0 {
		return fmt.Errorf("нет PID")
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return p.Signal(syscall.SIGTERM)
}

func terminateForce(pid int) error {
	if pid <= 0 {
		return fmt.Errorf("нет PID")
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return p.Kill()
}
