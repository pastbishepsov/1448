package main

import (
	"testing"
	"time"
)

// Ж1-и1: что забирается при отмене депозита — остаток кошелька не больше
// суммы, монеты не ниже нуля, отыгранное — в потери.
func TestPlanDepositVoid(t *testing.T) {
	cases := []struct {
		name                                        string
		wallet, coins, amount, given                int64
		wantRefund, wantUnrecovered, wantCoinsTaken int64
	}{
		{"нетронутый депозит — всё назад", 5000, 500, 5000, 500, 5000, 0, 500},
		{"часть отыграна — остаток назад, разница в потери", 1200, 500, 5000, 500, 1200, 3800, 500},
		{"кошелёк пуст — ничего не снять, вся сумма отыграна", 0, 500, 5000, 500, 0, 5000, 500},
		{"в кошельке больше суммы (другие депозиты) — снимаем ровно сумму", 9000, 500, 5000, 500, 5000, 0, 500},
		{"монеты частично потрачены — забираем что осталось", 5000, 120, 5000, 500, 5000, 0, 120},
		{"монет нет — не уходим в минус", 5000, 0, 5000, 500, 5000, 0, 0},
		{"депозит без монет (курс 0)", 5000, 700, 5000, 0, 5000, 0, 0},
		{"бонус таланта входит в выданные монеты", 5000, 1000, 5000, 575, 5000, 0, 575},
		{"нулевая сумма — ничего", 5000, 500, 0, 0, 0, 0, 0},
	}
	for _, tc := range cases {
		p := planDepositVoid(tc.wallet, tc.coins, tc.amount, tc.given)
		if p.Refund != tc.wantRefund || p.Unrecovered != tc.wantUnrecovered || p.CoinsTaken != tc.wantCoinsTaken {
			t.Errorf("%s: refund=%d unrecovered=%d coins=%d, ожидалось %d/%d/%d",
				tc.name, p.Refund, p.Unrecovered, p.CoinsTaken, tc.wantRefund, tc.wantUnrecovered, tc.wantCoinsTaken)
		}
		if p.Refund+p.Unrecovered != tc.amount && tc.amount > 0 {
			t.Errorf("%s: refund + unrecovered = %d, а сумма депозита %d — деньги потерялись", tc.name, p.Refund+p.Unrecovered, tc.amount)
		}
	}
}

// Права на отмену депозита — те же, что у продажи (canVoidSale): депозит без
// автора (старые записи до created_by) админ отменить не может, владелец — может.
func TestDepositVoidRightsWithoutAuthor(t *testing.T) {
	loc := time.FixedZone("CEST", 2*3600)
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, loc)
	from, to, _, _ := shiftWindow("", 8, now)
	at := now.Add(-2 * time.Hour)
	if ok, code := canVoidSale("admin", "", "11111111-1111-1111-1111-111111111111", at, from, to); ok || code != "not_yours" {
		t.Errorf("admin без автора: ok=%v code=%s, ожидалось not_yours", ok, code)
	}
	if ok, _ := canVoidSale("owner", "", "11111111-1111-1111-1111-111111111111", at, from, to); !ok {
		t.Errorf("owner без автора должен отменять")
	}
}
