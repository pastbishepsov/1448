package main

import (
	"testing"
	"time"
)

// К0-и5: десять промахов за 15 минут запирают аккаунт, окно остывает само,
// успешный вход снимает счётчик; регистр и пробелы логина не важны.
func TestLoginGuard(t *testing.T) {
	g := newLoginGuard()
	now := time.Date(2026, 9, 5, 20, 0, 0, 0, time.UTC)

	for i := 0; i < loginMaxFails-1; i++ {
		g.fail("Gamer1", now.Add(time.Duration(i)*time.Second))
	}
	if ok, _ := g.locked("gamer1", now.Add(time.Minute)); ok {
		t.Fatalf("%d промахов ещё не блокируют", loginMaxFails-1)
	}
	g.fail(" gamer1 ", now.Add(30*time.Second))
	ok, wait := g.locked("GAMER1", now.Add(time.Minute))
	if !ok {
		t.Fatal("десятый промах должен запереть аккаунт")
	}
	if wait <= 0 || wait > loginFailWindow {
		t.Errorf("странный срок ожидания: %v", wait)
	}
	// чужой аккаунт не задет
	if ok, _ := g.locked("gamer2", now.Add(time.Minute)); ok {
		t.Error("блокировка не должна цеплять другие логины")
	}
	// окно остыло — самая старая попытка вышла за 15 минут
	if ok, _ := g.locked("gamer1", now.Add(loginFailWindow+time.Second)); ok {
		t.Error("после окна блокировка должна сниматься сама")
	}
	// повторные промахи после остывания снова копятся с нуля
	for i := 0; i < loginMaxFails; i++ {
		g.fail("gamer1", now.Add(20*time.Minute))
	}
	if ok, _ := g.locked("gamer1", now.Add(20*time.Minute)); !ok {
		t.Error("новая серия промахов должна запереть снова")
	}
	g.reset("gamer1")
	if ok, _ := g.locked("gamer1", now.Add(20*time.Minute)); ok {
		t.Error("reset должен снять блокировку")
	}
}

// TRUSTED_PROXIES: пусто → nil (никому не верить), список → без пробелов и пустых.
func TestTrustedProxies(t *testing.T) {
	if got := trustedProxies(""); got != nil {
		t.Errorf("пусто → nil, получили %v", got)
	}
	if got := trustedProxies(" 10.0.0.0/8, 127.0.0.1 ,, "); len(got) != 2 || got[0] != "10.0.0.0/8" || got[1] != "127.0.0.1" {
		t.Errorf("список: %v", got)
	}
}
