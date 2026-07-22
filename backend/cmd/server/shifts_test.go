package main

import (
	"testing"
	"time"
)

func TestValidateShiftTemplate(t *testing.T) {
	cases := []struct {
		name  string
		nm    string
		start int
		end   int
		mask  int
		ok    bool
		code  string
	}{
		{"дневная валидна", "День", 480, 1200, 127, true, ""},
		{"ночная через полночь валидна", "Ночь", 1200, 480, 127, true, ""},
		{"пустое имя", "  ", 480, 1200, 127, false, "bad_name"},
		{"минуты за границей", "День", -1, 1200, 127, false, "bad_time"},
		{"конец за границей", "День", 480, 1440, 127, false, "bad_time"},
		{"нулевая длина", "Пшик", 480, 480, 127, false, "zero_length"},
		{"пустая маска дней", "День", 480, 1200, 0, false, "bad_days"},
		{"маска шире недели", "День", 480, 1200, 128, false, "bad_days"},
		{"только выходные — валидно", "Уикенд", 600, 1320, 96, true, ""},
	}
	for _, tc := range cases {
		ok, code := validateShiftTemplate(tc.nm, tc.start, tc.end, tc.mask)
		if ok != tc.ok || code != tc.code {
			t.Errorf("%s: validateShiftTemplate = (%v, %q), ожидалось (%v, %q)", tc.name, ok, code, tc.ok, tc.code)
		}
	}
}

func TestShiftActiveAt(t *testing.T) {
	loc := time.Local
	// 2026-07-24 — пятница; 2026-07-25 — суббота.
	at := func(y int, m time.Month, d, h, min int) time.Time { return time.Date(y, m, d, h, min, 0, 0, loc) }
	day := func(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, loc) }
	const allDays, friOnly, satOnly = 127, 1 << 4, 1 << 5
	cases := []struct {
		name    string
		start   int
		end     int
		mask    int
		t       time.Time
		active  bool
		shiftDy time.Time
	}{
		{"дневная в разгаре", 480, 1200, allDays, at(2026, 7, 24, 12, 0), true, day(2026, 7, 24)},
		{"дневная до начала", 480, 1200, allDays, at(2026, 7, 24, 7, 59), false, time.Time{}},
		{"дневная после конца", 480, 1200, allDays, at(2026, 7, 24, 20, 0), false, time.Time{}},
		{"ночная вечером — сегодняшняя", 1200, 480, allDays, at(2026, 7, 24, 23, 0), true, day(2026, 7, 24)},
		{"ночная утром — вчерашняя", 1200, 480, allDays, at(2026, 7, 25, 3, 0), true, day(2026, 7, 24)},
		{"пятничная ночная утром субботы жива", 1200, 480, friOnly, at(2026, 7, 25, 3, 0), true, day(2026, 7, 24)},
		{"пятничная ночная в субботу вечером мертва", 1200, 480, friOnly, at(2026, 7, 25, 23, 0), false, time.Time{}},
		{"субботняя дневная в пятницу выключена", 480, 1200, satOnly, at(2026, 7, 24, 12, 0), false, time.Time{}},
	}
	for _, tc := range cases {
		active, sd := shiftActiveAt(tc.start, tc.end, tc.mask, tc.t)
		if active != tc.active || (active && !sd.Equal(tc.shiftDy)) {
			t.Errorf("%s: shiftActiveAt = (%v, %v), ожидалось (%v, %v)", tc.name, active, sd, tc.active, tc.shiftDy)
		}
	}
}
