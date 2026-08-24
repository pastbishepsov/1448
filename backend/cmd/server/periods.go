package main

// Ачивочные периоды (трек Г, спринт Г5; GUEST.md, решение Р6).
//
// Периодические достижения (daily/weekly/monthly) сбрасываются НЕ в полночь,
// а в 14:48 клубного времени — фирменный резет, обыгрывающий название клуба.
// «Ачивочные сутки» идут с 14:48 до 14:48; неделя и месяц считаются по дате
// начала этих суток (ISO-неделя и календарный месяц). Отчётные клубные сутки
// (report_hour, Б10) живут отдельно и не трогаются — осознанное раздвоение.
//
// periodKey — ключ периода для user_achievements.period_key: в новом периоде
// та же ачивка выдаётся заново (unique(user, ach, period_key) в схеме с 005).

import (
	"fmt"
	"time"
)

const (
	achResetHour   = 14 // фирменный резет — 14:48
	achResetMinute = 48
)

// clubLocation — часовой пояс клуба (рынок — Польша; TimeZone=Europe/Warsaw
// стоит и в DSN). Фолбэк — локальная зона сервера.
var clubLocation = func() *time.Location {
	if loc, err := time.LoadLocation("Europe/Warsaw"); err == nil {
		return loc
	}
	return time.Local
}()

// achDayStart — начало ТЕКУЩИХ ачивочных суток (последние 14:48 клуба).
func achDayStart(now time.Time) time.Time {
	local := now.In(clubLocation)
	start := time.Date(local.Year(), local.Month(), local.Day(),
		achResetHour, achResetMinute, 0, 0, clubLocation)
	if local.Before(start) {
		start = start.AddDate(0, 0, -1)
	}
	return start
}

// achDayKey — ключ ачивочных суток ('2026-08-24' = сутки, начавшиеся 24-го в 14:48).
func achDayKey(now time.Time) string {
	return achDayStart(now).Format("2006-01-02")
}

// periodKeyFor — ключ периода для категории достижения; lifetime — без ключа.
func periodKeyFor(category string, now time.Time) *string {
	start := achDayStart(now)
	var key string
	switch category {
	case "daily":
		key = start.Format("2006-01-02")
	case "weekly":
		y, w := start.ISOWeek()
		key = fmt.Sprintf("%d-W%02d", y, w)
	case "monthly":
		key = start.Format("2006-01")
	default: // lifetime
		return nil
	}
	return &key
}

// nextAchReset — ближайший резет (следующие 14:48 клуба) — для UI «до резета».
func nextAchReset(now time.Time) time.Time {
	return achDayStart(now).AddDate(0, 0, 1)
}

// achWeekDayKeys — семь day_key текущей ачивочной ISO-недели (пн–вс).
func achWeekDayKeys(now time.Time) []string {
	start := achDayStart(now)
	monday := start.AddDate(0, 0, -(int(start.Weekday())+6)%7)
	keys := make([]string, 7)
	for i := 0; i < 7; i++ {
		keys[i] = monday.AddDate(0, 0, i).Format("2006-01-02")
	}
	return keys
}

// achMonthPrefix — префикс day_key текущего ачивочного месяца ('2026-08').
func achMonthPrefix(now time.Time) string {
	return achDayStart(now).Format("2006-01")
}
