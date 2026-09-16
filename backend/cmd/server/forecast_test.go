package main

import (
	"testing"
	"time"
)

// Ж5-и1: «хватит до» — учтённые минуты плюс остаток от старта без пауз;
// прошедшее время не обещаем.
func TestForecastUntil(t *testing.T) {
	start := time.Date(2026, 9, 16, 18, 0, 0, 0, time.UTC)
	cases := []struct {
		name      string
		pausedSec int
		billed    int
		left      int
		now       time.Time
		want      time.Time
	}{
		{"42 учтено, ещё на 18 — ровно до 19:00", 0, 42, 18, start.Add(42*time.Minute + 20*time.Second), start.Add(60 * time.Minute)},
		{"пауза 10 мин сдвигает конец", 600, 42, 18, start.Add(52*time.Minute + 20*time.Second), start.Add(70 * time.Minute)},
		{"деньги кончились (грейс нуля) — «сейчас»", 0, 30, 0, start.Add(31*time.Minute + 5*time.Second), start.Add(31*time.Minute + 5*time.Second)},
		{"только посадили — весь запас от старта", 0, 0, 90, start.Add(10 * time.Second), start.Add(90 * time.Minute)},
	}
	for _, tc := range cases {
		got := forecastUntil(start, tc.pausedSec, tc.billed, tc.left, tc.now)
		if got == nil || !got.Equal(tc.want) {
			t.Errorf("%s: forecastUntil = %v, ожидалось %v", tc.name, got, tc.want)
		}
	}
}
