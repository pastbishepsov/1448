package main

import (
	"strings"
	"testing"
)

// З11-и5: причина подарка обязательна (решение основателя 30.09) — обрезается, пустая и слишком длинная — отказ.
func TestCompReason(t *testing.T) {
	cases := []struct {
		in, want, code string
	}{
		{"победа в турнире", "победа в турнире", ""},
		{"  извинение за PC-05  ", "извинение за PC-05", ""},
		{"", "", "comp_reason"},
		{"   ", "", "comp_reason"},
		{strings.Repeat("я", 200), strings.Repeat("я", 200), ""}, // 200 символов — можно, считаем буквы, а не байты
		{strings.Repeat("я", 201), "", "comp_reason_long"},
	}
	for _, c := range cases {
		got, code := compReason(c.in)
		if got != c.want || code != c.code {
			t.Errorf("compReason(%q) = %q, %q; ждали %q, %q", c.in, got, code, c.want, c.code)
		}
	}
}

// Отмена подарка: денег не брали — вернуть нечего, только «не возвращали», и владельцу тоже (иначе подарок
// «вернули бы» наличными из ящика).
func TestPickRefundGift(t *testing.T) {
	cases := []struct {
		asked      string
		owner      bool
		want, code string
	}{
		{"", false, "none", ""},
		{"none", false, "none", ""},
		{"cash", true, "", "refund_unpaid"},
		{"card", true, "", "refund_unpaid"},
		{"wallet", true, "", "refund_unpaid"},
		{"cash", false, "", "refund_unpaid"},
	}
	for _, c := range cases {
		got, code := pickRefund("comp", c.asked, c.owner)
		if got != c.want || code != c.code {
			t.Errorf("pickRefund(comp, %q, owner=%v) = %q, %q; ждали %q, %q", c.asked, c.owner, got, code, c.want, c.code)
		}
	}
}

// compText — строка журнала: «подарено: за что».
func TestCompText(t *testing.T) {
	r := "день рождения"
	if got := compText(&r); got != "подарено: день рождения" {
		t.Errorf("compText = %q", got)
	}
	if got := compText(nil); got != "подарено" {
		t.Errorf("compText(nil) = %q", got)
	}
}
