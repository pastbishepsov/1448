package main

import "testing"

func TestValidateSensitivity(t *testing.T) {
	// валидный профиль
	if ok, _ := validateSensitivity(800, map[string]float64{"cs2": 2.0, "valorant": 0.4}); !ok {
		t.Error("валидный профиль отклонён")
	}
	// пустой набор игр — тоже валиден (игрок сохранил только DPI)
	if ok, _ := validateSensitivity(1600, map[string]float64{}); !ok {
		t.Error("профиль без игр отклонён")
	}

	cases := []struct {
		dpi      int
		games    map[string]float64
		wantCode string
	}{
		{50, nil, "bad_dpi"},                              // ниже минимума
		{40000, nil, "bad_dpi"},                           // выше максимума
		{800, map[string]float64{"cs2": 0}, "bad_sens"},   // ноль
		{800, map[string]float64{"cs2": -1}, "bad_sens"},  // отрицательная
		{800, map[string]float64{"cs2": 9999}, "bad_sens"},// абсурдно большая
		{800, map[string]float64{"": 1.0}, "bad_game"},    // пустой id
	}
	for _, tc := range cases {
		ok, code := validateSensitivity(tc.dpi, tc.games)
		if ok || code != tc.wantCode {
			t.Errorf("validateSensitivity(%d,%v) = %v/%q, ожидалось false/%q", tc.dpi, tc.games, ok, code, tc.wantCode)
		}
	}

	// границы DPI валидны
	if ok, _ := validateSensitivity(100, nil); !ok {
		t.Error("DPI=100 (граница) отклонён")
	}
	if ok, _ := validateSensitivity(32000, nil); !ok {
		t.Error("DPI=32000 (граница) отклонён")
	}
}
