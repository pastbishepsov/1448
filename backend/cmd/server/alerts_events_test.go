package main

import (
	"strings"
	"testing"
	"time"
)

// З22-и2: событийные алерты — когда товар «заканчивается» и что говорят тексты.

func TestLowStockCrossed(t *testing.T) {
	cases := []struct {
		name              string
		after, delta, low int
		want              bool
	}{
		{"6 → 5 при пороге 5 — дошёл", 5, -1, 5, true},
		{"8 → 2 одной продажей — перешёл", 2, -6, 5, true},
		{"5 → 4 — уже был на пороге, не событие", 4, -1, 5, false},
		{"7 → 6 — ещё выше порога", 6, -1, 5, false},
		{"порог 0 — не следим", 0, -3, 0, false},
		{"приход 4 → 6 — не событие", 6, 2, 5, false},
	}
	for _, tc := range cases {
		if got := lowStockCrossed(tc.after, tc.delta, tc.low); got != tc.want {
			t.Errorf("%s: %v, ожидалось %v", tc.name, got, tc.want)
		}
	}
	if lowStockText("Cola", 3, 5) != "📦 Заканчивается «Cola»: на складе 3, минимум — 5" ||
		lowStockText("Cola", 0, 5) != "📦 Закончилось «Cola»: на складе 0, минимум — 5" {
		t.Error("lowStockText: «заканчивается» с остатком, на нуле — «закончилось»")
	}
}

func TestVoidDeniedText(t *testing.T) {
	at := time.Date(2026, 11, 10, 18, 20, 0, 0, time.UTC)
	if got := voidDeniedText("admin1", "продажу «Cola» 6,00 zł", "admin2", at, "not_yours"); got !=
		"⛔ admin1: попытка отменить чужую операцию — продажу «Cola» 6,00 zł (от admin2, 10.11 18:20); сервер не дал — такое отменяет владелец" {
		t.Errorf("чужое: %q", got)
	}
	if got := voidDeniedText("admin1", "пополнение Guest31 на 50,00 zł", "", at, "too_old"); !strings.Contains(got,
		"попытка отменить операцию не из своей смены — пополнение Guest31 на 50,00 zł (10.11 18:20)") {
		t.Errorf("не из смены, автора нет: %q", got)
	}
}

func TestForgotText(t *testing.T) {
	start := time.Date(2026, 11, 9, 10, 2, 0, 0, time.UTC)
	now := time.Date(2026, 11, 10, 9, 58, 0, 0, time.UTC)
	if got := forgotText("admin1", "День", start, time.Date(2026, 11, 9, 20, 0, 0, 0, time.UTC), now); got !=
		"🚪 Смена admin1 «День» 09.11 с 10:02 закрылась сама: уход не отмечен, касса не сдана. Табель закрыл её при следующем "+
			"входе (10.11 09:58), по графику — до 20:00. Проверь табель и ящик" {
		t.Errorf("со сменой: %q", got)
	}
	if got := forgotText("admin1", "", start, start, now); !strings.Contains(got, "Смена admin1 09.11 с 10:02 закрылась") ||
		!strings.Contains(got, "смены по графику не было — часов ноль") {
		t.Errorf("без смены: %q", got)
	}
}
