package main

import "testing"

// Г6-и4: правило «всё или ничего» для заморозок — чистая арифметика решения,
// сколько штук потратить на разрыв. Вынесено из applyStreakFreezes, чтобы
// проверять без БД.
func TestFreezesForGap(t *testing.T) {
	cases := []struct {
		gap, stock, maxRow, want int
		why                      string
	}{
		{0, 3, 2, 0, "вчера был — нечего латать"},
		{1, 1, 2, 1, "один пропуск, одна заморозка"},
		{2, 2, 2, 2, "два пропуска, ровно две"},
		{2, 1, 2, 0, "не хватает — не тратим ни одной"},
		{3, 5, 2, 0, "разрыв длиннее лимита подряд — стрик уже не спасти"},
		{1, 0, 2, 0, "запаса нет"},
		{1, 3, 0, 0, "механика выключена владельцем"},
	}
	for _, c := range cases {
		if got := freezesForGap(c.gap, c.stock, c.maxRow); got != c.want {
			t.Errorf("%s: gap=%d stock=%d maxRow=%d → %d, ожидали %d",
				c.why, c.gap, c.stock, c.maxRow, got, c.want)
		}
	}
}
