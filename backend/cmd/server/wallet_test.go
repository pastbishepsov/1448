package main

// Юнит-тесты кошелька (Г0-и1): чистая арифметика без БД. Инвариант журнала
// «сумма операций = баланс» держится тем, что walletApply — единственная
// дверь: она считает баланс через walletNewBalance и пишет balance_after той
// же величиной. Здесь проверяем саму арифметику и конверсию грошей.

import (
	"errors"
	"testing"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

func TestWalletNewBalance(t *testing.T) {
	cases := []struct {
		name    string
		balance int64
		amount  int64
		want    int64
		wantErr bool
	}{
		{"пополнение с нуля", 0, 2000, 2000, false},
		{"списание в ноль", 1500, -1500, 0, false},
		{"обычное списание", 2000, -37, 1963, false},
		{"списание больше остатка — долгов нет (Р8)", 100, -101, 100, true},
		{"нулевая операция", 500, 0, 500, false},
	}
	for _, tc := range cases {
		got, err := walletNewBalance(tc.balance, tc.amount)
		if tc.wantErr {
			if !errors.Is(err, errWalletInsufficient) {
				t.Errorf("%s: ждали errWalletInsufficient, получили %v", tc.name, err)
			}
			if got != tc.balance {
				t.Errorf("%s: при отказе баланс должен остаться %d, получили %d", tc.name, tc.balance, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: неожиданная ошибка %v", tc.name, err)
		}
		if got != tc.want {
			t.Errorf("%s: ждали %d, получили %d", tc.name, tc.want, got)
		}
	}
}

// Инвариант журнала: последовательность операций через walletNewBalance
// даёт баланс, равный сумме проведённых сумм (отклонённые не считаются).
func TestWalletLedgerInvariant(t *testing.T) {
	ops := []int64{2000, -500, -1500, 3000, -37, -100000, 65}
	var balance, applied int64
	for _, amount := range ops {
		next, err := walletNewBalance(balance, amount)
		if err != nil {
			continue // отклонённая операция не попадает в журнал
		}
		balance = next
		applied += amount
	}
	if balance != applied {
		t.Errorf("инвариант нарушен: баланс %d, сумма проведённых операций %d", balance, applied)
	}
	if balance != 3028 {
		t.Errorf("контрольная сумма: ждали 3028, получили %d", balance)
	}
}

func TestGroszConversion(t *testing.T) {
	cases := []struct {
		pln   float64
		grosz int64
	}{
		{20, 2000},
		{20.5, 2050},
		{0.01, 1},
		{0.015, 2},  // округление к ближайшему грошу
		{999.99, 99999},
	}
	for _, tc := range cases {
		if got := models.GroszFromPLN(tc.pln); got != tc.grosz {
			t.Errorf("GroszFromPLN(%v): ждали %d, получили %d", tc.pln, tc.grosz, got)
		}
	}
	if got := models.PLNFromGrosz(2050); got != 20.5 {
		t.Errorf("PLNFromGrosz(2050): ждали 20.5, получили %v", got)
	}
	if got := models.PLNFromGrosz(1); got != 0.01 {
		t.Errorf("PLNFromGrosz(1): ждали 0.01, получили %v", got)
	}
}
