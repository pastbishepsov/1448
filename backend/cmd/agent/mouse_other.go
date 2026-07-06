//go:build !windows

package main

// Заглушки для не-Windows (сборка/тесты на Linux/CI). Настройки мыши читаются
// только на Windows — на других ОС агент честно сообщает «недоступно».

import "errors"

func readMouse() MouseInfo {
	return MouseInfo{Available: false, Note: "Чтение настроек мыши доступно только на Windows"}
}

func disableAccel() error {
	return errors.New("выключение акселерации доступно только на Windows")
}

func listProcesses() []string { return nil }
