package main

import (
	"testing"
	"time"
)

// Ж6-и2: корзина из запроса — одиночная позиция как до Ж6, список items,
// слияние одинаковых позиций, пустое количество = 1, потолки.
func TestSaleItemsFromRequest(t *testing.T) {
	cases := []struct {
		name     string
		req      saleRequest
		wantN    int
		wantQty0 int
		wantCode string
	}{
		{"одна позиция как раньше", saleRequest{GoodID: "a", Qty: 2}, 1, 2, ""},
		{"qty пусто — одна штука", saleRequest{GoodID: "a"}, 1, 1, ""},
		{"корзина из двух", saleRequest{Items: []saleItem{{"a", 1}, {"b", 3}}}, 2, 1, ""},
		{"одинаковые складываются", saleRequest{Items: []saleItem{{"a", 1}, {"b", 1}, {"a", 2}}}, 2, 3, ""},
		{"пусто вовсе", saleRequest{}, 0, 0, "invalid_request"},
		{"пустой good_id в корзине", saleRequest{Items: []saleItem{{"", 1}}}, 0, 0, "invalid_request"},
		{"больше потолка штук", saleRequest{GoodID: "a", Qty: maxSaleQty + 1}, 0, 0, "bad_qty"},
		{"сложились сверх потолка", saleRequest{Items: []saleItem{{"a", 60}, {"a", 60}}}, 0, 0, "bad_qty"},
	}
	for _, tc := range cases {
		items, code := saleItemsFromRequest(&tc.req)
		if code != tc.wantCode {
			t.Errorf("%s: code = %q, ожидалось %q", tc.name, code, tc.wantCode)
			continue
		}
		if code != "" {
			continue
		}
		if len(items) != tc.wantN || items[0].Qty != tc.wantQty0 {
			t.Errorf("%s: items = %+v, ожидалось %d позиций, первая qty %d", tc.name, items, tc.wantN, tc.wantQty0)
		}
	}
	many := saleRequest{}
	for i := 0; i <= maxReceiptItems; i++ {
		many.Items = append(many.Items, saleItem{GoodID: string(rune('a' + i)), Qty: 1})
	}
	if _, code := saleItemsFromRequest(&many); code != "too_many_items" {
		t.Errorf("чек из %d позиций: code = %q, ожидалось too_many_items", len(many.Items), code)
	}
	same := saleRequest{}
	for i := 0; i <= maxReceiptItems; i++ {
		same.Items = append(same.Items, saleItem{GoodID: "a", Qty: 1})
	}
	if items, code := saleItemsFromRequest(&same); code != "" || len(items) != 1 || items[0].Qty != maxReceiptItems+1 {
		t.Errorf("21 одинаковая строка должна слиться в одну: %+v %q", items, code)
	}
}

func TestSaleMethodOK(t *testing.T) {
	for m, want := range map[string]bool{"cash": true, "card": true, "blik": true, "wallet": false, "later": false, "": false} {
		if got := saleMethodOK(m); got != want {
			t.Errorf("saleMethodOK(%q) = %v, ожидалось %v", m, got, want)
		}
	}
}

// Ж6-и4: расхождение и текст владельцу.
func TestCloseoutMessage(t *testing.T) {
	counted := func(g int64) *int64 { return &g }
	cases := []struct {
		name     string
		expected int64
		counted  *int64
		note     string
		wantSub  string
		wantNone bool
	}{
		{"сошлось — молчим", 50000, counted(50000), "", "", true},
		{"недостача", 50000, counted(48000), "", "расхождение −20.00 zł", false},
		{"излишек", 50000, counted(50500), "", "расхождение +5.00 zł", false},
		{"без пересчёта", 50000, nil, "", "без пересчёта кассы (по операциям должно быть 500.00 zł)", false},
		{"заметка добавляется", 50000, counted(48000), "сдал Ане", "Передал смену: сдал Ане", false},
		{"сошлось с заметкой — всё равно молчим", 50000, counted(50000), "всё ок", "", true},
	}
	for _, tc := range cases {
		got := closeoutMessage(tc.expected, tc.counted, tc.note)
		if tc.wantNone {
			if got != "" {
				t.Errorf("%s: ожидалось пусто, получили %q", tc.name, got)
			}
			continue
		}
		if got == "" || !contains(got, tc.wantSub) {
			t.Errorf("%s: %q не содержит %q", tc.name, got, tc.wantSub)
		}
	}
	if d := closeoutDiff(50000, 48000); d != -2000 {
		t.Errorf("closeoutDiff = %d, ожидалось -2000", d)
	}
	e := cashExpectation{Deposits: 10000, Goods: 2500, Packs: 6000, Refunds: 500}
	if e.Total() != 18000 {
		t.Errorf("Total = %d, ожидалось 18000", e.Total())
	}
	_ = time.Now
}

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
