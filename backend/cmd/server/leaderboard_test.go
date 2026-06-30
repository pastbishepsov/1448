package main

import "testing"

func TestComputeRank(t *testing.T) {
	scores := []int64{100, 100, 50, 50, 10} // опыт всех игроков

	cases := []struct {
		my   int64
		want int
	}{
		{100, 1}, // никого выше → 1-е место (делёж с другим 100 — оба 1-е)
		{50, 3},  // двое со 100 выше → 3-е место
		{10, 5},  // четверо выше
		{200, 1}, // выше всех
		{0, 6},   // ниже всех пятерых
	}
	for _, tc := range cases {
		if got := computeRank(scores, tc.my); got != tc.want {
			t.Errorf("computeRank(my=%d) = %d, хотим %d", tc.my, got, tc.want)
		}
	}
}
