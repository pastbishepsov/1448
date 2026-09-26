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

// З8-и2: инкассация — админ с заметкой «по чьему указанию» (Д11), владелец и
// без неё; коррекция — только владелец и с причиной, в ящике от 0.
func TestCheckCashEvent(t *testing.T) {
	f := func(v float64) *float64 { return &v }
	collect := func(amount *float64, dest, note string) cashEventRequest {
		return cashEventRequest{Kind: "collect", AmountPLN: amount, Dest: dest, Note: note}
	}
	correct := func(stated *float64, note string) cashEventRequest {
		return cashEventRequest{Kind: "correct", StatedPLN: stated, Note: note}
	}
	cases := []struct {
		name  string
		req   cashEventRequest
		owner bool
		want  string
	}{
		{"инкассация админа с заметкой", collect(f(300), "safe", "по указанию Ани"), false, ""},
		{"инкассация админа без заметки", collect(f(300), "safe", "  "), false, "need_note"},
		{"инкассация владельца без заметки", collect(f(300), "bank", ""), true, ""},
		{"инкассация на ноль", collect(f(0), "safe", "x"), false, "bad_amount"},
		{"инкассация без суммы", collect(nil, "safe", "x"), false, "bad_amount"},
		{"три знака после запятой", collect(f(1.005), "safe", "x"), false, "bad_amount"},
		{"больше миллиона", collect(f(1_000_000.01), "safe", "x"), true, "bad_amount"},
		{"куда — в карман", collect(f(10), "pocket", "x"), false, "bad_dest"},
		{"заметка длиннее 200", collect(f(10), "safe", strings.Repeat("я", 201)), false, "bad_note"},
		{"коррекция админа", correct(f(250), "пересчитал"), false, "owner_only"},
		{"коррекция владельца", correct(f(250), "пересчитал вечером"), true, ""},
		{"в ящике ноль — можно", correct(f(0), "всё сдали в банк"), true, ""},
		{"в ящике меньше нуля", correct(f(-1), "x"), true, "bad_stated"},
		{"коррекция без причины", correct(f(250), ""), true, "need_reason"},
		{"неизвестный вид", cashEventRequest{Kind: "spend", AmountPLN: f(10), Dest: "safe", Note: "x"}, true, "bad_kind"},
	}
	for _, tc := range cases {
		if got := checkCashEvent(tc.req, tc.owner); got != tc.want {
			t.Errorf("%s: %q, ожидалось %q", tc.name, got, tc.want)
		}
	}
}

func TestCashEventText(t *testing.T) {
	safe, stated := "safe", int64(25000)
	collect := models.CashEvent{Kind: "collect", AmountGrosz: 30000, Dest: &safe, Note: "по указанию Ани"}
	if got := cashEventText(&collect); got != "инкассация 300.00 zł в сейф · по указанию Ани" {
		t.Errorf("журнал инкассации: %q", got)
	}
	if got := cashEventWhat(&collect); got != "300.00 zł в сейф" {
		t.Errorf("строка журнала — без вида, его называет метка: %q", got)
	}
	if got := cashCollectLine("admin1", &collect); got != "🧮 admin1: инкассация 300.00 zł в сейф — «по указанию Ани»" {
		t.Errorf("строка владельцу: %q", got)
	}
	correct := models.CashEvent{Kind: "correct", AmountGrosz: -15000, StatedGrosz: &stated, Note: "пересчитал вечером"}
	if got := cashEventText(&correct); got != "коррекция: в ящике 250.00 zł (поправка -150.00) · пересчитал вечером" {
		t.Errorf("журнал коррекции: %q", got)
	}
}
