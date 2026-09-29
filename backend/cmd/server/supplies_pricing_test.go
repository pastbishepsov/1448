package main

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// З10-и5: цены строк — каждая строка, где пришло больше нуля, ровно один раз; штук в сумме — сколько пришло.
func TestPricingPieces(t *testing.T) {
	a, b, z := uuid.UUID{15: 1}, uuid.UUID{15: 2}, uuid.UUID{15: 3}
	have := map[uuid.UUID]int{a: 30, b: 20, z: 0}
	names := map[uuid.UUID]string{a: "Cola", b: "Lay's", z: "Ноль"}
	pp := func(p ...pricePieceReq) []pricePieceReq { return p }
	ok := []priceLineReq{{a.String(), pp(pricePieceReq{20, 2.5}, pricePieceReq{10, 2.8})}, {b.String(), pp(pricePieceReq{20, 3.1})}}
	prices, total, fail := pricingPieces(ok, have, names)
	if fail != nil || total != 14000 || len(prices[a]) != 2 || prices[a][0] != (lotPrice{20, 250}) || prices[a][1] != (lotPrice{10, 280}) ||
		prices[b][0] != (lotPrice{20, 310}) {
		t.Fatalf("оценка: %v %d %+v", prices, total, fail)
	}
	cases := []struct {
		name string
		req  []priceLineReq
		code string
		msg  string
	}{
		{"нет строки", ok[:1], "pricing_lines", "«Lay's»: нет цен"},
		{"строка дважды", append(ok, ok[1]), "pricing_lines", "«Lay's»: строка — один раз"},
		{"чужая позиция", append(ok, priceLineReq{uuid.NewString(), pp(pricePieceReq{1, 1})}), "pricing_lines", "Этой позиции в поставке нет"},
		{"кривой id", []priceLineReq{{"кривой", pp(pricePieceReq{1, 1})}}, "pricing_lines", ""},
		{"штук меньше, чем пришло", []priceLineReq{{a.String(), pp(pricePieceReq{28, 2.5})}, ok[1]}, "pricing_qty", "«Cola»: цен на 28 шт, а пришло 30"},
		{"цен у строки нет", []priceLineReq{{a.String(), nil}, ok[1]}, "bad_prices", "«Cola»:"},
		{"цена меньше нуля", []priceLineReq{{a.String(), pp(pricePieceReq{30, -1})}, ok[1]}, "bad_prices", ""},
		{"три знака после запятой", []priceLineReq{{a.String(), pp(pricePieceReq{30, 2.555})}, ok[1]}, "bad_prices", ""},
		{"кусок без штук", []priceLineReq{{a.String(), pp(pricePieceReq{0, 1}, pricePieceReq{30, 1})}, ok[1]}, "bad_prices", ""},
		{"цена выше потолка", []priceLineReq{{a.String(), pp(pricePieceReq{30, maxSupplyPricePLN + 1})}, ok[1]}, "bad_prices", ""},
		{"строка в ноль с ценами", append(ok, priceLineReq{z.String(), pp(pricePieceReq{1, 1})}), "pricing_qty", "«Ноль»: цен на 1 шт, а пришло 0"},
		{"сумма поставки выше потолка", []priceLineReq{{a.String(), pp(pricePieceReq{30, 40000})}, ok[1]}, "pricing_total", ""},
	}
	for _, c := range cases {
		_, _, fail := pricingPieces(c.req, have, names)
		if fail == nil || fail.Code != c.code || !strings.Contains(fail.Message, c.msg) {
			t.Errorf("%s: %+v, ждали %s «%s»", c.name, fail, c.code, c.msg)
		}
	}
	many := []pricePieceReq{}
	for i := 0; i < maxPricePieces+1; i++ {
		many = append(many, pricePieceReq{1, 1})
	}
	if _, _, fail := pricingPieces([]priceLineReq{{a.String(), many}, ok[1]}, have, names); fail == nil || fail.Code != "bad_prices" {
		t.Errorf("11 цен у строки: %+v", fail)
	}
	if _, total, fail := pricingPieces([]priceLineReq{{a.String(), pp(pricePieceReq{30, 0})}, ok[1]}, have, names); fail != nil || total != 6200 {
		t.Errorf("цена ноль (подарок поставщика): %d %+v", total, fail)
	}
	if prices, _, fail := pricingPieces(append(ok, priceLineReq{z.String(), nil}), have, names); fail != nil || prices[z] != nil {
		t.Errorf("строка в ноль без цен — можно прислать: %v %+v", prices[z], fail)
	}
}

