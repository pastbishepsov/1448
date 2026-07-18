package main

import (
	"testing"

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
