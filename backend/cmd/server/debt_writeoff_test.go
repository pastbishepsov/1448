package main

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

// З11-и4: строка журнала списания — из чего долг: кухня и бар, пустые части не пишем.
func TestWriteOffParts(t *testing.T) {
	cases := []struct {
		w    models.DebtWriteOff
		want string
	}{
		{models.DebtWriteOff{KitchenCount: 1, KitchenGrosz: 800, BarCount: 2, BarGrosz: 1000}, "кухня 8.00 zł (1), бар 10.00 zł (2)"},
		{models.DebtWriteOff{KitchenCount: 3, KitchenGrosz: 2450}, "кухня 24.50 zł (3)"},
		{models.DebtWriteOff{BarCount: 1, BarGrosz: 400}, "бар 4.00 zł (1)"},
		{models.DebtWriteOff{}, ""},
	}
	for _, c := range cases {
		if got := writeOffParts(&c.w); got != c.want {
			t.Errorf("writeOffParts(%+v) = %q, ждали %q", c.w, got, c.want)
		}
	}
}

// «Дебиторка»: сперва ушедшие, не рассчитавшись (их долг сам не закроется), потом сидящие за ПК; внутри — самый
// старый долг первым.
func TestSortReceivables(t *testing.T) {
	t0 := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	id := func(n byte) uuid.UUID { return uuid.UUID{n} }
	rows := []receivable{
		{UserID: id(1), Since: t0.Add(3 * time.Hour), Computer: "PC-01"},
		{UserID: id(2), Since: t0.Add(2 * time.Hour)},
		{UserID: id(3), Since: t0.Add(time.Hour), Computer: "PC-02"},
		{UserID: id(4), Since: t0},
	}
	sortReceivables(rows)
	got := []byte{rows[0].UserID[0], rows[1].UserID[0], rows[2].UserID[0], rows[3].UserID[0]}
	if string(got) != string([]byte{4, 2, 3, 1}) {
		t.Errorf("порядок %v, ждали [4 2 3 1]", got)
	}
	if (receivable{KitchenGrosz: 800, BarGrosz: 1050}).total() != 1850 {
		t.Error("total — кухня + бар")
	}
}
