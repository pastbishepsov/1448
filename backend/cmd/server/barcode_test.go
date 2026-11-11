package main

import "testing"

// З23-и5а: штрихкод позиции — EAN-13 и EAN-8 с контрольной цифрой, UPC-A как EAN-13, строка журнала.

func TestBarcodeNorm(t *testing.T) {
	cases := []struct {
		in, want string
		ok       bool
	}{
		{"5901234123457", "5901234123457", true},     // EAN-13, польский префикс 590
		{" 590 1234 123457 ", "5901234123457", true}, // пробелы сканера
		{"96385074", "96385074", true},               // EAN-8
		{"036000291452", "0036000291452", true},      // UPC-A — EAN-13 с ведущим нулём
		{"", "", true},                               // штрихкода нет
		{"   ", "", true},
		{"5901234123458", "", false}, // контрольная не та
		{"96385075", "", false},
		{"590123412345", "", false},   // 12 цифр — как UPC-A контрольная не сходится
		{"59012341234", "", false},    // 11 цифр
		{"59012341234571", "", false}, // 14 цифр
		{"590123412345A", "", false},
		{"-5901234123457", "", false},
	}
	for _, tc := range cases {
		got, ok := barcodeNorm(tc.in)
		if got != tc.want || ok != tc.ok {
			t.Errorf("barcodeNorm(%q) = %q, %v; ожидалось %q, %v", tc.in, got, ok, tc.want, tc.ok)
		}
	}
	if !eanValid("4006381333931") || eanValid("4006381333932") || !eanValid("40170725") {
		t.Error("контрольная EAN: 4006381333931 и 40170725 — верные, 4006381333932 — нет")
	}
}

func TestBarcodeChange(t *testing.T) {
	a, b := "5901234123457", "96385074"
	cases := []struct {
		was  *string
		now  string
		want string
	}{
		{nil, "", ""},
		{&a, a, ""},
		{nil, a, "штрихкод: 5901234123457"},
		{&a, "", "штрихкод убран (был 5901234123457)"},
		{&a, b, "штрихкод 5901234123457 → 96385074"},
	}
	for _, tc := range cases {
		if got := barcodeChange(tc.was, tc.now); got != tc.want {
			t.Errorf("barcodeChange: %q, ожидалось %q", got, tc.want)
		}
	}
	if barcodePtr("") != nil || *barcodePtr(a) != a {
		t.Error("barcodePtr: пусто — nil, код — указатель на него")
	}
}
