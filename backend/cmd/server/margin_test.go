package main

import (
	"testing"

	"github.com/gin-gonic/gin"
)

// З10-и8: порядок «Что покупают» — на сколько продано (и с кошелька), потом выручка, потом имя.
func TestGoodsEconSort(t *testing.T) {
	rows := []goodsEcon{
		{Key: "чай", Sold: 500, Revenue: 500},
		{Key: "кухня с кошелька", Sold: 900, Revenue: 0},
		{Key: "Вода", Sold: 500, Revenue: 700}, // долг прошлого периода закрыли в этом
		{Key: "бутерброд", Sold: 500, Revenue: 500},
		{Key: "старый долг", Sold: 0, Revenue: 300},
	}
	got := []string{}
	for _, r := range goodsEconSort(rows, 0) {
		got = append(got, r.Key)
	}
	want := []string{"кухня с кошелька", "Вода", "бутерброд", "чай", "старый долг"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("порядок %v, ждали %v", got, want)
		}
	}
	if n := len(goodsEconSort(rows, 2)); n != 2 {
		t.Fatalf("первые 2: %d строк", n)
	}
	if rows[0].Key != "чай" {
		t.Fatal("сортировка испортила исходный срез")
	}
}

func TestMarginPct(t *testing.T) {
	cases := []struct {
		sold, cost int64
		want       float64
		null       bool
	}{
		{270000, 96000, 64.4, false}, // 1740 из 2700
		{310000, 106000, 65.8, false},
		{10000, 12000, -20, false}, // продали дешевле закупки
		{10000, 0, 100, false},     // подарок поставщика
		{0, 0, 0, true},
		{0, 500, 0, true},
	}
	for _, c := range cases {
		got := marginPct(c.sold, c.cost)
		if c.null {
			if got != nil {
				t.Fatalf("%d/%d: ждали null, вышло %v", c.sold, c.cost, *got)
			}
			continue
		}
		if got == nil || *got != c.want {
			t.Fatalf("%d/%d: ждали %v, вышло %v", c.sold, c.cost, c.want, got)
		}
	}
}

// Строка отчёта: без штук из партий с ценой — себестоимости и маржи нет (null), «без себестоимости» — все штуки.
func TestGoodsEconOut(t *testing.T) {
	r := goodsEcon{Revenue: 150000, Items: 11, Sold: 310000, Cost: 96000, Costed: 9, SoldCost: 270000}.out(gin.H{"name": "A"})
	if r["cost_pln"] != 960.0 || r["margin_pln"] != 1740.0 || *(r["margin_pct"].(*float64)) != 64.4 || r["no_cost_units"] != int64(2) ||
		r["sold_pln"] != 3100.0 || r["revenue_pln"] != 1500.0 || r["items"] != int64(11) || r["name"] != "A" {
		t.Fatalf("строка: %v", r)
	}
	e := goodsEcon{Revenue: 4000, Items: 2, Sold: 4000}.out(gin.H{})
	if e["cost_pln"] != nil || e["margin_pln"] != nil || e["margin_pct"] != nil || e["no_cost_units"] != int64(2) {
		t.Fatalf("без цены: %v", e)
	}
	tot := goodsEconTotal([]goodsEcon{{Items: 9, Sold: 270000, Cost: 96000, Costed: 9, SoldCost: 270000}, {Items: 2, Sold: 40000}})
	if tot.Items != 11 || tot.Sold != 310000 || tot.Costed != 9 || tot.SoldCost != 270000 || tot.Cost != 96000 {
		t.Fatalf("итог: %+v", tot)
	}
}
