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

// Отсечка берётся БЕЗ округления: точность даёт claim iat_ms. Оба прошлых
// захода (округление вниз, потом вверх) ломались о гранулярность секунды —
// история в комментарии к passwordCutoff.
func TestPasswordCutoff(t *testing.T) {
	base := time.Date(2026, 8, 24, 14, 48, 7, 400*int(time.Millisecond), time.UTC)
	if got := passwordCutoff(base); !got.Equal(base) {
		t.Errorf("passwordCutoff(%v) = %v, ждали тот же момент", base, got)
	}
}

// Главные инварианты отсечки: выданное РАНЬШЕ мертво, выданное ПОЗЖЕ и в тот
// же момент — живо, даже если всё уместилось в одну секунду.
func TestTokenIssuedBeforeMillis(t *testing.T) {
	cut := time.Date(2026, 8, 24, 14, 48, 7, 400*int(time.Millisecond), time.UTC)
	sec := cut.Unix()
	cases := []struct {
		name  string
		iatMs int64
		want  bool
	}{
		{"выдан за 300 мс до отсечки — мёртв", cut.UnixMilli() - 300, true},
		{"выдан на 1 мс раньше — мёртв", cut.UnixMilli() - 1, true},
		{"подписан моментом отсечки — жив", cut.UnixMilli(), false},
		{"выдан на 1 мс позже — жив", cut.UnixMilli() + 1, false},
		// Ровно этот случай ломался: вход в ту же секунду, что и сброс.
		{"вход через 200 мс после сброса — жив", cut.UnixMilli() + 200, false},
	}
	for _, tc := range cases {
		if got := tokenIssuedBefore(sec, tc.iatMs, &cut); got != tc.want {
			t.Errorf("%s: got %v, ждали %v", tc.name, got, tc.want)
		}
	}
}

// Легаси-токены (выданы до Е0-и5в, без iat_ms) судим по секундам и решаем в
// пользу безопасности: такой токен старый по определению, и лишний перелогин
// дешевле доступа, пережившего сброс пароля.
func TestTokenIssuedBeforeLegacy(t *testing.T) {
	cut := time.Date(2026, 8, 24, 14, 48, 7, 0, time.UTC)
	cases := []struct {
		name string
		iat  int64
		from *time.Time
		want bool
	}{
		{"отсечки нет — пускаем всех", cut.Unix() - 9999, nil, false},
		{"выдан раньше отсечки — мёртв", cut.Unix() - 1, &cut, true},
		{"та же секунда — считаем старым (без миллисекунд не отличить)", cut.Unix(), &cut, true},
		{"выдан позже отсечки — жив", cut.Unix() + 1, &cut, false},
		{"ни iat, ни iat_ms — судить не по чему", 0, &cut, false},
		{"нет ни того, ни отсечки", 0, nil, false},
	}
	for _, tc := range cases {
		if got := tokenIssuedBefore(tc.iat, 0, tc.from); got != tc.want {
			t.Errorf("%s: got %v, ждали %v", tc.name, got, tc.want)
		}
	}
}
