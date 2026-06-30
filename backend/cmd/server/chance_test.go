package main

import "testing"

// chance использует crypto/rand, но границы детерминированы.
func TestChanceBoundaries(t *testing.T) {
	if chance(0) {
		t.Error("chance(0) должно быть false")
	}
	if chance(-0.5) {
		t.Error("chance(<0) должно быть false")
	}
	if !chance(1) {
		t.Error("chance(1) должно быть true")
	}
	if !chance(2) {
		t.Error("chance(>1) должно быть true")
	}
}

// Грубая статистическая проверка: при p=0.5 доля успехов в разумном коридоре.
func TestChanceRoughDistribution(t *testing.T) {
	const n = 20000
	hits := 0
	for i := 0; i < n; i++ {
		if chance(0.5) {
			hits++
		}
	}
	ratio := float64(hits) / n
	if ratio < 0.46 || ratio > 0.54 {
		t.Errorf("доля при p=0.5 = %.3f, ожидаем ~0.5", ratio)
	}
}
