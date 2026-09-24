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

// З4-и2: окно смены — по стенным часам клуба, как у отчётов и шапки
// (makePeriod). «Полночь + 8 ч» в дни перевода часов — это 07:00 (25.10) или
// 09:00 (28.03): сводка смены жила бы в другом окне, чем отчёт «Деньги», а
// 25.10 в 07:30 «сейчас» не попадало бы ни в одну смену.
func TestShiftWindowПереводЧасов(t *testing.T) {
	loc, err := time.LoadLocation("Europe/Warsaw")
	if err != nil {
		t.Skip("нет tzdata — пропускаем проверку перевода часов")
	}
	for _, date := range []string{"2026-10-24", "2026-10-25", "2027-03-27", "2027-03-28"} {
		day, _ := time.ParseInLocation("2006-01-02", date, loc)
		from, to, key, ok := shiftWindow(date, 8, day)
		p := makePeriod(day, day, 8, "")
		if !ok || key != date || !from.Equal(p.From) || !to.Equal(p.To) || from.Hour() != 8 || to.In(loc).Hour() != 8 {
			t.Errorf("%s: окно [%v, %v), ожидалось [%v, %v) — с 08:00 до 08:00 по часам клуба", date, from, to, p.From, p.To)
		}
	}
	now := time.Date(2026, 10, 25, 7, 30, 0, 0, loc) // уже зимнее время, до границы суток
	from, to, key, _ := shiftWindow("", 8, now)
	if key != "2026-10-24" || now.Before(from) || !now.Before(to) {
		t.Errorf("25.10 07:30: смена %s, окно [%v, %v) — «сейчас» должно быть внутри смены 24.10", key, from, to)
	}
}
