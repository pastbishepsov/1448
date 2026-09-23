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
		g.fail("Gamer1", false, now.Add(time.Duration(i)*time.Second))
	}
	if ok, _ := g.locked("gamer1", false, now.Add(time.Minute)); ok {
		t.Fatalf("%d промахов ещё не блокируют", loginMaxFails-1)
	}
	g.fail(" gamer1 ", false, now.Add(30*time.Second))
	ok, wait := g.locked("GAMER1", false, now.Add(time.Minute))
	if !ok {
		t.Fatal("десятый промах должен запереть аккаунт")
	}
	if wait <= 0 || wait > loginFailWindow {
		t.Errorf("странный срок ожидания: %v", wait)
	}
	// чужой аккаунт не задет
	if ok, _ := g.locked("gamer2", false, now.Add(time.Minute)); ok {
		t.Error("блокировка не должна цеплять другие логины")
	}
	// окно остыло — самая старая попытка вышла за 15 минут
	if ok, _ := g.locked("gamer1", false, now.Add(loginFailWindow+time.Second)); ok {
		t.Error("после окна блокировка должна сниматься сама")
	}
	// повторные промахи после остывания снова копятся с нуля
	for i := 0; i < loginMaxFails; i++ {
		g.fail("gamer1", false, now.Add(20*time.Minute))
	}
	if ok, _ := g.locked("gamer1", false, now.Add(20*time.Minute)); !ok {
		t.Error("новая серия промахов должна запереть снова")
	}
	g.reset("gamer1")
	if ok, _ := g.locked("gamer1", false, now.Add(20*time.Minute)); ok {
		t.Error("reset должен снять блокировку")
	}
}

// З0-и3 (№28): персоналу после серии — не замок на 15 минут, а пауза loginStaffPace
// от последней неудачи; строка журнала — один раз на серию и заново после остывания.
func TestLoginGuardStaffPace(t *testing.T) {
	g := newLoginGuard()
	now := time.Date(2026, 9, 23, 20, 0, 0, 0, time.UTC)
	reports := 0
	for i := 0; i < loginMaxFails; i++ {
		if _, rep := g.fail("boss", true, now); rep {
			reports++
		}
	}
	if reports != 1 {
		t.Fatalf("строк журнала за серию: %d, ждали 1", reports)
	}
	if ok, wait := g.locked("boss", true, now.Add(10*time.Second)); !ok || wait != loginStaffPace-10*time.Second {
		t.Fatalf("через 10 с: locked=%v wait=%v, ждали паузу ещё %v", ok, wait, loginStaffPace-10*time.Second)
	}
	if ok, _ := g.locked("boss", true, now.Add(loginStaffPace)); ok {
		t.Fatal("после паузы попытка должна проходить")
	}
	if ok, _ := g.locked("boss", false, now.Add(loginStaffPace)); !ok {
		t.Fatal("гостю та же серия — прежний замок")
	}
	// неудача после паузы — снова пауза, но без второй строки журнала
	if n, rep := g.fail("boss", true, now.Add(loginStaffPace)); n != loginMaxFails+1 || rep {
		t.Fatalf("неудача после паузы: n=%d report=%v", n, rep)
	}
	if ok, _ := g.locked("boss", true, now.Add(loginStaffPace+time.Second)); !ok {
		t.Fatal("после новой неудачи — снова пауза")
	}
	// серия остыла — новая серия снова даёт строку журнала
	later := now.Add(loginFailWindow + loginStaffPace + time.Second)
	reports = 0
	for i := 0; i < loginMaxFails; i++ {
		if _, rep := g.fail("boss", true, later); rep {
			reports++
		}
	}
	if reports != 1 {
		t.Fatalf("новая серия после остывания: строк %d, ждали 1", reports)
	}
	g.reset("boss")
	if ok, _ := g.locked("boss", true, later); ok {
		t.Fatal("reset снимает паузу")
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
