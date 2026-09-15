package main

// Юнит-тесты правила посадки перед бронью (Г3, Р3 GUEST.md). Главный тест —
// контрольный пример основателя, буквально из его решения 2026-08-23.

import (
	"errors"
	"testing"
	"time"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

func TestSeatWindowFounderExample(t *testing.T) {
	// Бронь на 19:00, буфер 15 минут, гость хочет поиграть час.
	booking := time.Date(2026, 8, 23, 19, 0, 0, 0, time.UTC)
	const buffer = 15
	planned := 60

	at1800 := time.Date(2026, 8, 23, 18, 0, 0, 0, time.UTC)
	if w := seatWindowMin(booking, at1800, buffer); planned <= w {
		t.Errorf("18:00: окно %d мин — час НЕ должен влезать (пример основателя)", w)
	}

	at1745 := time.Date(2026, 8, 23, 17, 45, 0, 0, time.UTC)
	if w := seatWindowMin(booking, at1745, buffer); planned > w {
		t.Errorf("17:45: окно %d мин — час ДОЛЖЕН влезать (пример основателя)", w)
	}
}

func TestSeatWindowEdges(t *testing.T) {
	booking := time.Date(2026, 8, 23, 19, 0, 0, 0, time.UTC)
	// ровно на границе: 18:05 при буфере 15 → окно 40
	at := time.Date(2026, 8, 23, 18, 5, 0, 0, time.UTC)
	if w := seatWindowMin(booking, at, 15); w != 40 {
		t.Errorf("окно: ждали 40, получили %d", w)
	}
	// бронь уже почти началась — ноль, не минус
	late := time.Date(2026, 8, 23, 18, 50, 0, 0, time.UTC)
	if w := seatWindowMin(booking, late, 15); w != 0 {
		t.Errorf("после буфера: ждали 0, получили %d", w)
	}
	// нулевой буфер — окно целиком
	if w := seatWindowMin(booking, at, 0); w != 55 {
		t.Errorf("без буфера: ждали 55, получили %d", w)
	}
}

func TestBookingLock(t *testing.T) {
	booking := time.Date(2026, 8, 23, 19, 0, 0, 0, time.UTC)
	const lock = 10
	if isBookingLocked(booking, time.Date(2026, 8, 23, 18, 49, 0, 0, time.UTC), lock) {
		t.Error("18:49: ПК ещё не должен быть придержан")
	}
	if !isBookingLocked(booking, time.Date(2026, 8, 23, 18, 50, 0, 0, time.UTC), lock) {
		t.Error("18:50: ПК уже придержан под бронь (за 10 мин)")
	}
	if !isBookingLocked(booking, time.Date(2026, 8, 23, 19, 5, 0, 0, time.UTC), lock) {
		t.Error("19:05: бронь идёт — ПК придержан")
	}
}

// Ж0-и4 (Ж-Р5): пересечение с собственной бронью — только живые статусы,
// касание краями не пересечение.
func TestGuestBookingConflict(t *testing.T) {
	at := func(h, m int) time.Time { return time.Date(2026, 9, 15, h, m, 0, 0, time.UTC) }
	live := models.Booking{Status: models.BookingStatusConfirmed, StartTime: at(18, 0), DurationMin: 60}
	dead := models.Booking{Status: models.BookingStatusCancelled, StartTime: at(18, 0), DurationMin: 60}
	seated := models.Booking{Status: models.BookingStatusSeated, StartTime: at(18, 0), DurationMin: 60}
	cases := []struct {
		name  string
		list  []models.Booking
		start time.Time
		dur   time.Duration
		want  bool
	}{
		{"пересекается с живой", []models.Booking{live}, at(18, 30), time.Hour, true},
		{"внутри живой", []models.Booking{live}, at(18, 10), 20 * time.Minute, true},
		{"касание концом — можно", []models.Booking{live}, at(19, 0), time.Hour, false},
		{"касание началом — можно", []models.Booking{live}, at(17, 0), time.Hour, false},
		{"отменённая не держит", []models.Booking{dead}, at(18, 30), time.Hour, false},
		{"погашенная посадкой не держит", []models.Booking{seated}, at(18, 30), time.Hour, false},
		{"пусто", nil, at(18, 0), time.Hour, false},
	}
	for _, tc := range cases {
		if got := guestBookingConflict(tc.list, tc.start, tc.dur) != nil; got != tc.want {
			t.Errorf("%s: получили %v, ожидали %v", tc.name, got, tc.want)
		}
	}
}

func TestIsBookingOverlapErr(t *testing.T) {
	if isBookingOverlapErr(nil) {
		t.Error("nil — не ошибка пересечения")
	}
	if !isBookingOverlapErr(errors.New(`ERROR: conflicting key value violates exclusion constraint "bookings_no_overlap" (SQLSTATE 23P01)`)) {
		t.Error("EXCLUDE-ошибка не распознана")
	}
	if isBookingOverlapErr(errors.New("duplicate key value violates unique constraint (SQLSTATE 23505)")) {
		t.Error("UNIQUE спутан с EXCLUDE")
	}
}
