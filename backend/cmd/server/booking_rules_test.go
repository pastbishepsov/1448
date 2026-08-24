package main

// Юнит-тесты правила посадки перед бронью (Г3, Р3 GUEST.md). Главный тест —
// контрольный пример основателя, буквально из его решения 2026-08-23.

import (
	"testing"
	"time"
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
