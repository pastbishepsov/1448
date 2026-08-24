package main

import (
	"testing"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

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
		"",                      // пусто — чистка идёт отдельной веткой, не сюда
		"   ",                   // пробелы = пусто
		"guest",                 // без домена
		"guest@",                // без домена
		"@1448.pl",              // без локальной части
		"Гость <guest@1448.pl>", // форма с именем — в колонке нужен чистый адрес
		"guest@1448.pl, a@b.pl", // два адреса
		"gu est@1448.pl",        // пробел внутри
		"gu\"est@1448.pl",       // кавычка — nicknameSafe
		"guest<script>@1448.pl", // угловые скобки — nicknameSafe
		"guest@1448.pl\x00",     // управляющий символ
		"g'uest@1448.pl",        // прямой апостроф — nicknameSafe
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

// ── Е0-и3: поиск по нику, телефону и имени ────────────────────────────────

func TestEscapeLike(t *testing.T) {
	cases := map[string]string{
		"Ковальский": "Ковальский",
		"50%":        `50\%`,
		"a_b":        `a\_b`,
		`c\d`:        `c\\d`,
		"%_%":        `\%\_\%`,
	}
	for in, want := range cases {
		if got := escapeLike(in); got != want {
			t.Errorf("escapeLike(%q) = %q, ждали %q", in, got, want)
		}
	}
}

func TestPhoneDigits(t *testing.T) {
	cases := map[string]string{
		"+48 123-456-789": "48123456789",
		"123 456 789":     "123456789",
		"(48) 123456789":  "48123456789",
		"6789":            "6789", // хвост номера — минимально осмысленный запрос
		"789":             "",     // короче порога: «77» из ника тянуло бы пол-клуба
		"Ковальский":      "",
		"Гость-77":        "",
		"":                "",
	}
	for in, want := range cases {
		if got := phoneDigits(in); got != want {
			t.Errorf("phoneDigits(%q) = %q, ждали %q", in, got, want)
		}
	}
}

func TestGuestMatchReason(t *testing.T) {
	s := func(v string) *string { return &v }
	u := &models.User{
		Nickname:  "KowalPro",
		Phone:     s("+48123456789"),
		FirstName: s("Ян"),
		LastName:  s("Ковальский"),
	}
	cases := []struct {
		query string
		want  string
	}{
		{"kowal", "ник"},         // регистронезависимо
		{"KowalPro", "ник"},      //
		{"123456789", "телефон"}, // цифра к цифре
		{"+48 123-456-789", "телефон"},
		{"6789", "телефон"}, // хвост номера
		{"Ян", "имя"},
		{"ковальский", "фамилия"}, // регистр не важен
		{"Ян Ковальский", "имя и фамилия"},
		{"Новак", ""}, // не нашёлся ничем
		{"", ""},      // пустой запрос — причины нет
		{"   ", ""},
	}
	for _, tc := range cases {
		if got := guestMatchReason(u, tc.query); got != tc.want {
			t.Errorf("guestMatchReason(%q) = %q, ждали %q", tc.query, got, tc.want)
		}
	}

	// Ник главнее: цифры в нике не должны выдаваться за телефон.
	digits := &models.User{Nickname: "Гость-7777", Phone: s("+48999888777")}
	if got := guestMatchReason(digits, "7777"); got != "ник" {
		t.Errorf("совпадение по нику должно быть точнее телефона, получили %q", got)
	}
	// Пустые поля не роняют и не дают ложных совпадений.
	empty := &models.User{Nickname: "NoData"}
	for _, q := range []string{"Ян", "123456789", " "} {
		if got := guestMatchReason(empty, q); got != "" {
			t.Errorf("гость без данных совпал с %q как %q", q, got)
		}
	}
}
