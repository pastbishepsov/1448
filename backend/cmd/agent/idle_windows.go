//go:build windows

package main

// AFK-датчик (трек Г, Г2): секунды с последнего ввода (клавиатура/мышь) через
// GetLastInputInfo. Изоляция Windows-вызовов build-тегами — по образцу
// mouse_windows.go (user32 объявлен там же).

import (
	"syscall"
	"unsafe"
)

var (
	kernel32         = syscall.NewLazyDLL("kernel32.dll")
	procGetTickCount = kernel32.NewProc("GetTickCount")
	procGetLastInput = user32.NewProc("GetLastInputInfo")
)

type lastInputInfo struct {
	cbSize uint32
	dwTime uint32
}

// idleSeconds — сколько секунд не было ввода; -1 = датчик недоступен.
// Тики Windows 32-битные (переполнение ~49 дней) — вычитание в uint32
// переживает его корректно.
func idleSeconds() int {
	var lii lastInputInfo
	lii.cbSize = uint32(unsafe.Sizeof(lii))
	r, _, _ := procGetLastInput.Call(uintptr(unsafe.Pointer(&lii)))
	if r == 0 {
		return -1
	}
	tick, _, _ := procGetTickCount.Call()
	return int((uint32(tick) - lii.dwTime) / 1000)
}
