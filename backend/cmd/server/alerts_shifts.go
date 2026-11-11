package main

// З22-и4 (OWNER.md, трек З; №52, З-Р28): напоминания о сменах — тем же прогоном раз в 15 минут, что периодические
// алерты (З22-и3). За N часов до назначенной смены — сотруднику в канал, по нику: «📅 admin1, твоя смена «Ночь» сегодня в
// 20:00»; видят все — напоминание адресное, не про деньги и не про поведение. «Завтра никто не назначен на «Ночь»» —
// владельцу, начиная с заданного часа клуба: шаблон смены работает завтра (маска дней), а назначения нет. Раз за событие:
// напоминание — на назначение, «никто не назначен» — на завтрашний день и шаблон. Настройки — «Экономика» → «Алерты».

import (
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

const (
	shiftRemindDef = 2  // за сколько часов напомнить о смене; 0 — не напоминать
	shiftNobodyDef = 18 // с какого часа клуба сказать «завтра никто не назначен»; −1 — не говорить
)

var weekdaysShort = [...]string{"вс", "пн", "вт", "ср", "чт", "пт", "сб"}

// shiftSoon — пора ли напомнить о смене, начинающейся в start (чистая функция, тест): она ещё не началась и до неё не
// больше hours часов.
func shiftSoon(start, now time.Time, hours int64) bool {
	return hours > 0 && now.Before(start) && start.Sub(now) <= time.Duration(hours)*time.Hour
}

// untilText — «через 1 ч 45 мин», «через 40 мин» (чистая функция, тест): до start от now, по минутам вверх.
func untilText(start, now time.Time) string {
	m := int((start.Sub(now) + time.Minute - 1) / time.Minute)
	switch {
	case m < 60:
		return fmt.Sprintf("через %d мин", m)
	case m%60 == 0:
		return fmt.Sprintf("через %d ч", m/60)
	}
	return fmt.Sprintf("через %d ч %d мин", m/60, m%60)
}

// dayWord — «сегодня», «завтра» или «12.11» от клубных суток today (чистая функция, тест).
func dayWord(day, today time.Time) string {
	switch {
	case day.Equal(today):
		return "сегодня"
	case day.Equal(today.AddDate(0, 0, 1)):
		return "завтра"
	}
	return day.Format("02.01")
}

// alertShiftSoon — напоминания о назначенных сменах, до которых не больше hours часов; кто уже отметил приход — не
// нужно. Раз на назначение.
func alertShiftSoon(now time.Time, rh int, hours int64) int {
	today := clubDayOf(now, rh)
	var as []models.ShiftAssignment
	db.Preload("Shift").Preload("User").Where("date >= ? AND date <= ?", today.AddDate(0, 0, -1).Format("2006-01-02"),
		today.AddDate(0, 0, 1).Format("2006-01-02")).Find(&as)
	n := 0
	for _, a := range as {
		if a.Shift == nil || a.User == nil || !roleIsStaff(string(a.User.Role)) || a.User.Status != models.UserStatusActive {
			continue
		}
		start := wallAt(a.Date, a.Shift.StartMin)
		if !shiftSoon(start, now, hours) || openEntryFor(a.UserID) != nil {
			continue
		}
		text := fmt.Sprintf("📅 %s, твоя смена «%s» %s в %s (%s) — до %s", a.User.Nickname, a.Shift.Name,
			dayWord(clubDayOf(start, rh), today), start.Format("15:04"), untilText(start, now),
			shiftEndAt(a.Date, a.Shift.StartMin, a.Shift.EndMin).Format("15:04"))
		if alertSend("shift_soon", "a/"+a.ID.String(), text) {
			n++
		}
	}
	return n
}

// alertShiftNobody — «завтра никто не назначен»: с часа hour клуба, по активным шаблонам, работающим завтра, без
// единого назначения. Раз на день и шаблон.
func alertShiftNobody(now time.Time, rh int, hour int64) int {
	if hour < 0 || int64(now.Hour()) < hour {
		return 0
	}
	tomorrow := clubDayOf(now, rh).AddDate(0, 0, 1)
	var shifts []models.Shift
	db.Where("active").Order("sort, start_min").Find(&shifts)
	var taken []uuid.UUID
	db.Model(&models.ShiftAssignment{}).Where("date = ?", tomorrow.Format("2006-01-02")).Distinct().Pluck("shift_id", &taken)
	busy := map[uuid.UUID]bool{}
	for _, id := range taken {
		busy[id] = true
	}
	n := 0
	for _, s := range shifts {
		if s.DaysMask&(1<<dowMonday(tomorrow)) == 0 || busy[s.ID] {
			continue
		}
		text := fmt.Sprintf("🗓 Завтра (%s, %s) на «%s» %s–%s никто не назначен — «Персонал» → график", tomorrow.Format("02.01"),
			weekdaysShort[tomorrow.Weekday()], s.Name, fmtShiftTime(s.StartMin), fmtShiftTime(s.EndMin))
		if alertSend("shift_nobody", tomorrow.Format("2006-01-02")+"/"+s.ID.String(), text) {
			n++
		}
	}
	return n
}
