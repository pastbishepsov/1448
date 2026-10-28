package main

import (
	"testing"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

func TestBoostedXP(t *testing.T) {
	cases := []struct {
		base   int64
		effect float64
		want   int64
	}{
		{1500, 0.0, 1500},  // нет таланта
		{1500, 0.30, 1950}, // xp_boost ур.3 (+30%)
		{1500, 0.50, 2250}, // xp_boost ур.5 (+50%)
		{1000, 0.10, 1100}, // ур.1
		{0, 0.5, 0},        // нулевой базовый опыт
		{777, -0.2, 777},   // отрицательный эффект игнорируется
	}
	for _, tc := range cases {
		if got := boostedXP(tc.base, tc.effect); got != tc.want {
			t.Errorf("boostedXP(%d, %.2f) = %d, хотим %d", tc.base, tc.effect, got, tc.want)
		}
	}
}

// З16-и1 (З-Р25): начисление — по снимку сессии на её начало; ставки без снимка (сессия прежней версии сервера) —
// нынешние; ноль в снимке — тоже снимок; курс монет за пополнение — всегда нынешний.
func TestSessionRates(t *testing.T) {
	n := func(v int64) *int64 { return &v }
	cur := economyRateSet{XPPerMin: 100, CoinsPerMin: 100, CoinsPerPLN: 10} // владелец поднял ставки посреди сессии
	cases := []struct {
		name string
		s    models.Session
		want economyRateSet
	}{
		{"снимок на начало", models.Session{XPPerMin: n(10), CoinsPerMin: n(2)}, economyRateSet{10, 2, 10}},
		{"без снимка — нынешние", models.Session{}, cur},
		{"ноль монет в снимке — ноль", models.Session{XPPerMin: n(10), CoinsPerMin: n(0)}, economyRateSet{10, 0, 10}},
		{"снимок только XP", models.Session{XPPerMin: n(13)}, economyRateSet{13, 100, 10}},
	}
	for _, tc := range cases {
		if got := sessionRates(&tc.s, cur); got != tc.want {
			t.Errorf("%s: sessionRates = %+v, хотим %+v", tc.name, got, tc.want)
		}
	}
	// сценарий M ревью: 10 минут по снимку 2 монеты за минуту — 20 монет, а не 1000 по нынешним 100
	s := models.Session{XPPerMin: n(10), CoinsPerMin: n(2)}
	if xp, coins := sessionRewards(10, sessionRates(&s, cur), 0, AccountRank{XPMult: 1, CoinMult: 1}); xp != 100 || coins != 20 {
		t.Errorf("сценарий M: %d XP / %d монет, хотим 100 / 20", xp, coins)
	}
}
