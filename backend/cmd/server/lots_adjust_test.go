package main

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

// З10-и7: откуда партия словами — для списка партий «боя» и «Закупок».
func TestLotTitle(t *testing.T) {
	sid, other := uuid.New(), uuid.New()
	at := time.Date(2026, 9, 28, 16, 40, 0, 0, time.UTC)
	byID := map[uuid.UUID]time.Time{sid: at}
	cases := []struct {
		name string
		lot  models.StockLot
		want string
	}{
		{"начальный остаток", models.StockLot{Kind: models.LotInitial, ReceivedAt: at}, "начальный остаток"},
		{"нашлось — по часам клуба", models.StockLot{Kind: models.LotFound, ReceivedAt: at}, "нашлось " + at.In(clubLocation).Format("02.01 15:04")},
		{"строка поставки — время приёмки", models.StockLot{Kind: models.LotSupply, SupplyID: &sid}, supplyTitle(at)},
		{"поставки нет в списке", models.StockLot{Kind: models.LotSupply, SupplyID: &other}, "поставка"},
	}
	for _, c := range cases {
		if got := lotTitle(c.lot, byID); got != c.want {
			t.Errorf("%s: %q, ждали %q", c.name, got, c.want)
		}
	}
}
