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

// Ж6-и4: расхождение и текст владельцу. З1-и2: денежная строка — от системы,
// с ником сдавшего и без заметки; заметка — отдельно, без денег.
func TestCloseoutMessage(t *testing.T) {
	counted := func(g int64) *int64 { return &g }
	cases := []struct {
		name     string
		expected int64
		counted  *int64
		wantSub  string
	}{
		{"сошлось — строка с суммой (З1-и1)", 50000, counted(50000), "🧮 admin2 сдал смену: касса сошлась — в ящике 500.00 zł"},
		{"недостача", 50000, counted(48000), "в ящике 480.00 zł, должно быть 500.00 zł — расхождение −20.00 zł"},
		{"излишек", 50000, counted(50500), "расхождение +5.00 zł"},
		{"без пересчёта", 50000, nil, "🧮 admin2 сдал смену без пересчёта кассы (по операциям должно быть 500.00 zł)"},
	}
	for _, tc := range cases {
		got := closeoutMessage("admin2", tc.expected, tc.counted)
		if !contains(got, tc.wantSub) || contains(got, "Передал смену") {
			t.Errorf("%s: %q — ждали %q и без заметки", tc.name, got, tc.wantSub)
		}
	}
	if got := closeoutMessage("", 100, nil); !contains(got, "🧮 Сотрудник сдал смену") {
		t.Errorf("без ника: %q", got)
	}
	if got := closeoutNote("  сдал Ане  "); got != "Передал смену: сдал Ане" {
		t.Errorf("заметка: %q", got)
	}
	if got := closeoutNote("   "); got != "" {
		t.Errorf("пустая заметка — без сообщения, получили %q", got)
	}
	if d := closeoutDiff(50000, 48000); d != -2000 {
		t.Errorf("closeoutDiff = %d, ожидалось -2000", d)
	}
	// З1-и1: окно сдачи — от начала смены или от прошлой сдачи, если она позже
	day := time.Date(2026, 9, 23, 8, 0, 0, 0, time.UTC)
	entry := day.Add(-11 * time.Hour) // ночная: вчера 21:00
	early, late := day.Add(-12*time.Hour), day.Add(2*time.Hour)
	if got := closeoutFrom(&entry, nil, day); !got.Equal(entry) {
		t.Errorf("ночная смена: окно с %v, ожидалось с начала смены %v", got, entry)
	}
	if got := closeoutFrom(&entry, &early, day); !got.Equal(entry) {
		t.Errorf("сдача раньше смены окно не двигает: %v", got)
	}
	if got := closeoutFrom(&entry, &late, day); !got.Equal(late) {
		t.Errorf("сдача позже начала смены — окно с неё: %v", got)
	}
	if got := closeoutFrom(nil, nil, day); !got.Equal(day) {
		t.Errorf("без смены — с начала клубных суток: %v", got)
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
