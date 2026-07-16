package main

import (
	"testing"

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
