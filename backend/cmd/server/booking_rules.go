package main

// Правило посадки перед бронью (трек Г, спринт Г3; GUEST.md, решение Р3).
//
// Бронь — обещание клуба, посадка не должна его ломать:
//   - на ПК с чужой броней можно сесть, только если планируемое время
//     умещается в окно до брони МИНУС буфер (booking_buffer_min, деф. 15) —
//     буфер оставляет время на пересменку;
//   - за booking_lock_min (деф. 10) до начала ПК придержан: сесть может
//     только хозяин брони; его посадка гасит бронь статусом seated;
//   - сессия, начатая перед чужой броней, живёт с жёстким дедлайном
//     start − lock: предупреждения за ~15/~5 минут, затем штатное
//     завершение ended_reason=booking (см. billing.go).
//
// Контрольный пример основателя закреплён тестом (booking_rules_test.go):
// бронь 19:00, буфер 15 → в 18:00 «на час» отказ (60−15=45<60),
// в 17:45 — посадка (75−15=60≥60).

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

const (
	bookingBufferMinDef int64 = 15 // настройка booking_buffer_min
	bookingLockMinDef   int64 = 10 // настройка booking_lock_min
	defaultPlannedMin         = 30 // посадка без planned_min: минимальный сеанс

	// Г4: дисциплина броней
	bookingMinLevelDef   int64 = 3  // настройка booking_min_level (0 = всем)
	maxActiveBookingsDef int64 = 2  // настройка max_active_bookings (0 = без лимита)
	noShowMinDef         int64 = 15 // настройка no_show_min (0 = выкл)
	remindFirstMin             = 60 // первое напоминание: до брони ~час
	remindLastMin              = 15 // второе: ~15 минут
)

// seatWindowMin — сколько ЦЕЛЫХ минут можно отсидеть до брони с учётом
// буфера. Ноль — сесть уже нельзя.
func seatWindowMin(bookingStart, now time.Time, bufferMin int64) int {
	sec := int(bookingStart.Sub(now).Seconds()) - int(bufferMin)*60
	if sec <= 0 {
		return 0
	}
	return sec / 60
}

// isBookingLocked — ПК уже придержан под пришедшего по брони
// (за lockMin до начала и до конца брони).
func isBookingLocked(bookingStart, now time.Time, lockMin int64) bool {
	return !now.Before(bookingStart.Add(-time.Duration(lockMin) * time.Minute))
}

// liveBookingStatuses — брони, которые держат ПК (seated уже погашена).
var liveBookingStatuses = []models.BookingStatus{
	models.BookingStatusPending, models.BookingStatusConfirmed,
}

// nextRelevantBooking — ближайшая живая бронь ПК, чей конец ещё в будущем.
func nextRelevantBooking(computerID uuid.UUID, now time.Time) *models.Booking {
	var list []models.Booking
	db.Where("computer_id = ? AND status IN ?", computerID, liveBookingStatuses).
		Where("start_time + make_interval(mins => duration_min) > ?", now).
		Order("start_time").Limit(1).Find(&list)
	if len(list) == 0 {
		return nil
	}
	return &list[0]
}

// nextForeignBooking — то же, но чужая (для дедлайна сессии: собственная
// бронь гостя его не выгоняет — при посадке она гасится клеймом).
func nextForeignBooking(computerID, userID uuid.UUID, now time.Time) *models.Booking {
	var list []models.Booking
	db.Where("computer_id = ? AND user_id <> ? AND status IN ?", computerID, userID, liveBookingStatuses).
		Where("start_time + make_interval(mins => duration_min) > ?", now).
		Order("start_time").Limit(1).Find(&list)
	if len(list) == 0 {
		return nil
	}
	return &list[0]
}

// activeBookingsCount — живые брони гостя, чей конец ещё впереди (Г4-и2).
func activeBookingsCount(userID uuid.UUID, now time.Time) int64 {
	var n int64
	db.Model(&models.Booking{}).
		Where("user_id = ? AND status IN ?", userID, liveBookingStatuses).
		Where("start_time + make_interval(mins => duration_min) > ?", now).
		Count(&n)
	return n
}

