package main

// Юнит-тесты арифметики паузы (Г2): чистые функции, время — параметром.
// Ключевое: пауза выпадает из тарифицируемых минут и из XP, бюджет паузы
// считается от суммарной паузы сессии.

import (
	"testing"
	"time"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

func sess(startAgo time.Duration, pausedTotal int, pausedAgo time.Duration, now time.Time) *models.Session {
	s := &models.Session{StartedAt: now.Add(-startAgo), PausedTotalSec: pausedTotal}
	if pausedAgo > 0 {
		t := now.Add(-pausedAgo)
		s.PausedAt = &t
	}
	return s
}

func TestPausedDuration(t *testing.T) {
	now := time.Date(2026, 8, 23, 15, 0, 0, 0, time.UTC)
	if got := pausedDuration(sess(10*time.Minute, 0, 0, now), now); got != 0 {
		t.Errorf("без пауз: ждали 0, получили %d", got)
	}
	if got := pausedDuration(sess(10*time.Minute, 90, 0, now), now); got != 90 {
		t.Errorf("закрытая пауза 90с: получили %d", got)
	}
	if got := pausedDuration(sess(10*time.Minute, 60, 30*time.Second, now), now); got != 90 {
		t.Errorf("60с закрытых + 30с текущей: ждали 90, получили %d", got)
	}
}

func TestEffectiveMinutes(t *testing.T) {
	now := time.Date(2026, 8, 23, 15, 0, 0, 0, time.UTC)
	// 10 минут сессии, из них 90 с паузы: 510 с игры → floor 8, ceil 9
	s := sess(10*time.Minute, 90, 0, now)
	if got := effectiveElapsedMinutes(s, now); got != 8 {
		t.Errorf("floor: ждали 8, получили %d", got)
	}
	if got := effectiveMinutesCeil(s, now); got != 9 {
		t.Errorf("ceil: ждали 9, получили %d", got)
	}
	// пауза целиком покрывает сессию — нуль, не минус
	s2 := sess(2*time.Minute, 200, 0, now)
	if got := effectiveElapsedMinutes(s2, now); got != 0 {
		t.Errorf("пауза больше сессии: ждали 0, получили %d", got)
	}
	if got := effectiveMinutesCeil(s2, now); got != 0 {
		t.Errorf("ceil при нуле: ждали 0, получили %d", got)
	}
	// ровный час без пауз: floor == ceil == 60 (стык с биллингом Г1)
	s3 := sess(time.Hour, 0, 0, now)
	if effectiveElapsedMinutes(s3, now) != 60 || effectiveMinutesCeil(s3, now) != 60 {
		t.Errorf("час без пауз: floor/ceil должны быть 60")
	}
}

func TestPauseBudget(t *testing.T) {
	now := time.Date(2026, 8, 23, 15, 0, 0, 0, time.UTC)
	// лимит 15 мин, отгуляно 14 мин → осталось 60 с
	if got := pauseBudgetLeftSec(sess(time.Hour, 840, 0, now), 15, now); got != 60 {
		t.Errorf("остаток бюджета: ждали 60, получили %d", got)
	}
	// перерасход (автовозврат опоздал на тик) — ноль, не минус
	if got := pauseBudgetLeftSec(sess(time.Hour, 920, 0, now), 15, now); got != 0 {
		t.Errorf("перерасход: ждали 0, получили %d", got)
	}
	// текущая пауза тоже ест бюджет
	if got := pauseBudgetLeftSec(sess(time.Hour, 0, 14*time.Minute, now), 15, now); got != 60 {
		t.Errorf("открытая пауза: ждали 60, получили %d", got)
	}
}
