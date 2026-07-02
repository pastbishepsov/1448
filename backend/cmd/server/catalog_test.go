package main

import "testing"

func TestValidateCatalogApp(t *testing.T) {
	ok, _ := validateCatalogApp("cs2", "Counter-Strike 2", "game")
	if !ok {
		t.Error("валидное приложение отклонено")
	}
	cases := []struct {
		id, name, category, wantCode string
	}{
		{"CS2", "x", "game", "bad_id"},          // верхний регистр
		{"a", "x", "game", "bad_id"},            // слишком короткий
		{"тест", "x", "game", "bad_id"},         // не латиница
		{"has space", "x", "game", "bad_id"},    // пробел
		{"ok_id", "", "game", "bad_name"},       // пустое имя
		{"ok_id", "x", "games", "bad_category"}, // нет такой категории
	}
	for _, tc := range cases {
		ok, code := validateCatalogApp(tc.id, tc.name, tc.category)
		if ok || code != tc.wantCode {
			t.Errorf("(%q,%q,%q): ok=%v code=%q, ожидался %q", tc.id, tc.name, tc.category, ok, code, tc.wantCode)
		}
	}
}
