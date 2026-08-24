//go:build windows

package main

// Г9-и3: киоск-добивка — выключение «залипания», «фильтрации» и «озвучивания
// переключателей» клавиш через SystemParametersInfo. Пять нажатий Shift во
// время матча не должны ронять гостя в системный диалог Windows. dwFlags=0
// гасит и саму функцию, и её хоткей-активацию (5×Shift, Shift 8 сек,
// NumLock 5 сек). Вызывается на старте агента и на каждом session_start —
// вернуть«как было» некому: это гостевой ПК клуба, не личная машина.

import (
	"log"
	"unsafe"
)

const (
	spiSetStickyKeys = 0x003B
	spiSetToggleKeys = 0x0035
	spiSetFilterKeys = 0x0033
)

var procSystemParametersInfo = user32.NewProc("SystemParametersInfoW")

type stickyKeys struct {
	cbSize  uint32
	dwFlags uint32
}

type toggleKeys struct {
	cbSize  uint32
	dwFlags uint32
}

type filterKeys struct {
	cbSize      uint32
	dwFlags     uint32
	iWaitMSec   uint32
	iDelayMSec  uint32
	iRepeatMSec uint32
	iBounceMSec uint32
}

// disableAccessibilityShortcuts — выключить залипание/фильтрацию/переключатели.
func disableAccessibilityShortcuts() {
	sk := stickyKeys{dwFlags: 0}
	sk.cbSize = uint32(unsafe.Sizeof(sk))
	r1, _, _ := procSystemParametersInfo.Call(spiSetStickyKeys, uintptr(sk.cbSize), uintptr(unsafe.Pointer(&sk)), 0)

	tk := toggleKeys{dwFlags: 0}
	tk.cbSize = uint32(unsafe.Sizeof(tk))
	r2, _, _ := procSystemParametersInfo.Call(spiSetToggleKeys, uintptr(tk.cbSize), uintptr(unsafe.Pointer(&tk)), 0)

	fk := filterKeys{dwFlags: 0}
	fk.cbSize = uint32(unsafe.Sizeof(fk))
	r3, _, _ := procSystemParametersInfo.Call(spiSetFilterKeys, uintptr(fk.cbSize), uintptr(unsafe.Pointer(&fk)), 0)

	if r1 != 0 && r2 != 0 && r3 != 0 {
		log.Println("⌨ Залипание/фильтрация клавиш выключены (SystemParametersInfo)")
	} else {
		log.Printf("⌨ Залипание клавиш: часть вызовов не прошла (sticky=%d toggle=%d filter=%d)", r1, r2, r3)
	}
}
