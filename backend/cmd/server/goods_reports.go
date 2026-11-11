package main

// З23-и1а (OWNER.md, трек З; №48): «Товары» в «Отчётах» — сервер, владельцу. По каждой позиции за период: сколько
// продано — штук и на сколько (по моменту продажи, все способы, без подарков — тем же счётом, что маржа З10-и8), доля в
// продажах и в штуках, класс ABC по доле продаж (A — позиции, которые вместе дают первые 80 % продаж, B — следующие до
// 95 %, C — остальные; без продаж — без класса), стабильность — в скольких днях периода продажи были, давно ли
// последняя продажа (за всё время), склад сейчас и на сколько дней его хватит при темпе периода, себестоимость и маржа
// по партиям. Строки — живые позиции и выключенные, если в периоде их продавали; удалённые позиции (продажи без
// good_id) в отчёт не входят.

import (
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

// abcClasses — классы ABC по продажам позиций, уже упорядоченных по убыванию (чистая функция, тест): позиция — A, пока
// продажи позиций выше неё дают меньше 80 % всех, B — меньше 95 %, иначе C; первая с продажами — всегда A. Продаж
// нет — "" (без класса).
func abcClasses(sold []int64) []string {
	var total int64
	for _, v := range sold {
		if v > 0 {
			total += v
		}
	}
	out := make([]string, len(sold))
	var before int64
	for i, v := range sold {
		if v <= 0 || total <= 0 {
			continue
		}
		switch {
		case before*100 < 80*total:
			out[i] = "A"
		case before*100 < 95*total:
			out[i] = "B"
		default:
			out[i] = "C"
		}
		before += v
	}
	return out
}

// coverDays — на сколько дней хватит склада stock при темпе периода: items штук за days дней (чистая функция, тест);
// продаж не было — nil (темпа нет), одним знаком.
func coverDays(stock int, items int64, days int) *float64 {
	if items <= 0 || days <= 0 {
		return nil
	}
	v := math.Round(float64(stock)*float64(days)*10/float64(items)) / 10
	return &v
}

// sharePct — доля part от total в процентах одним знаком (чистая функция, тест); total 0 — 0.
func sharePct(part, total int64) float64 {
	if total <= 0 {
		return 0
	}
	return math.Round(float64(part)*1000/float64(total)) / 10
}

// goodsABCRow — позиция в отчёте «Товары».
type goodsABCRow struct {
	Good     models.Good
	Econ     goodsEcon
	DaysSold int
	Last     *time.Time
}

// goodsABC — отчёт «Товары» за период p на момент now (граница суток rh).
func goodsABC(p period, now time.Time, rh int) gin.H {
	from, to := p.From, p.To
	econ := map[string]goodsEcon{}
	var rows []goodsEcon
	db.Raw(fmt.Sprintf(goodsEconSQL, "s.good_id::text", "", "method <> 'comp' AND good_id IS NOT NULL"),
		map[string]any{"from": from, "to": to}).Scan(&rows)
	for _, r := range rows {
		econ[r.Key] = r
	}
	var days []struct {
		GoodID string
		N      int
	}
	db.Raw(`SELECT good_id::text AS good_id, COUNT(DISTINCT `+dayExpr("created_at", rh)+`) AS n FROM sales
	    WHERE voided_at IS NULL AND method <> 'comp' AND good_id IS NOT NULL AND created_at >= ? AND created_at < ?
	    GROUP BY good_id`, from, to).Scan(&days)
	daysSold := map[string]int{}
	for _, d := range days {
		daysSold[d.GoodID] = d.N
	}
	var lasts []struct {
		GoodID string
		At     time.Time
	}
	db.Raw(`SELECT good_id::text AS good_id, MAX(created_at) AS at FROM sales
	    WHERE voided_at IS NULL AND method <> 'comp' AND good_id IS NOT NULL GROUP BY good_id`).Scan(&lasts)
	last := map[string]time.Time{}
	for _, l := range lasts {
		last[l.GoodID] = l.At
	}
	var goods []models.Good
	db.Order("name").Find(&goods)
	list := make([]goodsABCRow, 0, len(goods))
	for _, g := range goods {
		e := econ[g.ID.String()]
		if !g.Active && e.Items == 0 {
			continue // выключенная и в периоде не продавалась — не о чем говорить
		}
		row := goodsABCRow{Good: g, Econ: e, DaysSold: daysSold[g.ID.String()]}
		if t, ok := last[g.ID.String()]; ok {
			t := t
			row.Last = &t
		}
		list = append(list, row)
	}
	sort.SliceStable(list, func(i, j int) bool {
		a, b := list[i].Econ, list[j].Econ
		if a.Sold != b.Sold {
			return a.Sold > b.Sold
		}
		if a.Items != b.Items {
			return a.Items > b.Items
		}
		return strings.ToLower(list[i].Good.Name) < strings.ToLower(list[j].Good.Name)
	})
	sold := make([]int64, len(list))
	var total goodsEcon
	for i, r := range list {
		sold[i] = r.Econ.Sold
		total.Items += r.Econ.Items
		total.Sold += r.Econ.Sold
		total.Cost += r.Econ.Cost
		total.Costed += r.Econ.Costed
		total.SoldCost += r.Econ.SoldCost
	}
	classes := abcClasses(sold)
	type classSum struct {
		N    int
		Sold int64
	}
	sums := map[string]*classSum{"A": {}, "B": {}, "C": {}, "": {}}
	today := clubDayOf(now, rh)
	out := make([]gin.H, 0, len(list))
	for i, r := range list {
		g, e := r.Good, r.Econ
		sums[classes[i]].N++
		sums[classes[i]].Sold += e.Sold
		row := e.out(gin.H{"id": g.ID, "name": g.Name, "category": g.Category, "active": g.Active, "stock": g.Stock,
			"low_stock": g.LowStock, "abc": classes[i], "share_pct": sharePct(e.Sold, total.Sold),
			"units_share_pct": sharePct(e.Items, total.Items), "days_sold": r.DaysSold, "days": p.Days,
			"cover_days": coverDays(g.Stock, e.Items, p.Days), "last_sold_at": nil, "days_since": nil})
		delete(row, "revenue_pln") // живые деньги — в «Деньгах»; здесь — продано на, все способы
		if r.Last != nil {
			row["last_sold_at"] = r.Last
			row["days_since"] = daysBetween(clubDayOf(r.Last.In(now.Location()), rh), today) - 1
		}
		out = append(out, row)
	}
	cls := gin.H{}
	for k, v := range sums {
		name := k
		if name == "" {
			name = "none"
		}
		cls[name] = gin.H{"count": v.N, "share_pct": sharePct(v.Sold, total.Sold)}
	}
	tot := total.out(gin.H{"goods": len(list)})
	delete(tot, "revenue_pln")
	return gin.H{"period": p.out(), "rows": out, "totals": tot, "classes": cls}
}

// GET /admin/reports/goods — «Товары» за период (owner, З23-и1а): тот же разбор периода, что у остальных отчётов.
func handleReportGoods(c *gin.Context) {
	p, _, rh, ok := periodFromQuery(c)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, goodsABC(p, time.Now(), rh))
}
