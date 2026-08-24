//go:build !windows

package main

// Заглушка для Linux/CI (Г9-и3): залипание клавиш — история Windows-киоска.
func disableAccessibilityShortcuts() {}
