package main

import (
	"testing"
	"time"
)

func TestBookingOverlaps(t *testing.T) {
	base := time.Date(2026, 7, 3, 18, 0, 0, 0, time.UTC)
	h := time.Hour

	cases := []struct {
		name           string
		aStart         time.Time
		aDur           time.Duration
		bStart         time.Time
		bDur           time.Duration
		expectsOverlap bool
	}{
		{"одинаковое время", base, h, base, h, true},
		{"внутри", base, 2 * h, base.Add(30 * time.Minute), h, true},
		{"частичное с конца", base, h, base.Add(30 * time.Minute), h, true},
		{"касание краями", base, h, base.Add(h), h, false},
		{"раздельные", base, h, base.Add(3 * h), h, false},
		{"вторая раньше с пересечением", base, h, base.Add(-30 * time.Minute), h, true},
		{"вторая раньше без пересечения", base, h, base.Add(-2 * h), h, false},
	}
	for _, tc := range cases {
		if got := bookingOverlaps(tc.aStart, tc.aDur, tc.bStart, tc.bDur); got != tc.expectsOverlap {
			t.Errorf("%s: overlap=%v, ожидалось %v", tc.name, got, tc.expectsOverlap)
		}
		// симметричность
		if got := bookingOverlaps(tc.bStart, tc.bDur, tc.aStart, tc.aDur); got != tc.expectsOverlap {
			t.Errorf("%s (зеркально): overlap=%v, ожидалось %v", tc.name, got, tc.expectsOverlap)
		}
	}
}

func TestValidateBookingTime(t *testing.T) {
	now := time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)

	if ok, _ := validateBookingTime(now.Add(2*time.Hour), 60, now); !ok {
		t.Error("нормальная бронь отклонена")
	}
	if ok, code := validateBookingTime(now.Add(-time.Hour), 60, now); ok || code != "in_past" {
		t.Errorf("бронь в прошлом прошла: %v %s", ok, code)
	}
	if ok, code := validateBookingTime(now.Add(time.Hour), 15, now); ok || code != "bad_duration" {
		t.Errorf("15 минут прошли: %v %s", ok, code)
	}
	if ok, code := validateBookingTime(now.Add(time.Hour), 600, now); ok || code != "bad_duration" {
		t.Errorf("10 часов прошли: %v %s", ok, code)
	}
	if ok, code := validateBookingTime(now.Add(40*24*time.Hour), 60, now); ok || code != "too_far" {
		t.Errorf("бронь через 40 дней прошла: %v %s", ok, code)
	}
	// граница: ровно 30 минут и ровно 8 часов — валидны
	if ok, _ := validateBookingTime(now.Add(time.Hour), 30, now); !ok {
		t.Error("30 минут отклонены")
	}
	if ok, _ := validateBookingTime(now.Add(time.Hour), 480, now); !ok {
		t.Error("480 минут отклонены")
	}
}
