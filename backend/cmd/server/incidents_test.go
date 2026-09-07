package main

// К8 (KIOSK.md): журнал проблем — чистые куски без БД.

import (
	"testing"
	"time"
)

func TestIncidentDurationMin(t *testing.T) {
	t0 := time.Date(2026, 9, 7, 21, 14, 0, 0, time.UTC)
	cases := []struct {
		sec  int
		want int
	}{{0, 0}, {1, 1}, {59, 1}, {60, 1}, {61, 2}, {370, 7}, {-5, 0}}
	for _, c := range cases {
		if got := incidentDurationMin(t0, t0.Add(time.Duration(c.sec)*time.Second)); got != c.want {
			t.Errorf("%d с → %d мин, ожидалось %d", c.sec, got, c.want)
		}
	}
}

func TestCompensationText(t *testing.T) {
	if !(compensation{}).empty() || (compensation{Minutes: 30}).empty() || (compensation{Grosz: 500}).empty() {
		t.Error("empty")
	}
	if (compensation{Minutes: 30}).text() != "+30 мин" {
		t.Errorf("минуты: %q", (compensation{Minutes: 30}).text())
	}
	if (compensation{Grosz: 550}).text() != "+5.50 zł" {
		t.Errorf("деньги: %q", (compensation{Grosz: 550}).text())
	}
	if (compensation{Minutes: 30, Grosz: 1000}).text() != "+30 мин и +10.00 zł" {
		t.Errorf("оба: %q", (compensation{Minutes: 30, Grosz: 1000}).text())
	}
}

func TestIncidentKindName(t *testing.T) {
	if incidentKindName("agent_lost") != "агент пропал" || incidentKindName("pc_hang") != "ПК завис" || incidentKindName("weird") != "weird" {
		t.Error("kind names")
	}
	if incidentNote("  ") != "" || incidentNote(" мышь ") != " — мышь" {
		t.Error("incidentNote")
	}
}
