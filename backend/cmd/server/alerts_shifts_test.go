package main

import (
	"testing"
	"time"
)

// З22-и4: когда напоминать о смене и как сказать, когда она.

func TestShiftSoon(t *testing.T) {
	now := time.Date(2026, 11, 10, 18, 0, 0, 0, time.UTC)
	at := func(h, m int) time.Time { return time.Date(2026, 11, 10, h, m, 0, 0, time.UTC) }
	cases := []struct {
		name  string
		start time.Time
		hours int64
		want  bool
	}{
		{"через 2 ч при пороге 2 — пора", at(20, 0), 2, true},
		{"через 2 ч 1 мин — рано", at(20, 1), 2, false},
		{"через 15 мин — пора", at(18, 15), 2, true},
		{"уже началась — не напоминаем", at(18, 0), 2, false},
		{"порог 0 — напоминания выключены", at(19, 0), 0, false},
	}
	for _, tc := range cases {
		if got := shiftSoon(tc.start, now, tc.hours); got != tc.want {
			t.Errorf("%s: %v, ожидалось %v", tc.name, got, tc.want)
		}
	}
}

func TestUntilText(t *testing.T) {
	now := time.Date(2026, 11, 10, 18, 0, 0, 0, time.UTC)
	cases := map[time.Duration]string{
		40 * time.Minute:                 "через 40 мин",
		2 * time.Hour:                    "через 2 ч",
		105 * time.Minute:                "через 1 ч 45 мин",
		104*time.Minute + 30*time.Second: "через 1 ч 45 мин", // минуты — вверх
	}
	for d, want := range cases {
		if got := untilText(now.Add(d), now); got != want {
			t.Errorf("%v: %q, ожидалось %q", d, got, want)
		}
	}
	today := time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)
	if dayWord(today, today) != "сегодня" || dayWord(today.AddDate(0, 0, 1), today) != "завтра" ||
		dayWord(today.AddDate(0, 0, 2), today) != "12.11" {
		t.Error("dayWord: сегодня, завтра, дальше — дата")
	}
}
