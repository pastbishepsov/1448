package main

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

// З8-и1: ящик клуба — остаток на начало + пришло − ушло ± коррекции; остаток
// после сдачи — пересчитанное, без пересчёта — ожидавшееся.
func TestDrawerExpected(t *testing.T) {
	g := func(v int64) *int64 { return &v }
	cases := []struct {
		name     string
		opening  int64
		flow     drawerFlow
		expected int64
	}{
		{"пусто — сколько оставили", 35000, drawerFlow{}, 35000},
		{"пополнение админа и владельца — оба в ящике (ложный +70 ревью)", 35000, drawerFlow{Deposits: 10000}, 45000},
		{"товары, кухня и пакеты", 0, drawerFlow{Deposits: 3000, Goods: 1250, Packs: 5000}, 9250},
		{"возвраты наличными уменьшают", 48000, drawerFlow{Packs: 5000, Refunds: 5000}, 48000},
		{"инкассация уменьшает, коррекция двигает в обе стороны", 50000, drawerFlow{Collections: 30000, Adjust: -1500}, 18500},
		{"коррекция вверх", 10000, drawerFlow{Adjust: 700}, 10700},
	}
	for _, tc := range cases {
		if got := drawerExpected(tc.opening, tc.flow); got != tc.expected {
			t.Errorf("%s: %d, ожидалось %d", tc.name, got, tc.expected)
		}
	}
	if f := (drawerFlow{Deposits: 1, Goods: 2, Packs: 4, Refunds: 8, Collections: 16}); f.In() != 7 || f.Out() != 24 {
		t.Errorf("пришло/ушло: %d/%d", f.In(), f.Out())
	}
	if got := drawerClosing(48000, g(47000)); got != 47000 {
		t.Errorf("пересчитали — остаток пересчитанный: %d", got)
	}
	if got := drawerClosing(48000, nil); got != 48000 {
		t.Errorf("без пересчёта — остаток ожидавшийся: %d", got)
	}
}

// Сдача ложится в ящик: нулевая точка, сдача вне ящика, обычная с расхождением.
func TestDrawerCloseout(t *testing.T) {
	g := func(v int64) *int64 { return &v }
	point := &drawerPoint{At: time.Now().Add(-time.Hour), Closing: 45000, By: uuid.New(), Counted: true}

	var co models.ShiftCloseout
	drawerCloseout(&co, drawerState{}, g(35000))
	if !co.Baseline || co.OpeningGrosz == nil || *co.OpeningGrosz != 35000 || co.ExpectedGrosz != 35000 ||
		co.DiffGrosz == nil || *co.DiffGrosz != 0 {
		t.Errorf("нулевая точка: пересчитанное = остаток на начало = ожидаемое, без расхождения: %+v", co)
	}
	if line := closeoutOwnerLine("admin1", &co); line != "🧮 Ящик: начальный остаток 350.00 zł, пересчитал admin1 — дальше касса считает ящик от этой суммы" {
		t.Errorf("строка владельцу о нулевой точке: %q", line)
	}
	if d := closeoutDetails(&co); d != "касса: начальный остаток ящика 350.00 zł" {
		t.Errorf("журнал нулевой точки: %q", d)
	}

	co = models.ShiftCloseout{Skipped: true}
	drawerCloseout(&co, drawerState{}, nil)
	if co.Baseline || co.OpeningGrosz != nil || co.ExpectedGrosz != 0 {
		t.Errorf("ящика нет и не считали — сдача вне ящика, точкой отсчёта не служит: %+v", co)
	}
	if line := closeoutOwnerLine("admin1", &co); !strings.Contains(line, "начального остатка ящика ещё нет") {
		t.Errorf("строка владельцу без ящика: %q", line)
	}

	co = models.ShiftCloseout{}
	flow := drawerFlow{Deposits: 10000, Goods: 500, Refunds: 500}
	drawerCloseout(&co, drawerState{Point: point, Flow: flow, Expected: drawerExpected(point.Closing, flow)}, g(54000))
	if co.Baseline || co.OpeningGrosz == nil || *co.OpeningGrosz != 45000 || co.ExpectedGrosz != 55000 ||
		co.InGrosz != 10500 || co.OutGrosz != 500 || co.DepositsGrosz != 10000 || co.RefundsGrosz != 500 ||
		co.DiffGrosz == nil || *co.DiffGrosz != -1000 {
		t.Errorf("обычная сдача: на начало 450, +105 −5, пересчитано 540 → −10: %+v", co)
	}
	if d := closeoutDetails(&co); d != "касса: должно быть 550.00 zł (на начало 450.00 + пришло 105.00 − ушло 5.00), пересчитано 540.00 zł, расхождение -10.00 zł" {
		t.Errorf("журнал обычной сдачи: %q", d)
	}
	if line := closeoutOwnerLine("admin2", &co); !strings.Contains(line, "расхождение −10.00 zł") {
		t.Errorf("строка владельцу: %q", line)
	}
}
