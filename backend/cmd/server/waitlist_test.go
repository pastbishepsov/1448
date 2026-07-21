package main

import (
	"testing"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

func TestCanJoinWaitlist(t *testing.T) {
	cases := []struct {
		name    string
		status  models.UserStatus
		role    models.UserRole
		session bool
		waiting bool
		free    int64
		ok      bool
		code    string
	}{
		{"гость при полном зале — можно", models.UserStatusActive, models.UserRolePlayer, false, false, 0, true, ""},
		{"есть свободные ПК — очередь не нужна", models.UserStatusActive, models.UserRolePlayer, false, false, 3, false, "computers_free"},
		{"уже в очереди", models.UserStatusActive, models.UserRolePlayer, false, true, 0, false, "already_waiting"},
		{"уже идёт сессия", models.UserStatusActive, models.UserRolePlayer, true, false, 0, false, "session_active"},
		{"забаненный — нельзя", models.UserStatusBanned, models.UserRolePlayer, false, false, 0, false, "banned"},
		{"админа не ставим", models.UserStatusActive, models.UserRoleAdmin, false, false, 0, false, "not_player"},
		{"владельца не ставим", models.UserStatusActive, models.UserRoleOwner, false, false, 0, false, "not_player"},
		{"роль проверяется раньше бана", models.UserStatusBanned, models.UserRoleAdmin, false, false, 0, false, "not_player"},
		{"сессия проверяется раньше очереди", models.UserStatusActive, models.UserRolePlayer, true, true, 0, false, "session_active"},
	}
	for _, tc := range cases {
		ok, code := canJoinWaitlist(tc.status, tc.role, tc.session, tc.waiting, tc.free)
		if ok != tc.ok || code != tc.code {
			t.Errorf("%s: canJoinWaitlist(%s, %s, %v, %v, %d) = (%v, %q), ожидалось (%v, %q)",
				tc.name, tc.status, tc.role, tc.session, tc.waiting, tc.free, ok, code, tc.ok, tc.code)
		}
	}
}

func TestWaitlistJoinErrorsCovered(t *testing.T) {
	// у каждого кода отказа canJoinWaitlist есть HTTP-статус и сообщение
	for _, code := range []string{"not_player", "banned", "session_active", "already_waiting", "computers_free"} {
		e, ok := waitlistJoinErrors[code]
		if !ok || e.status == 0 || e.message == "" {
			t.Errorf("код %q не покрыт waitlistJoinErrors", code)
		}
	}
}
