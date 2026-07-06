package main

// Чтение настроек мыши для «студии сенсы» (S2). Кроссплатформенное ядро:
// типы + чистая логика (тестируется). Реальные Windows-вызовы — в mouse_windows.go,
// заглушки для сборки на Linux/CI — в mouse_other.go.
//
// Безопасность: агент только ЧИТАЕТ системные настройки и умеет ОДНО фиксированное
// действие — выключить «повышенную точность указателя» (mouse acceleration).
// Произвольных команд/путей нет — веб-страница не может ничего сверх этого.

import (
	"sort"
	"strings"
)

// MouseInfo — снимок настроек мыши для гостевого экрана.
type MouseInfo struct {
	Available    bool     `json:"available"`     // удалось прочитать (только Windows)
	PointerSpeed int      `json:"pointer_speed"` // ползунок Windows 1–20, 10 = «1:1»
	Accel        bool     `json:"accel"`         // «повышенная точность указателя» включена
	Vendors      []string `json:"vendors"`       // найденный софт мыши (Logitech/Razer/...)
	Note         string   `json:"note,omitempty"`
}

// accelFromParams — включена ли акселерация по SPI_GETMOUSE [threshold1, threshold2, accel].
// Третий параметр 0 = выключена, ненулевой = включена.
func accelFromParams(p [3]int32) bool { return p[2] != 0 }

// pointerSpeedIdeal — 10 из 20 = движение курсора 1:1 без масштабирования ОС.
func pointerSpeedIdeal(speed int) bool { return speed == 10 }

// Известные вендоры софта мыши по подстроке имени процесса (lowercase).
var mouseVendorSigns = map[string][]string{
	"Logitech":    {"lghub", "logioptions", "logi_", "logitech"},
	"Razer":       {"razer", "rzsynapse"},
	"SteelSeries": {"steelseries"},
	"Glorious":    {"glorious"},
	"Corsair":     {"icue", "corsair"},
	"Pulsar":      {"pulsar"},
	"Logi Bolt":   {"logibolt"},
}

// detectMouseVendors — какие вендоры софта мыши запущены (по списку процессов).
// Чистая функция (тестируется).
func detectMouseVendors(procs []string) []string {
	found := map[string]bool{}
	for _, p := range procs {
		lp := strings.ToLower(p)
		for vendor, signs := range mouseVendorSigns {
			for _, s := range signs {
				if strings.Contains(lp, s) {
					found[vendor] = true
				}
			}
		}
	}
	out := make([]string, 0, len(found))
	for v := range found {
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}
