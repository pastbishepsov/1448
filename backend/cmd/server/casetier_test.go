package main

import (
	"testing"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

func TestRollCaseTier(t *testing.T) {
	valid := map[models.CaseTier]bool{
		models.CaseTierLight:  true,
		models.CaseTierMedium: true,
		models.CaseTierHeavy:  true,
		models.CaseTierTitan:  true,
		models.CaseTierGods:   true,
	}

	counts := map[models.CaseTier]int{}
	const n = 5000
	for i := 0; i < n; i++ {
		tr := rollCaseTier(0)
		if !valid[tr] {
			t.Fatalf("невалидный тир: %q", tr)
		}
		counts[tr]++
	}

	// Light доминирует (вес 6900 из 10000) — на 5000 бросков практически гарантированно.
	if counts[models.CaseTierLight] <= counts[models.CaseTierMedium] {
		t.Errorf("Light (%d) должен выпадать чаще Medium (%d)",
			counts[models.CaseTierLight], counts[models.CaseTierMedium])
	}
}
