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

// З14-и1 (№24): уход из персонала закрывает открытую смену — посреди смены сейчас, давнюю — по графику, без графика —
// началом.
func TestLeaveCloseAt(t *testing.T) {
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
		if got := leaveCloseAt(tc.start, tc.sameDay, tc.sched, tc.now); !got.Equal(tc.want) {
			t.Errorf("%s: %v, ждали %v", tc.name, got, tc.want)
		}
	}
}

// З14-и1 (решение основателя 16.10): кассу за ушедшего не сдаём — владельцу строка, что осталось за ним: ящик (сколько
// должно быть или что его ещё нет) и часы (чем закрыта смена). Время — по часам клуба, давняя смена — с датой.
func TestLeaveTexts(t *testing.T) {
	warsaw, err := time.LoadLocation("Europe/Warsaw")
	if err != nil {
		t.Fatal(err)
	}
	old := models.ClubLocation
	models.ClubLocation = warsaw
	defer func() { models.ClubLocation = old }()

	start := time.Date(2026, 10, 16, 12, 8, 0, 0, time.UTC) // 14:08 в Варшаве
	now := time.Date(2026, 10, 16, 14, 13, 0, 0, time.UTC)
	w := &models.WorkEntry{StartedAt: start, EndedAt: &now, Minutes: workMinutes(start, now), Note: dismissNote}
	if got, want := leaveShiftText(w, now), "смена с 14:08 закрыта (2 ч 05 м — на проверку), касса не сдана"; got != want {
		t.Errorf("журнал: %q, ждали %q", got, want)
	}
	want := "⚠ z14d уволен посреди смены (с 14:08) — осталось за него: касса не сдана — в ящике должно быть 123.45 zł, " +
		"пересчитайте (расхождение до пересчёта ляжет на следующую сдачу); часы — смена закрыта увольнением, 2 ч 05 м в табеле на проверку"
	if got := leaveOwnerLine("z14d", "уволен", w, true, now, true, 12345); got != want {
		t.Errorf("увольнение:\n%q\nждали\n%q", got, want)
	}
	d := &models.WorkEntry{StartedAt: start, EndedAt: &now, Minutes: 125, Note: demoteNote}
	if got := leaveOwnerLine("", "снят с прав", d, true, now, false, 0); got != "⚠ Сотрудник снят с прав посреди смены (с 14:08) — "+
		"осталось за него: касса не сдана — ящик ещё не заведён: пересчитанное станет его начальным остатком; часы — смена "+
		"закрыта снятием прав, 2 ч 05 м в табеле на проверку" {
		t.Errorf("снятие прав без ящика и без ника: %q", got)
	}
	yday := time.Date(2026, 10, 15, 7, 0, 0, 0, time.UTC) // 09:00 накануне — уход не отметил, уволили на следующий день
	s := &models.WorkEntry{StartedAt: yday, Minutes: 660, Note: dismissNote}
	if got := leaveOwnerLine("z14s", "уволен", s, false, now, true, 0); !strings.HasPrefix(got, "⚠ z14s уволен, а смена с 15.10 09:00 "+
		"осталась незакрытой — осталось за него: касса не сдана — в ящике должно быть 0.00 zł") ||
		!strings.HasSuffix(got, "; часы — смена закрыта по графику, 11 ч 00 м в табеле на проверку") {
		t.Errorf("давняя смена: %q", got)
	}
	s.Minutes = 0
	if got := leaveOwnerLine("z14s", "уволен", s, false, now, true, 0); !strings.HasSuffix(got, "; часы — у смены нет графика, закрыта без часов: поставь их в табеле") {
		t.Errorf("давняя без графика: %q", got)
	}
	if got, want := leaveShiftText(&models.WorkEntry{StartedAt: yday, Minutes: 660}, now), "смена с 15.10 09:00 закрыта (11 ч 00 м — на проверку), касса не сдана"; got != want {
		t.Errorf("журнал, давняя смена: %q, ждали %q", got, want)
	}
}

// З14-и2 (№25 «а»): последний рабочий день позже сегодняшних клубных суток — увольнение отложенное; прошёл — со
// следующих суток, а не в полночь и не в сам день.
func TestLeaveDeferredExpired(t *testing.T) {
	day := func(d int) time.Time { return time.Date(2026, 12, d, 0, 0, 0, 0, time.UTC) }
	if leaveDeferred(day(17), day(17)) || leaveDeferred(day(16), day(17)) || !leaveDeferred(day(30), day(17)) {
		t.Error("отложенное — только дата позже сегодняшних суток: сегодня и вчера — увольнение сейчас")
	}
	last := day(30)
	if leaveExpired(nil, day(31)) {
		t.Error("без даты увольнения доступ не кончается")
	}
	if leaveExpired(&last, day(30)) {
		t.Error("в последний рабочий день доступ ещё есть")
	}
	if !leaveExpired(&last, day(31)) {
		t.Error("со следующих клубных суток доступа нет")
	}
}

