package main

// Юнит-тесты арифметики биллинга (Г1): без БД и без ожиданий — время и
// состояние везде параметры. Ключевое свойство: тики ЛЮБОЙ нарезки дают ту же
// сумму, что один расчёт целиком (нет дрейфа округления), поэтому рестарт
// сервера посреди часа ничего не теряет и не задваивает.

import "testing"

func TestCostForMinutes(t *testing.T) {
	cases := []struct {
		name      string
		rateGrosz int64
		minutes   int
		want      int64
	}{
		{"час по 20 zł ровно", 2000, 60, 2000},
		{"полчаса по 20 zł", 2000, 30, 1000},
		{"одна минута по 20 zł — вверх до гроша", 2000, 1, 34}, // 33.33 → 34
		{"час по 23 zł (средний тариф)", 2300, 60, 2300},
		{"ноль минут", 2000, 0, 0},
		{"нулевая ставка", 0, 60, 0},
	}
	for _, tc := range cases {
		if got := costForMinutes(tc.rateGrosz, tc.minutes); got != tc.want {
			t.Errorf("%s: ждали %d, получили %d", tc.name, tc.want, got)
		}
	}
}

// Нет дрейфа: сумма подельных доначислений == стоимости целиком, при любой
// нарезке тиков и «неудобных» ставках.
func TestChargeDeltaNoDrift(t *testing.T) {
	rates := []int64{2000, 2300, 1999, 3333, 1050, 6000}
	for _, rate := range rates {
		// по одной минуте
		var total int64
		for m := 0; m < 127; m++ {
			total += chargeDelta(rate, m, 1)
		}
		if want := costForMinutes(rate, 127); total != want {
			t.Errorf("ставка %d: по минуте набежало %d, целиком %d", rate, total, want)
		}
		// рваная нарезка (рестарты посреди часа)
		total = 0
		already := 0
		for _, chunk := range []int{1, 7, 30, 2, 45, 13, 29} { // = 127
			total += chargeDelta(rate, already, chunk)
			already += chunk
		}
		if want := costForMinutes(rate, 127); total != want {
			t.Errorf("ставка %d: рваная нарезка дала %d, целиком %d", rate, total, want)
		}
	}
}

func TestMinutesAffordable(t *testing.T) {
	cases := []struct {
		name    string
		wallet  int64
		rate    int64
		already int
		want    int
	}{
		{"10 zł при 20 zł/час — 30 мин (критерий Г1)", 1000, 2000, 0, 30},
		{"20 zł при 20 zł/час — час", 2000, 2000, 0, 60},
		{"на первую минуту не хватает", 33, 2000, 0, 0},
		{"ровно на одну минуту", 34, 2000, 0, 1},
		{"пустой кошелёк", 0, 2000, 0, 0},
		{"продолжение: 10 zł после 30 оплаченных минут", 1000, 2000, 30, 30},
	}
	for _, tc := range cases {
		if got := minutesAffordable(tc.wallet, tc.rate, tc.already); got != tc.want {
			t.Errorf("%s: ждали %d, получили %d", tc.name, tc.want, got)
		}
	}
}

// Согласованность: affordable-минуты действительно оплачиваемы, а на одну
// больше — уже нет.
func TestMinutesAffordableConsistent(t *testing.T) {
	wallets := []int64{1, 34, 999, 1000, 2049, 5000}
	rates := []int64{1999, 2000, 2300, 3333}
	for _, w := range wallets {
		for _, r := range rates {
			for _, already := range []int{0, 17, 60} {
				m := minutesAffordable(w, r, already)
				if m > 0 && chargeDelta(r, already, m) > w {
					t.Errorf("w=%d r=%d already=%d: %d минут не по карману", w, r, already, m)
				}
				if chargeDelta(r, already, m+1) <= w {
					t.Errorf("w=%d r=%d already=%d: можно было %d минут, насчитали %d", w, r, already, m+1, m)
				}
			}
		}
	}
}

func TestMinutesLeft(t *testing.T) {
	// 12 минут запаса + 10 zł при 20 zł/час = 12 + 30 = 42
	if got := minutesLeft(12, 1000, 2000, 0); got != 42 {
		t.Errorf("минутный запас + деньги: ждали 42, получили %d", got)
	}
	// только запас, денег нет
	if got := minutesLeft(7, 0, 2000, 0); got != 7 {
		t.Errorf("только запас: ждали 7, получили %d", got)
	}
}
