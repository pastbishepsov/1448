package main

import "testing"

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
