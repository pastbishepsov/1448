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
	"time"

	"github.com/google/uuid"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

const (
	bookingBufferMinDef int64 = 15 // настройка booking_buffer_min
	bookingLockMinDef   int64 = 10 // настройка booking_lock_min
	defaultPlannedMin         = 30 // посадка без planned_min: минимальный сеанс
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

// claimBookingOnSeat — посадка хозяина гасит его ближайшую бронь на этом ПК
// (в т.ч. досрочно: сессия сама держит ПК, а бронь считается использованной).
func claimBookingOnSeat(computerID, userID uuid.UUID, now time.Time) *models.Booking {
	nb := nextRelevantBooking(computerID, now)
	if nb == nil || nb.UserID != userID {
		return nil
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
