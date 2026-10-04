package main

// З12-и3 «Расчёт «Прибыли»» (OWNER.md, трек З; №74; Д9; решение основателя 04.10 — потери склада вычитаются).
//
// «Прибыль» за период — сколько клуб заработал по деньгам:
//
//	доходы по видам — пополнения, товары, пакеты: всё, что за период оплачено живыми деньгами, и потом отменённое
//	тоже (правило «Выручки», moneySources, но без вычета отмен задним числом);
//	− возвраты живыми деньгами по дню отмены (З11-и3, returnsIn): отмена вычитается один раз и в свой день;
//	  возврат на кошелёк — не возврат денег, «не возвращали» — ноль;
//	− себестоимость проданного (любым способом — и с кошелька, и в долг) и подаренного (Д9) — по партиям, как маржа
//	  (З10-и8); штуки из партий без цены — «без себестоимости», в неё не входят;
//	− потери склада по себестоимости партий — бой, недостача (решение основателя 04.10)
//	= валовая прибыль;
//	− расходы по категориям — по дню операции, без сторно и без закупок товара (Д9: закупка уже в себестоимости)
//	= результат.
//
// Рядом — прошлый период того же размера, разбивки каждой строки, «Отработанная выручка» (З12-и2) и «Обязательства
// клуба» (З12-и1). Только владельцу: GET /admin/reports/profit; экран — З12-и4.

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

// profitCat — расходы одной категории за период.
type profitCat struct {
	ID    uuid.UUID
	Name  string
	Grosz int64
	Count int64
}

// profitAgg — «Прибыль» периода, в грошах.
type profitAgg struct {
	Deposits, Goods, Packs    int64 // доходы по видам
	DepositsN, GoodsN, PacksN int64
	Returns                   returnsTotals // по дню отмены
	Sold, Gifts               goodsEcon     // себестоимость проданного и подаренного
	Losses                    int64         // потери склада по себестоимости
	LossUnits, LossCosted     int64
	Expenses, ExpensesN       int64
	Purchases                 int64 // закупки товара за период — не расход (Д9), для сведения
	ByCat                     []profitCat
}

// lines — доходы, валовая прибыль и результат (чистая, тест).
func (a profitAgg) lines() (income, gross, result int64) {
	income = a.Deposits + a.Goods + a.Packs
	gross = income - a.Returns.Live[0] - a.Sold.Cost - a.Gifts.Cost - a.Losses
	return income, gross, gross - a.Expenses
}

// cogs — себестоимость проданного и подаренного.
func (a profitAgg) cogs() int64 { return a.Sold.Cost + a.Gifts.Cost }

// profitIncomeSQL — доходы [@from, @to) по видам: оплачено живыми деньгами, и потом отменённое тоже.
const profitIncomeSQL = `SELECT
  (SELECT COALESCE(SUM(ROUND(amount_pln * 100)), 0) FROM deposits WHERE created_at >= @from AND created_at < @to)::bigint AS deposits,
  (SELECT COUNT(*) FROM deposits WHERE created_at >= @from AND created_at < @to) AS deposits_n,
  (SELECT COALESCE(SUM(ROUND(total_pln * 100)), 0) FROM sales
    WHERE paid_at >= @from AND paid_at < @to AND method NOT IN ('wallet', 'comp'))::bigint AS goods,
  (SELECT COUNT(*) FROM sales WHERE paid_at >= @from AND paid_at < @to AND method NOT IN ('wallet', 'comp')) AS goods_n,
  (SELECT COALESCE(SUM(ROUND(price_pln * 100)), 0) FROM user_packages
    WHERE created_at >= @from AND created_at < @to AND method NOT IN ('wallet', 'comp'))::bigint AS packs,
  (SELECT COUNT(*) FROM user_packages WHERE created_at >= @from AND created_at < @to AND method NOT IN ('wallet', 'comp')) AS packs_n`

