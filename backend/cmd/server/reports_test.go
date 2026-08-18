package main

import (
	"testing"
	"time"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

func day(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func TestClubDayOf(t *testing.T) {
	cases := []struct {
		name string
		at   time.Time
		want time.Time
	}{
		{"после границы — сегодняшние сутки", time.Date(2026, 8, 18, 9, 0, 0, 0, time.UTC), day(2026, 8, 18)},
		{"ровно на границе — сегодняшние", time.Date(2026, 8, 18, 8, 0, 0, 0, time.UTC), day(2026, 8, 18)},
		{"ночь до границы — вчерашние", time.Date(2026, 8, 18, 3, 30, 0, 0, time.UTC), day(2026, 8, 17)},
		{"полночь — вчерашние", time.Date(2026, 8, 1, 0, 5, 0, 0, time.UTC), day(2026, 7, 31)},
	}
	for _, tc := range cases {
		if got := clubDayOf(tc.at, 8); !got.Equal(tc.want) {
			t.Errorf("%s: clubDayOf = %s, ожидалось %s", tc.name, got.Format("2006-01-02"), tc.want.Format("2006-01-02"))
		}
	}
}

func TestDaysBetweenСквозьПереводЧасов(t *testing.T) {
	loc, err := time.LoadLocation("Europe/Warsaw")
	if err != nil {
		t.Skip("нет tzdata — пропускаем проверку перевода часов")
	}
	// 29 марта 2026 — переход на летнее время: сутки короче на час,
	// деление разницы на 24 часа дало бы 30 дней вместо 31.
	from := time.Date(2026, 3, 1, 0, 0, 0, 0, loc)
	to := time.Date(2026, 3, 31, 0, 0, 0, 0, loc)
	if got := daysBetween(from, to); got != 31 {
		t.Errorf("daysBetween(март) = %d, ожидалось 31", got)
	}
}

func TestResolvePeriodПресеты(t *testing.T) {
	now := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC) // вторник
	cases := []struct {
		preset   string
		from, to string
		wantFrom string
		wantTo   string
		wantDays int
		wantErr  string
	}{
		{preset: "today", wantFrom: "2026-08-18", wantTo: "2026-08-18", wantDays: 1},
		{preset: "yesterday", wantFrom: "2026-08-17", wantTo: "2026-08-17", wantDays: 1},
		{preset: "", wantFrom: "2026-08-12", wantTo: "2026-08-18", wantDays: 7},
		{preset: "d7", wantFrom: "2026-08-12", wantTo: "2026-08-18", wantDays: 7},
		{preset: "d30", wantFrom: "2026-07-20", wantTo: "2026-08-18", wantDays: 30},
		{preset: "d90", wantFrom: "2026-05-21", wantTo: "2026-08-18", wantDays: 90},
		{preset: "month", wantFrom: "2026-08-01", wantTo: "2026-08-18", wantDays: 18},
		{preset: "prev_month", wantFrom: "2026-07-01", wantTo: "2026-07-31", wantDays: 31},
		{preset: "year", wantFrom: "2026-01-01", wantTo: "2026-08-18", wantDays: 230},
		{from: "2026-08-01", to: "2026-08-07", wantFrom: "2026-08-01", wantTo: "2026-08-07", wantDays: 7},
		{from: "2026-08-01", wantFrom: "2026-08-01", wantTo: "2026-08-18", wantDays: 18},
		{from: "вчера", wantErr: "bad_from"},
		{from: "2026-08-01", to: "31.08.2026", wantErr: "bad_to"},
		{from: "2026-08-10", to: "2026-08-01", wantErr: "bad_range"},
		{from: "2020-01-01", to: "2026-08-18", wantErr: "too_long"},
		{preset: "всё время", wantErr: "bad_preset"},
	}
	for _, tc := range cases {
		p, code := resolvePeriod(tc.preset, tc.from, tc.to, 8, now)
		if code != tc.wantErr {
			t.Errorf("resolvePeriod(%q,%q,%q) код = %q, ожидался %q", tc.preset, tc.from, tc.to, code, tc.wantErr)
			continue
		}
		if tc.wantErr != "" {
			continue
		}
		if got := p.FromDay.Format("2006-01-02"); got != tc.wantFrom {
			t.Errorf("%q: from = %s, ожидалось %s", tc.preset+tc.from, got, tc.wantFrom)
		}
		if got := p.ToDay.Format("2006-01-02"); got != tc.wantTo {
			t.Errorf("%q: to = %s, ожидалось %s", tc.preset+tc.from, got, tc.wantTo)
		}
		if p.Days != tc.wantDays {
			t.Errorf("%q: days = %d, ожидалось %d", tc.preset+tc.from, p.Days, tc.wantDays)
		}
		// окно всегда открывается и закрывается на границе клубных суток
		if p.From.Hour() != 8 || p.To.Hour() != 8 {
			t.Errorf("%q: окно %s..%s не по клубным суткам", tc.preset+tc.from, p.From, p.To)
		}
	}
}

func TestResolvePeriodНочьюДоГраницы(t *testing.T) {
	// 03:00 — клубные сутки ещё вчерашние: «сегодня» это 17-е.
	now := time.Date(2026, 8, 18, 3, 0, 0, 0, time.UTC)
	p, code := resolvePeriod("today", "", "", 8, now)
	if code != "" || p.FromDay.Format("2006-01-02") != "2026-08-17" {
		t.Fatalf("ночью «сегодня» = %s (код %q), ожидалось 2026-08-17", p.FromDay.Format("2006-01-02"), code)
	}
	if p.To.Format("2006-01-02 15") != "2026-08-18 08" {
		t.Errorf("окно закрывается %s, ожидалось 2026-08-18 08", p.To.Format("2006-01-02 15"))
	}
}

func TestPrevPeriod(t *testing.T) {
	now := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		preset   string
		from, to string
		wantFrom string
		wantTo   string
	}{
		{preset: "d7", wantFrom: "2026-08-05", wantTo: "2026-08-11"},
		{preset: "today", wantFrom: "2026-08-17", wantTo: "2026-08-17"},
		// календарный месяц сравнивается с тем же отрезком прошлого месяца
		{preset: "month", wantFrom: "2026-07-01", wantTo: "2026-07-18"},
		{preset: "prev_month", wantFrom: "2026-06-01", wantTo: "2026-06-30"},
		{preset: "year", wantFrom: "2025-01-01", wantTo: "2025-08-18"},
		{from: "2026-08-01", to: "2026-08-07", wantFrom: "2026-07-25", wantTo: "2026-07-31"},
	}
	for _, tc := range cases {
		p, code := resolvePeriod(tc.preset, tc.from, tc.to, 8, now)
		if code != "" {
			t.Fatalf("%q: неожиданная ошибка %q", tc.preset, code)
		}
		pr := prevPeriod(p, 8)
		if got := pr.FromDay.Format("2006-01-02"); got != tc.wantFrom {
			t.Errorf("%q: пред. from = %s, ожидалось %s", tc.preset+tc.from, got, tc.wantFrom)
		}
		if got := pr.ToDay.Format("2006-01-02"); got != tc.wantTo {
			t.Errorf("%q: пред. to = %s, ожидалось %s", tc.preset+tc.from, got, tc.wantTo)
		}
	}
}