// З10-и5: документ поставки — способ обязателен, «оплачен» по умолчанию, VAT не больше суммы.
func TestPricingDoc(t *testing.T) {
	today := time.Date(2026, 9, 29, 0, 0, 0, 0, time.Local)
	s := func(v string) *string { return &v }
	f := func(v float64) *float64 { return &v }
	no := false
	d, code := pricingDocOf(pricingRequest{Method: s("bank"), DocNo: s(" FV 12/09 "), DocDate: s("2026-09-28"), VatPLN: f(23), CounterpartyID: s("")}, today, 14000)
	if code != "" || d.DocNo != "FV 12/09" || d.DocDate == nil || d.DocDate.Format("2006-01-02") != "2026-09-28" || !d.Paid || d.Vat != 2300 ||
		d.CategoryID != nil || d.CounterpartyID != nil {
		t.Fatalf("документ: %+v %s", d, code)
	}
	if d, code := pricingDocOf(pricingRequest{Method: s("till"), Paid: &no}, today, 100); code != "" || d.Paid || d.DocDate != nil {
		t.Errorf("не оплачен, без даты: %+v %s", d, code)
	}
	cases := []struct {
		name string
		req  pricingRequest
		code string
	}{
		{"без способа", pricingRequest{}, "bad_method"},
		{"чужой способ", pricingRequest{Method: s("cash")}, "bad_method"},
		{"номер длиннее 64", pricingRequest{Method: s("bank"), DocNo: s(strings.Repeat("я", maxExpenseDoc+1))}, "bad_doc"},
		{"кривая дата", pricingRequest{Method: s("bank"), DocDate: s("2026-13-01")}, "bad_doc_date"},
		{"дата шесть лет назад", pricingRequest{Method: s("bank"), DocDate: s("2020-09-28")}, "bad_doc_date"},
		{"VAT больше суммы", pricingRequest{Method: s("bank"), VatPLN: f(141)}, "bad_vat"},
		{"VAT меньше нуля", pricingRequest{Method: s("bank"), VatPLN: f(-1)}, "bad_vat"},
		{"VAT три знака", pricingRequest{Method: s("bank"), VatPLN: f(1.005)}, "bad_vat"},
		{"кривая категория", pricingRequest{Method: s("bank"), CategoryID: s("кривая")}, "bad_category"},
		{"кривой контрагент", pricingRequest{Method: s("bank"), CounterpartyID: s("кривой")}, "bad_counterparty"},
	}
	for _, c := range cases {
		if _, code := pricingDocOf(c.req, today, 14000); code != c.code {
			t.Errorf("%s: %q, ждали %q", c.name, code, c.code)
		}
	}
}

// З10-и5: движения строки для разбивки — правки приёмки складываются с приходом, возврат знает своё списание.
func TestLineEntries(t *testing.T) {
	m := func(n byte) uuid.UUID { return uuid.UUID{15: n} }
	sale, order := m(20), m(21)
	moves := []lineMove{
		{ID: m(1), Reason: "supply", Qty: 35},                           // приёмка
		{ID: m(2), Reason: "sale", SaleID: &sale, Qty: -30},             // продажа
		{ID: m(3), Reason: "order", KitchenOrderID: &order, Qty: -2},    // заказ кухни
		{ID: m(4), Reason: "void", SaleID: &sale, Qty: 25},              // отмена продажи
		{ID: m(5), Reason: "supply", Qty: -25},                          // правка приёмки 35 → 10
		{ID: m(6), Reason: "void", KitchenOrderID: &order, Qty: 2},      // отмена заказа
		{ID: m(7), Reason: "void", SaleID: &uuid.UUID{15: 99}, Qty: 1},  // продажа до партий
		{ID: m(8), Reason: "adjust", Qty: -1},                           // «бой»
		{ID: m(9), Reason: "supply", Qty: 2},                            // правка приёмки вверх
		{ID: m(10), Reason: "sale", SaleID: &uuid.UUID{15: 98}, Qty: 0}, // ничего по строке
	}
	got, err := lineEntries(moves, 12)
	want := []lotEntry{{Move: m(1), Qty: 12, In: true}, {Move: m(2), Qty: -30}, {Move: m(3), Qty: -2}, {Move: m(4), Qty: 25, Source: &moves[1].ID},
		{Move: m(6), Qty: 2, Source: &moves[2].ID}, {Move: m(7), Qty: 1}, {Move: m(8), Qty: -1}}
	if err != nil || len(got) != len(want) {
		t.Fatalf("движения: %+v %v", got, err)
	}
	for i := range want {
		g, w := got[i], want[i]
		if g.Move != w.Move || g.Qty != w.Qty || g.In != w.In || (g.Source == nil) != (w.Source == nil) || (g.Source != nil && *g.Source != *w.Source) {
			t.Errorf("движение %d: %+v, ждали %+v", i, g, w)
		}
	}
	// и разбивка по ним: пришло 12 (35 − 25 + 2), продано и не вернулось 5 + «бой» 1, вернулось без источника 1
	pieces, _, err := lotSplit([]lotPrice{{8, 250}, {4, 280}}, got)
	if err != nil || piecesStr(pieces) != "3/8@250 4/4@280 " {
		t.Errorf("разбивка: %q %v", piecesStr(pieces), err)
	}
	if _, err := lineEntries([]lineMove{{ID: m(2), Reason: "sale", Qty: -1}}, 1); !errors.Is(err, errLotSplit) {
		t.Errorf("без прихода: %v", err)
	}
}
