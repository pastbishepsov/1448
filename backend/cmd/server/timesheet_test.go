package main

import (
	"testing"
	"time"
)

func TestWorkMinutes(t *testing.T) {
	base := time.Date(2026, 8, 18, 20, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		end  time.Time
		want int
	}{
		{"обычная ночная смена", base.Add(12 * time.Hour), 720},
		{"полчаса", base.Add(30 * time.Minute), 30},
		{"секунды не считаем за минуту", base.Add(59 * time.Second), 0},
		{"конец раньше начала — ноль, а не минус", base.Add(-2 * time.Hour), 0},
	}
	for _, tc := range cases {
		if got := workMinutes(base, tc.end); got != tc.want {
			t.Errorf("%s: workMinutes = %d, ожидалось %d", tc.name, got, tc.want)
		}
	}
}

func TestShiftEndAt(t *testing.T) {
	day := time.Date(2026, 8, 18, 0, 0, 0, 0, time.UTC)
	// дневная 8:00–20:00 заканчивается в тот же день
	if got := shiftEndAt(day, 480, 1200); got.Format("2006-01-02 15:04") != "2026-08-18 20:00" {
		t.Errorf("дневная кончается %s, ожидалось 2026-08-18 20:00", got.Format("2006-01-02 15:04"))
	}
	// ночная 20:00–8:00 — уже на следующие сутки
	if got := shiftEndAt(day, 1200, 480); got.Format("2006-01-02 15:04") != "2026-08-19 08:00" {
		t.Errorf("ночная кончается %s, ожидалось 2026-08-19 08:00", got.Format("2006-01-02 15:04"))
	}
}

func TestParseHM(t *testing.T) {
	cases := []struct {
		in   string
		want int
		ok   bool
	}{
		{"20:30", 1230, true},
		{"00:00", 0, true},
		{" 8:05 ", 485, true},
		{"23:59", 1439, true},
		{"24:00", 0, false},
		{"20:60", 0, false},
		{"20", 0, false},
		{"", 0, false},
	}
	for _, tc := range cases {
		got, ok := parseHM(tc.in)
		if ok != tc.ok || (ok && got != tc.want) {
			t.Errorf("parseHM(%q) = (%d, %v), ожидалось (%d, %v)", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

func TestComposeWorkTimes(t *testing.T) {
	day := time.Date(2026, 8, 18, 0, 0, 0, 0, time.UTC)
	// через полночь: конец уезжает на следующие сутки
	start, end := composeWorkTimes(day, 1200, 480)
	if start.Format("15:04") != "20:00" || end.Format("2006-01-02 15:04") != "2026-08-19 08:00" {
		t.Errorf("ночная: %s … %s", start.Format("2006-01-02 15:04"), end.Format("2006-01-02 15:04"))
	}
	if workMinutes(start, end) != 720 {
		t.Errorf("ночная длится %d минут, ожидалось 720", workMinutes(start, end))
	}
	// дневная остаётся в сутках
	start, end = composeWorkTimes(day, 480, 1200)
	if end.Format("2006-01-02") != "2026-08-18" {
		t.Errorf("дневная уехала на %s", end.Format("2006-01-02"))
	}
}

func TestPayoutFor(t *testing.T) {
	cases := []struct {
		name     string
		rateType string
		rate     float64
		mins     int64
		shifts   int64
		want     float64
		kind     string
	}{
		{"почасовая", "hour", 32.5, 690, 1, 373.75, "hour"},
		{"за смену", "shift", 220, 1380, 2, 440, "shift"},
		{"оклад не делим по периоду", "month", 6500, 9000, 15, 6500, "month"},
		{"ставки нет — не считаем", "none", 0, 690, 1, 0, ""},
	}
	for _, tc := range cases {
		amount, kind := payoutFor(tc.rateType, tc.rate, tc.mins, tc.shifts)
		if amount != tc.want || kind != tc.kind {
			t.Errorf("%s: payoutFor = (%v, %q), ожидалось (%v, %q)", tc.name, amount, kind, tc.want, tc.kind)
		}
	}
}
