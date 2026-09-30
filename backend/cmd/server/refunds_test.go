package main

import "testing"

// З11-и1: как вернули деньги отмены — решения основателя 30.09: админ — тем же способом или «не возвращали»,
// другой способ — владелец; на кошелёк — только оплаченное кошельком; долг вернуть нельзя.
func TestPickRefund(t *testing.T) {
	cases := []struct {
		paid, asked string
		owner       bool
		want, code  string
	}{
		{"cash", "", false, "cash", ""}, // не сказали — как платили
		{"card", "", false, "card", ""},
		{"wallet", "", false, "wallet", ""},
		{"", "", false, "none", ""},      // долг: денег не брали
		{"later", "", false, "none", ""}, // способ не из списка — из ящика не уходит
		{"cash", "cash", false, "cash", ""},
		{"cash", "none", false, "none", ""}, // «не возвращали» — и админу
		{"card", "none", false, "none", ""},
		{"card", "cash", false, "", "refund_owner_only"}, // картой платили — налом вернуть может владелец
		{"card", "cash", true, "cash", ""},
		{"cash", "blik", false, "", "refund_owner_only"},
		{"cash", "blik", true, "blik", ""},
		{"wallet", "cash", false, "", "refund_owner_only"},
		{"wallet", "cash", true, "cash", ""},
		{"cash", "wallet", false, "", "refund_wallet_only"}, // на кошелёк — только кошельковое, и владельцу
		{"cash", "wallet", true, "", "refund_wallet_only"},
		{"card", "wallet", true, "", "refund_wallet_only"},
		{"wallet", "wallet", false, "wallet", ""},
		{"", "cash", true, "", "refund_unpaid"},
		{"", "none", false, "none", ""},
		{"cash", "налом", true, "", "bad_refund_method"},
		{"cash", "CASH", true, "", "bad_refund_method"},
	}
	for _, c := range cases {
		got, code := pickRefund(c.paid, c.asked, c.owner)
		if got != c.want || code != c.code {
			t.Errorf("pickRefund(%q, %q, owner=%v) = %q, %q; ждали %q, %q", c.paid, c.asked, c.owner, got, code, c.want, c.code)
		}
	}
}

func TestRefundWords(t *testing.T) {
	for m, want := range map[string]string{"cash": "вернули наличными", "card": "вернули на карту", "blik": "вернули по BLIK",
		"wallet": "вернули на кошелёк", "none": "денег не возвращали"} {
		if got := refundRu(m); got != want {
			t.Errorf("refundRu(%q) = %q, ждали %q", m, got, want)
		}
	}
	for _, c := range []struct {
		refund int64
		method string
		want   string
	}{
		{1250, "cash", ", возврат 12.50 zł наличными"},
		{1250, "card", ", возврат 12.50 zł на карту"},
		{1250, "blik", ", возврат 12.50 zł по BLIK"},
		{1250, "wallet", ", возврат 12.50 zł на кошелёк гостя"},
		{0, "none", ", денег не возвращали"},
		{0, "cash", ""}, // всё отыграно — возвращать нечего
	} {
		if got := refundSuffix(c.refund, c.method); got != c.want {
			t.Errorf("refundSuffix(%d, %q) = %q, ждали %q", c.refund, c.method, got, c.want)
		}
	}
	if got := refundWhere("none"); got != "не возвращали" {
		t.Errorf("refundWhere(none) = %q", got)
	}
}
