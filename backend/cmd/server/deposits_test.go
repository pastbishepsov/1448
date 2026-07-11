package main

import "testing"

func TestDepositCoins(t *testing.T) {
	// без таланта: 50 zł → 500 монет, бонуса нет (курс по умолчанию 10)
	base, bonus := depositCoins(50, 0, 10)
	if base != 500 || bonus != 0 {
		t.Errorf("50zł/0: base=%d bonus=%d, ожидалось 500/0", base, bonus)
	}
	// coin_mint 3 уровня (0.15): 100 zł → 1000 + 150
	base, bonus = depositCoins(100, 0.15, 10)
	if base != 1000 || bonus != 150 {
		t.Errorf("100zł/0.15: base=%d bonus=%d, ожидалось 1000/150", base, bonus)
	}
	// дробная сумма округляется честно: 19.99 zł → 200 монет
	base, _ = depositCoins(19.99, 0, 10)
	if base != 200 {
		t.Errorf("19.99zł: base=%d, ожидалось 200", base)
	}
	// изменённый курс из настроек (А5): 50 zł × 20 → 1000 монет
	base, _ = depositCoins(50, 0, 20)
	if base != 1000 {
		t.Errorf("50zł/курс 20: base=%d, ожидалось 1000", base)
	}
}

func TestIsNightHour(t *testing.T) {
	night := []int{22, 23, 0, 3, 7}
	day := []int{8, 12, 18, 21}
	for _, h := range night {
		if !isNightHour(h) {
			t.Errorf("%d:00 должен быть ночью", h)
		}
	}
	for _, h := range day {
		if isNightHour(h) {
			t.Errorf("%d:00 должен быть днём", h)
		}
	}
}

func TestEffectiveRate(t *testing.T) {
	// без скидки
	if r := effectiveRate(23, 0); r != 23 {
		t.Errorf("23/0%% = %v, ожидалось 23", r)
	}
	// 10% скидка
	if r := effectiveRate(23, 10); r != 20.7 {
		t.Errorf("23/10%% = %v, ожидалось 20.7", r)
	}
	// скидка выше потолка режется до 30%
	if r := effectiveRate(20, 80); r != 14 {
		t.Errorf("20/80%% = %v, ожидалось 14 (кап 30%%)", r)
	}
	// отрицательная скидка не повышает цену
	if r := effectiveRate(20, -5); r != 20 {
		t.Errorf("20/-5%% = %v, ожидалось 20", r)
	}
}
