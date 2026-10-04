package main

import (
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
	if settingDefaults["acc_vat_moment"] != 0 || settingDefaults["acc_vat_payer"] != 1 || settingDefaults["acc_topup_vat"] != 23 {
		t.Errorf("значения по умолчанию: VAT при трате, плательщик VAT, ставка пополнения 23 %%")
	}
}
