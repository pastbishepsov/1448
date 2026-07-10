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
