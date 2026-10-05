package main

import (
	"fmt"
	"testing"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

// З13-и1: ставку берём только из списка; не прислана — значение по умолчанию (у новой — 23, у старой — прежняя).
func TestVatFromRequest(t *testing.T) {
	v := func(x int) *int { return &x }
	cases := []struct {
		in     *int
		def    int
		want   int
		wantOK bool
	}{
		{nil, models.VatDefault, 23, true},
		{nil, 8, 8, true},
		{v(8), 23, 8, true},
		{v(5), 23, 5, true},
		{v(0), 23, 0, true},
		{v(models.VatZW), 23, models.VatZW, true},
		{v(7), 23, 7, false},
		{v(22), 23, 22, false},
		{v(-2), 23, -2, false},
		{v(100), 23, 100, false},
	}
	for _, tc := range cases {
		got, ok := vatFromRequest(tc.in, tc.def)
		if got != tc.want || ok != tc.wantOK {
			t.Errorf("vatFromRequest(%v, %d) = %d, %v; ждали %d, %v", tc.in, tc.def, got, ok, tc.want, tc.wantOK)
		}
	}
}

// З13-и1: подписи ставок и текст ошибки — из одного списка; настройка «ставка пополнения» — только из него.
func TestVatLabelsAndChoices(t *testing.T) {
	if got := vatRatesText(); got != "23 %, 8 %, 5 %, 0 %, zw" {
		t.Errorf("vatRatesText() = %q", got)
	}
	choose := settingChoices["acc_topup_vat"]
	if choose == nil || !choose(23) || !choose(-1) || choose(7) {
		t.Errorf("acc_topup_vat принимает не те ставки")
	}
	for key := range settingChoices {
		if _, ok := settingBounds[key]; !ok {
			t.Errorf("settingChoices[%q] без settingBounds", key)
		}
	}
	if settingDefaults["acc_vat_moment"] != 0 || settingDefaults["acc_vat_payer"] != 1 || settingDefaults["acc_topup_vat"] != 23 ||
		settingDefaults["acc_gift_vat"] != 0 {
		t.Errorf("значения по умолчанию: VAT при трате, плательщик VAT, ставка пополнения 23 %%, подарки без VAT (З13-и3)")
	}
}

// З13-и3: VAT внутри брутто — как в дневном отчёте кассы: до гроша, половина — от нуля, у минуса — так же; zw и 0 % — ноль.
func TestVatOf(t *testing.T) {
	cases := []struct {
		gross int64
		r     int
		want  int64
	}{
		{12300, 23, 2300},   // 123 zł по 23 % — 23 zł
		{10800, 8, 800},     // 108 zł по 8 % — 8 zł
		{10500, 5, 500},     // 105 zł по 5 % — 5 zł
		{100, 23, 19},       // 18,699… → 19 грошей
		{1000, 5, 48},       // 47,619… → 48
		{1000, 8, 74},       // 74,074… → 74
		{-12300, 23, -2300}, // возврат — тем же счётом со знаком
		{-100, 23, -19},
		{5000, 0, 0},
		{5000, models.VatZW, 0},
		{0, 23, 0},
	}
	for _, tc := range cases {
		if got := vatOf(tc.gross, tc.r); got != tc.want {
			t.Errorf("vatOf(%d, %d) = %d; ждали %d", tc.gross, tc.r, got, tc.want)
		}
	}
}

// З13-и3: строки — в дни и ставки; VAT дня по ставке — из брутто дня, за период — сумма дней (не VAT от суммы);
// освобождённому от VAT всё — zw, VAT ноль; виды и способы — брутто со знаком.
func TestVatSum(t *testing.T) {
	rows := []vatRow{
		{Kind: "sales", D: "2026-10-01", Vat: 23, Method: "card", G: 100, N: 1},
		{Kind: "sales", D: "2026-10-01", Vat: 23, Method: "cash", G: 100, N: 1},
		{Kind: "sales", D: "2026-10-02", Vat: 23, Method: "card", G: 100, N: 1},
		{Kind: "time", D: "2026-10-02", Vat: 8, Method: "wallet", G: 1080, N: 2},
		{Kind: "returns", D: "2026-10-02", Vat: 23, Method: "cash", G: -100, N: 1},
	}
	a := vatSum(rows, true)
	d1, d2 := a.Days["2026-10-01"][23], a.Days["2026-10-02"][23]
	if d1 == nil || d1.Gross != 200 || d1.Vat != 37 || d2 == nil || d2.Gross != 0 || d2.Vat != 0 {
		t.Fatalf("дни по 23 %%: %+v %+v", d1, d2)
	}
	if r := a.Rates[23]; r == nil || r.Gross != 200 || r.Vat != 37 || r.N != 4 {
		t.Errorf("23 %% за период — сумма дней: %+v", r)
	}
	if r := a.Rates[8]; r == nil || r.Gross != 1080 || r.Vat != 80 {
		t.Errorf("8 %%: %+v", r)
	}
	if k := a.Kinds["returns"]; k == nil || k.Gross != -100 || a.Kinds["sales"].Gross != 300 || a.Methods["cash"] != 0 || a.Methods["card"] != 200 {
		t.Errorf("виды и способы: %+v %+v", a.Kinds, a.Methods)
	}
	z := vatSum(rows, false)
	if len(z.Rates) != 1 || z.Rates[models.VatZW] == nil || z.Rates[models.VatZW].Gross != 1280 || z.Rates[models.VatZW].Vat != 0 {
		t.Errorf("освобождённый от VAT — всё zw: %+v", z.Rates)
	}
	if got := vatOrder(map[int]*vatCell{models.VatZW: {}, 0: {}, 5: {}, 8: {}, 23: {}, 7: {}, 12: {}}); fmt.Sprint(got) != "[23 8 5 0 -1 12 7]" {
		t.Errorf("порядок ставок: %v", got)
	}
}
