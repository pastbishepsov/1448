package main

import (
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

// З23-и3: инвентаризация — разбор строк акта, итоги, себестоимость недостачи, журнал, акт в ответе и файл акта.

func TestInventoryLines(t *testing.T) {
	g1, g2, lot := uuid.New(), uuid.New(), uuid.New().String()
	n := func(v int) *int { return &v }
	s := func(v string) *string { return &v }
	line := func(g uuid.UUID, exp, cnt *int, lotID *string) inventoryLineReq {
		return inventoryLineReq{GoodID: g.String(), Expected: exp, Counted: cnt, LotID: lotID}
	}
	cases := []struct {
		name  string
		req   []inventoryLineReq
		owner bool
		code  string
		at    int
	}{
		{"две позиции, недостача из партии у владельца", []inventoryLineReq{line(g1, n(5), n(3), s(lot)), line(g2, n(0), n(2), nil)}, true, "", -1},
		{"пустая партия — со старых партий первыми", []inventoryLineReq{line(g1, n(5), n(3), s(" "))}, false, "", -1},
		{"пустой акт", nil, true, "bad_lines", -1},
		{"позиция дважды", []inventoryLineReq{line(g1, n(1), n(1), nil), line(g1, n(1), n(0), nil)}, true, "dup_line", 1},
		{"без «насчитали»", []inventoryLineReq{line(g1, n(1), nil, nil)}, true, "bad_line", 0},
		{"без «в системе»", []inventoryLineReq{line(g2, n(1), n(1), nil), line(g1, nil, n(1), nil)}, true, "bad_line", 1},
		{"насчитали минус", []inventoryLineReq{line(g1, n(1), n(-1), nil)}, true, "bad_line", 0},
		{"больше потолка склада", []inventoryLineReq{line(g1, n(1), n(maxStockQty+1), nil)}, true, "bad_line", 0},
		{"кривая позиция", []inventoryLineReq{{GoodID: "pc-01", Expected: n(1), Counted: n(1)}}, true, "bad_line", 0},
		{"партию выбирает владелец", []inventoryLineReq{line(g1, n(5), n(3), s(lot))}, false, "lot_owner_only", 0},
		{"партия у излишка", []inventoryLineReq{line(g1, n(3), n(5), s(lot))}, true, "bad_inv_lot", 0},
		{"партия у сошедшейся", []inventoryLineReq{line(g1, n(3), n(3), s(lot))}, true, "bad_inv_lot", 0},
		{"кривая партия", []inventoryLineReq{line(g1, n(5), n(3), s("x"))}, true, "bad_inv_lot", 0},
	}
	for _, tc := range cases {
		got, code, at := inventoryLines(tc.req, tc.owner)
		if code != tc.code || at != tc.at {
			t.Errorf("%s: %q в строке %d, ожидалось %q в %d", tc.name, code, at, tc.code, tc.at)
		}
		if code == "" && len(got) != len(tc.req) {
			t.Errorf("%s: строк %d", tc.name, len(got))
		}
	}
	got, _, _ := inventoryLines([]inventoryLineReq{line(g1, n(5), n(3), s(lot)), line(g2, n(0), n(2), nil)}, true)
	if got[0].Lot == nil || got[0].Lot.String() != lot || got[1].Lot != nil || got[1].Counted != 2 {
		t.Errorf("разбор: %+v", got)
	}
	many := make([]inventoryLineReq, maxInventoryLines+1)
	if _, code, _ := inventoryLines(many, true); code != "bad_lines" {
		t.Errorf("больше %d строк — bad_lines: %q", maxInventoryLines, code)
	}
}

func invTestRows() []invRow {
	p200, p250 := int64(200), int64(250)
	at := time.Date(2026, 9, 28, 16, 40, 0, 0, time.UTC) // 18:40 в Варшаве
	return []invRow{
		{Name: "Cola", Category: "напитки", Expected: 12, Counted: 9, Grosz: 500, // недостача 3: 2 по 2,00 и 1 без цены
			Lots: []invLot{{Kind: models.LotSupply, At: at, Qty: 2, Grosz: &p200}, {Kind: models.LotFound, At: at, Qty: 1}}},
		{Name: "Lay's", Expected: 4, Counted: 4, Grosz: 800},
		{Name: "Snickers", Expected: 2, Counted: 5, Grosz: 350, Lots: []invLot{{Kind: models.LotFound, At: at, Qty: 3}}},
		{Name: "Water", Expected: 6, Counted: 5, Grosz: 300, Lots: []invLot{{Kind: models.LotSupply, At: at, Qty: 1, Grosz: &p250}}},
	}
}

func TestInvTotals(t *testing.T) {
	rows := invTestRows()
	if c, n := rows[0].cost(); c != 400 || n != 1 {
		t.Errorf("Cola: себестоимость 4,00 и 1 шт без цены: %d, %d", c, n)
	}
	if c, n := rows[2].cost(); c != 0 || n != 0 {
		t.Errorf("излишек — себестоимости недостачи нет: %d, %d", c, n)
	}
	s := invTotals(rows)
	want := invSum{Positions: 4, Matched: 1, ShortPos: 2, SurplusPos: 1, ShortUnits: 4, SurplusUnits: 3,
		ShortGrosz: 3*500 + 300, SurplusGrosz: 3 * 350, Expected: 24, Counted: 23, Cost: 650, NoCost: 1}
	if s != want {
		t.Errorf("итоги: %+v, ожидалось %+v", s, want)
	}
	if got := invAuditText(12, s, "бар"); got != "Инвентаризация №12 · 4 позиции: сошлось 1, недостача — 2 позиции (4 шт), излишек — 1 позиция (3 шт) · бар" {
		t.Errorf("журнал: %q", got)
	}
	if got := invAuditText(3, invTotals(rows[1:2]), ""); got != "Инвентаризация №3 · 1 позиция: сошлось 1" {
		t.Errorf("журнал, всё сошлось: %q", got)
	}
	if invMoveNote(12, "") != "инвентаризация №12" || invMoveNote(7, "бар") != "инвентаризация №7 · бар" {
		t.Errorf("примечание корректировки: %q, %q", invMoveNote(12, ""), invMoveNote(7, "бар"))
	}
}

func TestInvLotsText(t *testing.T) {
	rows := invTestRows()
	if got := invLotsText(rows[0].Lots, 0); got != "поставка от 28.09 18:40: 2 × 2,00; нашлось 28.09 18:40: 1 × без цены" {
		t.Errorf("партии по-русски: %q", got)
	}
	if got := invLotsText(rows[0].Lots, 1); got != "dostawa z 28.09 18:40: 2 × 2,00; nadwyżka z 28.09 18:40: 1 × bez ceny" {
		t.Errorf("партии по-польски: %q", got)
	}
	if invLotTitle(invLot{Kind: models.LotInitial}, 1) != "stan początkowy" || invLotsText(nil, 0) != "" {
		t.Errorf("начальный остаток, пусто")
	}
}

func TestInventoryOut(t *testing.T) {
	rows := invTestRows()
	inv := models.Inventory{ID: uuid.New(), No: 12, Note: "бар"}
	adm := inventoryOut(inv, "admin1", rows, false)
	l0 := adm["lines"].([]gin.H)[0]
	if _, ok := l0["cost_pln"]; ok {
		t.Errorf("админу — без себестоимости: %v", l0)
	}
	if _, ok := adm["totals"].(gin.H)["cost_pln"]; ok {
		t.Errorf("админу — без себестоимости в итогах")
	}
	if l0["delta"] != -3 || l0["delta_pln"] != -15.0 || l0["price_pln"] != 5.0 {
		t.Errorf("строка Cola: %v", l0)
	}
	own := inventoryOut(inv, "boss", rows, true)
	ls := own["lines"].([]gin.H)
	if ls[0]["cost_pln"] != 4.0 || ls[0]["no_cost_units"] != 1 || ls[2]["cost_pln"] != nil || ls[3]["cost_pln"] != 2.5 {
		t.Errorf("себестоимость владельцу: %v · %v · %v", ls[0], ls[2], ls[3])
	}
	unpriced := []invRow{{Name: "Tic", Expected: 2, Counted: 0, Grosz: 100, Lots: []invLot{{Kind: models.LotFound, Qty: 2}}}}
	if x := inventoryOut(inv, "boss", unpriced, true)["lines"].([]gin.H)[0]; x["cost_pln"] != nil || x["no_cost_units"] != 2 {
		t.Errorf("недостача только из партий без цены — себестоимости нет (null): %v", x)
	}
	tot := own["totals"].(gin.H)
	if tot["short_pln"] != 18.0 || tot["surplus_pln"] != 10.5 || tot["net_pln"] != -7.5 || tot["cost_pln"] != 6.5 || tot["no_cost_units"] != 1 {
		t.Errorf("итоги владельцу: %v", tot)
	}
}

func TestInventorySheets(t *testing.T) {
	rows := invTestRows()
	s := invTotals(rows)
	sh := inventorySheet(rows, s, 1)
	if sh.name != "Pozycje" || sh.head[4] != "Stan ewidencyjny" || sh.head[5] != "Stan rzeczywisty" || sh.n != 4 || len(sh.rows) != 5 {
		t.Errorf("лист позиций: %q %v n=%d строк %d", sh.name, sh.head, sh.n, len(sh.rows))
	}
	csv := string(csvBytes(sh, bookFormat{sep: ';', decimal: ','}))
	for _, want := range []string{
		"1;Cola;szt.;напитки;12;9;-3;5,00;-15,00;4,00;dostawa z 28.09 18:40: 2 × 2,00; nadwyżka z 28.09 18:40: 1 × bez ceny;",
		"2;Lay's;szt.;;4;4;0;8,00;0,00;;;",
		"3;Snickers;szt.;;2;5;3;3,50;10,50;;nadwyżka z 28.09 18:40: 3 × bez ceny;",
		"Razem;;;;24;23;-1;;-7,50;6,50",
	} {
		if !strings.Contains(strings.ReplaceAll(csv, "\"", ""), want) {
			t.Errorf("CSV без %q:\n%s", want, csv)
		}
	}
	inv := models.Inventory{No: 12, Note: "бар", CreatedAt: time.Date(2026, 11, 11, 13, 5, 0, 0, time.UTC)}
	ab := inventoryAbout(invFirm{"14:48", "Firma", "1234563218", "Warszawa"}, inv, "admin1", s, inv.CreatedAt, "boss", 0)
	text := string(csvBytes(ab, bookFormat{sep: ';', decimal: ','}))
	for _, want := range []string{"Акт инвентаризации;№ 12", "Дата и время пересчёта;11.11.2026 14:05", "Провёл;admin1",
		"Заметка;бар", "Недостача по себестоимости;6,50", "Недостача без себестоимости, штук;1", "Излишек по цене продажи;10,50",
		"Файл сделан;11.11.2026 14:05 · boss"} {
		if !strings.Contains(text, want) {
			t.Errorf("лист «Акт» без %q:\n%s", want, text)
		}
	}
	if ab.name != "Акт" || inventoryAbout(invFirm{}, inv, "", invTotals(rows[1:2]), inv.CreatedAt, "", 1).name != "Arkusz spisu" {
		t.Errorf("имя листа «Акт»")
	}
	if strings.Contains(string(csvBytes(inventoryAbout(invFirm{}, inv, "", invTotals(rows[1:2]), inv.CreatedAt, "", 0), bookFormat{sep: ';'})), "без себестоимости") {
		t.Errorf("всё с ценой — строки «без себестоимости» нет")
	}
	for i, w := range []string{"1448-arkusz-spisu-12-2026-11-11.xlsx", "1448-akt-inventarizacii-12-2026-11-11.zip"} {
		if got := inventoryFileName(12, inv.CreatedAt, []string{"xlsx", "csv"}[i], 1-i); got != w {
			t.Errorf("имя файла: %q, ожидалось %q", got, w)
		}
	}
	for _, k := range []string{"sheet_about", "sheet_lines", "h_n", "h_good", "how_v", "total"} {
		if iw(0, k) == "" || iw(1, k) == "" {
			t.Errorf("слово %q — на обоих языках", k)
		}
	}
}
