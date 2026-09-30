package main

import (
	"testing"
	"time"
)

// З11-и3: месяц блока «Возвраты» — по умолчанию прошлый (решение основателя 30.09), по клубным суткам.
func TestReturnsMonth(t *testing.T) {
	at := func(y int, m time.Month, d, h int) time.Time { return time.Date(y, m, d, h, 0, 0, 0, time.UTC) }
	cases := []struct {
		name, month string
		now         time.Time
		ok          bool
		from, to    time.Time // окно: клубные сутки месяца
	}{
		{"по умолчанию — прошлый месяц", "", at(2026, 9, 30, 12), true, at(2026, 8, 1, 8), at(2026, 9, 1, 8)},
		{"1-е число до границы суток — ещё прошлый месяц, значит позапрошлый", "", at(2026, 9, 1, 7), true, at(2026, 7, 1, 8), at(2026, 8, 1, 8)},
		{"1-е число после границы — уже новый месяц", "", at(2026, 9, 1, 9), true, at(2026, 8, 1, 8), at(2026, 9, 1, 8)},
		{"январь — прошлый декабрь", "", at(2027, 1, 15, 12), true, at(2026, 12, 1, 8), at(2027, 1, 1, 8)},
		{"текущий месяц можно", "2026-09", at(2026, 9, 30, 12), true, at(2026, 9, 1, 8), at(2026, 10, 1, 8)},
		{"давний месяц можно", "2026-02", at(2026, 9, 30, 12), true, at(2026, 2, 1, 8), at(2026, 3, 1, 8)},
		{"будущий — нет", "2026-10", at(2026, 9, 30, 12), false, time.Time{}, time.Time{}},
		{"месяц без нуля — нет", "2026-9", at(2026, 9, 30, 12), false, time.Time{}, time.Time{}},
		{"тринадцатый — нет", "2026-13", at(2026, 9, 30, 12), false, time.Time{}, time.Time{}},
		{"с днём — нет", "2026-09-01", at(2026, 9, 30, 12), false, time.Time{}, time.Time{}},
		{"до 2000 — нет", "1999-12", at(2026, 9, 30, 12), false, time.Time{}, time.Time{}},
		{"слово — нет", "август", at(2026, 9, 30, 12), false, time.Time{}, time.Time{}},
	}
	for _, c := range cases {
		p, ok := returnsMonth(c.month, 8, c.now)
		if ok != c.ok {
			t.Errorf("%s: ok = %v, ждали %v", c.name, ok, c.ok)
			continue
		}
		if ok && (!p.From.Equal(c.from) || !p.To.Equal(c.to)) {
			t.Errorf("%s: окно %s — %s, ждали %s — %s", c.name, p.From, p.To, c.from, c.to)
		}
	}
}

// Итоги: живые деньги — наличными, на карту, по BLIK; кошелёк — отдельно; «не возвращали» — штуки и на сколько
// платили; «по операциям прошлых месяцев» — оплаченные до начала месяца, сколько по ним вернули.
func TestSumReturns(t *testing.T) {
	month := time.Date(2026, 8, 1, 8, 0, 0, 0, time.UTC)
	old, fresh := month.Add(-time.Hour), month.Add(time.Hour)
	rows := []returnRow{
		{Method: "cash", RefundGrosz: 1200, PaidGrosz: 1200, PaidAt: old},
		{Method: "cash", RefundGrosz: 3000, PaidGrosz: 5000, PaidAt: fresh}, // депозит: вернули остаток
		{Method: "card", RefundGrosz: 700, PaidGrosz: 700, PaidAt: fresh},
		{Method: "blik", RefundGrosz: 250, PaidGrosz: 250, PaidAt: old},
		{Method: "wallet", RefundGrosz: 900, PaidGrosz: 900, PaidAt: old},
		{Method: "none", RefundGrosz: 0, PaidGrosz: 1500, PaidAt: old},
		{Method: "cash", RefundGrosz: 0, PaidGrosz: 2000, PaidAt: fresh}, // всё отыграно — вернуть нечего
	}
	got := sumReturns(rows, month)
	want := returnsTotals{Count: 7, Live: [2]int64{5150, 5}, Cash: [2]int64{4200, 3}, Card: [2]int64{700, 1},
		Blik: [2]int64{250, 1}, Wallet: [2]int64{900, 1}, None: [2]int64{1500, 1}, Earlier: [2]int64{2350, 4}}
	if got != want {
		t.Errorf("sumReturns = %+v, ждали %+v", got, want)
	}
	if z := sumReturns(nil, month); z != (returnsTotals{}) {
		t.Errorf("пусто — нули, а не %+v", z)
	}
}

// Книга: из ящика — только наличные и не нуль, по порядку дня (returnsIn отдаёт свежие сверху).
func TestCashRefunds(t *testing.T) {
	t0 := time.Date(2026, 9, 30, 10, 0, 0, 0, time.UTC)
	rows := []returnRow{ // свежие сверху
		{Name: "d", Method: "cash", RefundGrosz: 100, VoidedAt: t0.Add(4 * time.Hour)},
		{Name: "c", Method: "card", RefundGrosz: 100, VoidedAt: t0.Add(3 * time.Hour)},
		{Name: "b", Method: "cash", RefundGrosz: 0, VoidedAt: t0.Add(2 * time.Hour)},
		{Name: "a", Method: "cash", RefundGrosz: 250, VoidedAt: t0.Add(time.Hour)},
		{Name: "n", Method: "none", RefundGrosz: 0, VoidedAt: t0},
	}
	got := cashRefunds(rows)
	if len(got) != 2 || got[0].Name != "a" || got[1].Name != "d" {
		t.Errorf("cashRefunds = %+v, ждали a, d", got)
	}
	if len(cashRefunds(nil)) != 0 {
		t.Error("пусто — пусто")
	}
}
