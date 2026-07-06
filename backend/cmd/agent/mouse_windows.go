//go:build windows

package main

// Реальное чтение/запись настроек мыши через user32.dll (только Windows).

import (
	"errors"
	"os/exec"
	"strings"
	"syscall"
	"unsafe"
)

var (
	user32              = syscall.NewLazyDLL("user32.dll")
	procSystemParamInfo = user32.NewProc("SystemParametersInfoW")
)

const (
	spiGetMouse      = 0x0003
	spiSetMouse      = 0x0004
	spiGetMouseSpeed = 0x0070
	spifSendChange   = 0x0002
)

// readMouse — снимок настроек мыши.
func readMouse() MouseInfo {
	info := MouseInfo{Available: true}

	var speed uint32
	if r, _, _ := procSystemParamInfo.Call(spiGetMouseSpeed, 0, uintptr(unsafe.Pointer(&speed)), 0); r != 0 {
		info.PointerSpeed = int(speed)
	}

	var mp [3]int32
	if r, _, _ := procSystemParamInfo.Call(spiGetMouse, 0, uintptr(unsafe.Pointer(&mp[0])), 0); r != 0 {
		info.Accel = accelFromParams(mp)
	}

	info.Vendors = detectMouseVendors(listProcesses())
	return info
}

// disableAccel — выключить «повышенную точность указателя» (accel = [0,0,0]).
func disableAccel() error {
	mp := [3]int32{0, 0, 0}
	r, _, err := procSystemParamInfo.Call(spiSetMouse, 0, uintptr(unsafe.Pointer(&mp[0])), spifSendChange)
	if r == 0 {
		if err != nil {
			return err
		}
		return errors.New("не удалось изменить настройку мыши")
	}
	return nil
}

// listProcesses — имена запущенных процессов (через tasklist).
func listProcesses() []string {
	out, err := exec.Command("tasklist", "/fo", "csv", "/nh").Output()
	if err != nil {
		return nil
	}
	var procs []string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "\"") {
			continue
		}
		if end := strings.Index(line[1:], "\""); end > 0 {
			procs = append(procs, line[1:1+end])
		}
	}
	return procs
}
