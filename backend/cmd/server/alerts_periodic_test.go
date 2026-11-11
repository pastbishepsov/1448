package main

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

// З22-и3: кому и когда уходят периодические алерты.

func TestDebtsToAlert(t *testing.T) {
	now := time.Date(2026, 11, 10, 15, 0, 0, 0, time.UTC)
	r := func(grosz int64, ago time.Duration) receivable {
		return receivable{UserID: uuid.New(), BarGrosz: grosz, Since: now.Add(-ago)}
	}
	rows := []receivable{
		r(4500, 30*time.Hour), // 45 zł 30 часов — да
		r(1000, 72*time.Hour), // 10 zł — меньше порога
		r(6000, 2*time.Hour),  // 60 zł, но 2 часа — ещё не сутки
		r(3000, 24*time.Hour), // ровно порог и ровно сутки — да
		{UserID: uuid.New(), KitchenGrosz: 2000, BarGrosz: 1500, Since: now.Add(-25 * time.Hour)}, // кухня + бар = 35 — да
	}
	got := debtsToAlert(rows, now, 3000)
	if len(got) != 3 || got[0].UserID != rows[0].UserID || got[1].UserID != rows[3].UserID || got[2].UserID != rows[4].UserID {
		t.Errorf("долги для алерта: %+v", got)
	}
}

func TestVoidShareHit(t *testing.T) {
	cases := []struct {
		ops, voided, pct int64
		want             bool
	}{
		{10, 4, 30, true},  // 40 % ≥ 30
		{10, 3, 30, true},  // ровно 30 %
		{10, 2, 20, false}, // 20 %, но отмен меньше трёх
		{40, 6, 20, false}, // 15 % < 20
		{0, 0, 20, false},  // операций нет
		{3, 3, 100, true},  // всё отменено
	}
	for _, tc := range cases {
		if got := voidShareHit(tc.ops, tc.voided, tc.pct); got != tc.want {
			t.Errorf("voidShareHit(%d, %d, %d) = %v, ожидалось %v", tc.ops, tc.voided, tc.pct, got, tc.want)
		}
	}
}

func TestRevenueLow(t *testing.T) {
	cases := []struct {
		name        string
		today, norm float64
		weeks       int
		pct         int64
		want        bool
	}{
		{"ниже на 40 % при пороге 30", 600, 1000, 8, 30, true},
		{"ровно на пороге", 700, 1000, 8, 30, true},
		{"ниже на 20 % — нет", 800, 1000, 8, 30, false},
		{"три недели истории — рано судить", 0, 1000, 3, 30, false},
		{"норма 80 zł — к этому часу обычно почти ничего", 0, 80, 8, 30, false},
		{"выше нормы — нет", 1500, 1000, 8, 30, false},
	}
	for _, tc := range cases {
		if got := revenueLow(tc.today, tc.norm, tc.weeks, tc.pct); got != tc.want {
			t.Errorf("%s: %v, ожидалось %v", tc.name, got, tc.want)
		}
	}
}
