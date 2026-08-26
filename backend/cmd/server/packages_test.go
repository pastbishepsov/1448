package main

import (
	"testing"
	"time"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

// Е2-и1: границы каталога. Опечатка в цене или минутах дороже обычной —
// именно она определяет, сколько клуб будет должен гостю временем.
func TestValidatePackage(t *testing.T) {
	cases := []struct {
		name    string
		nm      string
		minutes int
		price   float64
		days    int
		code    string
	}{
		{"нормальный пакет", "3 часа STANDARD", 180, 45, 0, ""},
		{"акция со сроком", "3 часа на неделю", 180, 39, 7, ""},
		{"без имени", "  ", 180, 45, 0, "bad_name"},
		{"имя длиннее 64", string(make([]rune, 65)), 180, 45, 0, "bad_name"},
		{"ноль минут", "пустой", 0, 45, 0, "bad_minutes"},
		{"минусовые минуты", "минус", -60, 45, 0, "bad_minutes"},
		{"абонемент на год", "год", maxPackageMinutes + 1, 45, 0, "bad_minutes"},
		{"бесплатный пакет", "подарок", 180, 0, 0, "bad_price"},
		{"цена с лишним нулём", "опечатка", 180, maxPackagePrice + 1, 0, "bad_price"},
		{"отрицательный срок", "минус дней", 180, 45, -1, "bad_days"},
		{"срок за горизонтом", "вечность", 180, 45, maxPackageDays + 1, "bad_days"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ok, code := validatePackage(c.nm, c.minutes, c.price, c.days)
			if (c.code == "") != ok || code != c.code {
				t.Fatalf("validatePackage(%q, %d, %v, %d) = %v, %q; ждали %q",
					c.nm, c.minutes, c.price, c.days, ok, code, c.code)
			}
		})
	}
}

// Ради цены часа внутри пакета его и покупают: если она не ниже цены зоны,
// пакет бессмыслен, и увидеть это должен и владелец, и гость — поэтому
// считает сервер, а не каждый клиент по-своему.
func TestPackageHourPLN(t *testing.T) {
	cases := []struct {
		name    string
		minutes int
		price   float64
		want    float64
	}{
		{"3 часа за 45 — 15 zł/час", 180, 45, 15},
		{"5 часов за 90 — 18 zł/час", 300, 90, 18},
		{"час за час", 60, 20, 20},
		{"90 минут за 25 — округление до гроша", 90, 25, 16.67},
		{"пустой пакет не делит на ноль", 0, 45, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := packageHourPLN(c.minutes, c.price); got != c.want {
				t.Fatalf("packageHourPLN(%d, %v) = %v; ждали %v", c.minutes, c.price, got, c.want)
			}
		})
	}
}

// Подпись срока читает человек у стойки: «бессрочно» словом, а не «0 дн».
func TestDaysSuffix(t *testing.T) {
	if got := daysSuffix(0); got != " · бессрочно" {
		t.Fatalf("daysSuffix(0) = %q", got)
	}
	if got := daysSuffix(7); got != " · 7 дн" {
		t.Fatalf("daysSuffix(7) = %q", got)
	}
}

// ── Е2-и2: выдача и отмена ───────────────────────────────────────────

// Срок считается от ВЫДАЧИ: гость покупает свой срок, а не остаток чужого.
func TestPackageExpiry(t *testing.T) {
	now := time.Date(2026, 8, 25, 14, 48, 0, 0, time.UTC)
	if got := packageExpiry(0, now); got != nil {
		t.Fatalf("0 дней = бессрочно (Р11), а получили %v", got)
	}
	got := packageExpiry(7, now)
	if got == nil || !got.Equal(now.AddDate(0, 0, 7)) {
		t.Fatalf("7 дней от выдачи = %v; получили %v", now.AddDate(0, 0, 7), got)
	}
}

func TestPackageLive(t *testing.T) {
	now := time.Date(2026, 8, 25, 14, 48, 0, 0, time.UTC)
	past := now.Add(-time.Minute)
	future := now.Add(time.Hour)
	cases := []struct {
		name string
		p    models.UserPackage
		want bool
	}{
		{"бессрочный с остатком", models.UserPackage{MinutesLeft: 60}, true},
		{"со сроком в будущем", models.UserPackage{MinutesLeft: 60, ExpiresAt: &future}, true},
		{"просроченный", models.UserPackage{MinutesLeft: 60, ExpiresAt: &past}, false},
		{"отыгранный до нуля", models.UserPackage{MinutesLeft: 0}, false},
		{"отменённый", models.UserPackage{MinutesLeft: 60, VoidedAt: &past}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := packageLive(&c.p, now); got != c.want {
				t.Fatalf("packageLive(%s) = %v; ждали %v", c.name, got, c.want)
			}
		})
	}
}

// «Сгорит через 0 дней» звучит как «уже сгорел» — считаем вверх, пока пакет
// действительно жив.
func TestDaysLeft(t *testing.T) {
	now := time.Date(2026, 8, 25, 14, 48, 0, 0, time.UTC)
	cases := []struct {
		name string
		exp  time.Time
		want int
	}{
		{"ровно сутки", now.Add(24 * time.Hour), 1},
		{"остаток дня — ещё день", now.Add(3 * time.Hour), 1},
		{"полторы недели", now.Add(10 * 24 * time.Hour), 10},
		{"уже сгорел", now.Add(-time.Hour), 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := daysLeft(c.exp, now); got != c.want {
				t.Fatalf("daysLeft(%s) = %d; ждали %d", c.name, got, c.want)
			}
		})
	}
}

// Возврат при отмене — доля цены за НЕОТЫГРАННЫЕ минуты, вниз до гроша:
// отыгранное клуб уже отдал, и округление в пользу гостя означало бы
// заплатить за него дважды.
func TestRefundForVoid(t *testing.T) {
	cases := []struct {
		name        string
		price       int64
		total, left int
		want        int64
	}{
		{"ничего не отыграно — вернуть всё", 4500, 180, 180, 4500},
		{"отыграна половина", 4500, 180, 90, 2250},
		{"отыграно всё — возврата нет", 4500, 180, 0, 0},
		{"остаток трети, вниз до гроша", 5000, 180, 60, 1666},
		{"остаток больше выданного не бывает", 4500, 180, 999, 4500},
		{"бесплатный пакет", 0, 180, 180, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := refundForVoid(c.price, c.total, c.left); got != c.want {
				t.Fatalf("refundForVoid(%d, %d, %d) = %d; ждали %d",
					c.price, c.total, c.left, got, c.want)
			}
		})
	}
}

// Е2-и5: пакет со сроком сгорает молча — если о нём не сказать. Проверяем
// именно границу: предупреждаем ДО срока и ровно один раз за окно.
func TestPackWarnDue(t *testing.T) {
	now := time.Date(2026, 8, 26, 14, 48, 0, 0, time.UTC)
	cases := []struct {
		name    string
		expires time.Time
		days    int64
		want    bool
	}{
		{"сгорит завтра — пора", now.Add(24 * time.Hour), 3, true},
		{"сгорит через час — пора", now.Add(time.Hour), 3, true},
		{"ровно на границе окна", now.Add(72 * time.Hour), 3, true},
		{"ещё далеко — молчим", now.Add(96 * time.Hour), 3, false},
		{"уже сгорел — предупреждать поздно", now.Add(-time.Hour), 3, false},
		{"предупреждения выключены", now.Add(time.Hour), 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := packWarnDue(c.expires, now, c.days); got != c.want {
				t.Fatalf("packWarnDue(%s) = %v; ждали %v", c.name, got, c.want)
			}
		})
	}
}