// З14-и2: строка журнала о завершённом отложенном увольнении — дата, причина, смена, будущие смены.
func TestLeaveDoneText(t *testing.T) {
	day := time.Date(2026, 12, 30, 0, 0, 0, 0, time.UTC)
	if got, want := leaveDoneText("z14l", day, "", "", 0), "z14l · уволен 2026-12-30 (последний рабочий день прошёл)"; got != want {
		t.Errorf("без подробностей: %q, ждали %q", got, want)
	}
	got := leaveDoneText("z14l", day, "переезд", "смена с 30.12 20:00 закрыта (12 ч 00 м — на проверку), касса не сдана", 2)
	if want := "z14l · уволен 2026-12-30 (последний рабочий день прошёл) · переезд, смена с 30.12 20:00 закрыта (12 ч 00 м — " +
		"на проверку), касса не сдана, снят с 2 будущих смен"; got != want {
		t.Errorf("со всем: %q, ждали %q", got, want)
	}
}

// З14-и4 (№26; З-Р19): причина увольнения в журнале — «—»; всё остальное в строке как было, запятые в причине — её часть.
func TestEraseReason(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"увольнение сейчас: причина, смена, снят со смен",
			"z14e · уволен 2026-10-17 · опоздания, грубость, смена с 10:23 закрыта (0 ч 05 м — на проверку), касса не сдана, снят с 1 будущих смен",
			"z14e · уволен 2026-10-17 · —, смена с 10:23 закрыта (0 ч 05 м — на проверку), касса не сдана, снят с 1 будущих смен"},
		{"только причина", "z14e · уволен 2026-10-17 · ушёл учиться", "z14e · уволен 2026-10-17 · —"},
		{"причина с « · » внутри", "z14e · уволен 2026-10-17 · a · b, снят с 2 будущих смен", "z14e · уволен 2026-10-17 · —, снят с 2 будущих смен"},
		{"завершение отложенного", "z14l · уволен 2026-10-16 (последний рабочий день прошёл) · переезд, смена с 16.10 09:00 закрыта (11 ч 00 м — на проверку), касса не сдана, снят с 3 будущих смен",
			"z14l · уволен 2026-10-16 (последний рабочий день прошёл) · —, смена с 16.10 09:00 закрыта (11 ч 00 м — на проверку), касса не сдана, снят с 3 будущих смен"},
		{"назначенный уход", "z14l · уходит 2026-12-30 · переезд, снимется со смен после этой даты: 2", "z14l · уходит 2026-12-30 · —, снимется со смен после этой даты: 2"},
		{"прежняя З14-и1: касса без пересчёта", "z14d · уволен 2026-10-16 · x, смена с 14:08 закрыта (2 ч 05 м — на проверку), касса без пересчёта",
			"z14d · уволен 2026-10-16 · —, смена с 14:08 закрыта (2 ч 05 м — на проверку), касса без пересчёта"},
	}
	for _, tc := range cases {
		if got, ok := eraseReason(tc.in); !ok || got != tc.want {
			t.Errorf("%s:\n got %q (%v)\nждали %q", tc.name, got, ok, tc.want)
		}
	}
	for _, in := range []string{
		"z14e · уволен 2026-10-17",                        // причины нет
		"z14e · уволен 2026-10-17, снят с 1 будущих смен", // причины нет, хвост есть
		"z14e · уходит 2026-12-30, снимется со смен после этой даты: 1",
		"z14e · уволен 2026-10-17 · —, снят с 1 будущих смен", // уже стёрто
		"z14e · увольнение 2026-12-30 отменено",               // не та строка
		"z14e: ставка: почасовая 25.00 → почасовая 30.00",     // строки о ставке — учёт клуба
	} {
		if got, ok := eraseReason(in); ok || got != in {
			t.Errorf("не трогаем %q: got %q (%v)", in, got, ok)
		}
	}
}

func TestErasableFields(t *testing.T) {
	full := models.StaffProfile{FullName: "Ян", Phone: "+48", Note: "ключ", RateType: "hour", RateAmount: 25, Position: "кассир"}
	if got := strings.Join(erasableFields(&full), ","); got != "ФИО,телефон,заметка,ставка" {
		t.Fatalf("всё личное: %q", got)
	}
	empty := models.StaffProfile{RateType: "none", Position: "кассир"} // должность — не личное
	if got := erasableFields(&empty); len(got) != 0 {
		t.Fatalf("стирать нечего: %v", got)
	}
}
