package main

// Запись и чтение прогресса по ачивочным суткам (трек Г, спринт Г5).
//
// recordSessionProgress пишет завершённую сессию в user_progress (сутки
// завершения). Дев-оверрайд минут (тесты начислений) пишет минуты и как
// активные — оверрайда в проде не существует (SERVER_ENV=production).
//
// gatherPeriodicStats собирает всё, что нужно conditionMet: сегодняшние
// минуты/визиты, неделя, месяц, стрик визитов подряд (от сегодняшних суток
// назад). Стрик и агрегаты считаются из последних 62 строк — периодическим
// условиям большего не нужно.

import (
	"time"

	"github.com/google/uuid"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

// recordSessionProgress — учесть завершённую сессию в суточном прогрессе.
func recordSessionProgress(userID uuid.UUID, minutes, activeMinutes int, startedAt, now time.Time, devOverride bool) {
	if minutes <= 0 {
		return
	}
	if devOverride {
		activeMinutes = minutes // тестовое начисление честно тестирует и ачивки
	}
	if activeMinutes > minutes {
		activeMinutes = minutes
	}
	db.Exec(`INSERT INTO user_progress (user_id, day_key, minutes, active_minutes, sessions, first_session_at)
		VALUES (?, ?, ?, ?, 1, ?)
		ON CONFLICT (user_id, day_key) DO UPDATE SET
			minutes = user_progress.minutes + EXCLUDED.minutes,
			active_minutes = user_progress.active_minutes + EXCLUDED.active_minutes,
			sessions = user_progress.sessions + 1,
			first_session_at = LEAST(user_progress.first_session_at, EXCLUDED.first_session_at)`,
		userID, achDayKey(now), minutes, activeMinutes, startedAt)
}

// gatherPeriodicStats — статистика периодов для conditionMet (Г5).
func gatherPeriodicStats(userID uuid.UUID, now time.Time) playerStats {
	var rows []models.UserProgress
	db.Where("user_id = ?", userID).Order("day_key DESC").Limit(62).Find(&rows)

	byDay := map[string]*models.UserProgress{}
	for i := range rows {
		byDay[rows[i].DayKey] = &rows[i]
	}

	s := playerStats{}
	today := achDayKey(now)
	if p, ok := byDay[today]; ok {
		s.MinutesToday = p.Minutes
		s.ActiveToday = p.ActiveMinutes
		s.VisitsToday = p.Sessions
		s.KitchenToday = p.KitchenOrders
	}
	for _, k := range achWeekDayKeys(now) {
		if p, ok := byDay[k]; ok {
			s.MinutesWeek += p.Minutes
			s.ActiveWeek += p.ActiveMinutes
			if p.Sessions > 0 {
				s.VisitsWeek++
			}
		}
	}
	month := achMonthPrefix(now)
	for k, p := range byDay {
		if len(k) >= 7 && k[:7] == month && p.Sessions > 0 {
			s.VisitsMonth++
		}
	}
	// стрик: дни с визитом подряд, начиная с сегодняшних суток назад
	day := achDayStart(now)
	for {
		p, ok := byDay[day.Format("2006-01-02")]
		if !ok || p.Sessions == 0 {
			break
		}
		s.VisitStreak++
		day = day.AddDate(0, 0, -1)
	}
	return s
}
