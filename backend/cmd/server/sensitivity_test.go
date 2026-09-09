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

// С2-и1 (SENS.md, Т4): пересчёт сенсы под DPI мыши клуба — cm/360 не меняется.
func TestRescaleSensKeepsCm360(t *testing.T) {
	games := map[string]float64{"cs2": 2.0, "valorant": 0.629}
	out := rescaleSens(games, 1600, 800) // профиль на 1600 → мышь клуба 800
	if out["cs2"] != 4.0 {
		t.Fatalf("cs2: ждал 4.0, получил %v", out["cs2"])
	}
	if out["valorant"] != 1.258 {
		t.Fatalf("valorant: ждал 1.258, получил %v", out["valorant"])
	}
	// cm/360 = 360*2.54/(yaw*sens*dpi): одинаков до и после
	cm := func(s float64, dpi int) float64 { return 360 * 2.54 / (0.022 * s * float64(dpi)) }
	if a, b := cm(2.0, 1600), cm(out["cs2"], 800); a < b-1e-9 || a > b+1e-9 {
		t.Fatalf("cm/360 поплыл: %v → %v", a, b)
	}
	back := rescaleSens(out, 800, 1600)
	if back["cs2"] != 2.0 {
		t.Fatalf("обратный пересчёт: %v", back["cs2"])
	}
}

func TestSensPayloadRescalesOnlyWhenNeeded(t *testing.T) {
	games := map[string]float64{"cs2": 2.0}
	if p := sensPayload(800, games, 0); p["dpi"] != 800 || p["rescaled_from"] != nil {
		t.Fatalf("mouse_dpi=0: ждал как есть, получил %v", p)
	}
	if p := sensPayload(800, games, 800); p["rescaled_from"] != nil {
		t.Fatalf("тот же DPI: пересчёта быть не должно, %v", p)
	}
	p := sensPayload(800, games, 1600)
	if p["dpi"] != 1600 || p["rescaled_from"] != 800 || p["games"].(map[string]float64)["cs2"] != 1.0 {
		t.Fatalf("800→1600: ждал dpi 1600, cs2 1.0, rescaled_from 800; получил %v", p)
	}
}
