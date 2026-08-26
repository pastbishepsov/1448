package main

import (
	"testing"
	"time"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

// Е5-и1: кому и когда показывать тур. Ошибка здесь стоит либо необученного
// админа, либо экскурсии на каждый вход — оба варианта дорогие по-своему.
func TestNeedsTour(t *testing.T) {
	seen := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name    string
		role    models.UserRole
		version int
		at      *time.Time
		want    bool
	}{
		{"новый админ", models.UserRoleAdmin, 0, nil, true},
		{"новый владелец", models.UserRoleOwner, 0, nil, true},
		{"гостю тур не нужен — он живёт в шелле", models.UserRolePlayer, 0, nil, false},
		{"уже видел текущую версию", models.UserRoleAdmin, 1, &seen, false},
		{"видел старую — интерфейс с тех пор поменялся", models.UserRoleAdmin, 0, &seen, true},
		{"видел будущую (откат версии) — не навязываем", models.UserRoleAdmin, 2, &seen, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := needsTour(c.role, c.version, c.at, 1); got != c.want {
				t.Fatalf("needsTour(%s) = %v; ждали %v", c.name, got, c.want)
			}
		})
	}
}
