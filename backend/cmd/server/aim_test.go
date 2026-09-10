package main

import (
	"math"
	"testing"
	"time"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

func TestAimCm360(t *testing.T) {
	// 2.0 @ 800 DPI → 41563.64 / 1600 = 25.98 см
	if got := aimCm360(2.0, 800); math.Abs(got-25.98) > 0.005 {
		t.Fatalf("cm360(2.0, 800) = %v, want 25.98", got)
	}
	// тот же прицел при 1600 DPI и сенсе 1.0
	if a, b := aimCm360(2.0, 800), aimCm360(1.0, 1600); a != b {
		t.Fatalf("инвариант cm/360 нарушен: %v vs %v", a, b)
	}
	if aimCm360(0, 800) != 0 || aimCm360(2, 0) != 0 {
		t.Fatal("ноль на входе — ноль на выходе")
	}
}

func okReq() aimSessionRequest {
	a := 89
	raw := true
	return aimSessionRequest{Sens: 2.9, DPI: 800, Cur: 2.0, Lo: 2.63, Hi: 3.19, Conf: "high", Verdict: "rec",
		GainPct: 15.2, Acc1: &a, RtMs: 212, MtMs: 401, Tp: 8.08, Mode: "quick", Rounds: 8, AbRounds: 2, Raw: &raw, Hz: 1000, Lang: "en"}
}

func TestValidateAimSession(t *testing.T) {
	r := okReq()
	if code := validateAimSession(&r); code != "" {
		t.Fatalf("валидный прогон отвергнут: %s", code)
	}
	if r.Coach != "none" {
		t.Fatalf("пустой coach должен стать none, got %q", r.Coach)
	}
	cases := []struct {
		name string
		mut  func(*aimSessionRequest)
		want string
	}{
		{"sens 0", func(r *aimSessionRequest) { r.Sens = 0 }, "bad_sens"},
		{"dpi 50", func(r *aimSessionRequest) { r.DPI = 50 }, "bad_dpi"},
		{"lo>hi", func(r *aimSessionRequest) { r.Lo, r.Hi = 3, 2 }, "bad_range"},
		{"conf слово", func(r *aimSessionRequest) { r.Conf = "высокая" }, "bad_conf"},
		{"verdict", func(r *aimSessionRequest) { r.Verdict = "win" }, "bad_verdict"},
		{"acc1 101", func(r *aimSessionRequest) { v := 101; r.Acc1 = &v }, "bad_acc1"},
		{"rt", func(r *aimSessionRequest) { r.RtMs = 99999 }, "bad_ms"},
		{"tp", func(r *aimSessionRequest) { r.Tp = 77 }, "bad_tp"},
		{"mode", func(r *aimSessionRequest) { r.Mode = "turbo" }, "bad_mode"},
		{"rounds", func(r *aimSessionRequest) { r.Rounds = 61 }, "bad_rounds"},
		{"coach", func(r *aimSessionRequest) { r.Coach = "left" }, "bad_coach"},
	}
	for _, c := range cases {
		r := okReq()
		c.mut(&r)
		if code := validateAimSession(&r); code != c.want {
			t.Errorf("%s: code %q, want %q", c.name, code, c.want)
		}
	}
	// язык вне ru/en/pl — тихо ru, не ошибка
	r = okReq()
	r.Lang = "de"
	if code := validateAimSession(&r); code != "" || r.Lang != "ru" {
		t.Fatalf("lang de: code %q lang %q", code, r.Lang)
	}
}

func TestAimWarmPickAndOut(t *testing.T) {
	now := time.Now()
	rows := []models.AimSession{ // новые первыми
		{Sim: true, Sens: 9, Cm360: 5, Verdict: "rec", CreatedAt: now},                    // стенд — мимо
		{Partial: 4, Sens: 4, Cm360: 13, Verdict: "none", CreatedAt: now.Add(-time.Hour)}, // прерван — мимо
		{Sens: 2.0, Cm360: 26, Cur: 2.2, CurCm: 23.6, Verdict: "cur", DPI: 800, CreatedAt: now.Add(-2 * time.Hour)},
		{Sens: 3.0, Cm360: 17.3, Verdict: "rec", DPI: 800, CreatedAt: now.Add(-3 * time.Hour)},
	}
	w := aimWarmPick(rows)
	if w == nil || w.Sens != 2.0 {
		t.Fatalf("warm должен быть первый настоящий полный прогон, got %+v", w)
	}
	out := aimWarmOut(w)
	// A/B подтвердил текущую → стартуем от неё, а не от рекомендации
	if out["cm360"] != 23.6 || out["sens"] != 2.2 || out["verdict"] != "cur" {
		t.Fatalf("warm cur: %+v", out)
	}
	if aimWarmPick(rows[:2]) != nil {
		t.Fatal("только сим и частичные — warm нет")
	}
	if aimWarmOut(nil) != nil {
		t.Fatal("nil → nil")
	}
	st := aimStats(rows)
	if st["count"] != 2 || st["last_rt"] != 0 || st["first_rt"] != 0 {
		t.Fatalf("stats: %+v", st)
	}
	acc := 90
	rows[2].RtMs, rows[3].RtMs, rows[2].Tp, rows[3].Tp, rows[3].Acc1 = 185, 210, 7.5, 8.1, &acc
	st = aimStats(rows)
	if st["first_rt"] != 210 || st["last_rt"] != 185 || st["best_tp"] != 8.1 || st["best_acc1"] != 90 {
		t.Fatalf("stats «210 → 185»: %+v", st)
	}
}

func TestRankAim(t *testing.T) {
	a80, a95 := 80, 95
	now := time.Now()
	best := []aimBest{
		{UserID: "u1", Tp: 7.5, Acc1: &a80, CreatedAt: now},
		{UserID: "u2", Tp: 8.1, Acc1: nil, CreatedAt: now},
		{UserID: "u3", Tp: 8.1, Acc1: &a95, CreatedAt: now.Add(time.Hour)}, // тот же tp, точнее первый выстрел → выше
		{UserID: "u4", Tp: 6.0, CreatedAt: now},
	}
	top, myRank, myTp := rankAim(best, "u1", 2)
	if len(top) != 2 || top[0].UserID != "u3" || top[1].UserID != "u2" || top[0].Rank != 1 || top[1].Rank != 2 {
		t.Fatalf("top: %+v", top)
	}
	if myRank != 3 || myTp != 7.5 {
		t.Fatalf("me: rank %d tp %v", myRank, myTp)
	}
	if _, r, _ := rankAim(best, "nobody", 10); r != 0 {
		t.Fatalf("чужой — место 0, got %d", r)
	}
	if top[0].IsMe || top[1].IsMe {
		t.Fatal("is_me не должен стоять у чужих")
	}
}
