package main

import (
	"testing"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

// applyXP: проверяем начисление опыта и повышение уровня.
func TestApplyXP(t *testing.T) {
	cases := []struct {
		name        string
		startLevel  int
		startXP     int64
		gain        int64
		wantLevel   int
		wantXP      int64
		wantLevelUp int
	}{
		{"без повышения", 1, 0, 500, 1, 500, 0},
		{"ровно на уровень", 1, 0, 1000, 2, 0, 1},       // XP(1)=1000
		{"перепрыгнул остаток", 1, 0, 1500, 2, 500, 1},  // 1500-1000=500
		{"ноль опыта", 3, 100, 0, 3, 100, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			u := &models.User{Level: tc.startLevel, XPCurrent: tc.startXP}
			got := applyXP(u, tc.gain)
			if u.Level != tc.wantLevel {
				t.Errorf("level = %d, хотим %d", u.Level, tc.wantLevel)
			}
			if u.XPCurrent != tc.wantXP {
				t.Errorf("xp_current = %d, хотим %d", u.XPCurrent, tc.wantXP)
			}
			if got != tc.wantLevelUp {
				t.Errorf("levelsGained = %d, хотим %d", got, tc.wantLevelUp)
			}
		})
	}
}

// applyXP должен выдавать по очку навыка за уровень.
func TestApplyXPSkillpoints(t *testing.T) {
	u := &models.User{Level: 1}
	applyXP(u, 100000) // заведомо несколько уровней
	if u.SkillpointsAvailable != u.Level-1 {
		t.Errorf("skillpoints = %d, ожидаем %d (по 1 за уровень)", u.SkillpointsAvailable, u.Level-1)
	}
}

// tierForLevel: границы тиров кейсов за уровень.
func TestTierForLevel(t *testing.T) {
	cases := map[int]models.CaseTier{
		1:  models.CaseTierLight,
		4:  models.CaseTierLight,
		5:  models.CaseTierMedium,
		14: models.CaseTierMedium,
		15: models.CaseTierHeavy,
		30: models.CaseTierTitan,
		49: models.CaseTierTitan,
		50: models.CaseTierGods,
	}
	for level, want := range cases {
		if got := tierForLevel(level); got != want {
			t.Errorf("tierForLevel(%d) = %s, хотим %s", level, got, want)
		}
	}
}
