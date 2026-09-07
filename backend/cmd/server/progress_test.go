package main

// К5 «Живой прогресс» (KIOSK.md): раскладка сессии по границе 14:48 и
// частота живой проверки ачивок в тике биллинга.

import "testing"

func TestSessionDayPieces(t *testing.T) {
	// сессия 13:00→16:00 (180 мин) через резет 14:48 = 108 мин вчера + 72 сегодня
	p := sessionDayPieces(180, 150, wsaw(2026, 9, 7, 13, 0), wsaw(2026, 9, 7, 16, 0))
	if len(p) != 2 {
		t.Fatalf("ждали 2 куска, получили %+v", p)
	}
	if p[0].DayKey != "2026-09-06" || p[0].Minutes != 108 || p[1].DayKey != "2026-09-07" || p[1].Minutes != 72 {
		t.Errorf("13:00→16:00: %+v", p)
	}
	if p[0].ActiveMinutes+p[1].ActiveMinutes != 150 || p[0].ActiveMinutes != 90 {
		t.Errorf("активные минуты делятся той же долей (90+60): %+v", p)
	}
	if !p[0].FirstAt.Equal(wsaw(2026, 9, 7, 13, 0)) || !p[1].FirstAt.Equal(wsaw(2026, 9, 7, 14, 48)) {
		t.Errorf("first_at: старт сессии и резет: %+v", p)
	}

	// целиком внутри одних суток — один кусок, сутки СТАРТА (утро = вчерашние)
	p = sessionDayPieces(60, 60, wsaw(2026, 9, 7, 3, 0), wsaw(2026, 9, 7, 4, 0))
	if len(p) != 1 || p[0].DayKey != "2026-09-06" || p[0].Minutes != 60 || p[0].ActiveMinutes != 60 {
		t.Errorf("03:00→04:00: %+v", p)
	}
	if !p[0].Night {
		t.Error("старт в 03:00 — ночная сессия")
	}

	// ночной флаг — только у первого куска
	p = sessionDayPieces(1000, 1000, wsaw(2026, 9, 6, 23, 0), wsaw(2026, 9, 7, 15, 40))
	if len(p) != 2 || !p[0].Night || p[1].Night {
		t.Errorf("ночь только у куска старта: %+v", p)
	}
	if p[0].Minutes+p[1].Minutes != 1000 {
		t.Errorf("сумма минут сохраняется: %+v", p)
	}

	// закончилась ровно в 14:48 — второго куска нет
	p = sessionDayPieces(108, 100, wsaw(2026, 9, 7, 13, 0), wsaw(2026, 9, 7, 14, 48))
	if len(p) != 1 || p[0].DayKey != "2026-09-06" || p[0].Minutes != 108 {
		t.Errorf("до 14:48 включительно — одни сутки: %+v", p)
	}
	// началась ровно в 14:48 — уже сегодняшние
	p = sessionDayPieces(30, 30, wsaw(2026, 9, 7, 14, 48), wsaw(2026, 9, 7, 15, 18))
	if len(p) != 1 || p[0].DayKey != "2026-09-07" {
		t.Errorf("старт в 14:48 — сегодняшние сутки: %+v", p)
	}

	// дев-оверрайд: 500 «минут» за минуту реального времени — один кусок, без деления
	p = sessionDayPieces(500, 500, wsaw(2026, 9, 7, 15, 0), wsaw(2026, 9, 7, 15, 1))
	if len(p) != 1 || p[0].Minutes != 500 || p[0].ActiveMinutes != 500 {
		t.Errorf("оверрайд: %+v", p)
	}
	// ноль минут (сессия короче минуты) — один кусок с нулём, не паника
	p = sessionDayPieces(0, 0, wsaw(2026, 9, 7, 15, 0), wsaw(2026, 9, 7, 15, 0))
	if len(p) != 1 || p[0].Minutes != 0 {
		t.Errorf("ноль: %+v", p)
	}
	// двое суток подряд (сессия > 24 ч) — три куска, сумма сходится
	p = sessionDayPieces(1500, 1200, wsaw(2026, 9, 6, 10, 0), wsaw(2026, 9, 7, 11, 0))
	if len(p) != 2 { // 10:00 6-го → 14:48 6-го → 11:00 7-го: границ одна (14:48 6-го)
		t.Fatalf("ждали 2 куска: %+v", p)
	}
	p = sessionDayPieces(1500, 1200, wsaw(2026, 9, 6, 10, 0), wsaw(2026, 9, 7, 15, 0))
	sum, act := 0, 0
	for _, x := range p {
		sum += x.Minutes
		act += x.ActiveMinutes
	}
	if len(p) != 3 || sum != 1500 || act != 1200 || p[0].DayKey != "2026-09-05" || p[1].DayKey != "2026-09-06" || p[2].DayKey != "2026-09-07" {
		t.Errorf("29 часов через два резета: %+v", p)
	}
	// активных не больше минут в каждом куске
	for _, x := range p {
		if x.ActiveMinutes > x.Minutes {
			t.Errorf("active > minutes: %+v", x)
		}
	}
}

func TestCrossedEvery(t *testing.T) {
	cases := []struct {
		prev, cur int
		want      bool
	}{
		{4, 5, true}, {5, 6, false}, {59, 60, true}, {0, 1, false}, {3, 3, false},
		{0, 7, true},    // догон после рестарта — одна проверка
		{9, 15, true},   // пересечено две границы — всё равно одна
		{10, 14, false}, // между границами
	}
	for _, c := range cases {
		if got := crossedEvery(c.prev, c.cur, achLiveCheckMin); got != c.want {
			t.Errorf("crossedEvery(%d,%d) = %v", c.prev, c.cur, got)
		}
	}
	if crossedEvery(4, 5, 0) {
		t.Error("шаг 0 — никогда")
	}
}

func TestConditionUnit(t *testing.T) {
	for ct, want := range map[string]string{
		"active_minutes_today": "min", "minutes_week": "min", "active_minutes_month": "min",
		"hours_played": "hours", "visit_streak": "days",
		"kitchen_today": "count", "visits_week": "count", "night_session_today": "count", "profile_name": "count",
	} {
		if got := conditionUnit(ct); got != want {
			t.Errorf("conditionUnit(%s) = %s, ожидалось %s", ct, got, want)
		}
	}
}
