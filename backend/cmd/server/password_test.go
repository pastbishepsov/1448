package main

import (
	"strings"
	"testing"
	"time"
)

// Е0-и2: временный пароль диктуют вслух через стойку — форма и алфавит важны
// не меньше энтропии.
func TestGenerateTempPassword(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		p, err := generateTempPassword()
		if err != nil {
			t.Fatalf("generateTempPassword: %v", err)
		}
		groups := strings.Split(p, "-")
		if len(groups) != tempPassGroups {
			t.Fatalf("групп %d, ждали %d (%q)", len(groups), tempPassGroups, p)
		}
		for _, g := range groups {
			if len(g) != tempPassGroupLen {
				t.Fatalf("группа %q длиной %d, ждали %d", g, len(g), tempPassGroupLen)
			}
		}
		// Похожие символы исключены: 0/O, 1/l/I, 5/S — иначе пароль
		// переспрашивают, и очередь у стойки стоит.
		for _, r := range strings.ReplaceAll(p, "-", "") {
			if !strings.ContainsRune(tempPassAlphabet, r) {
				t.Fatalf("символ %q вне алфавита (%q)", r, p)
			}
		}
		if strings.ContainsAny(p, "01lIiOoSs5") {
			t.Fatalf("в пароле похожие символы: %q", p)
		}
		if code, ok := validatePasswordLen(strings.ReplaceAll(p, "-", "")); !ok {
			t.Fatalf("временный пароль не проходит собственную валидацию: %s (%q)", code, p)
		}
		seen[p] = true
	}
	if len(seen) < 190 {
		t.Errorf("повторов слишком много: уникальных %d из 200", len(seen))
	}
}

func TestValidatePasswordLen(t *testing.T) {
	cases := []struct {
		in       string
		wantOK   bool
		wantCode string
	}{
		{"", false, "password_short"},
		{"12345", false, "password_short"},
		{"123456", true, ""},
		{strings.Repeat("x", 72), true, ""},
		{strings.Repeat("x", 73), false, "password_long"}, // bcrypt режет на 72 — молча не принимаем
	}
	for _, tc := range cases {
		code, ok := validatePasswordLen(tc.in)
		if ok != tc.wantOK || code != tc.wantCode {
			t.Errorf("validatePasswordLen(len=%d) = (%q, %v), ждали (%q, %v)",
				len(tc.in), code, ok, tc.wantCode, tc.wantOK)
		}
	}
}

// Отсечка округляется ВВЕРХ: секунда, в которую меняли пароль, умирает
// целиком. Округление вниз оставляло дыру — refresh, выданный в ту же секунду,
// что и сброс, переживал отсечку (поймано живым e2e).
func TestPasswordCutoff(t *testing.T) {
	base := time.Date(2026, 8, 24, 14, 48, 7, 0, time.UTC)
	cases := []struct {
		name string
		now  time.Time
		want time.Time
	}{
		{"ровно на секунде", base, base.Add(time.Second)},
		{"середина секунды", base.Add(400 * time.Millisecond), base.Add(time.Second)},
		{"почти следующая", base.Add(999 * time.Millisecond), base.Add(time.Second)},
		{"следующая секунда", base.Add(time.Second), base.Add(2 * time.Second)},
	}
	for _, tc := range cases {
		got := passwordCutoff(tc.now)
		if !got.Equal(tc.want) {
			t.Errorf("%s: passwordCutoff(%v) = %v, ждали %v", tc.name, tc.now, got, tc.want)
		}
		// Главный инвариант: токен, выданный в секунду смены (и раньше),
		// отсечку НЕ переживает; подписанный самой отсечкой — переживает.
		if !tokenIssuedBefore(tc.now.Unix(), &got) {
			t.Errorf("%s: токен из секунды смены пережил отсечку", tc.name)
		}
		if tokenIssuedBefore(got.Unix(), &got) {
			t.Errorf("%s: пара, подписанная отсечкой, себя же и отвергла", tc.name)
		}
	}
}

// Отсечка токенов. Равенство секунд — «не раньше»: на нём держится пара,
// выданная в момент отсечки.
func TestTokenIssuedBefore(t *testing.T) {
	base := time.Date(2026, 8, 24, 14, 48, 0, 0, time.UTC)
	cases := []struct {
		name string
		iat  int64
		from *time.Time
		want bool
	}{
		{"отсечки нет — пускаем всех", base.Unix() - 9999, nil, false},
		{"выдан раньше отсечки", base.Unix() - 1, &base, true},
		{"выдан в ту же секунду", base.Unix(), &base, false},
		{"выдан позже отсечки", base.Unix() + 1, &base, false},
		{"iat прочитать не вышло", 0, &base, false},
		{"старый токен без iat и без отсечки", 0, nil, false},
	}
	for _, tc := range cases {
		if got := tokenIssuedBefore(tc.iat, tc.from); got != tc.want {
			t.Errorf("%s: got %v, ждали %v", tc.name, got, tc.want)
		}
	}
}
