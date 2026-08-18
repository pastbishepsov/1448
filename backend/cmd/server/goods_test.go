package main

import (
	"testing"
	"time"
)

func TestValidateGood(t *testing.T) {
	cases := []struct {
		name     string
		title    string
		category string
		price    float64
		low      int
		want     string
	}{
		{"обычная позиция", "Кола 0.5", "Напитки", 8, 6, ""},
		{"без категории тоже можно", "Сникерс", "", 7.5, 0, ""},
		{"пробелы по краям — не имя", "   ", "Напитки", 8, 0, "bad_name"},
		{"имя длиннее 64", string(make([]rune, 65)), "", 8, 0, "bad_name"},
		{"нулевая цена", "Вода", "", 0, 0, "bad_price"},
		{"отрицательная цена", "Вода", "", -1, 0, "bad_price"},
		{"цена выше потолка", "Кресло", "", 10001, 0, "bad_price"},
		{"отрицательный порог", "Вода", "", 5, -1, "bad_low"},
	}
	for _, tc := range cases {
		_, code := validateGood(tc.title, tc.category, tc.price, tc.low)
		if code != tc.want {
			t.Errorf("%s: validateGood = %q, ожидалось %q", tc.name, code, tc.want)
		}
	}
}

func TestValidateGoodКатегорияДлиннее32(t *testing.T) {
	if _, code := validateGood("Кола", string(make([]rune, 33)), 8, 0); code != "bad_category" {
		t.Errorf("длинная категория дала %q, ожидалось bad_category", code)
	}
}

func TestCanVoidSale(t *testing.T) {
	dayFrom := time.Date(2026, 8, 18, 8, 0, 0, 0, time.UTC)
	dayTo := dayFrom.AddDate(0, 0, 1)
	inShift := time.Date(2026, 8, 18, 14, 0, 0, 0, time.UTC)
	yesterday := time.Date(2026, 8, 17, 14, 0, 0, 0, time.UTC)

	cases := []struct {
		name     string
		role     string
		saleBy   string
		user     string
		at       time.Time
		wantOK   bool
		wantCode string
	}{
		{"владелец отменяет чужую и старую", "owner", "a1", "o1", yesterday, true, ""},
		{"админ отменяет свою за смену", "admin", "a1", "a1", inShift, true, ""},
		{"админ не отменяет чужую", "admin", "a2", "a1", inShift, false, "not_yours"},
		{"админ не отменяет вчерашнюю", "admin", "a1", "a1", yesterday, false, "too_old"},
		{"граница смены: ровно начало — можно", "admin", "a1", "a1", dayFrom, true, ""},
		{"граница смены: ровно конец — уже нельзя", "admin", "a1", "a1", dayTo, false, "too_old"},
	}
	for _, tc := range cases {
		ok, code := canVoidSale(tc.role, tc.saleBy, tc.user, tc.at, dayFrom, dayTo)
		if ok != tc.wantOK || code != tc.wantCode {
			t.Errorf("%s: canVoidSale = (%v, %q), ожидалось (%v, %q)", tc.name, ok, code, tc.wantOK, tc.wantCode)
		}
	}
}

func TestRoundPLN(t *testing.T) {
	cases := []struct{ in, want float64 }{
		{7.5 * 3, 22.5},
		{8.33 * 3, 24.99},
		{0.1 * 3, 0.3}, // классическая ловушка float
		{12.005, 12.01},
	}
	for _, tc := range cases {
		if got := roundPLN(tc.in); got != tc.want {
			t.Errorf("roundPLN(%v) = %v, ожидалось %v", tc.in, got, tc.want)
		}
	}
}
