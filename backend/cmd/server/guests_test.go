package main

import "testing"

// Е0-и1: e-mail для правки у стойки. Инвариант тот же, что у ника после
// QA Б9–Б11: значение едет в разметку админки, поэтому кавычки и угловые
// скобки не проходят даже там, где RFC их формально допускает.
func TestValidateEmail(t *testing.T) {
	good := map[string]string{
		"guest@1448.pl":            "guest@1448.pl",
		"  guest@1448.pl  ":        "guest@1448.pl", // пробелы по краям срезаются
		"a.b+tag@sub.example.com":  "a.b+tag@sub.example.com",
		"ПользовательЪ@1448.pl":    "ПользовательЪ@1448.pl", // юникод в локальной части
		"long.name-1448@gmail.com": "long.name-1448@gmail.com",
	}
	for in, want := range good {
		got, ok := validateEmail(in)
		if !ok || got != want {
			t.Errorf("validateEmail(%q) = (%q, %v), ждали (%q, true)", in, got, ok, want)
		}
	}

	bad := []string{
		"",                       // пусто — чистка идёт отдельной веткой, не сюда
		"   ",                    // пробелы = пусто
		"guest",                  // без домена
		"guest@",                 // без домена
		"@1448.pl",               // без локальной части
		"Гость <guest@1448.pl>",  // форма с именем — в колонке нужен чистый адрес
		"guest@1448.pl, a@b.pl",  // два адреса
		"gu est@1448.pl",         // пробел внутри
		"gu\"est@1448.pl",        // кавычка — nicknameSafe
		"guest<script>@1448.pl",  // угловые скобки — nicknameSafe
		"guest@1448.pl\x00",      // управляющий символ
		"g'uest@1448.pl",         // прямой апостроф — nicknameSafe
	}
	for _, in := range bad {
		if got, ok := validateEmail(in); ok {
			t.Errorf("validateEmail(%q) = (%q, true), ждали отказ", in, got)
		}
	}

	// длина: колонка email — 255
	long := make([]byte, 250)
	for i := range long {
		long[i] = 'a'
	}
	if _, ok := validateEmail(string(long) + "@1448.pl"); ok {
		t.Error("слишком длинный e-mail прошёл")
	}
}

// guestTakenReason — точный код только когда уникальное поле в патче одно;
// иначе честное «одно из», а не угадывание.
func TestGuestTakenReason(t *testing.T) {
	cases := []struct {
		name     string
		updates  map[string]any
		wantCode string
	}{
		{"только ник", map[string]any{"nickname": "egor"}, "nickname_taken"},
		{"только телефон", map[string]any{"phone": "+48123456789"}, "phone_taken"},
		{"только e-mail", map[string]any{"email": "a@b.pl"}, "email_taken"},
		{"ник и телефон", map[string]any{"nickname": "egor", "phone": "+48123456789"}, "taken"},
		{"все три", map[string]any{"nickname": "e", "phone": "+48123456789", "email": "a@b.pl"}, "taken"},
		{"ни одного уникального", map[string]any{"first_name": "Егор"}, "taken"},
		// стирание (nil) не может упереться в уникальность — поле не считаем
		{"телефон стирают, ник меняют", map[string]any{"nickname": "egor", "phone": nil}, "nickname_taken"},
		{"стирают только телефон", map[string]any{"phone": nil}, "taken"},
	}
	for _, tc := range cases {
		code, msg := guestTakenReason(tc.updates)
		if code != tc.wantCode {
			t.Errorf("%s: код %q, ждали %q", tc.name, code, tc.wantCode)
		}
		if msg == "" {
			t.Errorf("%s: пустое сообщение", tc.name)
		}
	}
}
