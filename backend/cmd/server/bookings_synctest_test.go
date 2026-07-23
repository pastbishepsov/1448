package main

// Истечение брони на виртуальном времени (Go 1.25+, testing/synctest):
// «проживаем» часы за миллисекунды и проверяем, что временные правила брони
// ведут себя одинаково в любой момент — без ручной подгонки дат и sleep'ов.

import (
	"testing"
	"testing/synctest"
	"time"
)

func TestBookingExpiresVirtualTime(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		start := time.Now().Add(2 * time.Hour) // бронь через два часа
		const durMin = 60

		if ok, code := validateBookingTime(start, durMin, time.Now()); !ok {
			t.Fatalf("будущая бронь должна проходить, отказ: %s", code)
		}

		// Время подошло: в момент старта слот ещё валиден
		// (льгота −1 минута в validateBookingTime).
		time.Sleep(2 * time.Hour)
		if ok, code := validateBookingTime(start, durMin, time.Now()); !ok {
			t.Fatalf("бронь в момент старта должна проходить, отказ: %s", code)
		}

		// Опоздали на 2 минуты — слот истёк, бронь на этот старт не создать.
		time.Sleep(2 * time.Minute)
		if ok, code := validateBookingTime(start, durMin, time.Now()); ok || code != "in_past" {
			t.Fatalf("просроченный старт должен давать in_past, получено ok=%v code=%q", ok, code)
		}
	})
}

// Бронь держит ПК ровно своё окно [start, start+duration) и отпускает после.
func TestBookingWindowReleasesComputer(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		dur := 60 * time.Minute
		start := time.Now().Add(30 * time.Minute)
		probe := time.Minute // «этот ПК нужен прямо сейчас, на минуту»

		if bookingOverlaps(start, dur, time.Now(), probe) {
			t.Fatal("до старта брони ПК должен быть свободен")
		}
		time.Sleep(30 * time.Minute) // старт окна
		if !bookingOverlaps(start, dur, time.Now(), probe) {
			t.Fatal("в окне брони ПК должен быть занят")
		}
		time.Sleep(59 * time.Minute) // последняя минута окна
		if !bookingOverlaps(start, dur, time.Now(), probe) {
			t.Fatal("на последней минуте окна ПК должен быть занят")
		}
		time.Sleep(time.Minute) // окно кончилось — бронь истекла
		if bookingOverlaps(start, dur, time.Now(), probe) {
			t.Fatal("после конца окна бронь истекла — ПК должен быть свободен")
		}
	})
}
