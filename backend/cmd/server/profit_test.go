package main

import "testing"

// З12-и3: доходы − возвраты живыми деньгами − себестоимость проданного и подаренного − потери склада = валовая
// прибыль; − расходы = результат. Возврат на кошелёк и «не возвращали» прибыль не трогают; результат бывает
// меньше нуля.
func TestProfitLines(t *testing.T) {
	a := profitAgg{Deposits: 10000, Goods: 2400, Packs: 5000,
		Sold: goodsEcon{Cost: 600}, Gifts: goodsEcon{Cost: 300}, Losses: 300, Expenses: 5000}
	a.Returns.Live = [2]int64{800, 1}
	a.Returns.Wallet = [2]int64{5000, 1} // на кошелёк — не возврат денег
	a.Returns.None = [2]int64{1200, 1}   // «не возвращали» — деньги остались у клуба
	income, gross, result := a.lines()
	if income != 17400 || gross != 17400-800-600-300-300 || result != gross-5000 {
		t.Errorf("lines() = %d, %d, %d", income, gross, result)
	}
	a.Expenses = 20000
	if _, _, r := a.lines(); r != 15400-20000 {
		t.Errorf("убыток: результат = %d, ждали %d", r, 15400-20000)
	}
	if got := a.cogs(); got != 900 {
		t.Errorf("cogs() = %d, ждали 900", got)
	}
}

// З12-и4: затраты дня в ряду «Обзора» — возвраты, себестоимость, потери и расходы; доход в них не входит.
func TestProfitDayCosts(t *testing.T) {
	d := profitDay{Income: 10000, Returns: 800, Cogs: 900, Losses: 300, Expenses: 5000}
	if got := d.costs(); got != 7000 {
		t.Errorf("costs() = %d, ждали 7000", got)
	}
	if got := (profitDay{Income: 500}).costs(); got != 0 {
		t.Errorf("день без затрат: costs() = %d, ждали 0", got)
	}
}