func TestPrevPeriodМесяцНеВылезаетЗаКрай(t *testing.T) {
	// 31 марта: у февраля столько дней нет — прошлый период обрезается концом месяца.
	now := time.Date(2026, 3, 31, 12, 0, 0, 0, time.UTC)
	p, _ := resolvePeriod("month", "", "", 8, now)
	pr := prevPeriod(p, 8)
	if got := pr.ToDay.Format("2006-01-02"); got != "2026-02-28" {
		t.Errorf("пред. период марта заканчивается %s, ожидалось 2026-02-28", got)
	}
}

func sh(name string, start, end, mask int, active bool) models.Shift {
	return models.Shift{Name: name, StartMin: start, EndMin: end, DaysMask: mask, Active: active}
}

func TestOpenMinutesPerDay(t *testing.T) {
	monday := day(2026, 8, 17)  // понедельник
	tuesday := day(2026, 8, 18) // вторник
	cases := []struct {
		name   string
		shifts []models.Shift
		at     time.Time
		want   int
	}{
		{"смен нет — считаем круглосуточно", nil, monday, 1440},
		{"все смены выключены — круглосуточно",
			[]models.Shift{sh("День", 480, 1200, 127, false)}, monday, 1440},
		{"дневная 8:00–20:00", []models.Shift{sh("День", 480, 1200, 127, true)}, monday, 720},
		{"день + ночь = сутки",
			[]models.Shift{sh("День", 480, 1200, 127, true), sh("Ночь", 1200, 480, 127, true)}, tuesday, 1440},
		{"пересечение смен не удваивается",
			[]models.Shift{sh("A", 480, 1200, 127, true), sh("B", 600, 1300, 127, true)}, monday, 820},
		{"смена только по понедельникам: во вторник клуб закрыт",
			[]models.Shift{sh("День", 480, 1200, 1, true)}, tuesday, 0},
		{"ночная понедельника даёт вторнику утренний хвост",
			[]models.Shift{sh("Ночь", 1200, 480, 1, true)}, tuesday, 480},
		{"ночная понедельника: сам понедельник — вечерняя половина",
			[]models.Shift{sh("Ночь", 1200, 480, 1, true)}, monday, 240},
	}
	for _, tc := range cases {
		if got := openMinutesPerDay(tc.shifts, tc.at); got != tc.want {
			t.Errorf("%s: openMinutesPerDay = %d, ожидалось %d", tc.name, got, tc.want)
		}
	}
}

