package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// З7-и1: у каждой настройки из settingBounds есть значение по умолчанию, и
// оно в своих же границах — иначе «было» в журнале и форма «Экономики» врут.
func TestSettingDefaultsMatchBounds(t *testing.T) {
	for key, def := range settingDefaults {
		b, ok := settingBounds[key]
		if !ok {
			t.Errorf("%s: есть значение по умолчанию, но нет границ в settingBounds", key)
			continue
		}
		if def < b[0] || def > b[1] {
			t.Errorf("%s: по умолчанию %d вне границ %d–%d", key, def, b[0], b[1])
		}
	}
	for key := range settingBounds {
		if _, ok := settingDefaults[key]; !ok {
			t.Errorf("%s: есть границы, но нет значения по умолчанию в settingDefaults", key)
		}
	}
}

// З7-и1 (№41): в журнал и в базу — только то, что изменилось, «было → стало»,
// ключи по алфавиту.
func TestSettingChanges(t *testing.T) {
	was := gin.H{"report_hour": int64(8), "seg_lost_days": int64(21), "xp_per_min": int64(10)}
	keys, changes := settingChanges(was, map[string]int64{"xp_per_min": 12, "report_hour": 8, "seg_lost_days": 22})
	if !reflect.DeepEqual(keys, []string{"seg_lost_days", "xp_per_min"}) ||
		!reflect.DeepEqual(changes, []string{"seg_lost_days: 21→22", "xp_per_min: 10→12"}) {
		t.Errorf("две правки из трёх: ключи %v, журнал %v", keys, changes)
	}
	keys, changes = settingChanges(was, map[string]int64{"report_hour": 8, "seg_lost_days": 21})
	if len(keys) != 0 || len(changes) != 0 {
		t.Errorf("без правок: ключи %v, журнал %v — должно быть пусто", keys, changes)
	}
}

// З18-и4б (решение основателя 07.11, З-Р38): сутки клуба по умолчанию — с полуночи, и граница читается в одном месте —
// clubDayHour. Раньше каждая из 28 ручек читала settingInt64("report_hour", 8) со своим значением по умолчанию: новая
// ручка с другим числом развела бы сутки у отчётов и у кассы.
func TestClubDayHourOnePlace(t *testing.T) {
	if reportHourDef != 0 || settingDefaults["report_hour"] != 0 {
		t.Errorf("по умолчанию сутки с полуночи: reportHourDef %d, settingDefaults %d", reportHourDef, settingDefaults["report_hour"])
	}
	files, _ := filepath.Glob("*.go")
	reads := 0
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(src), "\n") {
			if !strings.Contains(line, `settingInt64("report_hour"`) {
				continue
			}
			reads++
			if !strings.Contains(line, `settingInt64("report_hour", reportHourDef)`) || !strings.Contains(line, "func clubDayHour()") {
				t.Errorf("%s:%d: граница суток — только через clubDayHour(): %s", f, i+1, strings.TrimSpace(line))
			}
		}
	}
	if reads != 1 {
		t.Errorf("report_hour читается в %d местах, а должен в одном — clubDayHour", reads)
	}
}