// bookingSweep — дисциплина броней на тике фонового джоба (Г4):
// напоминания «через ~60/~15 минут» (по одному разу), затем no-show после
// no_show_min от начала. Гость в клубе за другим ПК — бронь честно гасится
// seated (человек пришёл), иначе no_show: гостю уведомление, счётчик в
// карточке, освободившееся окно зовёт вейтлист (Б9).
func bookingSweep(now time.Time) {
	var live []models.Booking
	db.Preload("Computer").
		Where("status IN ?", liveBookingStatuses).
		Where("start_time + make_interval(mins => duration_min) > ?", now).
		Find(&live)
	if len(live) == 0 {
		return
	}
	noShow := settingInt64("no_show_min", noShowMinDef)
	for i := range live {
		b := &live[i]
		pcName := ""
		if b.Computer != nil {
			pcName = b.Computer.Name
		}
		if leftMin := int(b.StartTime.Sub(now).Minutes()); leftMin >= 0 {
			// до начала: напоминания
			if leftMin <= remindLastMin && b.Remind15At == nil {
				notifyUser(b.UserID, "booking_reminder", map[string]any{
					"minutes_left": leftMin, "start_time": b.StartTime, "computer": pcName})
				db.Model(b).Updates(map[string]any{"remind15_at": now, "remind60_at": now})
			} else if leftMin <= remindFirstMin && b.Remind60At == nil {
				notifyUser(b.UserID, "booking_reminder", map[string]any{
					"minutes_left": leftMin, "start_time": b.StartTime, "computer": pcName})
				db.Model(b).Update("remind60_at", now)
			}
			continue
		}
		// бронь началась: no-show по таймеру
		if noShow <= 0 || now.Sub(b.StartTime) < time.Duration(noShow)*time.Minute {
			continue
		}
		var inClub int64
		db.Model(&models.Session{}).
			Where("user_id = ? AND club_id = ? AND status = ?",
				b.UserID, b.ClubID, models.SessionStatusActive).
			Count(&inClub)
		newStatus := models.BookingStatusNoShow
		if inClub > 0 {
			newStatus = models.BookingStatusSeated
		}
		res := db.Model(&models.Booking{}).
			Where("id = ? AND status IN ?", b.ID, liveBookingStatuses).
			Update("status", newStatus)
		if res.Error != nil || res.RowsAffected == 0 {
			continue
		}
		if newStatus == models.BookingStatusNoShow {
			notifyUser(b.UserID, "booking_no_show", map[string]any{
				"start_time": b.StartTime, "computer": pcName})
			hub.AdminBroadcast("booking", map[string]any{
				"kind": "no_show", "computer": pcName, "start_time": b.StartTime})
			checkWaitlistNotify(b.ClubID) // окно освободилось — зовём очередь
		}
	}
}

// claimBookingOnSeat — посадка хозяина гасит его ближайшую бронь на этом ПК
// (в т.ч. досрочно: сессия сама держит ПК, а бронь считается использованной).
//
// Ревью 26.08: «досрочно» ограничено окном. Раньше гасилась ЛЮБАЯ будущая
// бронь гостя на этой машине — забронировал субботу, зашёл во вторник и сел за
// тот же ПК, и субботняя бронь молча становилась seated. Вернуть её нельзя:
// отмена принимает только pending|confirmed, восстановление — только
// cancelled. Клеймим только бронь, которая уже началась или начнётся в
// пределах окна придержания.
func claimBookingOnSeat(computerID, userID uuid.UUID, now time.Time) *models.Booking {
	nb := nextRelevantBooking(computerID, now)
	if nb == nil || nb.UserID != userID {
		return nil
	}
	claimAhead := time.Duration(settingInt64("booking_lock_min", bookingLockMinDef)) * time.Minute
	if nb.StartTime.After(now.Add(claimAhead)) {
		return nil // бронь ещё далеко — гость просто сел раньше, она остаётся
	}
	res := db.Model(&models.Booking{}).
		Where("id = ? AND status IN ?", nb.ID, liveBookingStatuses).
		Update("status", models.BookingStatusSeated)
	if res.Error != nil || res.RowsAffected == 0 {
		return nil
	}
	nb.Status = models.BookingStatusSeated
	hub.AdminBroadcast("booking", map[string]any{
		"kind": "seated", "computer_id": computerID.String(), "start_time": nb.StartTime,
	})
	return nb
}

