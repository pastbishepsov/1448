package main

import "testing"

func TestValidateZone(t *testing.T) {
	cases := []struct {
		name string
		zone string
		rate float64
		want string
	}{
		{"обычная зона", "VIP", 35, ""},
		{"название придумывает владелец", "Симрейсинг", 60, ""},
		{"копеечная цена всё равно цена", "Утренняя", 0.5, ""},
		{"пустое название", "   ", 23, "bad_name"},
		{"название длиннее 32", string(make([]rune, 33)), 23, "bad_name"},
		{"нулевая цена", "VIP", 0, "bad_rate"},
		{"отрицательная цена", "VIP", -5, "bad_rate"},
		{"опечатка в три нуля", "VIP", 23000, "bad_rate"},
	}
	for _, tc := range cases {
		_, code := validateZone(tc.zone, tc.rate)
		if code != tc.want {
			t.Errorf("%s: validateZone = %q, ожидалось %q", tc.name, code, tc.want)
		}
	}
}

func TestRateForComputer(t *testing.T) {
	vip := 35.0
	zero := 0.0
	cases := []struct {
		name     string
		zoneRate *float64
		club     float64
		want     float64
	}{
		{"есть зона — её цена", &vip, 23, 35},
		{"нет зоны — клубный тариф", nil, 23, 23},
		{"битая цена зоны — не обнуляем сессию", &zero, 23, 23},
	}
	for _, tc := range cases {
		if got := rateForComputer(tc.zoneRate, tc.club); got != tc.want {
			t.Errorf("%s: rateForComputer = %v, ожидалось %v", tc.name, got, tc.want)
		}
	}
}
