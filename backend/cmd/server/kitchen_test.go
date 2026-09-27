package main

import (
	"testing"
	"time"

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

// З8-и5: очередь кухни — живые заказы и последние закрытые (приходят от новых
// к старым) одним списком от старых к новым.
func TestKitchenQueue(t *testing.T) {
	at := func(m int) time.Time { return time.Date(2026, 9, 26, 8, m, 0, 0, time.UTC) }
	live := []models.KitchenOrder{{Name: "a", CreatedAt: at(1)}, {Name: "d", CreatedAt: at(40)}}
	closed := []models.KitchenOrder{{Name: "e", CreatedAt: at(50)}, {Name: "c", CreatedAt: at(30)}, {Name: "b", CreatedAt: at(10)}}
	got := ""
	for _, o := range kitchenQueue(live, closed) {
		got += o.Name
	}
	if got != "abcde" {
		t.Fatalf("очередь %q, ждали abcde", got)
	}
	if n := len(kitchenQueue(nil, nil)); n != 0 {
		t.Fatalf("пустая очередь — %d заказов", n)
	}
}

// З8-и6: отмена оплаченного заказа — куда вернули деньги и что сказать админу,
// которому отмена чужой или вчерашней продажи не положена.
func TestRefundWhere(t *testing.T) {
	for m, want := range map[string]string{"wallet": "на кошелёк гостя", "cash": "наличными", "card": "на карту", "blik": "по BLIK"} {
		if got := refundWhere(m); got != want {
			t.Errorf("refundWhere(%q) = %q, ждали %q", m, got, want)
		}
	}
	day := time.Date(2026, 9, 27, 8, 0, 0, 0, time.UTC)
	for _, sale := range []struct{ by, at string }{{"other", "2026-09-27T10:00:00Z"}, {"me", "2026-09-26T10:00:00Z"}} {
		at, _ := time.Parse(time.RFC3339, sale.at)
		ok, code := canVoidSale(string(models.UserRoleAdmin), sale.by, "me", at, day, day.Add(24*time.Hour))
		if ok || kitchenRefundErrors[code] == "" {
			t.Errorf("продажа %s от %s: ok=%v, код %q без сообщения", sale.by, sale.at, ok, code)
		}
	}
}
