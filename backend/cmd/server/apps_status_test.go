package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// З20-и1: снимок apps_status от агента — что принимается и что отклоняется целиком.
func TestParseAppsStatus(t *testing.T) {
	snap, err := parseAppsStatus(json.RawMessage(`{"apps": {"cs2": {"available": true}, "gta5": {"available": false,
		"reason": "path_unreachable", "kind": "file"}, "fresh": {"available": true, "reason": " checking "}},
		"running": {"app_id": "cs2", "pid": 1234}}`))
	if err != nil {
		t.Fatalf("годный снимок отклонён: %v", err)
	}
	want := map[string]appState{"cs2": {true, ""}, "gta5": {false, "path_unreachable"}, "fresh": {true, "checking"}}
	if len(snap) != len(want) {
		t.Fatalf("снимок: %v, ждали %v", snap, want)
	}
	for id, w := range want {
		if snap[id] != w {
			t.Fatalf("%s: %+v, ждали %+v", id, snap[id], w)
		}
	}

	empty, err := parseAppsStatus(json.RawMessage(`{"apps": {}}`))
	if err != nil || len(empty) != 0 {
		t.Fatalf("пустой снимок (на ПК нет приложений каталога) — годный: %v %v", empty, err)
	}

	long := strings.Repeat("я", 40)
	if s, err := parseAppsStatus(json.RawMessage(`{"apps": {"x": {"available": false, "reason": "` + long + `"}}}`)); err != nil ||
		s["x"].Reason != strings.Repeat("я", appsStatusMaxReason) {
		t.Fatalf("длинная причина обрезается до %d символов: %+v %v", appsStatusMaxReason, s["x"], err)
	}

	many := make([]string, 0, appsStatusMaxApps+1)
	for i := 0; i <= appsStatusMaxApps; i++ {
		many = append(many, fmt.Sprintf(`"a%d": {"available": true}`, i))
	}
	for name, raw := range map[string]string{
		"не JSON":          `{"apps": `,
		"пустой payload":   ``,
		"null":             `null`,
		"нет apps":         `{"running": null}`,
		"apps: null":       `{"apps": null}`,
		"нет available":    `{"apps": {"cs2": {"reason": "checking"}}}`,
		"приложение null":  `{"apps": {"cs2": null}}`,
		"пустой id":        `{"apps": {"": {"available": true}}}`,
		"id с пробелами":   `{"apps": {" cs2": {"available": true}}}`,
		"длинный id":       `{"apps": {"` + strings.Repeat("a", appsStatusMaxID+1) + `": {"available": true}}}`,
		"больше 1000":      `{"apps": {` + strings.Join(many, ", ") + `}}`,
		"available строка": `{"apps": {"cs2": {"available": "yes"}}}`,
	} {
		if s, err := parseAppsStatus(json.RawMessage(raw)); err == nil {
			t.Errorf("%s: снимок принят (%v), ждали отказ целиком", name, s)
		}
	}
}
