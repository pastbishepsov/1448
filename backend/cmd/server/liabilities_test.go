package main

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

// З12-и1: клуб должен гостям — кошельки + пакеты + монеты + минуты; одна цифра — за вычетом дебиторки (может
// уйти в минус, если гости должны больше).
func TestLiabSum(t *testing.T) {
	cases := []struct {
		w, p, c, m, r, owed, net int64
	}{
		{0, 0, 0, 0, 0, 0, 0},
		{150000, 4950, 1234, 333, 0, 156517, 156517},
		{150000, 4950, 1234, 333, 20000, 156517, 136517},
		{1000, 0, 0, 0, 2500, 1000, -1500},
	}
	for _, c := range cases {
		owed, net := liabSum(c.w, c.p, c.c, c.m, c.r)
		if owed != c.owed || net != c.net {
			t.Errorf("liabSum(%d, %d, %d, %d, %d) = %d, %d; ждали %d, %d", c.w, c.p, c.c, c.m, c.r, owed, net, c.owed, c.net)
		}
	}
}

// Список «Сгорело»: свежие сверху; в один момент — пакеты раньше монет, дальше по id.
func TestSortBurned(t *testing.T) {
	t0 := time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)
	id := func(s string) uuid.UUID { return uuid.MustParse("00000000-0000-0000-0000-" + s) }
	rows := []burnedRow{
		{Kind: "coins", ID: id("000000000001"), At: t0},
		{Kind: "package", ID: id("000000000002"), At: t0.Add(-time.Hour)},
		{Kind: "package", ID: id("000000000004"), At: t0},
		{Kind: "package", ID: id("000000000003"), At: t0},
		{Kind: "coins", ID: id("000000000005"), At: t0.Add(time.Minute)},
	}
	sortBurned(rows)
	want := []string{"000000000005", "000000000003", "000000000004", "000000000001", "000000000002"}
	for i, w := range want {
		if got := rows[i].ID.String()[24:]; got != w {
			t.Fatalf("позиция %d: %s (%s), ждали %s", i, got, rows[i].Kind, w)
		}
	}
}
