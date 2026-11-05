package main

import (
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

func TestValidateZone(t *testing.T) {
	cases := []struct {
		name string
		zone string
		rate float64
		want string
	}{
		{"обычная зона", "VIP", 35, ""},
		{"название придумывает владелец", "Симрейсинг", 60, ""},
		{"копеечная цена всё равно цена", "Утренняя", 0.5, ""},
		{"пустое название", "   ", 23, "bad_name"},
		{"название длиннее 32", string(make([]rune, 33)), 23, "bad_name"},
		{"нулевая цена", "VIP", 0, "bad_rate"},
		{"отрицательная цена", "VIP", -5, "bad_rate"},
		{"опечатка в три нуля", "VIP", 23000, "bad_rate"},
		{"меньше копейки (З6-и3)", "VIP", 0.004, "bad_rate"},
		{"три знака после запятой (З6-и3)", "VIP", 1.005, "bad_rate"},
	}
	for _, tc := range cases {
		_, code := validateZone(tc.zone, tc.rate)
		if code != tc.want {
			t.Errorf("%s: validateZone = %q, ожидалось %q", tc.name, code, tc.want)
		}
	}
}

func TestRateForComputer(t *testing.T) {
	vip := 35.0
	zero := 0.0
	cases := []struct {
		name     string
		zoneRate *float64
		club     float64
		want     float64
	}{
		{"есть зона — её цена", &vip, 23, 35},
		{"нет зоны — клубный тариф", nil, 23, 23},
		{"битая цена зоны — не обнуляем сессию", &zero, 23, 23},
	}
	for _, tc := range cases {
		if got := rateForComputer(tc.zoneRate, tc.club); got != tc.want {
			t.Errorf("%s: rateForComputer = %v, ожидалось %v", tc.name, got, tc.want)
		}
	}
}

// З16-и1а: пересадка спрашивает про тариф только между разными зонами; ПК без зоны — тоже «зона» (клубный тариф).
func TestSameZoneID(t *testing.T) {
	std, vip := uuid.New(), uuid.New()
	std2 := std
	cases := []struct {
		name string
		a, b *uuid.UUID
		want bool
	}{
		{"Standard → Standard", &std, &std2, true},
		{"Standard → VIP", &std, &vip, false},
		{"без зоны → без зоны", nil, nil, true},
		{"без зоны → VIP", nil, &vip, false},
		{"VIP → без зоны", &vip, nil, false},
	}
	for _, tc := range cases {
		if got := sameZoneID(tc.a, tc.b); got != tc.want {
			t.Errorf("%s: sameZoneID = %v, ожидалось %v", tc.name, got, tc.want)
		}
	}
	if zoneLabel("") != "без зоны" || zoneLabel("VIP") != "VIP" {
		t.Errorf("zoneLabel: %q, %q", zoneLabel(""), zoneLabel("VIP"))
	}
}

// З16-и2: зона сессии — её снимок (и без зоны — тоже снимок); снимка нет — зона ПК, и только тогда её спрашиваем.
func TestZoneOfSession(t *testing.T) {
	std, vip := uuid.New(), uuid.New()
	xp := int64(10)
	asked := 0
	pc := func() *uuid.UUID { asked++; return &vip }
	if got := zoneOfSession(&models.Session{ZoneID: &std, XPPerMin: &xp}, pc); got == nil || *got != std || asked != 0 {
		t.Errorf("снимок Standard, ПК уже в VIP: %v, зону ПК спросили %d раз", got, asked)
	}
	if got := zoneOfSession(&models.Session{XPPerMin: &xp}, pc); got != nil || asked != 0 {
		t.Errorf("снимок без зоны, ПК в VIP: %v, зону ПК спросили %d раз", got, asked)
	}
	if got := zoneOfSession(&models.Session{ZoneID: &std}, pc); got == nil || *got != vip || asked != 1 {
		t.Errorf("сессия без снимка — зона ПК: %v, зону ПК спросили %d раз", got, asked)
	}
}

// З18-и1: что держит зону от удаления — сначала то, что мастер не решает, последними — живые минуты.
func TestZoneDeleteBlock(t *testing.T) {
	cases := []struct {
		name string
		h    zoneHold
		want string
	}{
		{"пустая зона — удаляется", zoneHold{}, ""},
		{"ПК — первыми", zoneHold{Computers: 3, Sales: 1, Packages: 1, Minutes: 30, Guests: 1}, "has_computers"},
		{"продажи пакетов — навсегда, даже без каталога", zoneHold{Sales: 2, Minutes: 30, Guests: 1}, "package_history"},
		{"непроданные пакеты каталога", zoneHold{Packages: 1, Minutes: 30, Guests: 1}, "has_packages"},
		{"только живые минуты — мастер", zoneHold{Minutes: 30, Guests: 1}, "has_minutes"},
	}
	for _, tc := range cases {
		if got := zoneDeleteBlock(tc.h); got != tc.want {
			t.Errorf("%s: zoneDeleteBlock = %q, ожидалось %q", tc.name, got, tc.want)
		}
	}
	if got := zoneMinutesText(zoneHold{Minutes: 45, Guests: 2}); !strings.HasPrefix(got, "У 2 гостей 45 мин этой зоны") {
		t.Errorf("текст 409: %q", got)
	}
	for n, want := range map[int64]string{1: "1 гостя", 2: "2 гостей", 5: "5 гостей", 11: "11 гостей", 21: "21 гостя", 111: "111 гостей"} {
		if got := guestsGen(n); got != want {
			t.Errorf("guestsGen(%d) = %q, ожидалось %q", n, got, want)
		}
	}
}

// З18-и1: перенос и сжигание — по гостям в порядке первой выдачи, злотые — по тарифу каждой выдачи.
func TestMinutesByGuest(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	got := minutesByGuest([]models.MinuteGrant{
		{UserID: a, MinutesLeft: 30, RatePLN: 10},
		{UserID: b, MinutesLeft: 20, RatePLN: 10},
		{UserID: a, MinutesLeft: 15, RatePLN: 30},
	})
	if len(got) != 2 || got[0].UserID != a || got[0].Minutes != 45 || got[0].PLN != 12.5 ||
		got[1].UserID != b || got[1].Minutes != 20 || got[1].PLN != 3.33 {
		t.Errorf("minutesByGuest = %+v", got)
	}
	if got := minutesByGuest(nil); len(got) != 0 {
		t.Errorf("без выдач — пусто: %+v", got)
	}
}
