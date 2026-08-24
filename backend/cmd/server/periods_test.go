package main

// Юнит-тесты фирменного резета 14:48 (Г5, Р6 GUEST.md): границы суток,
// перелив недели и месяца — всё по клубному времени (Europe/Warsaw).

import (
	"testing"
	"time"
)

func wsaw(y int, m time.Month, d, hh, mm int) time.Time {
	return time.Date(y, m, d, hh, mm, 0, 0, clubLocation)
}

func TestAchDayBoundary1448(t *testing.T) {
	// 24 августа, 14:47 — ещё сутки «23-го»; 14:48 — уже «24-го».
	if k := achDayKey(wsaw(2026, 8, 24, 14, 47)); k != "2026-08-23" {
		t.Errorf("14:47: ждали сутки 2026-08-23, получили %s", k)
	}
	if k := achDayKey(wsaw(2026, 8, 24, 14, 48)); k != "2026-08-24" {
		t.Errorf("14:48: ждали сутки 2026-08-24, получили %s", k)
	}
	// раннее утро принадлежит вчерашним суткам
	if k := achDayKey(wsaw(2026, 8, 24, 3, 0)); k != "2026-08-23" {
		t.Errorf("03:00: ждали 2026-08-23, получили %s", k)
	}
}

func TestPeriodKeys(t *testing.T) {
	now := wsaw(2026, 8, 24, 15, 0) // сутки 24-го (понедельник)
	if k := periodKeyFor("daily", now); k == nil || *k != "2026-08-24" {
		t.Errorf("daily: %v", k)
	}
	if k := periodKeyFor("weekly", now); k == nil || *k != "2026-W35" {
		t.Errorf("weekly: ждали 2026-W35, получили %v", *k)
	}
	if k := periodKeyFor("monthly", now); k == nil || *k != "2026-08" {
		t.Errorf("monthly: %v", *k)
	}
	if k := periodKeyFor("lifetime", now); k != nil {
		t.Errorf("lifetime без ключа, получили %v", *k)
	}
}

func TestWeekRolloverAt1448(t *testing.T) {
	// Понедельник 24.08, 14:00 — ачивочные сутки ещё воскресные (23.08),
	// значит и неделя прошлая; в 14:48 неделя перещёлкивается.
	before := periodKeyFor("weekly", wsaw(2026, 8, 24, 14, 0))
	after := periodKeyFor("weekly", wsaw(2026, 8, 24, 14, 48))
	if *before == *after {
		t.Errorf("неделя должна перещёлкнуться в 14:48 понедельника: %s vs %s", *before, *after)
	}
	if *before != "2026-W34" || *after != "2026-W35" {
		t.Errorf("ждали W34→W35, получили %s→%s", *before, *after)
	}
}

func TestMonthRolloverAt1448(t *testing.T) {
	// 1 сентября до 14:48 — ещё август.
	if k := periodKeyFor("monthly", wsaw(2026, 9, 1, 10, 0)); *k != "2026-08" {
		t.Errorf("01.09 10:00: ждали 2026-08, получили %s", *k)
	}
	if k := periodKeyFor("monthly", wsaw(2026, 9, 1, 14, 48)); *k != "2026-09" {
		t.Errorf("01.09 14:48: ждали 2026-09, получили %s", *k)
	}
}

func TestNextAchReset(t *testing.T) {
	now := wsaw(2026, 8, 24, 15, 0)
	want := wsaw(2026, 8, 25, 14, 48)
	if got := nextAchReset(now); !got.Equal(want) {
		t.Errorf("резет: ждали %v, получили %v", want, got)
	}
	// до 14:48 резет — сегодня же
	now2 := wsaw(2026, 8, 24, 10, 0)
	want2 := wsaw(2026, 8, 24, 14, 48)
	if got := nextAchReset(now2); !got.Equal(want2) {
		t.Errorf("резет утром: ждали %v, получили %v", want2, got)
	}
}

func TestAchWeekDayKeys(t *testing.T) {
	keys := achWeekDayKeys(wsaw(2026, 8, 26, 16, 0)) // среда 26-го, сутки 26-го
	if keys[0] != "2026-08-24" || keys[6] != "2026-08-30" || len(keys) != 7 {
		t.Errorf("неделя пн–вс: %v", keys)
	}
}
