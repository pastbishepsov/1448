package main

import (
	"strings"
	"testing"
	"time"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

func TestCanChangeStaffRole(t *testing.T) {
	cases := []struct {
		name    string
		role    models.UserRole
		target  string
		actor   string
		promote bool
		ok      bool
		code    string
	}{
		{"гостя — в админы", models.UserRolePlayer, "u1", "o1", true, true, ""},
		{"админа — снять", models.UserRoleAdmin, "a1", "o1", false, true, ""},
		{"админа назначить нельзя (уже)", models.UserRoleAdmin, "a1", "o1", true, false, "already_staff"},
		{"гостя снять нельзя (не админ)", models.UserRolePlayer, "u1", "o1", false, false, "not_admin"},
		{"владельца не трогаем (назначить)", models.UserRoleOwner, "o2", "o1", true, false, "cannot_touch_owner"},
		{"владельца не трогаем (снять)", models.UserRoleOwner, "o2", "o1", false, false, "cannot_touch_owner"},
		{"себя не трогаем", models.UserRoleOwner, "o1", "o1", false, false, "cannot_touch_self"},
	}
	for _, tc := range cases {
		ok, code := canChangeStaffRole(tc.role, tc.target, tc.actor, tc.promote)
		if ok != tc.ok || code != tc.code {
			t.Errorf("%s: canChangeStaffRole(%s, %s, %s, %v) = (%v, %q), ожидалось (%v, %q)",
				tc.name, tc.role, tc.target, tc.actor, tc.promote, ok, code, tc.ok, tc.code)
		}
	}
}

func TestFutureShiftsFrom(t *testing.T) {
	// снятие среди дня: сегодняшняя смена остаётся в графике, чистим с завтра
	now := time.Date(2026, 8, 18, 15, 0, 0, 0, time.UTC)
	if got := futureShiftsFrom(now, 8); got.Format("2006-01-02") != "2026-08-19" {
		t.Errorf("днём: чистим с %s, ожидалось 2026-08-19", got.Format("2006-01-02"))
	}
	// снятие ночью до границы клубных суток: идут ещё вчерашние сутки,
	// значит «сегодня» это 17-е, а чистить надо с 18-го
	night := time.Date(2026, 8, 18, 3, 0, 0, 0, time.UTC)
	if got := futureShiftsFrom(night, 8); got.Format("2006-01-02") != "2026-08-18" {
		t.Errorf("ночью: чистим с %s, ожидалось 2026-08-18", got.Format("2006-01-02"))
	}
}

func dt(s string) *time.Time {
	d, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return &d
}

func TestValidateStaffProfile(t *testing.T) {
	cases := []struct {
		name      string
		full      string
		phone     string
		position  string
		rateType  string
		rate      float64
		hired     *time.Time
		dismissed *time.Time
		want      string
	}{
		{"пустая карточка — норм", "", "", "", "none", 0, nil, nil, ""},
		{"обычная", "Иван Ковальский", "+48 600 000 000", "старший смены", "hour", 32.5, dt("2026-01-15"), nil, ""},
		{"оклад", "А", "", "", "month", 6500, nil, nil, ""},
		{"ставка без суммы", "", "", "", "shift", 0, nil, nil, "rate_needed"},
		{"сумма без типа ставки", "", "", "", "none", 100, nil, nil, "rate_extra"},
		{"неизвестный тип ставки", "", "", "", "piecework", 10, nil, nil, "bad_rate_type"},
		{"отрицательная ставка", "", "", "", "hour", -5, nil, nil, "bad_rate"},
		{"ФИО длиннее 128", string(make([]rune, 129)), "", "", "none", 0, nil, nil, "bad_name"},
		{"телефон длиннее 32", "", string(make([]rune, 33)), "", "none", 0, nil, nil, "bad_phone"},
		{"уволен раньше, чем нанят", "", "", "", "none", 0, dt("2026-05-01"), dt("2026-04-01"), "bad_dates"},
		{"уволен в день найма — можно", "", "", "", "none", 0, dt("2026-05-01"), dt("2026-05-01"), ""},
	}
	for _, tc := range cases {
		_, code := validateStaffProfile(tc.full, tc.phone, tc.position, "", tc.rateType, tc.rate, tc.hired, tc.dismissed)
		if code != tc.want {
			t.Errorf("%s: validateStaffProfile = %q, ожидалось %q", tc.name, code, tc.want)
		}
	}
}

func TestParseDateOpt(t *testing.T) {
	if d, ok := parseDateOpt(""); !ok || d != nil {
		t.Errorf("пустая строка должна дать nil без ошибки, вышло (%v, %v)", d, ok)
	}
	if d, ok := parseDateOpt("  2026-08-18 "); !ok || d == nil || d.Format("2006-01-02") != "2026-08-18" {
		t.Errorf("дата с пробелами не разобралась: (%v, %v)", d, ok)
	}
	if _, ok := parseDateOpt("18.08.2026"); ok {
		t.Error("18.08.2026 не должна проходить — формат ГГГГ-ММ-ДД")
	}
}

func TestValidateHire(t *testing.T) {
	cases := []struct{ name, nick, pass, want string }{
		{"нормальный найм", "kasia", "s3cret!", ""},
		{"кириллица в нике — можно", "Катя", "s3cret!", ""},
		{"короткий ник", "ka", "s3cret!", "bad_nickname"},
		{"ник длиннее 32", string(make([]rune, 33)), "s3cret!", "bad_nickname"},
		{"кавычка в нике — XSS-вектор", `ka"sia`, "s3cret!", "nickname_charset"},
		{"угловая скобка в нике", "<b>", "s3cret!", "nickname_charset"},
		{"короткий пароль", "kasia", "12345", "bad_password"},
		{"пароль длиннее 72 байт (bcrypt)", "kasia", string(make([]byte, 73)), "bad_password"},
	}
	for _, tc := range cases {
		_, code := validateHire(tc.nick, tc.pass)
		if code != tc.want {
			t.Errorf("%s: validateHire = %q, ожидалось %q", tc.name, code, tc.want)
		}
	}
}

// З14-и1 (№24): увольнение закрывает открытую смену — посреди смены сейчас, давнюю — по графику, без графика — началом.
func TestDismissCloseAt(t *testing.T) {
	at := func(d, h, m int) time.Time { return time.Date(2026, 10, d, h, m, 0, 0, time.UTC) }
	sched := at(15, 20, 0)
	late := at(17, 9, 0)
	cases := []struct {
		name    string
		start   time.Time
		sameDay bool
		sched   *time.Time
		now     time.Time
		want    time.Time
	}{
		{"посреди смены этих суток — сейчас", at(16, 14, 8), true, &sched, at(16, 16, 13), at(16, 16, 13)},
		{"часы сервера отстали — не раньше начала", at(16, 14, 8), true, nil, at(16, 14, 0), at(16, 14, 8)},
		{"давняя — конец по графику", at(15, 8, 0), false, &sched, at(16, 10, 0), sched},
		{"давняя, график ещё не кончился — сейчас", at(15, 8, 0), false, &late, at(16, 10, 0), at(16, 10, 0)},
		{"давняя без графика — ноль часов", at(15, 8, 0), false, nil, at(16, 10, 0), at(15, 8, 0)},
		{"график раньше прихода — ноль часов", at(15, 21, 0), false, &sched, at(16, 10, 0), at(15, 21, 0)},
	}
	for _, tc := range cases {
		if got := dismissCloseAt(tc.start, tc.sameDay, tc.sched, tc.now); !got.Equal(tc.want) {
			t.Errorf("%s: %v, ждали %v", tc.name, got, tc.want)
		}
	}
}

// З14-и1: строки журнала и канала о смене уволенного — по часам клуба, часы «на проверку», касса — сколько должно
// быть в ящике или что ящика ещё нет.
func TestDismissTexts(t *testing.T) {
	warsaw, err := time.LoadLocation("Europe/Warsaw")
	if err != nil {
		t.Fatal(err)
	}
	old := models.ClubLocation
	models.ClubLocation = warsaw
	defer func() { models.ClubLocation = old }()

	start := time.Date(2026, 10, 16, 12, 8, 0, 0, time.UTC) // 14:08 в Варшаве
	end := time.Date(2026, 10, 16, 14, 13, 0, 0, time.UTC)
	w := &models.WorkEntry{StartedAt: start, EndedAt: &end, Minutes: workMinutes(start, end)}
	if got, want := dismissShiftText(w, end), "смена с 14:08 закрыта (2 ч 05 м — на проверку), касса без пересчёта"; got != want {
		t.Errorf("журнал: %q, ждали %q", got, want)
	}
	opening := int64(10000)
	co := &models.ShiftCloseout{OpeningGrosz: &opening, ExpectedGrosz: 12345, Skipped: true, CreatedAt: end}
	want := "🧮 z14d уволен: смена с 14:08 закрыта увольнением — 2 ч 05 м в табеле на проверку; касса не сдана, без пересчёта: " +
		"в ящике должно быть 123.45 zł — пересчитай"
	if got := dismissOwnerLine("z14d", w, co); got != want {
		t.Errorf("канал: %q, ждали %q", got, want)
	}
	if got := dismissOwnerLine("", w, &models.ShiftCloseout{Skipped: true, CreatedAt: end}); !strings.HasPrefix(got, "🧮 Сотрудник уволен: смена с 14:08") ||
		!strings.HasSuffix(got, "без пересчёта — начального остатка ящика ещё нет") {
		t.Errorf("без ящика и без ника: %q", got)
	}
	night := time.Date(2026, 10, 15, 18, 0, 0, 0, time.UTC) // 20:00 накануне — уволили на следующий день
	if got := dismissSince(night, end); got != "15.10 20:00" {
		t.Errorf("смена со вчера: %q, ждали «15.10 20:00»", got)
	}
	stale := &models.WorkEntry{StartedAt: time.Date(2026, 10, 15, 7, 0, 0, 0, time.UTC), Minutes: 660} // 09:00–20:00 накануне
	if got, want := dismissShiftText(stale, end), "смена с 15.10 09:00 закрыта (11 ч 00 м — на проверку), касса без пересчёта"; got != want {
		t.Errorf("давняя смена — с датой: %q, ждали %q", got, want)
	}
}
