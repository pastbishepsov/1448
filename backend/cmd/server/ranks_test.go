package main

import "testing"

func TestAccountRankFor(t *testing.T) {
	cases := []struct {
		hours     int
		wantLevel int
		wantName  string
	}{
		{0, 1, "Новичок"},
		{9, 1, "Новичок"},
		{10, 2, "Завсегдатай"},
		{24, 2, "Завсегдатай"},
		{25, 3, "Ветеран"},
		{50, 4, "Мастер"},
		{99, 4, "Мастер"},
		{100, 5, "Элита"},
		{200, 6, "Легенда"},
		{399, 6, "Легенда"},
		{400, 7, "Бессмертный"},
		{5000, 7, "Бессмертный"},
	}
	for _, tc := range cases {
		got, _ := accountRankFor(tc.hours)
		if got.Level != tc.wantLevel || got.Name != tc.wantName {
			t.Errorf("accountRankFor(%d) = %d/%q, ожидалось %d/%q",
				tc.hours, got.Level, got.Name, tc.wantLevel, tc.wantName)
		}
	}
}

func TestAccountRankNext(t *testing.T) {
	// на старте есть следующий ранг с порогом 10ч
	_, next := accountRankFor(0)
	if next == nil || next.Level != 2 || next.MinHours != 10 {
		t.Errorf("next после 0ч: %+v, ожидался ранг 2 (10ч)", next)
	}
	// на максимуме следующего нет
	if _, n := accountRankFor(400); n != nil {
		t.Errorf("на максимуме next должен быть nil, получили %+v", n)
	}
}

func TestRankMonotonic(t *testing.T) {
	// множители и бонусы не убывают с рангом (иначе прогресс «наказывает»)
	for i := 1; i < len(accountRanks); i++ {
		a, b := accountRanks[i-1], accountRanks[i]
		if b.XPMult < a.XPMult || b.CoinMult < a.CoinMult ||
			b.CaseChanceBonus < a.CaseChanceBonus || b.TierBoost < a.TierBoost {
			t.Errorf("ранг %d слабее ранга %d по бонусам", b.Level, a.Level)
		}
		if b.MinHours <= a.MinHours {
			t.Errorf("порог ранга %d (%dч) не больше ранга %d (%dч)", b.Level, b.MinHours, a.Level, a.MinHours)
		}
	}
	// ранг 1 — нейтральный (без бонусов), чтобы новичок играл на «чистой» базе
	r1 := accountRanks[0]
	if r1.XPMult != 1 || r1.CoinMult != 1 || r1.CaseChanceBonus != 0 || r1.TierBoost != 0 {
		t.Errorf("ранг 1 должен быть нейтральным, получили %+v", r1)
	}
}

func TestSessionCaseChance(t *testing.T) {
	// база 0.20, без бонусов
	if got := sessionCaseChance(0, 0); got != baseSessionCaseChance {
		t.Errorf("база = %v, ожидалось %v", got, baseSessionCaseChance)
	}
	// талант + ранг складываются
	if got := sessionCaseChance(0.30, 0.25); got != 0.75 {
		t.Errorf("0.20+0.30+0.25 = %v, ожидалось 0.75", got)
	}
	// потолок
	if got := sessionCaseChance(0.60, 0.40); got != maxSessionCaseChance {
		t.Errorf("должен упереться в потолок %v, получили %v", maxSessionCaseChance, got)
	}
}
