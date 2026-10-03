package main

import "testing"

// З12-и2: списание пакета стоит долю его цены — разность «отыграно после» и «отыграно до», поэтому тики в сумме дают
// ровно цену отыгранного; подаренный пакет — 0.
func TestPackSpendGrosz(t *testing.T) {
	cases := []struct {
		price             int64
		total, before, to int
		gift              bool
		want              int64
	}{
		{5000, 180, 0, 60, false, 1667},    // 50 zł за 180 мин: первый час — 16,67
		{5000, 180, 60, 120, false, 1666},  // второй — 16,66: округление не копится
		{5000, 180, 120, 180, false, 1667}, // третий — 16,67, вместе ровно 50,00
		{5000, 180, 160, 180, false, 556},  // последние 20 минут
		{5000, 180, 0, 60, true, 0},        // подаренный — денег не было
		{5000, 180, 60, 60, false, 0},      // ничего не сняли
		{5000, 180, 170, 200, false, 278},  // больше пакета не бывает: потолок — вся цена
	}
	for _, c := range cases {
		if got := packSpendGrosz(c.price, c.total, c.before, c.to, c.gift); got != c.want {
			t.Errorf("packSpendGrosz(%d, %d, %d→%d, gift=%v) = %d; ждали %d", c.price, c.total, c.before, c.to, c.gift, got, c.want)
		}
	}
	// поминутные тики — в сумме ровно цена пакета
	var sum int64
	for m := 1; m <= 180; m++ {
		sum += packSpendGrosz(5000, 180, m-1, m, false)
	}
	if sum != 5000 {
		t.Errorf("180 тиков по минуте = %d; ждали 5000", sum)
	}
}

// «Отработанная выручка» — кошелёк, пакеты, оценка до журнала, товары, кухня; «отдано временем» в неё не входит.
func TestEarnedTotal(t *testing.T) {
	e := earnedAgg{Wallet: 334, Packs: 556, Est: 1000, Goods: 2100, Kitchen: 700, Given: 1000, GivenMin: 30}
	if got := e.total(); got != 334+556+1000+2100+700 {
		t.Errorf("total() = %d", got)
	}
}
