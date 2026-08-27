//go:build windows

package main

// Системные вызовы жизненного цикла игры под Windows. Логика — в launch.go;
// здесь только то, что нельзя выполнить на Linux, поэтому тестов тут нет.

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"unsafe"
)

const stillActive = 259 // STILL_ACTIVE из GetExitCodeProcess

// hidden — служебные команды агента (taskkill, cmd /C start) не должны
// мигать чёрным окном перед гостем.
func hidden(c *exec.Cmd) *exec.Cmd {
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return c
}

// startFile — прямой запуск exe: даёт настоящий PID процесса игры.
// Рабочая папка по умолчанию — папка самого exe (многие игры без этого
// не находят свои ресурсы).
func startFile(target string, args []string, dir string) (*exec.Cmd, error) {
	target = strings.TrimSpace(target)
	cmd := exec.Command(target, args...)
	if dir == "" {
		dir = filepath.Dir(target)
	}
	cmd.Dir = dir
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return cmd, nil
}

// startProtocol — steam://, ms-settings:, com.epicgames.launcher:// и прочее
// отдаём обработчику Windows. PID игры при этом неизвестен — его ловит
// attachProcess по имени процесса из agent.json.
func startProtocol(target string, args []string) error {
	cmdArgs := append([]string{"/C", "start", "", strings.TrimSpace(target)}, args...)
	return hidden(exec.Command("cmd", cmdArgs...)).Start()
}

// processAlive — жив ли процесс. Без внешних команд: OpenProcess +
// GetExitCodeProcess, вызывается фоновым опросом раз в несколько секунд.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	h, err := syscall.OpenProcess(syscall.PROCESS_QUERY_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer syscall.CloseHandle(h)
	var code uint32
	if err := syscall.GetExitCodeProcess(h, &code); err != nil {
		return true // хэндл открылся — процесс есть, код просто не отдали
	}
	return code == stillActive
}

// findProcessByName — PID процесса по имени (cs2.exe). Снимок Toolhelp32,
// без запуска tasklist — ничего не мигает и не стоит миллисекунд.
func findProcessByName(name string) (int, bool) {
	name = strings.TrimSpace(name)
	if name == "" {
		return 0, false
	}
	snap, err := syscall.CreateToolhelp32Snapshot(syscall.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return 0, false
	}
	defer syscall.CloseHandle(snap)

	var e syscall.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	want := strings.ToLower(name)
	self := uint32(os.Getpid())
	for err = syscall.Process32First(snap, &e); err == nil; err = syscall.Process32Next(snap, &e) {
		if e.ProcessID == self || e.ProcessID == 0 {
			continue
		}
		if strings.ToLower(syscall.UTF16ToString(e.ExeFile[:])) == want {
			return int(e.ProcessID), true
		}
	}
	return 0, false
}

// terminateGentle — попросить закрыться (WM_CLOSE окнам дерева процессов).
func terminateGentle(pid int) error {
	if pid <= 0 {
		return fmt.Errorf("нет PID")
	}
	return hidden(exec.Command("taskkill", "/PID", strconv.Itoa(pid), "/T")).Run()
}

// terminateForce — прибить дерево процессов.
func terminateForce(pid int) error {
	if pid <= 0 {
		return fmt.Errorf("нет PID")
	}
	return hidden(exec.Command("taskkill", "/PID", strconv.Itoa(pid), "/F", "/T")).Run()
}
