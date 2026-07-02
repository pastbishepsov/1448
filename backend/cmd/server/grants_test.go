package main

import "testing"

func TestValidateGrant(t *testing.T) {
	// валидные
	if ok, _ := validateGrant("xp", 500, "", "компенсация за сбой"); !ok {
		t.Error("валидное XP-начисление отклонено")
	}
	if ok, _ := validateGrant("case", 0, "heavy", "приз турнира"); !ok {
		t.Error("валидное кейс-начисление отклонено")
	}

	cases := []struct {
		typ      string
		amount   int64
		tier     string
		reason   string
		wantCode string
	}{
		{"xp", 500, "", "   ", "reason_required"},   // причина из пробелов
		{"xp", 0, "", "x", "bad_amount"},            // ноль XP
		{"xp", -5, "", "x", "bad_amount"},           // отрицательный
		{"xp", 100001, "", "x", "bad_amount"},       // сверх лимита
		{"case", 0, "golden", "x", "bad_tier"},      // нет такого тира
		{"case", 0, "", "x", "bad_tier"},            // тир пуст
		{"coins", 100, "", "x", "bad_type"},         // монеты руками не даём
	}
	for _, tc := range cases {
		ok, code := validateGrant(tc.typ, tc.amount, tc.tier, tc.reason)
		if ok || code != tc.wantCode {
			t.Errorf("(%s,%d,%q,%q): ok=%v code=%q, ожидался %q",
				tc.typ, tc.amount, tc.tier, tc.reason, ok, code, tc.wantCode)
		}
	}

	// границы валидны
	if ok, _ := validateGrant("xp", 1, "", "x"); !ok {
		t.Error("1 XP отклонён")
	}
	if ok, _ := validateGrant("xp", 100000, "", "x"); !ok {
		t.Error("100000 XP отклонены")
	}
}
