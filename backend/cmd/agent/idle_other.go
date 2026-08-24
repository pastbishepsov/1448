//go:build !windows

package main

// Заглушка AFK-датчика для Linux/CI (Г2): -1 = датчика нет, сервер по такому
// значению AFK не судит (без датчика гость всегда «активен»).
func idleSeconds() int { return -1 }
