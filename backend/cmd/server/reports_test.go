package main

import (
	"math"
	"testing"
	"time"

	"github.com/google/uuid"

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

// З12-и4а: календарный месяц — прошлый целиком, сравнение с прошлым календарным месяцем целиком; текущий — как «Этот
// месяц» (сравнение — тот же отрезок); будущий и не «ГГГГ-ММ» — ошибка.
func TestMonthPeriod(t *testing.T) {
	now := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		m                                  string
		wantFrom, wantTo, prevFrom, prevTo string
		wantErr                            string
	}{
		{m: "2026-08", wantFrom: "2026-08-01", wantTo: "2026-08-18", prevFrom: "2026-07-01", prevTo: "2026-07-18"},
		{m: "2026-07", wantFrom: "2026-07-01", wantTo: "2026-07-31", prevFrom: "2026-06-01", prevTo: "2026-06-30"},
		{m: "2026-03", wantFrom: "2026-03-01", wantTo: "2026-03-31", prevFrom: "2026-02-01", prevTo: "2026-02-28"},
		{m: "2026-01", wantFrom: "2026-01-01", wantTo: "2026-01-31", prevFrom: "2025-12-01", prevTo: "2025-12-31"},
		{m: "2026-09", wantErr: "bad_month"},
		{m: "2026-13", wantErr: "bad_month"},
		{m: "август", wantErr: "bad_month"},
	}
	for _, tc := range cases {
		p, code := monthPeriod(tc.m, 8, now)
		if code != tc.wantErr {
			t.Errorf("monthPeriod(%q) код = %q, ожидался %q", tc.m, code, tc.wantErr)
			continue
		}
		if tc.wantErr != "" {
			continue
		}
		pr := prevPeriod(p, 8)
		d := func(x time.Time) string { return x.Format("2006-01-02") }
		got := d(p.FromDay) + ".." + d(p.ToDay) + " / " + d(pr.FromDay) + ".." + d(pr.ToDay)
		if want := tc.wantFrom + ".." + tc.wantTo + " / " + tc.prevFrom + ".." + tc.prevTo; got != want {
			t.Errorf("monthPeriod(%q): %s, ожидалось %s", tc.m, got, want)
		}
		if p.From.Hour() != 8 || p.To.Hour() != 8 {
			t.Errorf("%q: окно %s..%s не по клубным суткам", tc.m, p.From, p.To)
		}
	}
	// 1 сентября в 03:00 клубные сутки ещё августовские: сентябрь — будущий, август — текущий, по 31-е
	night := time.Date(2026, 9, 1, 3, 0, 0, 0, time.UTC)
	if _, code := monthPeriod("2026-09", 8, night); code != "bad_month" {
		t.Errorf("ночью 1 сентября «2026-09» — код %q, ожидался bad_month", code)
	}
	if p, code := monthPeriod("2026-08", 8, night); code != "" || p.Preset != "month" || p.ToDay.Format("2006-01-02") != "2026-08-31" {
		t.Errorf("ночью 1 сентября «2026-08» = %s (%s, код %q), ожидался текущий месяц по 2026-08-31", p.ToDay.Format("2006-01-02"), p.Preset, code)
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

// З15-и4 (№30): смена момента в «Персонал → по сменам» — по табелю. Деньги — к смене того, кто принял; иначе — смена по
// графику этого момента, если её люди на месте, а нет — заступившая последней; записи без имени смены не в счёт; конец
// записи — уже не её момент.
// З17-и2 (№13): сессия режется по часам стенных часов клуба, паузы — пропорционально, ночь — в своих клубных сутках.
func TestSliceSession(t *testing.T) {
	waw, err := time.LoadLocation("Europe/Warsaw")
	if err != nil {
		t.Skip("нет tzdata:", err)
	}
	at := func(m time.Month, d, hh, mm int) time.Time { return time.Date(2026, m, d, hh, mm, 0, 0, waw) }
	type piece struct {
		day     int
		hour    int
		minutes float64
	}
	cases := []struct {
		name       string
		start, end time.Time
		played     float64
		want       []piece
	}{
		{"18:00–23:00, 5 часов — по часу в 18–22", at(11, 13, 18, 0), at(11, 13, 23, 0), 300,
			[]piece{{13, 18, 60}, {13, 19, 60}, {13, 20, 60}, {13, 21, 60}, {13, 22, 60}}},
		{"ночь 23:00–02:00 — в сутках начала", at(11, 13, 23, 0), at(11, 14, 2, 0), 180,
			[]piece{{13, 23, 60}, {13, 0, 60}, {13, 1, 60}}},
		{"07:30–08:30 через report_hour: утро — вчерашние сутки, 8:00 — новые", at(11, 14, 7, 30), at(11, 14, 8, 30), 60,
			[]piece{{13, 7, 30}, {14, 8, 30}}},
		{"10:00–12:00 с часом паузы — 60 минут поровну", at(11, 13, 10, 0), at(11, 13, 12, 0), 60,
			[]piece{{13, 10, 30}, {13, 11, 30}}},
		{"18:30–19:15 — 30 и 15", at(11, 13, 18, 30), at(11, 13, 19, 15), 45, []piece{{13, 18, 30}, {13, 19, 15}}},
		{"конца нет — всё в час начала", at(11, 13, 18, 30), at(11, 13, 18, 30), 45, []piece{{13, 18, 45}}},
		{"осенний перевод 25.10: 01:30 летнего — 03:30 зимнего, час «2» дважды", at(10, 25, 1, 30),
			time.Date(2026, 10, 25, 3, 30, 0, 0, waw), 180, []piece{{24, 1, 30}, {24, 2, 60}, {24, 2, 60}, {24, 3, 30}}},
		{"весенний перевод 29.03: 01:30–03:30 — часа «2» нет", at(3, 29, 1, 30), at(3, 29, 3, 30), 60,
			[]piece{{28, 1, 30}, {28, 3, 30}}},
	}
	for _, tc := range cases {
		got := sliceSession(tc.start, tc.end, tc.played, 8)
		ok := len(got) == len(tc.want)
		sum := 0.0
		for i := 0; ok && i < len(got); i++ {
			w := tc.want[i]
			ok = got[i].Day.Day() == w.day && got[i].Hour == w.hour && math.Abs(got[i].Minutes-w.minutes) < 1e-9
		}
		for _, g := range got {
			sum += g.Minutes
		}
		if !ok || math.Abs(sum-tc.played) > 1e-9 {
			t.Errorf("%s: %+v", tc.name, got)
		}
	}
	if sliceSession(at(11, 13, 18, 0), at(11, 13, 19, 0), 0, 8) != nil {
		t.Errorf("сессия без минут — без кусков")
	}
}

// З17-и2: часы суток и «день недели × час» — минуты, сессии шли, ёмкость по сменам; ночная смена понедельника даёт
// понедельнику утро вторника.
func TestLoadByHour(t *testing.T) {
	waw, err := time.LoadLocation("Europe/Warsaw")
	if err != nil {
		t.Skip("нет tzdata:", err)
	}
	mon := time.Date(2026, 11, 16, 0, 0, 0, 0, waw) // понедельник
	p := period{FromDay: mon, ToDay: mon, Days: 1}
	open := openByClubHour([]models.Shift{sh("Ночь", 1200, 480, 1, true)}, p, 8)
	if open[0][20] != 60 || open[0][7] != 60 || open[0][12] != 0 || open[1][7] != 0 {
		t.Fatalf("ёмкость ночной смены понедельника: пн 20:00 %d, пн 7:00 (утро вт) %d, пн 12:00 %d, вт 7:00 %d",
			open[0][20], open[0][7], open[0][12], open[1][7])
	}
	at := func(d, hh int) time.Time { return time.Date(2026, 11, d, hh, 0, 0, 0, waw) }
	hours, heat := loadByHour([]loadSession{
		{Start: at(16, 20), End: at(16, 23), Minutes: 180}, // 20–22
		{Start: at(16, 21), End: at(16, 22), Minutes: 60},  // 21
		{Start: at(17, 1), End: at(17, 3), Minutes: 120},   // 1–2 ночи — понедельник
	}, 8, open, 10)
	if hours[21].Minutes != 120 || hours[21].Sessions != 2 || hours[21].Open != 600 || hours[20].Sessions != 1 ||
		heat[0][1].Minutes != 60 || heat[1][1].Minutes != 0 || hours[12].Open != 0 {
		t.Errorf("21:00 %+v, 20:00 %+v, пн 1:00 %+v, вт 1:00 %+v, 12:00 %+v", hours[21], hours[20], heat[0][1], heat[1][1], hours[12])
	}
	if c := cellOut(hours[21]); c["occupancy_pct"] != 20.0 || c["hours"] != 2.0 {
		t.Errorf("21:00: 120 минут на 10 ПК × 60 — 20 %%, 2 ч: %v", c)
	}
	if c := cellOut(hours[12]); c["occupancy_pct"] != nil {
		t.Errorf("12:00 клуб закрыт — занятости нет: %v", c)
	}
}

// З17-и1 (№7, З-Р6): занятость зоны — от всех её ПК, ремонт — не в ёмкости, нули видны, удалённая зона — без ПК.
func TestLoadZones(t *testing.T) {
	std, vip, tmp := uuid.New(), uuid.New(), uuid.New()
	zones := []models.Zone{{ID: std, Name: "Standard"}, {ID: vip, Name: "VIP"}, {ID: tmp, Name: "Tmp"}}
	pcs := map[string]loadPCs{std.String(): {Total: 9}, vip.String(): {Total: 3, Repair: 1}, "": {Total: 1}}
	vs, ss := vip.String(), std.String()
	use := []loadUse{
		{ZoneID: &vs, Zone: "VIP", Minutes: 360, Cnt: 1},
		{ZoneID: &ss, Zone: "Standard", Minutes: 120, Cnt: 1},
		{Zone: "", Minutes: 60, Cnt: 2},    // ПК без зоны
		{Zone: "Old", Minutes: 30, Cnt: 1}, // зону удалили, имя осталось в снимке
	}
	got := loadZones(zones, pcs, use, 1440)
	type want struct {
		zone     string
		pcs, rep int64
		hours    float64
		sessions int64
		occ      any
		deleted  bool
	}
	wants := []want{
		{"VIP", 3, 1, 6, 1, 12.5, false},     // 360 / (2 ПК не в ремонте × 1440)
		{"Standard", 9, 0, 2, 1, 0.9, false}, // 120 / (9 × 1440)
		{"без зоны", 1, 0, 1, 2, 4.2, false}, // 60 / 1440
		{"Old", 0, 0, 0.5, 1, nil, true},     // ПК нет — делить не на что
		{"Tmp", 0, 0, 0, 0, nil, false},      // зона без ПК и сессий — с нулями
	}
	if len(got) != len(wants) {
		t.Fatalf("строк %d, ждали %d: %v", len(got), len(wants), got)
	}
	for i, w := range wants {
		g := got[i]
		_, del := g["deleted"]
		if g["zone"] != w.zone || g["computers"] != w.pcs || g["repair"] != w.rep || g["hours"] != w.hours ||
			g["sessions"] != w.sessions || g["occupancy_pct"] != w.occ || del != w.deleted {
			t.Errorf("строка %d: %v, ждали %+v", i, g, w)
		}
	}
	if loadZones(zones[:2], map[string]loadPCs{vs: {Total: 3}}, []loadUse{{ZoneID: &vs, Minutes: 360, Cnt: 1}}, 1440)[0]["occupancy_pct"] != 8.3 {
		t.Errorf("6 часов на одном ПК из трёх — 8,3 %%, а не 25 %%")
	}
}

func TestShiftByTimesheet(t *testing.T) {
	base := time.Date(2026, 10, 18, 0, 0, 0, 0, time.UTC)
	at := func(hh, mm int) time.Time { return base.Add(time.Duration(hh*60+mm) * time.Minute) }
	spans := []workSpan{
		{User: "a1", Start: at(8, 0), End: at(20, 10), Shift: "День"},
		{User: "a2", Start: at(17, 56), End: at(23, 59), Shift: "Ночь"}, // пришёл на «Ночь» заранее
		{User: "a3", Start: at(21, 0), End: at(22, 0), Shift: ""},       // пришёл вне смены
	}
	cases := []struct {
		name           string
		at             time.Time
		author, onDuty string
		want           string
	}{
		{"деньги дневного в 20:05 — его «День», хоть по графику уже ночь", at(20, 5), "a1", "Ночь", "День"},
		{"деньги ночного, пришедшего заранее, в 18:30 — его «Ночь»", at(18, 30), "a2", "День", "Ночь"},
		{"сессия в 18:30 — по графику «День», дневной на месте: «День»", at(18, 30), "", "День", "День"},
		{"сессия в 20:05 — по графику «Ночь», ночной на месте: «Ночь»", at(20, 5), "", "Ночь", "Ночь"},
		{"деньги владельца без записи в 12:00 — смена, что работала", at(12, 0), "boss", "День", "День"},
		{"деньги a3 вне смены в 21:30 — смена, что работала", at(21, 30), "a3", "Ночь", "Ночь"},
		{"шаблона в графике нет (удалён), идут обе — заступившая последней", at(18, 30), "", "", "Ночь"},
		{"20:10 у дневного — запись уже кончилась, «Ночь»", at(20, 10), "a1", "Ночь", "Ночь"},
		{"08:00 — начало записи, «День»", at(8, 0), "", "День", "День"},
		{"07:00 — по табелю никого", at(7, 0), "", "Ночь", ""},
	}
	for _, tc := range cases {
		if got := shiftByTimesheet(spans, tc.at, tc.author, tc.onDuty); got != tc.want {
			t.Errorf("%s: got %q, ждали %q", tc.name, got, tc.want)
		}
	}
}
