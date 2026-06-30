package main

import "testing"

func TestBoostedXP(t *testing.T) {
	cases := []struct {
		base   int64
		effect float64
		want   int64
	}{
		{1500, 0.0, 1500},  // нет таланта
		{1500, 0.30, 1950}, // xp_boost ур.3 (+30%)
		{1500, 0.50, 2250}, // xp_boost ур.5 (+50%)
		{1000, 0.10, 1100}, // ур.1
		{0, 0.5, 0},        // нулевой базовый опыт
		{777, -0.2, 777},   // отрицательный эффект игнорируется
	}
	for _, tc := range cases {
		if got := boostedXP(tc.base, tc.effect); got != tc.want {
			t.Errorf("boostedXP(%d, %.2f) = %d, хотим %d", tc.base, tc.effect, got, tc.want)
		}
	}
}
