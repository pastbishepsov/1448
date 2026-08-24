package main

import (
	"testing"
	"time"
)

func strptr(s string) *string { return &s }
func intptr(i int) *int       { return &i }

func TestValidateProfilePatch(t *testing.T) {
	cases := []struct {
		name     string
		nickname *string
		avatar   *int
		wantOK   bool
		wantCode string
	}{
		{"ник + аватар ок", strptr("egor"), intptr(3), true, ""},
		{"только ник", strptr("egor"), nil, true, ""},
		{"только аватар", nil, intptr(1), true, ""},
		{"ничего", nil, nil, false, "empty"},
		{"ник короткий", strptr("ab"), nil, false, "nickname_short"},
		{"ник из пробелов", strptr("   x  "), nil, false, "nickname_short"},
		{"кириллица 4 символа ок", strptr("Егор"), nil, true, ""},
		{"кириллица 2 символа коротко", strptr("Ег"), nil, false, "nickname_short"},
		{"аватар 0 нельзя", nil, intptr(0), false, "avatar_invalid"},
		{"аватар выше максимума", nil, intptr(13), false, "avatar_invalid"},
		{"аватар на границе", nil, intptr(12), true, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ok, code := validateProfilePatch(tc.nickname, tc.avatar)
			if ok != tc.wantOK || code != tc.wantCode {
				t.Errorf("= (%v,%q), хотим (%v,%q)", ok, code, tc.wantOK, tc.wantCode)
			}
		})
	}
}

func TestNicknameSafe(t *testing.T) {
	cases := []struct {
		name string
		nick string
		ok   bool
	}{
		{"обычный латинский", "gustav_q", true},
		{"кириллица и дефис", "Гость-77", true},
		{"точка и цифры", "user.42", true},
		{"одинарная кавычка — инъекция в onclick", "x',alert(1),'", false},
		{"двойная кавычка — разрыв атрибута", `a"b_cd`, false},
		{"угловые скобки — тег", "<img_src=x>", false},
		{"бэктик", "aa`bb", false},
		{"бэкслэш", "aa\\bb", false},
		{"амперсанд", "a&b_c", false},
		{"управляющий символ", "ab\x1bcd", false},
	}
	for _, tc := range cases {
		if got := nicknameSafe(tc.nick); got != tc.ok {
			t.Errorf("%s: nicknameSafe(%q) = %v, ожидалось %v", tc.name, tc.nick, got, tc.ok)
		}
	}
}

// ── Г8: валидации анкеты ──────────────────────────────────────────────

func TestValidateBirthDate(t *testing.T) {
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	if _, ok := validateBirthDate("2010-05-10", now); !ok {
		t.Error("16 лет — должно пройти")
	}
	if _, ok := validateBirthDate("2021-08-25", now); ok {
		t.Error("4 года (ещё нет 6) — должно отбиться")
	}
	if _, ok := validateBirthDate("2020-08-24", now); !ok {
		t.Error("ровно 6 лет сегодня — должно пройти")
	}
	if _, ok := validateBirthDate("1920-01-01", now); ok {
		t.Error("106 лет — должно отбиться")
	}
	if _, ok := validateBirthDate("10.05.2010", now); ok {
		t.Error("кривой формат — должно отбиться")
	}
}

func TestValidateHandle(t *testing.T) {
	if h, ok := validateHandle("@kotik_1448"); !ok || h != "kotik_1448" {
		t.Errorf("@ должен срезаться: %q %v", h, ok)
	}
	if _, ok := validateHandle("a"); ok {
		t.Error("1 символ — должно отбиться")
	}
	if _, ok := validateHandle("есть пробел"); ok {
		t.Error("пробел — должно отбиться")
	}
	if _, ok := validateHandle(`ник"с кавычкой`); ok {
		t.Error("кавычка — должно отбиться")
	}
}

func TestValidateFavorites(t *testing.T) {
	games := map[string]bool{"cs2": true, "dota2": true, "valorant": true, "fortnite": true}
	if out, ok := validateFavorites([]string{"cs2", "dota2"}, games); !ok || len(out) != 2 {
		t.Error("две игры из каталога — должно пройти")
	}
	if _, ok := validateFavorites([]string{"cs2", "dota2", "valorant", "fortnite"}, games); ok {
		t.Error("4 игры — больше лимита, должно отбиться")
	}
	if _, ok := validateFavorites([]string{"cs2", "cs2"}, games); ok {
		t.Error("дубль — должно отбиться")
	}
	if _, ok := validateFavorites([]string{"minecraft"}, games); ok {
		t.Error("не из каталога — должно отбиться")
	}
	if out, ok := validateFavorites([]string{}, games); !ok || len(out) != 0 {
		t.Error("пустой список (стирание) — должно пройти")
	}
}

func TestIsLeapYear(t *testing.T) {
	for y, want := range map[int]bool{2024: true, 2026: false, 2000: true, 1900: false} {
		if isLeapYear(y) != want {
			t.Errorf("isLeapYear(%d) != %v", y, want)
		}
	}
}
