package main

import (
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

// З9-и3б: возврат владельцу — сумма от 0,01 zł и не больше двух знаков, откуда — из кассы или с банка,
// заметка до 200 символов. Остаток долга проверяется под замком, не здесь.
func TestCheckRepayment(t *testing.T) {
	f := func(v float64) *float64 { return &v }
	long := make([]rune, 201)
	for i := range long {
		long[i] = 'я'
	}
	cases := []struct {
		name string
		req  repaymentRequest
		code string
	}{
		{"из кассы", repaymentRequest{AmountPLN: f(200), Method: "till"}, ""},
		{"с банка, с заметкой", repaymentRequest{AmountPLN: f(0.01), Method: "bank", Note: "за роутер"}, ""},
		{"без суммы", repaymentRequest{Method: "till"}, "bad_amount"},
		{"ноль", repaymentRequest{AmountPLN: f(0), Method: "till"}, "bad_amount"},
		{"минус", repaymentRequest{AmountPLN: f(-5), Method: "bank"}, "bad_amount"},
		{"три знака", repaymentRequest{AmountPLN: f(1.005), Method: "bank"}, "bad_amount"},
		{"больше миллиона", repaymentRequest{AmountPLN: f(1_000_000.01), Method: "bank"}, "bad_amount"},
		{"карта — не способ возврата", repaymentRequest{AmountPLN: f(10), Method: "card"}, "bad_method"},
		{"без способа", repaymentRequest{AmountPLN: f(10)}, "bad_method"},
		{"заметка 201 символ", repaymentRequest{AmountPLN: f(10), Method: "till", Note: string(long)}, "bad_note"},
	}
	for _, tc := range cases {
		if got := checkRepayment(tc.req); got != tc.code {
			t.Errorf("%s: %q, ожидалось %q", tc.name, got, tc.code)
		}
	}
}

// Возврат из кассы — «ушло» ящика: должно быть меньше; с банка в ящик не попадает вовсе (drawerFlowFor
// берёт только method = till). В сдаче — своей строкой, инкассации из «ушло» его не вычитают как свои.
func TestDrawerOwnerRepayment(t *testing.T) {
	flow := drawerFlow{Deposits: 20000, Collections: 5000, Expenses: 1500, Owner: 4000}
	if got := drawerExpected(30000, flow); got != 39500 {
		t.Errorf("300 на начало + 200 − 50 инкассация − 15 расход − 40 владельцу = 395: %d", got)
	}
	if flow.Out() != 10500 {
		t.Errorf("ушло: %d", flow.Out())
	}
	if out := flow.out(); out["owner_pln"] != 40.0 || out["out_pln"] != 105.0 {
		t.Errorf("в ответе — owner_pln и общее «ушло»: %v", out)
	}
	point := &drawerPoint{At: time.Now().Add(-time.Hour), Closing: 30000, By: uuid.New(), Counted: true}
	var co models.ShiftCloseout
	counted := int64(39500)
	drawerCloseout(&co, drawerState{Point: point, Flow: flow, Expected: drawerExpected(point.Closing, flow)}, &counted)
	if co.OwnerGrosz != 4000 || co.OutGrosz != 10500 || *co.DiffGrosz != 0 {
		t.Errorf("сдача: владельцу 40, ушло 105, сошлось: %+v", co)
	}
	row := closeoutOut(&co)
	if row["owner_pln"] != 40.0 || row["collections_pln"] != 50.0 || row["expenses_pln"] != 15.0 {
		t.Errorf("сдача наружу: владельцу 40, инкассации 50 (без возврата владельцу), расходы 15: %v", row)
	}
}

// Долг — потрачено личными минус возвращено; строка журнала и отказ «больше долга» — словами.
func TestOwnerDebtWords(t *testing.T) {
	d := ownerDebtSums{Spent: 50000, Repaid: 12000}
	if d.Debt() != 38000 {
		t.Errorf("долг: %d", d.Debt())
	}
	if out := d.out(); out["debt_pln"] != 380.0 || out["spent_pln"] != 500.0 || out["repaid_pln"] != 120.0 {
		t.Errorf("наружу: %v", out)
	}
	r := models.OwnerRepayment{AmountGrosz: 20000, Method: models.RepayTill, Note: "за роутер"}
	if got := repaymentWhat(&r); got != "200.00 zł из кассы · за роутер" {
		t.Errorf("журнал: %q", got)
	}
	r = models.OwnerRepayment{AmountGrosz: 5, Method: models.RepayBank}
	if got := repaymentWhat(&r); got != "0.05 zł с банка" {
		t.Errorf("журнал без заметки: %q", got)
	}
	r.CreatedAt = time.Date(2026, 9, 27, 19, 40, 0, 0, time.UTC) // 21:40 по Варшаве (летнее время)
	if got := repaymentWhen(&r); os.Getenv("CLUB_TZ") == "" && got != " (внесён 27.09 21:40)" {
		t.Errorf("отмена — когда внесён, по часам клуба: %q", got)
	}
	if got := overDebtMessage(38000); got != "Клуб должен владельцу 380.00 zł — больше вернуть нельзя" {
		t.Errorf("отказ: %q", got)
	}
	if got := overDebtMessage(0); got != "Клуб ничего не должен владельцу — возвращать нечего" {
		t.Errorf("отказ без долга: %q", got)
	}
}
