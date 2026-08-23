package main

import (
	"testing"
	"time"
)

func TestBurnAmount(t *testing.T) {
	cases := []struct {
		name    string
		balance int64
		pct     int64
		want    int64
	}{
		{"10% от тысячи", 1000, 10, 100},
		{"округляем вверх", 455, 10, 46},
		{"мелкий остаток добиваем, чтобы хвост не висел вечно", 5, 10, 5},
		{"почти пустой баланс — под ноль", 3, 10, 3},
		{"пустой баланс", 0, 10, 0},
		{"таяние выключено", 1000, 0, 0},
		{"сто процентов — всё", 1000, 100, 1000},
	}
	for _, tc := range cases {
		if got := burnAmount(tc.balance, tc.pct); got != tc.want {
			t.Errorf("%s: burnAmount(%d,%d) = %d, ожидалось %d", tc.name, tc.balance, tc.pct, got, tc.want)
		}
	}
}

// Баланс неактивного гостя должен сходиться на нет за обозримый срок, а не
// висеть вечно хвостом: проверяем, что 10% в неделю добивают тысячу монет.
func TestТаяниеДоходитДоНуля(t *testing.T) {
	balance := int64(1000)
	weeks := 0
	for balance > 0 && weeks < 200 {
		balance -= burnAmount(balance, 10)
		weeks++
	}
	if balance != 0 {
		t.Fatalf("баланс застрял на %d монетах", balance)
	}
	if weeks > 60 {
		t.Errorf("тысяча монет таяла %d недель — слишком долго", weeks)
	}
	t.Logf("тысяча монет сошла на нет за %d недель", weeks)
}

func TestShouldBurn(t *testing.T) {
	now := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)
	ago := func(d int) time.Time { return now.AddDate(0, 0, -d) }
	ptr := func(t time.Time) *time.Time { return &t }

	cases := []struct {
		name        string
		lastSession time.Time
		lastBurn    *time.Time
		want        bool
	}{
		{"играл вчера — не трогаем", ago(1), nil, false},
		{"не был 89 дней — ещё рано", ago(89), nil, false},
		{"не был 90 дней — пора", ago(90), nil, true},
		{"не был полгода, ни разу не жгли", ago(180), nil, true},
		{"жгли три дня назад — ждём неделю", ago(180), ptr(ago(3)), false},
		{"жгли восемь дней назад — снова пора", ago(180), ptr(ago(8)), true},
		{"сгорание было ДО последней сессии — человек возвращался", ago(95), ptr(ago(200)), true},
	}
	for _, tc := range cases {
		if got := shouldBurn(tc.lastSession, tc.lastBurn, now, 90, 10); got != tc.want {
			t.Errorf("%s: shouldBurn = %v, ожидалось %v", tc.name, got, tc.want)
		}
	}
	if shouldBurn(ago(180), nil, now, 90, 0) {
		t.Error("при нулевом проценте жечь нельзя")
	}
	if shouldBurn(ago(180), nil, now, 0, 10) {
		t.Error("при нулевом сроке неприкосновенности жечь нельзя")
	}
}

func TestShouldWarn(t *testing.T) {
	now := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)
	ago := func(d int) time.Time { return now.AddDate(0, 0, -d) }
	cases := []struct {
		name string
		days int
		want bool
	}{
		{"играл недавно — рано пугать", 30, false},
		{"75 дней — за две недели до старта, пора", 76, true},
		{"89 дней — последний день окна", 89, true},
		{"90 дней — уже тает, предупреждать поздно", 90, false},
		{"полгода — давно тает", 180, false},
	}
	for _, tc := range cases {
		if got := shouldWarn(ago(tc.days), now, 90, 14); got != tc.want {
			t.Errorf("%s: shouldWarn(%d дней) = %v, ожидалось %v", tc.name, tc.days, got, tc.want)
		}
	}
}
