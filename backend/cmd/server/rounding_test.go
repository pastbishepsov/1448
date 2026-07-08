package main

// Правило цифр (DESIGN.md): всё, что видит игрок, кратно 5; пороги уровней — 100.

import (
	"testing"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

func TestRoundToStep(t *testing.T) {
	cases := []struct{ v, step, want int64 }{
		{0, 5, 0}, {2, 5, 0}, {3, 5, 5}, {167, 5, 165}, {168, 5, 170},
		{768, 5, 770}, {1234, 5, 1235}, {2639, 100, 2600}, {4656, 100, 4700},
		{7, 0, 7},  // некорректный шаг — без изменений
		{-3, 5, -3}, // отрицательные не трогаем
	}
	for _, c := range cases {
		if got := models.RoundToStep(c.v, c.step); got != c.want {
			t.Errorf("RoundToStep(%d,%d)=%d, ожидалось %d", c.v, c.step, got, c.want)
		}
	}
}

func TestXPForNextLevelRounded(t *testing.T) {
	// пороги кратны 100, кривая монотонно растёт
	prev := int64(0)
	for n := 1; n <= 60; n++ {
		xp := models.XPForNextLevel(n)
		if xp%100 != 0 {
			t.Errorf("XP(%d)=%d не кратен 100", n, xp)
		}
		if xp <= prev {
			t.Errorf("XP(%d)=%d не растёт (пред. %d)", n, xp, prev)
		}
		prev = xp
	}
	// реперные точки
	for n, want := range map[int]int64{1: 1000, 2: 2600, 3: 4700, 4: 7000, 5: 9500} {
		if got := models.XPForNextLevel(n); got != want {
			t.Errorf("XP(%d)=%d, ожидалось %d", n, got, want)
		}
	}
}

func TestCaseRollMultiplesOfFive(t *testing.T) {
	tiers := []models.CaseTier{models.CaseTierLight, models.CaseTierMedium,
		models.CaseTierHeavy, models.CaseTierTitan, models.CaseTierGods}
	for _, tier := range tiers {
		cs := models.Case{Tier: tier}
		for i := 0; i < 2000; i++ {
			_, coins, err := cs.Roll(1.0)
			if err != nil {
				t.Fatalf("%s: ошибка ролла: %v", tier, err)
			}
			if coins%5 != 0 {
				t.Fatalf("%s: дроп %d не кратен 5", tier, coins)
			}
		}
	}
}
