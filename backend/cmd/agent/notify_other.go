//go:build !windows

package main

import "log"

// Не Windows (стенд, разработка): тоста нет — строка в лог, чтобы e2e видел,
// что агент его получил.
func showToast(appID, title, text string) error {
	log.Printf("🔔 (тост только на Windows) %s — %s", title, text)
	return nil
}