// ── Ж0-и3/и4 (ADMIN.md, трек Ж, решение Ж-Р5): одна бронь = один ПК ──────
//
// До ревью 14.09 walk-in бронь от стойки обходила и бан, и лимит живых броней
// (max_active_bookings, Г4-и2), а пересечение броней ОДНОГО гостя не проверял
// никто: Guest15 получал две живые брони на одно время на разных ПК. Проверки
// собраны в одну функцию — у гостя из PWA и у админа за стойкой правила одни.

// guestBookingConflict — чистое правило (тест): какая из живых броней гостя
// пересекается с [start, start+dur). Касание краями не пересечение.
func guestBookingConflict(existing []models.Booking, start time.Time, dur time.Duration) *models.Booking {
	for i := range existing {
		b := &existing[i]
		if b.Status != models.BookingStatusPending && b.Status != models.BookingStatusConfirmed {
			continue
		}
		if bookingOverlaps(start, dur, b.StartTime, time.Duration(b.DurationMin)*time.Minute) {
			return b
		}
	}
	return nil
}

// bookingGuestChecks — бан, лимит живых броней и пересечение с собственной
// бронью. Вернул код — обработчик отвечает 409 с message.
func bookingGuestChecks(user *models.User, start time.Time, durationMin int, now time.Time) (code, message string) {
	if user.Status == models.UserStatusBanned {
		return "banned", "Аккаунт заблокирован — бронь не оформляется"
	}
	if maxB := settingInt64("max_active_bookings", maxActiveBookingsDef); maxB > 0 &&
		activeBookingsCount(user.ID, now) >= maxB {
		return "booking_limit", fmt.Sprintf(
			"У гостя уже %d живых брони — лимит. Отмени лишнюю или дождись её времени", maxB)
	}
	dur := time.Duration(durationMin) * time.Minute
	var own []models.Booking
	db.Preload("Computer").
		Where("user_id = ? AND status IN ? AND start_time BETWEEN ? AND ?",
			user.ID, liveBookingStatuses, start.Add(-24*time.Hour), start.Add(24*time.Hour)).
		Find(&own)
	if b := guestBookingConflict(own, start, dur); b != nil {
		pc := "другой ПК"
		if b.Computer != nil {
			pc = b.Computer.Name
		}
		end := b.StartTime.Add(time.Duration(b.DurationMin) * time.Minute)
		return "guest_busy", fmt.Sprintf("У гостя уже есть бронь на %s–%s (%s) — одна бронь на одно время",
			b.StartTime.Local().Format("15:04"), end.Local().Format("15:04"), pc)
	}
	return "", ""
}

// isBookingOverlapErr — ограничение bookings_no_overlap (миграция 061,
// SQLSTATE 23P01): два запроса успели пройти проверку в коде, победил один.
// З7-и5: та же гонка может кончиться и взаимной блокировкой (40P01). EXCLUDE
// сверяет пересечение уже после записи, и две встречные записи ждут друг друга,
// пока база не снимет одну. Запись брони — один оператор, и ждать его может
// только встречная запись брони на тот же ПК, так что 40P01 здесь значит одно:
// слот взял встречный запрос. Раньше это было 500 «не получилось сохранить».
func isBookingOverlapErr(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "bookings_no_overlap") || strings.Contains(msg, "23P01") ||
		strings.Contains(msg, "exclusion constraint") ||
		strings.Contains(msg, "40P01") || strings.Contains(msg, "deadlock detected")
}

const slotBusyRaceMessage = "Этот ПК только что забронировали на это время — выбери другой ПК или время"
