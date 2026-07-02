package main

import "testing"

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
