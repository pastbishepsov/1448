package main

import (
	"testing"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

// З24-и5: сезонные пакеты — проверка полей, окно списания, окно продажи, условия словами, журнал правки.

func seasonPack() models.TimePackage {
	return models.TimePackage{Name: "Будни", SpendDaysMask: 127, SpendStartMin: 0, SpendEndMin: 1440}
}

func TestValidateSeason(t *testing.T) {
	cases := []struct {
		name string
		edit func(p *models.TimePackage)
		want string
	}{
		{"обычный пакет", func(p *models.TimePackage) {}, ""},
		{"будни до 17:00, декабрь, 1 на гостя, только новым", func(p *models.TimePackage) {
			p.SpendDaysMask, p.SpendEndMin, p.SellFrom, p.SellTo, p.PerGuest, p.NewOnly = 31, 1020, dayPtr("2026-12-01"), dayPtr("2026-12-31"), 1, true
		}, ""},
		{"ночь через полночь", func(p *models.TimePackage) { p.SpendStartMin, p.SpendEndMin = 1320, 360 }, ""},
		{"продажа наоборот", func(p *models.TimePackage) { p.SellFrom, p.SellTo = dayPtr("2026-12-31"), dayPtr("2026-12-01") }, "bad_sell_dates"},
		{"на гостя −1", func(p *models.TimePackage) { p.PerGuest = -1 }, "bad_per_guest"},
		{"на гостя 101", func(p *models.TimePackage) { p.PerGuest = 101 }, "bad_per_guest"},
		{"без дней", func(p *models.TimePackage) { p.SpendDaysMask = 0 }, "bad_spend_days"},
		{"начало 24:00", func(p *models.TimePackage) { p.SpendStartMin = 1440 }, "bad_spend_time"},
		{"конец 00:00", func(p *models.TimePackage) { p.SpendEndMin = 0 }, "bad_spend_time"},
		{"пустое окно", func(p *models.TimePackage) { p.SpendStartMin, p.SpendEndMin = 600, 600 }, "bad_spend_time"},
	}
	for _, tc := range cases {
		p := seasonPack()
		tc.edit(&p)
		if got := validateSeason(p); got != tc.want {
			t.Errorf("%s: %q, ожидалось %q", tc.name, got, tc.want)
		}
	}
	p := seasonPack()
	if code := (seasonRequest{SellFrom: strPtr("01.12.2026")}).apply(&p); code != "bad_sell_dates" {
		t.Errorf("дата не ГГГГ-ММ-ДД — bad_sell_dates, а не %q", code)
	}
	p = seasonPack()
	if code := (seasonRequest{SellFrom: strPtr("2026-12-01"), SellTo: strPtr("")}).apply(&p); code != "" || p.SellFrom == nil || p.SellTo != nil {
		t.Errorf("с 01.12, «по» пустое — без границы: %q %v %v", code, p.SellFrom, p.SellTo)
	}
}

func strPtr(s string) *string { return &s }

func TestSpendOpen(t *testing.T) {
	cases := []struct {
		name             string
		mask, start, end int
		at               string
		want             bool
	}{
		{"всегда", 127, 0, 1440, "2026-11-14 03:00", true},
		{"будни до 17:00 — среда 16:59", monFri, 0, 1020, "2026-11-11 16:59", true},
		{"будни до 17:00 — среда 17:00", monFri, 0, 1020, "2026-11-11 17:00", false},
		{"будни до 17:00 — суббота 10:00", monFri, 0, 1020, "2026-11-14 10:00", false},
		{"будни целиком — суббота", monFri, 0, 1440, "2026-11-14 12:00", false},
		{"ночь пятницы — суббота 05:00", friday, 1320, 360, "2026-11-14 05:00", true},
		{"ночь пятницы — суббота 22:00", friday, 1320, 360, "2026-11-14 22:00", false},
	}
	for _, tc := range cases {
		day, hm := tc.at[:10], tc.at[11:]
		if got := spendOpen(tc.mask, tc.start, tc.end, wallT(day, hm)); got != tc.want {
			t.Errorf("%s: %v, ожидалось %v", tc.name, got, tc.want)
		}
	}
}

func TestSaleState(t *testing.T) {
	p := seasonPack()
	p.SellFrom, p.SellTo = dayPtr("2026-12-01"), dayPtr("2026-12-31")
	cases := map[string]string{"2026-11-30 23:59": "not_yet", "2026-12-01 00:00": "", "2026-12-31 23:59": "", "2027-01-01 00:00": "sale_over"}
	for at, want := range cases {
		if got := saleState(p, wallT(at[:10], at[11:])); got != want {
			t.Errorf("%s: %q, ожидалось %q", at, got, want)
		}
	}
	if saleState(seasonPack(), wallT("2026-11-11", "12:00")) != "" {
		t.Error("без окна — продаётся всегда")
	}
}

func TestSeasonText(t *testing.T) {
	p := seasonPack()
	if seasonText(p) != "" {
		t.Errorf("обычный пакет — без условий: %q", seasonText(p))
	}
	p.SellFrom, p.SellTo, p.PerGuest, p.NewOnly, p.SpendDaysMask, p.SpendEndMin = dayPtr("2026-12-01"), dayPtr("2026-12-31"), 1, true, monFri, 1020
	if got := seasonText(p); got != "продаётся 01.12.2026–31.12.2026 · 1 на гостя · только новым · тратится Пн–Пт 00:00–17:00" {
		t.Errorf("условия словами: %q", got)
	}
	if got := spendText(satSun, 0, 1440); got != "Сб, Вс" {
		t.Errorf("выходные целиком: %q", got)
	}
	was := seasonPack()
	now := p
	got := seasonChanges(was, now)
	want := []string{"продажа: всегда → 01.12.2026–31.12.2026", "на гостя: без ограничения → 1", "только новым", "списание: всегда → Пн–Пт 00:00–17:00"}
	if len(got) != len(want) {
		t.Fatalf("журнал правки: %q", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("журнал правки [%d]: %q, ожидалось %q", i, got[i], want[i])
		}
	}
	if len(seasonChanges(now, now)) != 0 {
		t.Error("без изменений — пусто")
	}
}
