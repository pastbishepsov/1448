package main

import "testing"

func TestCoinsForMinutes(t *testing.T) {
	cases := []struct {
		name  string
		mins  int
		rate  float64
		spend int64
		want  int64
	}{
		// час в Standard: 23 zł × 20 монет = 460
		{"час по 23 zł при курсе 20", 60, 23, 20, 460},
		// час в VIP дороже — и в монетах тоже
		{"час по 35 zł при курсе 20", 60, 35, 20, 700},
		{"полчаса — половина цены", 30, 23, 20, 230},
		// округление вверх до пятёрки: 23 × 10/60 × 20 = 76.67 → 80
		{"округляем вверх, клуб не теряет", 10, 23, 20, 80},
		{"ноль минут — ноль монет", 0, 23, 20, 0},
		{"курс не задан — ноль", 60, 23, 0, 0},
		{"цена зоны не задана — ноль", 60, 0, 20, 0},
	}
	for _, tc := range cases {
		if got := coinsForMinutes(tc.mins, tc.rate, tc.spend); got != tc.want {
			t.Errorf("%s: coinsForMinutes = %d, ожидалось %d", tc.name, got, tc.want)
		}
	}
}

func TestMinutesForCoins(t *testing.T) {
	cases := []struct {
		name  string
		coins int64
		rate  float64
		spend int64
		want  int
	}{
		{"460 монет = час по 23 zł", 460, 23, 20, 60},
		{"700 монет в VIP = час", 700, 35, 20, 60},
		{"на 100 монет в VIP хватит на 8 минут", 100, 35, 20, 8},
		{"пустой баланс — ноль минут", 0, 23, 20, 0},
	}
	for _, tc := range cases {
		if got := minutesForCoins(tc.coins, tc.rate, tc.spend); got != tc.want {
			t.Errorf("%s: minutesForCoins = %d, ожидалось %d", tc.name, got, tc.want)
		}
	}
}

func TestCoinValuePLN(t *testing.T) {
	cases := []struct {
		coins int64
		spend int64
		want  float64
	}{
		{460, 20, 23},
		{1000, 20, 50},
		{123, 20, 6.15},
		{1000, 0, 0}, // курс не задан — не выдумываем стоимость
	}
	for _, tc := range cases {
		if got := coinValuePLN(tc.coins, tc.spend); got != tc.want {
			t.Errorf("coinValuePLN(%d,%d) = %v, ожидалось %v", tc.coins, tc.spend, got, tc.want)
		}
	}
}

// Обратимость: за сколько монет купили минуты, столько минут и получим назад.
func TestКонвертерСходится(t *testing.T) {
	for _, rate := range []float64{15, 23, 35, 60} {
		for _, mins := range []int{30, 60, 120} {
			coins := coinsForMinutes(mins, rate, 20)
			back := minutesForCoins(coins, rate, 20)
			if back < mins || back > mins+2 { // округление вверх даёт до пары лишних минут
				t.Errorf("цена %v zł/ч, %d мин → %d монет → %d мин", rate, mins, coins, back)
			}
		}
	}
}

// Сколько процентов от цены часа гость отыгрывает монетами: цифра, ради
// которой владелец и просил конвертер.
func TestКэшбекЗаЧасИгры(t *testing.T) {
	// 2 монеты в минуту = 120 монет за час; при курсе 20 это 6 zł
	if got := coinValuePLN(120, 20); got != 6 {
		t.Fatalf("час игры даёт %v zł монетами, ожидалось 6", got)
	}
	// на часе за 23 zł это 26%, на часе за 60 zł — 10%
	if pct := 6.0 / 23 * 100; pct < 25.9 || pct > 26.2 {
		t.Errorf("кэшбек на 23 zł = %.1f%%", pct)
	}
}
