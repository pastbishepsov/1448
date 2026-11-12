package main

import (
	"strings"
	"testing"
)

// З24-и4: массовые правки цен — проверка запроса, новая цена с округлением, изменение словами, строка журнала.

func TestValidateBulk(t *testing.T) {
	ok := bulkRequest{Kind: "goods", Pct: 10}
	cases := []struct {
		name string
		edit func(q *bulkRequest)
		want string
	}{
		{"товары +10 %", func(q *bulkRequest) {}, ""},
		{"зоны +2 zł до целых", func(q *bulkRequest) { q.Kind, q.Pct, q.AddPLN, q.Round = "zones", 0, 2, 1 }, ""},
		{"пакеты −7,5 % до 0,50", func(q *bulkRequest) { q.Kind, q.Pct, q.Round = "packages", -7.5, 0.5 }, ""},
		{"чужой вид", func(q *bulkRequest) { q.Kind = "users" }, "bad_kind"},
		{"ни процентов, ни злотых, ни округления", func(q *bulkRequest) { q.Pct = 0 }, "bad_op"},
		{"только округление до 0,50", func(q *bulkRequest) { q.Pct, q.Round = 0, 0.5 }, ""},
		{"и то и другое", func(q *bulkRequest) { q.AddPLN = 1 }, "bad_op"},
		{"−95 %", func(q *bulkRequest) { q.Pct = -95 }, "bad_pct"},
		{"+301 %", func(q *bulkRequest) { q.Pct = 301 }, "bad_pct"},
		{"три знака в процентах", func(q *bulkRequest) { q.Pct = 1.005 }, "bad_pct"},
		{"+1001 zł", func(q *bulkRequest) { q.Pct, q.AddPLN = 0, 1001 }, "bad_add"},
		{"три знака в злотых", func(q *bulkRequest) { q.Pct, q.AddPLN = 0, 0.005 }, "bad_add"},
		{"округление до 0,10", func(q *bulkRequest) { q.Round = 0.1 }, "bad_round"},
	}
	for _, tc := range cases {
		q := ok
		tc.edit(&q)
		if got := validateBulk(q); got != tc.want {
			t.Errorf("%s: %q, ожидалось %q", tc.name, got, tc.want)
		}
	}
}

func TestBulkNewPrice(t *testing.T) {
	cases := []struct {
		was, pct, add, round, want float64
	}{
		{20, 10, 0, 0, 22},             // +10 %
		{19.99, 10, 0, 0, 21.99},       // 21,989 → 21,99
		{5, 7.5, 0, 0, 5.38},           // 5,375 → 5,38 (половина — вверх)
		{5.2, 10, 0, 0.5, 5.5},         // 5,72 → 5,50
		{5.4, 10, 0, 0.5, 6},           // 5,94 → 6,00
		{4.55, 10, 0, 0.5, 5},          // 5,005 → 5,00
		{4.5, 0, 0.25, 0.5, 5},         // 4,75 → 5,00 (половина шага — вверх)
		{12.49, 0, 0, 1, 12},           // только округление: до целых
		{12.74, 0, 0, 0.5, 12.5},       // только округление: до 0,50
		{30, -10, 0, 0, 27},            // −10 %
		{20, 0, 2.5, 0, 22.5},          // +2,50 zł
		{3, 0, -2.5, 0, 0.5},           // −2,50 zł
		{0.3, -50, 0, 0.5, 0},          // 0,15 → 0,00 — до нуля (за границу — ловит bulkOutOfRange)
		{1000.01, 0.01, 0, 0, 1000.11}, // 1000,11001 → 1000,11
	}
	for _, tc := range cases {
		if got := bulkNewPrice(tc.was, tc.pct, tc.add, tc.round); got != tc.want {
			t.Errorf("%v %+v%% %+v zł шаг %v: %v, ожидалось %v", tc.was, tc.pct, tc.add, tc.round, got, tc.want)
		}
	}
	rows := []bulkRow{{Name: "A", Was: 1, Will: 0, Max: 500}, {Name: "B", Was: 400, Will: 520, Max: 500}, {Name: "C", Was: 10, Will: 11, Max: 500},
		{Name: "D", Was: 0.004, Will: 0.004, Max: 500}}
	bad := bulkOutOfRange(rows)
	if len(bad) != 2 || bad[0]["name"] != "A" || bad[1]["name"] != "B" {
		t.Errorf("за границы — A (до нуля) и B (выше потолка), неизменная D не в счёт: %v", bad)
	}
}

func TestBulkOpText(t *testing.T) {
	cases := []struct {
		pct, add, round float64
		want            string
	}{
		{10, 0, 0, "+10 %"},
		{-7.5, 0, 0.5, "−7,5 %, до 0,50"},
		{0, 2, 1, "+2,00 zł, до целых"},
		{0, -2.5, 0, "−2,50 zł"},
		{0, 0, 0.5, "округление до 0,50"},
	}
	for _, tc := range cases {
		if got := bulkOpText(tc.pct, tc.add, tc.round); got != tc.want {
			t.Errorf("%v %v %v: %q, ожидалось %q", tc.pct, tc.add, tc.round, got, tc.want)
		}
	}
}

func TestBulkLogText(t *testing.T) {
	rows := []bulkRow{{Name: "Cola", Was: 5, Will: 5.5}, {Name: "Woda", Was: 3, Will: 3}, {Name: "Lay's", Was: 6, Will: 6.5}}
	if got := bulkLogText("товары «Напитки»", "+10 %, до 0,50", rows); got != "товары «Напитки»: +10 %, до 0,50 — 2 из 3: Cola 5,00 → 5,50 zł; Lay's 6,00 → 6,50 zł" {
		t.Errorf("строка журнала: %q", got)
	}
	many := []bulkRow{}
	for i := 0; i < 13; i++ {
		many = append(many, bulkRow{Name: "P" + strings.Repeat("x", i%2), Was: 1, Will: 2})
	}
	got := bulkLogText("пакеты", "+100 %", many)
	if !strings.HasPrefix(got, "пакеты: +100 % — 13 из 13: ") || !strings.HasSuffix(got, " zł; и ещё 3") || strings.Count(got, " → ") != 10 {
		t.Errorf("больше 10 — первые 10 и «и ещё N»: %q", got)
	}
}
