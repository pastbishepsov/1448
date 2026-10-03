package main

// З10-и8 (OWNER.md, трек З; №70; решение основателя 29.09): маржа и стоимость
// склада — только владельцу, в отчёте «Деньги». Себестоимость проданного — по
// цене партий, из которых ушли проданные штуки (доли движений, З10-и3…и5), по
// нынешней цене партии: оценили поставку позже — маржа пересчитается. Считаются
// все продажи — и с кошелька (кухня), и в долг — по моменту продажи, как «Штук»
// в «Что покупают». Маржа — от цены продажи тех же штук, включая кошелёк
// (решение основателя 29.09): иначе позиция, которую берут в основном с
// кошелька, показала бы маржу в минусе; «Выручка» остаётся живыми деньгами
// (Р7). Штуки из партий без цены (и проданные до партий) — «без себестоимости»:
// в маржу не входят ни ценой продажи, ни себестоимостью. Суммы — брутто, как
// цены продажи и закупки (нетто для плательщика VAT — со ставками VAT, З13-и1).
// З11-и5: подарки (comp) — не продажа: в марже и «Что покупают» их нет, они —
// «Подарено» тем же счётом себестоимости (giftsEcon).

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

// goodsEcon — продажи товаров строки разреза (позиция или категория) за период,
// в грошах.
type goodsEcon struct {
	Key      string
	Revenue  int64 // выручка: живые деньги по расчёту, без кошелька (Р7, Ж6) — как было
	Items    int64 // штук по моменту продажи, все способы
	Sold     int64 // продано на: цена продажи этих штук, все способы
	Cost     int64 // себестоимость штук из партий с ценой
	Costed   int64 // сколько таких штук
	SoldCost int64 // продано на — только эти штуки: база маржи
}

// goodsEconSQL — продажи периода по ключу: %[1]s — ключ строки, %[2]s —
// присоединение для него, %[3]s — какие продажи (З11-и5: без подарков или только они). Проданная штука — продажа по моменту продажи
// (created_at), любым способом, не отменённая; её партии — доли движений
// продажи у стойки (sale_id) или заказа кухни, который она оплатила
// (kitchen_order_id). Выручка — как у «Что покупают» до З10-и8: по расчёту
// (paid_at), без кошелька.
const goodsEconSQL = `WITH s AS (
    SELECT id, name, good_id, qty, ROUND(total_pln * 100)::bigint AS total,
           (created_at >= @from AND created_at < @to) AS sold,
           (method <> 'wallet' AND paid_at >= @from AND paid_at < @to) AS live
    FROM sales
    WHERE voided_at IS NULL AND %[3]s AND ((created_at >= @from AND created_at < @to) OR (paid_at >= @from AND paid_at < @to))),
  mv AS (
    SELECT s.id AS sale_id, m.id AS move_id FROM s JOIN stock_moves m ON m.sale_id = s.id WHERE s.sold
    UNION ALL
    SELECT s.id, m.id FROM s JOIN kitchen_orders o ON o.sale_id = s.id JOIN stock_moves m ON m.kitchen_order_id = o.id WHERE s.sold),
  c AS (
    SELECT mv.sale_id, SUM(-lm.qty * l.price_grosz) AS cost, SUM(-lm.qty) AS costed
    FROM mv JOIN lot_moves lm ON lm.stock_move_id = mv.move_id JOIN stock_lots l ON l.id = lm.lot_id
    WHERE l.price_grosz IS NOT NULL
    GROUP BY mv.sale_id)
SELECT %[1]s AS key,
    COALESCE(SUM(s.total) FILTER (WHERE s.live), 0) AS revenue,
    COALESCE(SUM(s.qty) FILTER (WHERE s.sold), 0) AS items,
    COALESCE(SUM(s.total) FILTER (WHERE s.sold), 0) AS sold,
    COALESCE(SUM(c.cost) FILTER (WHERE s.sold), 0) AS cost,
    COALESCE(SUM(c.costed) FILTER (WHERE s.sold), 0) AS costed,
    COALESCE(SUM(ROUND(s.total::numeric * c.costed / s.qty)) FILTER (WHERE s.sold), 0) AS sold_cost
FROM s LEFT JOIN c ON c.sale_id = s.id %[2]s
GROUP BY 1`

// goodsEconBy — продажи товаров за [from, to): по позиции (имя в продаже, как
// «Что покупают» было) или по категории позиции (позицию удалили — "").
func goodsEconBy(from, to time.Time, byCategory bool) []goodsEcon {
	key, join := "s.name", ""
	if byCategory {
		key, join = "COALESCE(g.category, '')", "LEFT JOIN goods g ON g.id = s.good_id"
	}
	var out []goodsEcon
	db.Raw(fmt.Sprintf(goodsEconSQL, key, join, "method <> 'comp'"), map[string]any{"from": from, "to": to}).Scan(&out)
	return out
}

// giftsEcon — подаренные товары за [from, to) (З11-и5): штуки, цена и
// себестоимость по партиям — тем же счётом, что маржа, одной строкой.
func giftsEcon(from, to time.Time) goodsEcon {
	var out []goodsEcon
	db.Raw(fmt.Sprintf(goodsEconSQL, "''", "", "method = 'comp'"), map[string]any{"from": from, "to": to}).Scan(&out)
	return goodsEconTotal(out)
}

