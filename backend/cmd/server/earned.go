package main

// З12-и2 «Расход пакетов и отработанная выручка» (OWNER.md, трек З; №49; З-Р9; Д7; решение основателя 03.10 —
// минуты за монеты и компенсации — не выручка, отдельной строкой).
//
// «Отработанная выручка» за период — сколько клуб заработал, отдав гостям время и товар, а не сколько денег
// пришло (это кассовая «Выручка»): время с кошелька (списания за сессии, по моменту списания) + минуты пакетов по
// их цене (журнал pack_spends, по моменту списания; подаренный пакет — 0 zł) + товары у стойки по моменту
// продажи, любым способом, кроме подарков + кухня по моменту выдачи (заказ «выдан»), кроме подаренной.
// Пополнения и проданные пакеты сюда не входят — это предоплата: она зарабатывается, когда её отыгрывают.
// Продажа в долг заработана в момент продажи, даже если долг потом списали (списание — «Потери»).
//
// До журнала (Д7) — оценка: минуты пакетов сессий, начатых в окне, которых нет в журнале, × средняя цена минуты
// пакетов, проданных за деньги (не подарков; отменённые — тоже: цену минуты отмена не меняет).
//
// «Отдано временем» (решение основателя 03.10) — не выручка, рядом отдельной строкой: минуты за монеты и
// компенсации (зонные и пул без зоны), сыгранные в сессиях, начатых в окне, по тарифу зоны сессии.
//
// Число — в GET /admin/reports/money (`earned`, только владельцу); на экран оно выйдет в «Прибыли» (З12-и4).

import (
	"math"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

// earnedAgg — части «Отработанной выручки» за период, в грошах и минутах.
type earnedAgg struct {
	Wallet   int64   // время с кошелька
	Packs    int64   // минуты пакетов по журналу
	PackMin  int64   // сколько минут пакетов по журналу
	GiftMin  int64   // из них — с подаренных пакетов (0 zł)
	EstMin   int64   // минут пакетов без журнала (до З12-и2)
	Est      int64   // их оценка по средней цене минуты
	Avg      float64 // средняя цена минуты пакета, грошей
	Goods    int64   // товары у стойки
	Kitchen  int64   // кухня
	GivenMin int64   // отдано временем: минут за монеты и компенсации
	Given    int64   // — по тарифу зоны
}

// total — «Отработанная выручка»: всё, кроме «отдано временем» (чистая, тест).
func (e earnedAgg) total() int64 { return e.Wallet + e.Packs + e.Est + e.Goods + e.Kitchen }

// earnedSQL — части за [@from, @to). Сессии — засчитанные (не снятые посадки), по моменту начала, как «наиграно».
const earnedSQL = `WITH
  e AS (SELECT COALESCE(SUM(GREATEST(sessions.pack_minutes_used - COALESCE(l.m, 0), 0)), 0)::bigint AS est_min
          FROM sessions
          LEFT JOIN (SELECT session_id, SUM(minutes) AS m FROM pack_spends WHERE session_id IS NOT NULL GROUP BY session_id) l ON l.session_id = sessions.id
         WHERE ` + sessionCounted + ` AND sessions.pack_minutes_used > 0 AND sessions.started_at >= @from AND sessions.started_at < @to),
  a AS (SELECT SUM(price_pln * 100)::numeric / NULLIF(SUM(minutes_total), 0) AS avg
          FROM user_packages WHERE method <> 'comp')
SELECT
  (SELECT COALESCE(-SUM(amount_grosz), 0) FROM wallet_transactions
    WHERE kind = 'session_spend' AND amount_grosz < 0 AND created_at >= @from AND created_at < @to)::bigint AS wallet,
  (SELECT COALESCE(SUM(grosz), 0) FROM pack_spends WHERE created_at >= @from AND created_at < @to)::bigint AS packs,
  (SELECT COALESCE(SUM(minutes), 0) FROM pack_spends WHERE created_at >= @from AND created_at < @to)::bigint AS pack_min,
  (SELECT COALESCE(SUM(ps.minutes), 0) FROM pack_spends ps JOIN user_packages p ON p.id = ps.user_package_id
    WHERE p.method = 'comp' AND ps.created_at >= @from AND ps.created_at < @to)::bigint AS gift_min,
  e.est_min, COALESCE(ROUND(e.est_min * a.avg), 0)::bigint AS est, COALESCE(a.avg, 0)::float8 AS avg,
  (SELECT COALESCE(SUM(ROUND(total_pln * 100)), 0) FROM sales s
    WHERE s.voided_at IS NULL AND s.method <> 'comp' AND s.created_at >= @from AND s.created_at < @to
      AND NOT EXISTS (SELECT 1 FROM kitchen_orders o WHERE o.sale_id = s.id))::bigint AS goods,
  (SELECT COALESCE(SUM(ROUND(total_pln * 100)), 0) FROM kitchen_orders
    WHERE status = 'done' AND COALESCE(pay_method, '') <> 'comp' AND status_at >= @from AND status_at < @to)::bigint AS kitchen,
  (SELECT COALESCE(SUM(coin_minutes_used), 0) FROM sessions
    WHERE ` + sessionCounted + ` AND coin_minutes_used > 0 AND started_at >= @from AND started_at < @to)::bigint AS given_min,
  (SELECT COALESCE(SUM(ROUND(coin_minutes_used * base_rate_pln * 100 / 60.0)), 0) FROM sessions
    WHERE ` + sessionCounted + ` AND coin_minutes_used > 0 AND started_at >= @from AND started_at < @to)::bigint AS given
FROM e, a`

// earnedAggIn — части «Отработанной выручки» за [from, to): одна функция для «Обзора» и для листа выгрузки
// «Отработано по дням» (З13-и5).
func earnedAggIn(from, to time.Time) earnedAgg {
	var e earnedAgg
	db.Raw(earnedSQL, map[string]any{"from": from, "to": to}).Scan(&e)
	return e
}

// earnedIn — «Отработанная выручка» за [from, to) и «отдано временем».
func earnedIn(from, to time.Time) gin.H {
	e := earnedAggIn(from, to)
	pln := models.PLNFromGrosz
	return gin.H{
		"pln":        pln(e.total()),
		"wallet_pln": pln(e.Wallet),
		"packs_pln":  pln(e.Packs), "packs_minutes": e.PackMin, "gift_minutes": e.GiftMin,
		// Д7: до журнала расхода — оценка по средней цене минуты пакета
		"estimate_pln": pln(e.Est), "estimate_minutes": e.EstMin, "pack_minute_pln": math.Round(e.Avg*100) / 10000,
		"goods_pln": pln(e.Goods), "kitchen_pln": pln(e.Kitchen),
		// решение основателя 03.10: не выручка — рядом отдельной строкой
		"given": gin.H{"pln": pln(e.Given), "minutes": e.GivenMin},
	}
}
