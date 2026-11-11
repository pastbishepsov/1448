package main

import (
	"testing"
	"time"
)

// З22-и1: ключ «раз за день» и виды алертов.

func TestAlertDayKey(t *testing.T) {
	at := func(d, h, m int) time.Time { return time.Date(2026, 11, d, h, m, 0, 0, time.UTC) }
	cases := []struct {
		name string
		t    time.Time
		rh   int
		want string
	}{
		{"сутки с полуночи — календарный день", at(10, 0, 30), 0, "2026-11-10/test"},
		{"своя граница 08:00: ночь до неё — вчерашние сутки", at(10, 7, 59), 8, "2026-11-09/test"},
		{"ровно на границе — сегодняшние", at(10, 8, 0), 8, "2026-11-10/test"},
	}
	for _, tc := range cases {
		if got := alertDayKey(tc.t, tc.rh, "test"); got != tc.want {
			t.Errorf("%s: %q, ожидалось %q", tc.name, got, tc.want)
		}
	}
}

func TestAlertKindsSettings(t *testing.T) {
	// свой выключатель вида — настоящая настройка: есть границы 0…1 и значение по умолчанию
	for kind, k := range alertKinds {
		if k.Setting == "" {
			continue
		}
		if b, ok := settingBounds[k.Setting]; !ok || b != [2]int64{0, 1} {
			t.Errorf("алерт %s: настройка %s без границ 0…1 в settingBounds", kind, k.Setting)
		}
		if _, ok := settingDefaults[k.Setting]; !ok {
			t.Errorf("алерт %s: у настройки %s нет значения по умолчанию", kind, k.Setting)
		}
	}
	if b := settingBounds["alerts_on"]; b != [2]int64{0, 1} || settingDefaults["alerts_on"] != 1 {
		t.Error("alerts_on — 0…1, по умолчанию 1 (присылать)")
	}
	if !alertKinds["test"].OwnerOnly {
		t.Error("проверочный алерт — только владельцу")
	}
}
