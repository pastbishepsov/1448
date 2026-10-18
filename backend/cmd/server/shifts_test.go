package main

import (
	"testing"
	"time"

	"github.com/pastbishepsov/1448/backend/internal/models"
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

// З15-и1 (№22; З-Р16): пересечение шаблонов — хоть в один день недели; ночь захватывает утро следующего дня, стык — не
// пересечение.
func TestShiftsOverlap(t *testing.T) {
	const all, mon, tue, sun = 127, 1, 2, 64
	cases := []struct {
		name               string
		as, ae, am, bs, be int
		bm                 int
		want               bool
	}{
		{"День 08–20 и Ночь 20–08 — стык, не пересечение", 480, 1200, all, 1200, 480, all, false},
		{"День 08–20 и Вечер 15–23", 480, 1200, all, 900, 1380, all, true},
		{"Ночь 20–08 и утро 07–09", 1200, 480, all, 420, 540, all, true},
		{"одно время, разные дни", 480, 1200, mon, 480, 1200, tue, false},
		{"ночь воскресенья — утро понедельника", 1200, 480, sun, 360, 600, mon, true},
		{"ночь понедельника — не утро понедельника", 1200, 480, mon, 360, 600, mon, false},
		{"вложенная", 600, 720, all, 480, 1200, all, true},
	}
	for _, tc := range cases {
		if got := shiftsOverlap(tc.as, tc.ae, tc.am, tc.bs, tc.be, tc.bm); got != tc.want {
			t.Errorf("%s: got %v, ждали %v", tc.name, got, tc.want)
		}
		if got := shiftsOverlap(tc.bs, tc.be, tc.bm, tc.as, tc.ae, tc.am); got != tc.want {
			t.Errorf("%s (наоборот): got %v, ждали %v", tc.name, got, tc.want)
		}
	}
}

func TestOverlapText(t *testing.T) {
	day := models.Shift{Name: "День", StartMin: 480, EndMin: 1200}
	night := models.Shift{Name: "Ночь", StartMin: 1200, EndMin: 480}
	if got := overlapText([]models.Shift{day}); got != "со сменой «День» 08:00–20:00" {
		t.Errorf("одна: %q", got)
	}
	if got := overlapText([]models.Shift{day, night}); got != "со сменами «День» 08:00–20:00, «Ночь» 20:00–08:00" {
		t.Errorf("две: %q", got)
	}
}

// З15-и1: смена при приходе — по назначению: та, что идёт сейчас (из нескольких — начавшаяся последней), иначе ближайшая,
// что начнётся не позже чем через early.
func TestPickAssigned(t *testing.T) {
	loc := time.FixedZone("club", 2*3600)
	d := time.Date(2026, 10, 18, 0, 0, 0, 0, loc)
	at := func(hh, mm int) time.Time { return d.Add(time.Duration(hh*60+mm) * time.Minute) }
	nS, nE := shiftSpan(d.AddDate(0, 0, -1), 1200, 480) // ночь вчерашнего дня: 20:00 – 08:00
	dS, dE := shiftSpan(d, 480, 1200)                   // день: 08:00 – 20:00
	if !nS.Equal(at(-4, 0)) || !nE.Equal(at(8, 0)) || !dS.Equal(at(8, 0)) || !dE.Equal(at(20, 0)) {
		t.Fatalf("окна: ночь %v–%v, день %v–%v", nS, nE, dS, dE)
	}
	night := assignedWindow{Day: d.AddDate(0, 0, -1), Start: nS, End: nE}
	day := assignedWindow{Day: d, Start: dS, End: dE}
	cases := []struct {
		name string
		c    []assignedWindow
		t    time.Time
		want int
	}{
		{"назначен на день, пришёл в 07:50 — его день", []assignedWindow{day}, at(7, 50), 0},
		{"назначен на ночь и день, в 07:50 — ещё ночь", []assignedWindow{night, day}, at(7, 50), 0},
		{"назначен на ночь и день, в 08:01 — уже день", []assignedWindow{night, day}, at(8, 1), 1},
		{"назначен на день, пришёл в 04:00 — рано (больше трёх часов)", []assignedWindow{day}, at(4, 0), -1},
		{"назначен на день, пришёл в 05:00 — ровно три часа", []assignedWindow{day}, at(5, 0), 0},
		{"назначен на день, пришёл в 21:00 — день кончился", []assignedWindow{day}, at(21, 0), -1},
		{"назначений нет", nil, at(12, 0), -1},
	}
	for _, tc := range cases {
		if got := pickAssigned(tc.c, tc.t, 3*time.Hour); got != tc.want {
			t.Errorf("%s: got %d, ждали %d", tc.name, got, tc.want)
		}
	}
}
