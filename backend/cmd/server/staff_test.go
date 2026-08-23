package main

import (
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