// goodsEconTotal — итог строк разреза (категории покрывают все продажи).
func goodsEconTotal(rows []goodsEcon) goodsEcon {
	var t goodsEcon
	for _, r := range rows {
		t.Revenue += r.Revenue
		t.Items += r.Items
		t.Sold += r.Sold
		t.Cost += r.Cost
		t.Costed += r.Costed
		t.SoldCost += r.SoldCost
	}
	return t
}

// goodsEconSort — порядок «Что покупают» (чистая, тест): сначала на сколько
// продано (все способы — кошелёк тоже покупка), потом выручка, потом имя; n > 0
// — первые n.
func goodsEconSort(rows []goodsEcon, n int) []goodsEcon {
	out := append([]goodsEcon(nil), rows...)
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Sold != b.Sold {
			return a.Sold > b.Sold
		}
		if a.Revenue != b.Revenue {
			return a.Revenue > b.Revenue
		}
		return strings.ToLower(a.Key) < strings.ToLower(b.Key)
	})
	if n > 0 && len(out) > n {
		out = out[:n]
	}
	return out
}

// marginPct — маржа в процентах от продаж с известной себестоимостью, одним
// знаком (чистая, тест); продаж на ноль — nil.
func marginPct(soldCost, cost int64) *float64 {
	if soldCost <= 0 {
		return nil
	}
	v := math.Round(float64(soldCost-cost)*1000/float64(soldCost)) / 10
	return &v
}

// out — строка отчёта: выручка и штуки — как были; продано на — все способы;
// себестоимость и маржа — по штукам из партий с ценой, таких нет — null;
// «без себестоимости» — остальные проданные штуки.
func (e goodsEcon) out(row gin.H) gin.H {
	row["revenue_pln"] = models.PLNFromGrosz(e.Revenue)
	row["items"] = e.Items
	row["sold_pln"] = models.PLNFromGrosz(e.Sold)
	row["costed_units"] = e.Costed
	row["no_cost_units"] = e.Items - e.Costed
	row["cost_pln"], row["margin_pln"], row["margin_pct"] = nil, nil, nil
	if e.Costed > 0 {
		row["cost_pln"] = models.PLNFromGrosz(e.Cost)
		row["margin_pln"] = models.PLNFromGrosz(e.SoldCost - e.Cost)
		row["margin_pct"] = marginPct(e.SoldCost, e.Cost)
	}
	return row
}

// goodsReport — «Что покупают» (15 позиций), все категории и итог маржи за
// период; в итоге — и маржа прошлого периода («было»); delta — её рост в
// процентах, только если в прошлом периоде маржа была больше нуля.
func goodsReport(p, prev period) (top, cats []gin.H, total gin.H, delta *float64) {
	byName := goodsEconSort(goodsEconBy(p.From, p.To, false), 15)
	top = make([]gin.H, 0, len(byName))
	for _, r := range byName {
		top = append(top, r.out(gin.H{"name": r.Key}))
	}
	byCat := goodsEconBy(p.From, p.To, true)
	cats = make([]gin.H, 0, len(byCat))
	for _, r := range goodsEconSort(byCat, 0) {
		cats = append(cats, r.out(gin.H{"category": r.Key}))
	}
	cur, old := goodsEconTotal(byCat), goodsEconTotal(goodsEconBy(prev.From, prev.To, true))
	total = cur.out(gin.H{})
	delete(total, "revenue_pln") // выручка товаров — в итоге отчёта (totals.goods_pln)
	total["prev_margin_pln"] = nil
	if old.Costed > 0 {
		total["prev_margin_pln"] = models.PLNFromGrosz(old.SoldCost - old.Cost)
		if cur.Costed > 0 && old.SoldCost > old.Cost { // от нуля и минуса процент роста бессмыслен
			delta = pctDelta(float64(cur.SoldCost-cur.Cost), float64(old.SoldCost-old.Cost))
		}
	}
	return top, cats, total, delta
}

// stockValue — склад сейчас: остатки по себестоимости партий с ценой, штук в
// партиях без цены и по рознице — остаток × нынешняя цена позиции.
func stockValue() gin.H {
	var r struct {
		Cost   int64
		Units  int64
		NoCost int64
	}
	db.Raw(`SELECT COALESCE(SUM(qty_left * price_grosz) FILTER (WHERE price_grosz IS NOT NULL), 0) AS cost,
	        COALESCE(SUM(qty_left) FILTER (WHERE price_grosz IS NOT NULL), 0) AS units,
	        COALESCE(SUM(qty_left) FILTER (WHERE price_grosz IS NULL), 0) AS no_cost
	    FROM stock_lots WHERE qty_left > 0`).Scan(&r)
	var retail int64
	db.Raw(`SELECT COALESCE(SUM(ROUND(price_pln * 100)::bigint * stock), 0) FROM goods WHERE stock > 0`).Scan(&retail)
	return gin.H{"cost_pln": models.PLNFromGrosz(r.Cost), "costed_units": r.Units, "no_cost_units": r.NoCost,
		"units": r.Units + r.NoCost, "retail_pln": models.PLNFromGrosz(retail)}
}
