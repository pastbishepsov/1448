package main

import (
	"testing"

	"github.com/google/uuid"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

// Ж2-и1: порядок расхода — гранты зоны → пул без зоны → остаток пакетам и кошельку.
func TestSplitMinuteSpend(t *testing.T) {
	cases := []struct {
		name                  string
		delta, grant          int
		coin                  int64
		wantG, wantC, wantRst int
	}{
		{"всё закрывают гранты зоны", 5, 30, 10, 5, 0, 0},
		{"гранты кончились — добирает пул", 5, 2, 10, 2, 3, 0},
		{"грантов нет — пул целиком", 5, 0, 10, 0, 5, 0},
		{"пула не хватает — остаток пакетам/кошельку", 10, 3, 4, 3, 4, 3},
		{"ничего нет — всё кошельку", 7, 0, 0, 0, 0, 7},
		{"нулевая дельта", 0, 30, 10, 0, 0, 0},
		{"отрицательные остатки не ломают арифметику", 4, -1, -5, 0, 0, 4},
	}
	for _, tc := range cases {
		g, c, r := splitMinuteSpend(tc.delta, tc.grant, tc.coin)
		if g != tc.wantG || c != tc.wantC || r != tc.wantRst {
			t.Errorf("%s: grant=%d coin=%d rest=%d, ожидалось %d/%d/%d", tc.name, g, c, r, tc.wantG, tc.wantC, tc.wantRst)
		}
		if tc.delta > 0 && g+c+r != tc.delta {
			t.Errorf("%s: минуты потерялись: %d+%d+%d != %d", tc.name, g, c, r, tc.delta)
		}
	}
}

// grantFor — снимок зоны и остаток = выданному.
func TestGrantFor(t *testing.T) {
	uid, aid, ref := uuid.New(), uuid.New(), uuid.New()
	z := models.Zone{ID: uuid.New(), Name: "VIP", RatePLN: 30}
	g := grantFor(uid, &z, 60, 30, models.MinuteGrantRedeem, &ref, &aid)
	if g.ZoneID == nil || *g.ZoneID != z.ID || g.ZoneName != "VIP" || g.MinutesLeft != 60 || g.MinutesTotal != 60 ||
		g.Source != "redeem" || g.RefID == nil || *g.RefID != ref || g.RatePLN != 30 {
		t.Errorf("грант с зоной собран неверно: %+v", g)
	}
	g2 := grantFor(uid, nil, 15, 23, models.MinuteGrantCompensation, nil, nil)
	if g2.ZoneID != nil || g2.ZoneName != "" || g2.MinutesLeft != 15 || g2.Source != "compensation" || g2.RatePLN != 23 {
		t.Errorf("грант без зоны собран неверно: %+v", g2)
	}
}

// Отмена выдачи: монеты — пропорционально НЕотыгранному остатку гранта
// (критерий Ж2: после 10 из 30 минут возвращаются 2/3 монет).
func TestRefundCoinsForVoidByGrantRemainder(t *testing.T) {
	if got := refundCoinsForVoid(300, 30, 20); got != 200 {
		t.Errorf("30 мин за 300 монет, отыграно 10 → вернуть 200, получили %d", got)
	}
	if got := refundCoinsForVoid(300, 30, 30); got != 300 {
		t.Errorf("нетронутая выдача → все 300, получили %d", got)
	}
	if got := refundCoinsForVoid(300, 30, 0); got != 0 {
		t.Errorf("всё отыграно → 0, получили %d", got)
	}
}
