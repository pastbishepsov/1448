package main

// Бронирование ПК. Правила MVP:
//   - бронь только в будущем, длительность 30–480 минут;
//   - один ПК не может быть забронирован дважды на пересекающееся время
//     (учитываются статусы pending/confirmed);
//   - если computer_id не указан — берём первый ПК клуба без конфликтов;
//   - MVP: бронь сразу confirmed, без предоплаты (prepaid=false).
//     Предоплата и лимиты по уровню — после подключения платежей.

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

const (
	bookingMinDuration = 30 * time.Minute
	bookingMaxDuration = 8 * time.Hour
)

// bookingOverlaps — пересекаются ли два интервала [start, start+dur).
// Касание краями (конец одного == начало другого) пересечением НЕ считается.
func bookingOverlaps(aStart time.Time, aDur time.Duration, bStart time.Time, bDur time.Duration) bool {
	aEnd := aStart.Add(aDur)
	bEnd := bStart.Add(bDur)
	return aStart.Before(bEnd) && bStart.Before(aEnd)
}

// validateBookingTime — чистая проверка времени брони (тестируется отдельно).
func validateBookingTime(start time.Time, durationMin int, now time.Time) (ok bool, code string) {
	d := time.Duration(durationMin) * time.Minute
	if d < bookingMinDuration || d > bookingMaxDuration {
		return false, "bad_duration"
	}
	if start.Before(now.Add(-time.Minute)) {
		return false, "in_past"
	}
	if start.After(now.Add(30 * 24 * time.Hour)) {
		return false, "too_far"
	}
	return true, ""
}

type createBookingRequest struct {
	ComputerID  *string `json:"computer_id"` // опционально: иначе первый свободный
	StartTime   string  `json:"start_time" binding:"required"` // RFC3339
	DurationMin int     `json:"duration_min"`
	Notes       *string `json:"notes"`
}

// POST /clubs/:id/bookings — создать бронь (JWT).
func handleCreateBooking(c *gin.Context) {
	userID, err := uuid.Parse(c.GetString("user_id"))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": "invalid_user", "message": "Некорректный пользователь"})
		return
	}

	var club models.Club
	if err := db.First(&club, "id = ? AND is_active = ?", c.Param("id"), true).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "club_not_found", "message": "Клуб не найден"})
		return
	}

	var req createBookingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_request", "message": err.Error()})
		return
	}
	if req.DurationMin == 0 {
		req.DurationMin = 60
	}
	start, err := time.Parse(time.RFC3339, req.StartTime)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "bad_time", "message": "start_time — в формате RFC3339, например 2026-07-03T18:00:00+02:00"})
		return
	}
	if ok, code := validateBookingTime(start, req.DurationMin, time.Now()); !ok {
		msg := map[string]string{
			"bad_duration": "Длительность брони: от 30 минут до 8 часов",
			"in_past":      "Бронь в прошлом невозможна",
			"too_far":      "Бронь возможна максимум за 30 дней",
		}[code]
		c.JSON(http.StatusBadRequest, gin.H{"code": code, "message": msg})
		return
	}
	dur := time.Duration(req.DurationMin) * time.Minute

	// Кандидаты: конкретный ПК или все ПК клуба (кроме обслуживания).
	var candidates []models.Computer
	q := db.Where("club_id = ? AND status <> ?", club.ID, models.ComputerStatusMaintenance).Order("name")
	if req.ComputerID != nil && *req.ComputerID != "" {
		q = q.Where("id = ?", *req.ComputerID)
	}
	if err := q.Find(&candidates).Error; err != nil || len(candidates) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"code": "no_computer", "message": "Подходящий ПК не найден"})
		return
	}

	// Активные брони клуба в окне ±1 день вокруг запрошенного времени.
	var existing []models.Booking
	db.Where("club_id = ? AND status IN ? AND start_time BETWEEN ? AND ?",
		club.ID,
		[]models.BookingStatus{models.BookingStatusPending, models.BookingStatusConfirmed},
		start.Add(-24*time.Hour), start.Add(24*time.Hour),
	).Find(&existing)

	busy := map[string]bool{}
	for _, b := range existing {
		if bookingOverlaps(start, dur, b.StartTime, time.Duration(b.DurationMin)*time.Minute) {
			busy[b.ComputerID.String()] = true
		}
	}

	var pick *models.Computer
	for i := range candidates {
		if !busy[candidates[i].ID.String()] {
			pick = &candidates[i]
			break
		}
	}
	if pick == nil {
		c.JSON(http.StatusConflict, gin.H{"code": "slot_busy", "message": "На это время всё занято — выбери другое время"})
		return
	}

	// Б7-и1: предоплаты до Stripe/BLIK не существует — флаг честный:
	// false у всех (как у walk-in). Талант priority_booking заиграет со
	// спринтом 6 (платежи): будет освобождать от обязательной предоплаты.
	booking := models.Booking{
		UserID:      userID,
		ComputerID:  pick.ID,
		ClubID:      club.ID,
		Status:      models.BookingStatusConfirmed,
		StartTime:   start,
		DurationMin: req.DurationMin,
		Prepaid:     false,
		Notes:       req.Notes,
	}
	if err := db.Create(&booking).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "db_error", "message": err.Error()})
		return
	}

	// Live-событие в админку (А6; Б4-и3 — теперь с ником гостя).
	var creator models.User
	db.First(&creator, "id = ?", userID)
	hub.AdminBroadcast("booking", map[string]any{
		"kind": "create", "nickname": creator.Nickname,
		"computer": pick.Name, "start_time": booking.StartTime,
	})

	c.JSON(http.StatusCreated, gin.H{
		"booking_id": booking.ID, "status": booking.Status,
		"computer": pick.Name, "club": club.Name,
		"start_time": booking.StartTime, "duration_min": booking.DurationMin,
		"prepaid": booking.Prepaid,
	})
}

// GET /me/bookings — мои брони (будущие сверху).
func handleGetMyBookings(c *gin.Context) {
	var bookings []models.Booking
	db.Preload("Computer").Preload("Club").
		Where("user_id = ?", c.GetString("user_id")).
		Order("start_time DESC").Limit(50).
		Find(&bookings)
	c.JSON(http.StatusOK, gin.H{"count": len(bookings), "bookings": bookings})
}

// DELETE /me/bookings/:id — отмена своей будущей брони.
// Б4-и3: админ-лента узнаёт live (раньше гостевая отмена терялась).
func handleCancelBooking(c *gin.Context) {
	var booking models.Booking
	if err := db.Preload("Computer").
		First(&booking, "id = ? AND user_id = ?", c.Param("id"), c.GetString("user_id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "booking_not_found", "message": "Бронь не найдена"})
		return
	}
	if booking.Status != models.BookingStatusPending && booking.Status != models.BookingStatusConfirmed {
		c.JSON(http.StatusConflict, gin.H{"code": "not_cancellable", "message": "Эту бронь уже нельзя отменить"})
		return
	}
	if err := db.Model(&booking).Update("status", models.BookingStatusCancelled).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "db_error", "message": err.Error()})
		return
	}
	var u models.User
	db.First(&u, "id = ?", booking.UserID)
	pcName := ""
	if booking.Computer != nil {
		pcName = booking.Computer.Name
	}
	hub.AdminBroadcast("booking", map[string]any{
		"kind": "cancel", "nickname": u.Nickname,
		"computer": pcName, "start_time": booking.StartTime,
	})
	c.JSON(http.StatusOK, gin.H{"booking_id": booking.ID, "status": models.BookingStatusCancelled})
}