func TestOpenMinutesInPeriod(t *testing.T) {
	now := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)
	p, _ := resolvePeriod("d7", "", "", 8, now)
	shifts := []models.Shift{sh("День", 480, 1200, 127, true)}
	if got := openMinutesInPeriod(shifts, p); got != 7*720 {
		t.Errorf("openMinutesInPeriod = %d, ожидалось %d", got, 7*720)
	}
}

func TestPctDelta(t *testing.T) {
	cases := []struct {
		now, prev float64
		want      *float64
	}{
		{120, 100, ptrF(20)},
		{50, 100, ptrF(-50)},
		{0, 0, ptrF(0)},
		{100, 0, nil}, // рост с нуля — процента нет
	}
	for _, tc := range cases {
		got := pctDelta(tc.now, tc.prev)
		if (got == nil) != (tc.want == nil) {
			t.Errorf("pctDelta(%v,%v) = %v, ожидалось %v", tc.now, tc.prev, got, tc.want)
			continue
		}
		if got != nil && *got != *tc.want {
			t.Errorf("pctDelta(%v,%v) = %v, ожидалось %v", tc.now, tc.prev, *got, *tc.want)
		}
	}
}

func ptrF(v float64) *float64 { return &v }

func TestFillDaysБезДыр(t *testing.T) {
	now := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)
	p, _ := resolvePeriod("d7", "", "", 8, now)
	out := fillDays(p, map[string][2]float64{"2026-08-14": {300, 3}})
	if len(out) != 7 {
		t.Fatalf("дней в ряду %d, ожидалось 7", len(out))
	}
	if out[0]["date"] != "2026-08-12" || out[6]["date"] != "2026-08-18" {
		t.Errorf("край ряда: %v … %v", out[0]["date"], out[6]["date"])
	}
	if out[2]["value"].(float64) != 300 {
		t.Errorf("день с данными потерялся: %v", out[2])
	}
	if out[3]["value"].(float64) != 0 {
		t.Errorf("пустой день должен быть нулём, а не дырой: %v", out[3])
	}
}
