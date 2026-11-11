package main

import (
	"reflect"
	"testing"

	"github.com/gin-gonic/gin"
)

// З21-и2а: месяцы когорт и сборка строк когорт.

func TestMonthAdd(t *testing.T) {
	cases := []struct {
		m    string
		k    int
		want string
	}{
		{"2026-11", 1, "2026-12"},
		{"2026-11", 2, "2027-01"},
		{"2026-11", -11, "2025-12"},
		{"2026-01", -1, "2025-12"},
		{"2026-11", 0, "2026-11"},
		{"кривой", 1, ""},
	}
	for _, tc := range cases {
		if got := monthAdd(tc.m, tc.k); got != tc.want {
			t.Errorf("monthAdd(%q, %d) = %q, ожидалось %q", tc.m, tc.k, got, tc.want)
		}
	}
	if cohortState("2026-10", "2026-11") != "done" || cohortState("2026-11", "2026-11") != "going" ||
		cohortState("2026-12", "2026-11") != "future" {
		t.Error("cohortState: прошлый — done, текущий — going, будущий — future")
	}
}

func TestCohortsOf(t *testing.T) {
	g := func(first string, money float64, months ...string) *cohortGuest {
		ms := map[string]bool{first: true}
		for _, m := range months {
			ms[m] = true
		}
		return &cohortGuest{First: first, Months: ms, Money: money}
	}
	guests := map[string]*cohortGuest{
		"a": g("2026-08", 30, "2026-09", "2026-11"), // вернулся через 1 и через 3 (месяц идёт)
		"b": g("2026-08", 10),                       // не вернулся
		"c": g("2026-08", 0.1, "2026-10"),           // через 2
		"d": g("2026-10", 19.5, "2026-11"),          // через 1 (месяц идёт)
		"e": g("2026-11", 5),                        // этот месяц — дальше будущее
		"z": g("2025-01", 99, "2026-08"),            // давняя когорта — не в окне и в чужую не попадает
	}
	type row struct {
		Month string
		Size  int
		Rev   float64
		LTV   any
		Back  [][3]any // состояние, число, доля
	}
	conv := func(r gin.H) row {
		var bs [][3]any
		for _, b := range r["back"].([]gin.H) {
			bs = append(bs, [3]any{b["state"], b["n"], b["pct"]})
		}
		return row{r["month"].(string), r["size"].(int), r["revenue_pln"].(float64), r["ltv_pln"], bs}
	}
	want := []row{
		{"2026-07", 0, 0, nil, [][3]any{{"done", 0, nil}, {"done", 0, nil}, {"done", 0, nil}}},
		{"2026-08", 3, 40.1, 13.37, [][3]any{{"done", 1, 33.3}, {"done", 1, 33.3}, {"going", 1, 33.3}}},
		{"2026-10", 1, 19.5, 19.5, [][3]any{{"going", 1, 100.0}, {"future", nil, nil}, {"future", nil, nil}}},
		{"2026-11", 1, 5, 5.0, [][3]any{{"future", nil, nil}, {"future", nil, nil}, {"future", nil, nil}}},
	}
	rows := cohortsOf(guests, []string{"2026-07", "2026-08", "2026-10", "2026-11"}, "2026-11")
	if len(rows) != len(want) {
		t.Fatalf("строк %d, ожидалось %d", len(rows), len(want))
	}
	for i, r := range rows {
		if got := conv(r); !reflect.DeepEqual(got, want[i]) {
			t.Errorf("когорта %s: %+v, ожидалось %+v", want[i].Month, got, want[i])
		}
	}
}
