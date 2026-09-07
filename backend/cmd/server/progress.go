package main

// Запись и чтение прогресса по ачивочным суткам (трек Г, спринт Г5; К5).
//
// recordSessionProgress пишет завершённую сессию в user_progress. К5-и1:
// сессия, пережившая резет 14:48, раскладывается по двум суткам — часть до
// резета во «вчерашние», часть после — в «сегодняшние» (sessionDayPieces);
// раньше вся сессия падала в сутки завершения. Дев-оверрайд минут (тесты
// начислений) пишет минуты и как активные — оверрайда в проде не существует
// (SERVER_ENV=production).
//
// gatherPeriodicStats собирает всё, что нужно conditionMet: сегодняшние
// минуты/визиты, неделя, месяц, стрик визитов подряд (от сегодняшних суток
// назад). Стрик и агрегаты считаются из последних 62 строк — периодическим
// условиям большего не нужно. К5-и1: идущая сессия гостя досчитывается
// «на лету» теми же кусками — прогресс дневных ачивок живёт во время игры,
// а не появляется скачком после завершения; в БД при этом ничего не пишется,
// finishSession запишет сессию один раз, целиком.

import (
	"time"

	"github.com/google/uuid"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

// dayPiece — кусок сессии внутри одних ачивочных суток.
type dayPiece struct {
	DayKey        string
	Minutes       int
	ActiveMinutes int
	Night         bool      // сессия НАЧАЛАСЬ ночью — только у первого куска (Г6)
	FirstAt       time.Time // начало куска: старт сессии или резет 14:48
}

// sessionDayPieces — разложить сессию [startedAt, endedAt] по ачивочным
// суткам (чистая функция, тест). minutes/activeMinutes — итог сессии БЕЗ
// пауз; между кусками они делятся по доле календарного времени (когда именно
// были паузы, сессия не помнит; для сессии без пауз 13:00→16:00 это ровно
// 108 мин вчера + 72 сегодня). Куски с нулём минут не отдаются, кроме случая,
// когда сессия целиком короче минуты — тогда один кусок суток старта.
func sessionDayPieces(minutes, activeMinutes int, startedAt, endedAt time.Time) []dayPiece {
	if minutes < 0 {
		minutes = 0
	}
	if activeMinutes > minutes {
		activeMinutes = minutes
	}
	if activeMinutes < 0 {
		activeMinutes = 0
	}
	if endedAt.Before(startedAt) {
		endedAt = startedAt
	}
	night := isNightHour(startedAt.In(clubLocation).Hour())
	total := endedAt.Sub(startedAt).Seconds()
	first := dayPiece{DayKey: achDayKey(startedAt), Minutes: minutes, ActiveMinutes: activeMinutes, Night: night, FirstAt: startedAt}
	boundary := achDayStart(startedAt).AddDate(0, 0, 1) // первый резет после старта
	if total <= 0 || !boundary.Before(endedAt) {
		return []dayPiece{first} // сессия целиком внутри одних суток
	}
	var out []dayPiece
	from, leftMin, leftAct := startedAt, minutes, activeMinutes
	for boundary.Before(endedAt) {
		share := boundary.Sub(from).Seconds() / total
		m := int(float64(minutes)*share + 0.5)
		a := int(float64(activeMinutes)*share + 0.5)
		if m > leftMin {
			m = leftMin
		}
		if a > leftAct {
			a = leftAct
		}
		if a > m {
			a = m
		}
		if m > 0 {
			out = append(out, dayPiece{DayKey: achDayKey(from), Minutes: m, ActiveMinutes: a, Night: night && from.Equal(startedAt), FirstAt: from})
			leftMin -= m
			leftAct -= a
		}
		from = boundary
		boundary = boundary.AddDate(0, 0, 1)
	}
	if leftAct > leftMin {
		leftAct = leftMin
	}
	if leftMin > 0 || len(out) == 0 {
		out = append(out, dayPiece{DayKey: achDayKey(from), Minutes: leftMin, ActiveMinutes: leftAct, Night: night && from.Equal(startedAt), FirstAt: from})
	}
	return out
}

// recordSessionProgress — учесть завершённую сессию в суточном прогрессе.
func recordSessionProgress(userID uuid.UUID, minutes, activeMinutes int, startedAt, now time.Time, devOverride bool) {
	if minutes <= 0 {
		return
	}
	if devOverride {
		activeMinutes = minutes // тестовое начисление честно тестирует и ачивки
	}
	for _, p := range sessionDayPieces(minutes, activeMinutes, startedAt, now) {
		night := 0
		if p.Night { // Г6: «Ночная смена»
			night = 1
		}
		db.Exec(`INSERT INTO user_progress (user_id, day_key, minutes, active_minutes, sessions, night_sessions, first_session_at)
			VALUES (?, ?, ?, ?, 1, ?, ?)
			ON CONFLICT (user_id, day_key) DO UPDATE SET
				minutes = user_progress.minutes + EXCLUDED.minutes,
				active_minutes = user_progress.active_minutes + EXCLUDED.active_minutes,
				sessions = user_progress.sessions + 1,
				night_sessions = user_progress.night_sessions + EXCLUDED.night_sessions,
				first_session_at = LEAST(user_progress.first_session_at, EXCLUDED.first_session_at)`,
			userID, p.DayKey, p.Minutes, p.ActiveMinutes, night, p.FirstAt)
	}
}

// liveSessionPieces — куски ИДУЩЕЙ сессии гостя на момент now (К5-и1): минуты
// без пауз (вниз, как считает биллинг), активные — из строки сессии (их ведёт
// тик биллинга). Ожидание [Готов!] — не игра: не считается.
func liveSessionPieces(userID uuid.UUID, now time.Time) []dayPiece {
	var rows []models.Session // Find, не First: без сессии — не «ошибка» в логе
	db.Where("user_id = ? AND status = ?", userID, models.SessionStatusActive).
		Order("started_at DESC").Limit(1).Find(&rows)
	if len(rows) == 0 || sessionWaitingReady(&rows[0]) {
		return nil
	}
	s := &rows[0]
	minutes := effectiveElapsedMinutes(s, now)
	if minutes <= 0 {
		return nil
	}
	return sessionDayPieces(minutes, s.ActiveMinutes, s.StartedAt, now)
}

// gatherPeriodicStats — статистика периодов для conditionMet (Г5), с
// досчётом идущей сессии (К5-и1).
func gatherPeriodicStats(userID uuid.UUID, now time.Time) playerStats {
	return periodicStatsWith(userID, now, liveSessionPieces(userID, now))
}

// periodicStatsWith — то же с уже собранными кусками идущей сессии.
func periodicStatsWith(userID uuid.UUID, now time.Time, live []dayPiece) playerStats {
	var rows []models.UserProgress
	db.Where("user_id = ?", userID).Order("day_key DESC").Limit(62).Find(&rows)

	byDay := map[string]*models.UserProgress{}
	for i := range rows {
		byDay[rows[i].DayKey] = &rows[i]
	}
	for _, p := range live {
		row, ok := byDay[p.DayKey]
		if !ok {
			row = &models.UserProgress{UserID: userID, DayKey: p.DayKey}
			byDay[p.DayKey] = row
		}
		row.Minutes += p.Minutes
		row.ActiveMinutes += p.ActiveMinutes
		row.Sessions++
		if p.Night {
			row.NightSessions++
		}
	}

	s := playerStats{}
	today := achDayKey(now)
	if p, ok := byDay[today]; ok {
		s.MinutesToday = p.Minutes
		s.ActiveToday = p.ActiveMinutes
		s.VisitsToday = p.Sessions
		s.KitchenToday = p.KitchenOrders
		s.NightToday = p.NightSessions
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
		if len(k) >= 7 && k[:7] == month {
			s.MinutesMonth += p.Minutes
			s.ActiveMonth += p.ActiveMinutes
			if p.Sessions > 0 {
				s.VisitsMonth++
			}
		}
	}
	// Стрик: дни подряд, начиная с сегодняшних суток назад. Г6-и4: день,
	// прикрытый заморозкой, цепочку НЕ рвёт, но и визитом не считается —
	// «неделя без пропусков» по-прежнему требует семи реальных приходов.
	day := achDayStart(now)
	for {
		p, ok := byDay[day.Format("2006-01-02")]
		if !ok || (p.Sessions == 0 && !p.Frozen) {
			break
		}
		if p.Sessions > 0 {
			s.VisitStreak++
		}
		day = day.AddDate(0, 0, -1)
	}
	return s
}

// fullPlayerStats — периоды + lifetime-счётчики: одна точка сборки для
// /me/achievements, тика биллинга (К5-и2), завершения сессии и кухни. Часы
// за всё время включают идущую сессию (К5-и1) — «Пятьсот» тоже живёт.
func fullPlayerStats(userID uuid.UUID, now time.Time) playerStats {
	uid := userID.String()
	live := liveSessionPieces(userID, now)
	stats := periodicStatsWith(userID, now, live)
	liveMin := 0
	for _, p := range live {
		liveMin += p.Minutes
	}
	stats.HoursPlayed = int((userMinutesPlayed(uid) + int64(liveMin)) / 60)
	stats.LoginCount = 1
	stats.DepositCount = userDepositCount(uid)
	stats.BookingsCount = userBookingsCount(uid)
	return stats
}
