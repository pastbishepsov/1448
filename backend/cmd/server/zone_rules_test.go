package main

import (
	"testing"
	"time"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

// З24-и1: расписание цен зоны — дни словами, проверка правила, цена в момент, следующая смена цены, наложение правил.

func wallT(day string, hhmm string) time.Time {
	t, err := time.ParseInLocation("2006-01-02 15:04", day+" "+hhmm, clubLocation)
	if err != nil {
		panic(err)
	}
	return t
}

func dayPtr(s string) *time.Time {
	t, _ := ruleDate(s)
	return t
}

const (
	monFri = 1 | 2 | 4 | 8 | 16
	satSun = 32 | 64
	friday = 16
)

func TestDaysMaskText(t *testing.T) {
	cases := map[int]string{127: "каждый день", monFri: "Пн–Пт", satSun: "Сб, Вс", 1 | 2 | 4 | 16: "Пн–Ср, Пт", 1: "Пн",
		1 | 4 | 16 | 64: "Пн, Ср, Пт, Вс", 2 | 4: "Вт, Ср"}
	for mask, want := range cases {
		if got := daysMaskText(mask); got != want {
			t.Errorf("daysMaskText(%d) = %q, ожидалось %q", mask, got, want)
		}
	}
	if ruleClock(1440) != "24:00" || ruleClock(0) != "00:00" || ruleClock(1080) != "18:00" {
		t.Error("ruleClock: 1440 — 24:00, 0 — 00:00, 1080 — 18:00")
	}
	r := models.ZonePriceRule{DaysMask: monFri, StartMin: 1080, EndMin: 1320, RatePLN: 20, ValidFrom: dayPtr("2026-12-01"), ValidTo: dayPtr("2026-12-31")}
	if got := zoneRuleText(r); got != "Пн–Пт 18:00–22:00 — 20,00 zł/ч, с 01.12.2026 по 31.12.2026" {
		t.Errorf("правило словами: %q", got)
	}
}

func TestValidateZoneRule(t *testing.T) {
	ok := models.ZonePriceRule{DaysMask: monFri, StartMin: 1080, EndMin: 1320, RatePLN: 20}
	cases := []struct {
		name string
		edit func(r *models.ZonePriceRule)
		want string
	}{
		{"вечер будней", func(r *models.ZonePriceRule) {}, ""},
		{"через полночь", func(r *models.ZonePriceRule) { r.StartMin, r.EndMin = 1320, 360 }, ""},
		{"весь день", func(r *models.ZonePriceRule) { r.StartMin, r.EndMin = 0, 1440 }, ""},
		{"без дней", func(r *models.ZonePriceRule) { r.DaysMask = 0 }, "bad_days"},
		{"дни больше недели", func(r *models.ZonePriceRule) { r.DaysMask = 128 }, "bad_days"},
		{"начало 24:00", func(r *models.ZonePriceRule) { r.StartMin = 1440 }, "bad_time"},
		{"конец 00:00", func(r *models.ZonePriceRule) { r.EndMin = 0 }, "bad_time"},
		{"пустое окно", func(r *models.ZonePriceRule) { r.EndMin = 1080 }, "zero_length"},
		{"цена ноль", func(r *models.ZonePriceRule) { r.RatePLN = 0 }, "bad_rate"},
		{"цена три знака", func(r *models.ZonePriceRule) { r.RatePLN = 20.005 }, "bad_rate"},
		{"срок наоборот", func(r *models.ZonePriceRule) { r.ValidFrom, r.ValidTo = dayPtr("2026-12-31"), dayPtr("2026-12-01") }, "bad_dates"},
		{"длинная заметка", func(r *models.ZonePriceRule) { r.Note = string(make([]rune, 65)) }, "bad_note"},
	}
	for _, tc := range cases {
		r := ok
		tc.edit(&r)
		if got := validateZoneRule(r); got != tc.want {
			t.Errorf("%s: %q, ожидалось %q", tc.name, got, tc.want)
		}
	}
	if d, okd := ruleDate("2026-12-01"); !okd || d == nil || d.Format("2006-01-02 15:04 MST") != "2026-12-01 00:00 CET" {
		t.Errorf("дата срока — полночь клуба: %v", d)
	}
	if d, okd := ruleDate(""); !okd || d != nil {
		t.Error("пустая дата — без границы")
	}
	if _, okd := ruleDate("01.12.2026"); okd {
		t.Error("дата не в ГГГГ-ММ-ДД — ошибка")
	}
}

func TestPriceAt(t *testing.T) {
	evening := models.ZonePriceRule{DaysMask: monFri, StartMin: 1080, EndMin: 1320, RatePLN: 20}
	night := models.ZonePriceRule{DaysMask: friday, StartMin: 1320, EndMin: 360, RatePLN: 10}
	weekend := models.ZonePriceRule{DaysMask: satSun, StartMin: 0, EndMin: 1440, RatePLN: 25, ValidFrom: dayPtr("2026-11-21")}
	rules := []models.ZonePriceRule{evening, night, weekend}
	cases := []struct {
		at   time.Time
		want float64
	}{
		{wallT("2026-11-11", "17:59"), 15}, // среда до вечера — цена зоны
		{wallT("2026-11-11", "18:00"), 20}, // с 18:00 — вечер
		{wallT("2026-11-11", "21:59"), 20},
		{wallT("2026-11-11", "22:00"), 15}, // конец окна — снова цена зоны
		{wallT("2026-11-13", "21:59"), 20}, // пятница вечер
		{wallT("2026-11-13", "22:00"), 10}, // ночь пятницы
		{wallT("2026-11-14", "05:59"), 10}, // утро субботы — ночь пятницы
		{wallT("2026-11-14", "06:00"), 15}, // выходные — со срока 21.11
		{wallT("2026-11-21", "00:00"), 10}, // ночь пятницы 20.11 перекрывает начало субботы? — нет: правило выходных с 00:00, ниже
		{wallT("2026-11-15", "03:00"), 15}, // ночь субботы — пятничного правила нет
		{wallT("2026-11-22", "12:00"), 25}, // воскресенье после 21.11 — выходные
		{wallT("2026-11-23", "00:00"), 15}, // понедельник 00:00 — выходные кончились
	}
	// ночь пятницы 20.11 22:00 — 21.11 06:00 и выходные с 21.11 00:00 накладываются: сервер такого не пустит, а ruleAt
	// берёт первое по порядку — ночь
	for _, tc := range cases {
		if got := priceAt(rules, 15, tc.at); got != tc.want {
			t.Errorf("%s: %v, ожидалось %v", tc.at.Format("02.01 Mon 15:04"), got, tc.want)
		}
	}
	if !rulesOverlap(night, weekend) {
		t.Error("ночь пятницы и выходные с 00:00 субботы накладываются")
	}
	dec := models.ZonePriceRule{DaysMask: 127, StartMin: 1320, EndMin: 360, RatePLN: 12, ValidTo: dayPtr("2026-12-31")}
	if priceAt([]models.ZonePriceRule{dec}, 15, wallT("2027-01-01", "03:00")) != 12 || priceAt([]models.ZonePriceRule{dec}, 15, wallT("2027-01-01", "22:30")) != 15 {
		t.Error("срок — по дню начала окна: ночь 31.12 до утра 01.01 — по правилу, вечер 01.01 — уже нет")
	}
}

func TestNextPriceChange(t *testing.T) {
	evening := models.ZonePriceRule{DaysMask: monFri, StartMin: 1080, EndMin: 1200, RatePLN: 20}
	late := models.ZonePriceRule{DaysMask: monFri, StartMin: 1200, EndMin: 1320, RatePLN: 25}
	rules := []models.ZonePriceRule{evening, late}
	check := func(now time.Time, wantAt string, wantRate float64) {
		t.Helper()
		got, rate, ok := nextPriceChange(rules, 15, now)
		if wantAt == "" {
			if ok {
				t.Errorf("%s: смены цены нет, а вышло %s %v", now.Format("02.01 15:04"), got.Format("02.01 15:04"), rate)
			}
			return
		}
		if !ok || got.Format("2006-01-02 15:04") != wantAt || rate != wantRate {
			t.Errorf("%s: %s %v (%v), ожидалось %s %v", now.Format("02.01 15:04"), got.Format("2006-01-02 15:04"), rate, ok, wantAt, wantRate)
		}
	}
	check(wallT("2026-11-11", "17:00"), "2026-11-11 18:00", 20)
	check(wallT("2026-11-11", "18:30"), "2026-11-11 20:00", 25) // стык правил — сразу следующая цена, без цены зоны
	check(wallT("2026-11-11", "21:00"), "2026-11-11 22:00", 15)
	check(wallT("2026-11-13", "23:00"), "2026-11-16 18:00", 20) // пятница поздно — понедельник
	if _, _, ok := nextPriceChange(nil, 15, wallT("2026-11-11", "12:00")); ok {
		t.Error("без правил смены цены нет")
	}
	far := []models.ZonePriceRule{{DaysMask: 127, StartMin: 600, EndMin: 700, RatePLN: 30, ValidFrom: dayPtr("2026-12-24")}}
	check2, _, ok := nextPriceChange(far, 15, wallT("2026-11-11", "12:00"))
	if ok {
		t.Errorf("правило через полтора месяца — дальше недели: %v", check2)
	}
}

func TestRulesOverlap(t *testing.T) {
	r := func(mask, start, end int, from, to string) models.ZonePriceRule {
		return models.ZonePriceRule{DaysMask: mask, StartMin: start, EndMin: end, RatePLN: 20, ValidFrom: dayPtr(from), ValidTo: dayPtr(to)}
	}
	cases := []struct {
		name string
		a, b models.ZonePriceRule
		want bool
	}{
		{"будни и выходные вечером", r(monFri, 1080, 1320, "", ""), r(satSun, 1080, 1320, "", ""), false},
		{"будни и пятница 21–23", r(monFri, 1080, 1320, "", ""), r(friday, 1260, 1380, "", ""), true},
		{"ночь пятницы и утро субботы до 8", r(friday, 1320, 360, "", ""), r(32, 300, 480, "", ""), true},
		{"ночь пятницы и суббота с 6 — стык", r(friday, 1320, 360, "", ""), r(32, 360, 480, "", ""), false},
		{"ночь воскресенья и утро понедельника", r(64, 1320, 360, "", ""), r(1, 300, 420, "", ""), true},
		{"декабрь и январь — сроки не пересекаются", r(monFri, 1080, 1320, "2026-12-01", "2026-12-31"), r(monFri, 1080, 1320, "2027-01-01", ""), false},
		{"декабрь и с 31.12", r(monFri, 1080, 1320, "2026-12-01", "2026-12-31"), r(monFri, 1080, 1320, "2026-12-31", ""), true},
		{"ночь 31.12 до утра и с 01.01 утром", r(127, 1320, 360, "", "2026-12-31"), r(127, 300, 420, "2027-01-01", ""), true},
		{"один день — четверг 24.12 — и понедельники", r(127, 1080, 1320, "2026-12-24", "2026-12-24"), r(1, 1080, 1320, "", ""), false},
		{"один день и тот же четверг", r(127, 1080, 1320, "2026-12-24", "2026-12-24"), r(8, 1140, 1200, "", ""), true},
	}
	for _, tc := range cases {
		if got := rulesOverlap(tc.a, tc.b); got != tc.want {
			t.Errorf("%s: %v, ожидалось %v", tc.name, got, tc.want)
		}
		if got := rulesOverlap(tc.b, tc.a); got != tc.want {
			t.Errorf("%s (наоборот): %v, ожидалось %v", tc.name, got, tc.want)
		}
	}
}