// profitIn — «Прибыль» периода p.
func profitIn(p period) profitAgg {
	var a profitAgg
	var in struct{ Deposits, DepositsN, Goods, GoodsN, Packs, PacksN int64 }
	db.Raw(profitIncomeSQL, map[string]any{"from": p.From, "to": p.To}).Scan(&in)
	a.Deposits, a.DepositsN, a.Goods, a.GoodsN, a.Packs, a.PacksN = in.Deposits, in.DepositsN, in.Goods, in.GoodsN, in.Packs, in.PacksN
	a.Returns = sumReturns(returnsIn(db, p.From, p.To), p.From)
	a.Sold = goodsEconTotal(goodsEconBy(p.From, p.To, false))
	a.Gifts = giftsEcon(p.From, p.To)
	a.Losses, a.LossCosted = lossesByCost(p.From, p.To)
	db.Model(&models.StockMove{}).Select("COALESCE(-SUM(delta),0)").
		Where("created_at >= ? AND created_at < ? AND reason = ? AND delta < 0", p.From, p.To, "adjust").Scan(&a.LossUnits)
	from, to := p.FromDay.Format("2006-01-02"), p.ToDay.Format("2006-01-02")
	db.Raw(`SELECT c.id, c.name, COALESCE(SUM(e.gross_grosz), 0)::bigint AS grosz, COUNT(*) AS count
	          FROM expenses e JOIN expense_categories c ON c.id = e.category_id
	         WHERE e.storno_at IS NULL AND NOT c.purchase AND e.op_date >= ? AND e.op_date <= ?
	         GROUP BY c.id, c.name ORDER BY grosz DESC, c.name`, from, to).Scan(&a.ByCat)
	for _, c := range a.ByCat {
		a.Expenses += c.Grosz
		a.ExpensesN += c.Count
	}
	db.Raw(`SELECT COALESCE(SUM(e.gross_grosz), 0)::bigint FROM expenses e JOIN expense_categories c ON c.id = e.category_id
	         WHERE e.storno_at IS NULL AND c.purchase AND e.op_date >= ? AND e.op_date <= ?`, from, to).Scan(&a.Purchases)
	return a
}

// GET /admin/reports/profit — «Прибыль» за период и прошлый период для сравнения (owner).
func handleReportProfit(c *gin.Context) {
	p, prev, _, ok := periodFromQuery(c)
	if !ok {
		return
	}
	cur, old := profitIn(p), profitIn(prev)
	income, gross, result := cur.lines()
	oIncome, oGross, oResult := old.lines()
	pln := models.PLNFromGrosz
	cats := make([]gin.H, 0, len(cur.ByCat))
	for _, x := range cur.ByCat {
		cats = append(cats, gin.H{"category_id": x.ID, "category": x.Name, "pln": pln(x.Grosz), "count": x.Count})
	}
	r := cur.Returns
	pct := func(a, b int64) *float64 { return pctDelta(float64(a), float64(b)) }
	c.JSON(http.StatusOK, gin.H{
		"period": p.out(), "prev_period": prev.out(),
		"income": gin.H{"pln": pln(income), "deposits_pln": pln(cur.Deposits), "deposits": cur.DepositsN,
			"goods_pln": pln(cur.Goods), "sales": cur.GoodsN, "packs_pln": pln(cur.Packs), "packs": cur.PacksN},
		"returns": gin.H{"pln": pln(r.Live[0]), "count": r.Live[1], "cash_pln": pln(r.Cash[0]), "card_pln": pln(r.Card[0]),
			"blik_pln": pln(r.Blik[0])},
		"cogs": gin.H{"pln": pln(cur.cogs()), "sold_pln": pln(cur.Sold.Cost), "sold_units": cur.Sold.Items,
			"gifts_pln": pln(cur.Gifts.Cost), "gift_units": cur.Gifts.Items,
			"no_cost_units": cur.Sold.Items - cur.Sold.Costed + cur.Gifts.Items - cur.Gifts.Costed},
		// решение основателя 04.10: бой и недостача — по себестоимости партий
		"losses":    gin.H{"pln": pln(cur.Losses), "units": cur.LossUnits, "no_cost_units": cur.LossUnits - cur.LossCosted},
		"gross_pln": pln(gross),
		"expenses": gin.H{"pln": pln(cur.Expenses), "count": cur.ExpensesN, "by_category": cats,
			"purchases_pln": pln(cur.Purchases)}, // Д9: закупки — не расход, они в себестоимости
		"result_pln": pln(result),
		"prev": gin.H{"income_pln": pln(oIncome), "returns_pln": pln(old.Returns.Live[0]), "cogs_pln": pln(old.cogs()),
			"losses_pln": pln(old.Losses), "gross_pln": pln(oGross), "expenses_pln": pln(old.Expenses), "result_pln": pln(oResult)},
		// рост в процентах — у строк, которые не бывают меньше нуля; валовую и результат сравнивают по prev
		"delta": gin.H{"income": pct(income, oIncome), "returns": pct(r.Live[0], old.Returns.Live[0]),
			"cogs": pct(cur.cogs(), old.cogs()), "losses": pct(cur.Losses, old.Losses), "expenses": pct(cur.Expenses, old.Expenses)},
		// рядом (З12-и4): сколько заработано отданным временем и товаром и сколько клуб должен гостям
		"earned": earnedIn(p.From, p.To), "liabilities": liabilitiesNow(),
	})
}
