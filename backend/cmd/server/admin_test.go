package main

import (
	"testing"
	"time"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

func TestCanSetComputerStatus(t *testing.T) {
	cases := []struct {
		name string
		cur  models.ComputerStatus
		next models.ComputerStatus
		want bool
	}{
		{"свободный — в ремонт", models.ComputerStatusAvailable, models.ComputerStatusMaintenance, true},
		{"из ремонта — в строй", models.ComputerStatusMaintenance, models.ComputerStatusAvailable, true},
		{"занятый в ремонт нельзя", models.ComputerStatusInSession, models.ComputerStatusMaintenance, false},
		{"занятый в строй нельзя", models.ComputerStatusInSession, models.ComputerStatusAvailable, false},
		{"резерв в ремонт нельзя", models.ComputerStatusReserved, models.ComputerStatusMaintenance, false},
		{"повтор того же статуса — ок", models.ComputerStatusMaintenance, models.ComputerStatusMaintenance, true},
		{"в in_session руками нельзя", models.ComputerStatusAvailable, models.ComputerStatusInSession, false},
	}
	for _, tc := range cases {
		if got := canSetComputerStatus(tc.cur, tc.next); got != tc.want {
			t.Errorf("%s: canSetComputerStatus(%s→%s) = %v, ожидалось %v", tc.name, tc.cur, tc.next, got, tc.want)
		}
	}
}

func TestCanTargetUser(t *testing.T) {
	cases := []struct {
		name   string
		role   models.UserRole
		target string
		actor  string
		ok     bool
		code   string
	}{
		{"гостя — можно", models.UserRolePlayer, "u1", "a1", true, ""},
		{"админа нельзя", models.UserRoleAdmin, "u2", "a1", false, "cannot_touch_staff"},
		{"владельца нельзя", models.UserRoleOwner, "u3", "a1", false, "cannot_touch_staff"},
		{"самого себя нельзя", models.UserRoleAdmin, "a1", "a1", false, "cannot_touch_self"},
		{"self проверяется раньше роли", models.UserRolePlayer, "x1", "x1", false, "cannot_touch_self"},
	}
	for _, tc := range cases {
		ok, code := canTargetUser(tc.role, tc.target, tc.actor)
		if ok != tc.ok || code != tc.code {
			t.Errorf("%s: canTargetUser(%s, %s, %s) = (%v, %q), ожидалось (%v, %q)",
				tc.name, tc.role, tc.target, tc.actor, ok, code, tc.ok, tc.code)
		}
	}
}

func TestAdminDayCapExceeded(t *testing.T) {
	cases := []struct {
		name            string
		used, add, cap  float64
		want            bool
	}{
		{"лимит 0 — не ограничен", 999999, 1000, 0, false},
		{"в пределах", 300, 100, 500, false},
		{"ровно в потолок — можно", 400, 100, 500, false},
		{"перебор", 450, 100, 500, true},
		{"первая же операция больше лимита", 0, 600, 500, true},
	}
	for _, tc := range cases {
		if got := adminDayCapExceeded(tc.used, tc.add, tc.cap); got != tc.want {
			t.Errorf("%s: adminDayCapExceeded(%v, %v, %v) = %v, ожидалось %v",
				tc.name, tc.used, tc.add, tc.cap, got, tc.want)
		}
	}
}

func TestRoleIsStaff(t *testing.T) {
	cases := map[string]bool{
		"admin":  true,
		"owner":  true,
		"player": false,
		"":       false, // старые токены без роли — не персонал
		"Admin":  false, // регистр важен: роли в БД только строчные
	}
	for role, want := range cases {
		if got := roleIsStaff(role); got != want {
			t.Errorf("roleIsStaff(%q) = %v, ожидалось %v", role, got, want)
		}
	}
}

// Ж3-и1: одно «сегодня» — клубные сутки от report_hour: 07:59 — ещё вчера,
// 08:00 — уже сегодня; ночная смена не рвётся полуночью.
func TestClubDayStartAt(t *testing.T) {
	loc := time.FixedZone("CEST", 2*3600)
	at := func(h, m int) time.Time { return time.Date(2026, 9, 16, h, m, 0, 0, loc) }
	yesterday8 := time.Date(2026, 9, 15, 8, 0, 0, 0, loc)
	today8 := time.Date(2026, 9, 16, 8, 0, 0, 0, loc)
	cases := []struct {
		name string
		now  time.Time
		want time.Time
	}{
		{"00:30 — ещё вчерашние сутки", at(0, 30), yesterday8},
		{"07:59 — ещё вчера", at(7, 59), yesterday8},
		{"08:00 — уже сегодня", at(8, 0), today8},
		{"23:50 — сегодня", at(23, 50), today8},
	}
	for _, tc := range cases {
		if got := clubDayStartAt(8, tc.now); !got.Equal(tc.want) {
			t.Errorf("%s: %v, ожидалось %v", tc.name, got, tc.want)
		}
	}
	// report_hour 0 — клубные сутки совпадают с календарными
	if got := clubDayStartAt(0, at(0, 30)); !got.Equal(time.Date(2026, 9, 16, 0, 0, 0, 0, loc)) {
		t.Errorf("report_hour 0: %v", got)
	}
}

// Ж5-и3: сортировка списка гостей — только из белого списка, направление
// по умолчанию зависит от столбца (ник — по алфавиту, числа и даты — с большего).
func TestUsersOrder(t *testing.T) {
	cases := []struct {
		name          string
		sort, dir     string
		wantCol, want string
		wantOK        bool
	}{
		{"пусто — свежие сверху", "", "", "last_active_at", "desc", true},
		{"ник — по алфавиту", "nickname", "", "LOWER(nickname)", "asc", true},
		{"кошелёк — с большего", "wallet", "", "wallet_grosz", "desc", true},
		{"монеты вверх — как просили", "coins", "asc", "coins_balance", "asc", true},
		{"дата регистрации", "created", "desc", "created_at", "desc", true},
		{"уровень", "level", "", "level", "desc", true},
		{"столбец вне списка", "password_hash", "", "", "", false},
		{"инъекция в столбце", "nickname; DROP TABLE users", "", "", "", false},
		{"направление вне списка", "wallet", "sideways", "", "", false},
	}
	for _, tc := range cases {
		col, _, dir, ok := usersOrder(tc.sort, tc.dir)
		if ok != tc.wantOK || col != tc.wantCol || dir != tc.want {
			t.Errorf("%s: usersOrder(%q,%q) = (%q,%q,%v), ожидалось (%q,%q,%v)",
				tc.name, tc.sort, tc.dir, col, dir, ok, tc.wantCol, tc.want, tc.wantOK)
		}
	}
}
