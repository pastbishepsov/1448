package main

import (
	"testing"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

// Поток заказа кухни (Г7): вперёд можно, назад и после финала — нельзя.
func TestAllowedKitchenNext(t *testing.T) {
	ok := [][2]string{
		{models.KitchenNew, models.KitchenAccepted},
		{models.KitchenNew, models.KitchenDone}, // у стойки: принял и сразу выдал
		{models.KitchenAccepted, models.KitchenPreparing},
		{models.KitchenAccepted, models.KitchenDone},
		{models.KitchenPreparing, models.KitchenDelivering},
		{models.KitchenPreparing, models.KitchenDone},
		{models.KitchenDelivering, models.KitchenDone},
	}
	for _, p := range ok {
		if !allowedKitchenNext(p[0], p[1]) {
			t.Errorf("%s → %s должен быть разрешён", p[0], p[1])
		}
	}
	bad := [][2]string{
		{models.KitchenAccepted, models.KitchenAccepted}, // на месте не стоим
		{models.KitchenPreparing, models.KitchenAccepted}, // назад нельзя
		{models.KitchenDone, models.KitchenDelivering},    // финал не двигается
		{models.KitchenDone, models.KitchenDone},
		{models.KitchenCancelled, models.KitchenAccepted}, // отменённый не оживает
		{models.KitchenNew, models.KitchenCancelled},      // отмена — отдельным путём (с возвратами)
		{models.KitchenNew, "чебурек"},
	}
	for _, p := range bad {
		if allowedKitchenNext(p[0], p[1]) {
			t.Errorf("%s → %s должен быть запрещён", p[0], p[1])
		}
	}
}
