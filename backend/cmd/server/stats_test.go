package main

import (
	"testing"
	"time"
)

func TestShiftWindow(t *testing.T) {
	loc := time.Local
	day := func(y int, m time.Month, d, h int) time.Time { return time.Date(y, m, d, h, 0, 0, 0, loc) }
	cases := []struct {
		name    string
		date    string
		hour    int
		now     time.Time
		wantKey string
		wantOK  bool
		from    time.Time
	}{
		{"явная дата", "2026-07-20", 8, day(2026, 7, 21, 12), "2026-07-20", true, day(2026, 7, 20, 8)},
		{"пустая дата днём — сегодняшняя смена", "", 8, day(2026, 7, 21, 12), "2026-07-21", true, day(2026, 7, 21, 8)},
		{"пустая дата ночью — ещё вчерашняя смена", "", 8, day(2026, 7, 21, 3), "2026-07-20", true, day(2026, 7, 20, 8)},
		{"ровно на границе — уже новая смена", "", 8, day(2026, 7, 21, 8), "2026-07-21", true, day(2026, 7, 21, 8)},
		{"граница в полночь", "", 0, day(2026, 7, 21, 0), "2026-07-21", true, day(2026, 7, 21, 0)},
		{"кривая дата", "21.07.2026", 8, day(2026, 7, 21, 12), "", false, time.Time{}},
	}
	for _, tc := range cases {
		from, to, key, ok := shiftWindow(tc.date, tc.hour, tc.now)
		if ok != tc.wantOK || key != tc.wantKey {
			t.Errorf("%s: shiftWindow = (key %q, ok %v), ожидалось (%q, %v)", tc.name, key, ok, tc.wantKey, tc.wantOK)
			continue
		}
		if !ok {
			continue
		}
		if !from.Equal(tc.from) || !to.Equal(tc.from.Add(24*time.Hour)) {
			t.Errorf("%s: окно [%v, %v), ожидалось [%v, %v)", tc.name, from, to, tc.from, tc.from.Add(24*time.Hour))
		}
	}
}
