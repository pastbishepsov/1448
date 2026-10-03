package main

// З11-и3 «Возвраты за прошлый месяц» (OWNER.md, трек З; №17; З-Р12; решение
// основателя 30.09).
//
// Возврат — операция своего дня: отмена ложится в день отмены (voided_at, по
// клубным суткам), как бы давно ни платили. «Выручка» считает оплату по дню
// оплаты и без отменённых (moneySources): отмена июльской продажи в сентябре
// убирает её из июля задним числом, а деньги гостю отдали в сентябре. Блок
// показывает, сколько, как, кому и за что вернули.
//
// Блок в «Деньгах» — всегда за прошлый месяц, от периода отчёта не зависит
// (решение основателя 30.09: «возвраты будут высвечиваться только за прошлый
// месяц»); month=ГГГГ-ММ — любой месяц не позже текущего: для проверок и для
// фильтра за несколько месяцев, если его попросят.
//
// Что считается: отмены депозитов, продаж (и чеков, и расчётов кухни — это
// продажи) и выданных пакетов, за которые брали деньги. Неоплаченный долг и
// подарок (З11-и5) — нет: денег не брали, возвращать нечего. Сумма — как у ящика (drawer.go):
// депозит — снятое с кошелька, продажа — вся цена, пакет — доля за
// неотыгранное; «не возвращали» — ноль. Живые деньги — наличными, на карту, по
// BLIK; на кошелёк — отдельно: из клуба деньги не ушли, снова выросло
// обязательство перед гостем.
//
// Строки «выручка за вычетом возвратов» нет: «Выручка» уже без отменённых, и
// вычесть возвраты ещё раз по дню отмены — вычесть дважды. «Доходы с
// отменёнными − возвраты по дню отмены» — это «Прибыль» (З12-и3): возвраты она
// возьмёт отсюда (returnsIn).
//
// Наличные возвраты — ещё и в кассовой книге (cashbook.go), по дням: кто, кому,
// за что и сколько отдал из ящика.

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

const maxReturnItems = 300 // строк в списке блока; итоги — по всем

// returnRow — одна отмена, за которую брали деньги.
type returnRow struct {
	Kind        string // deposit | sale | package
	ID          uuid.UUID
	Name        string // товар или пакет; у депозита пусто
	Qty         int
	ReceiptID   *uuid.UUID
	UserID      *uuid.UUID
	PaidGrosz   int64 // сколько платили
	PaidMethod  string
	PaidAt      time.Time
	RefundGrosz int64  // сколько вернули; «не возвращали» — 0
	Method      string // как вернули: cash | card | blik | wallet | none
	VoidedAt    time.Time
	VoidedBy    *uuid.UUID
}

// returnsSQL — отмены с деньгами в [from, to) по моменту отмены, свежие сверху.
// Способ — как вернули (у отменённых до З11 — как платили), сумма — как у ящика.
const returnsSQL = `
SELECT 'deposit' AS kind, id, '' AS name, 1 AS qty, NULL::uuid AS receipt_id, user_id,
       ROUND(amount_pln * 100)::bigint AS paid_grosz, method AS paid_method, created_at AS paid_at,
       CASE WHEN COALESCE(refund_method, method) IN ('cash', 'card', 'blik', 'wallet') THEN refund_grosz ELSE 0 END AS refund_grosz,
       COALESCE(refund_method, method) AS method, voided_at, voided_by
  FROM deposits WHERE voided_at >= ? AND voided_at < ?
UNION ALL
SELECT 'sale', id, name, qty, receipt_id, user_id,
       ROUND(total_pln * 100)::bigint, method, paid_at,
       CASE WHEN COALESCE(refund_method, method) IN ('cash', 'card', 'blik', 'wallet') THEN ROUND(total_pln * 100)::bigint ELSE 0 END,
       COALESCE(refund_method, method), voided_at, voided_by
  FROM sales WHERE paid_at IS NOT NULL AND method <> 'comp' AND voided_at >= ? AND voided_at < ?
UNION ALL
SELECT 'package', id, name, 1, NULL::uuid, user_id,
       ROUND(price_pln * 100)::bigint, method, created_at,
       CASE WHEN COALESCE(refund_method, method) IN ('cash', 'card', 'blik', 'wallet') THEN refund_grosz ELSE 0 END,
       COALESCE(refund_method, method), voided_at, voided_by
  FROM user_packages WHERE method <> 'comp' AND voided_at >= ? AND voided_at < ?
ORDER BY voided_at DESC, kind, id`

// returnsIn — отмены с деньгами в [from, to), свежие сверху.
func returnsIn(q *gorm.DB, from, to time.Time) []returnRow {
	var rows []returnRow
	q.Raw(returnsSQL, from, to, from, to, from, to).Scan(&rows)
	return rows
}

// returnsTotals — итоги возвратов: гроши и штуки.
type returnsTotals struct {
	Count                          int64
	Live, Cash, Card, Blik, Wallet [2]int64 // вернули
	None                           [2]int64 // «не возвращали»: на сколько их оплатили
	Earlier                        [2]int64 // вернули по операциям, оплаченным раньше
}

// sumReturns — итоги по строкам (чистая, тест); earlierThan — с какого момента
// оплата «своя» (начало месяца), раньше — «по операциям прошлых месяцев».
func sumReturns(rows []returnRow, earlierThan time.Time) returnsTotals {
	var t returnsTotals
	add := func(a *[2]int64, g int64) { a[0] += g; a[1]++ }
	for _, r := range rows {
		t.Count++
		switch r.Method {
		case "cash":
			add(&t.Cash, r.RefundGrosz)
		case "card":
			add(&t.Card, r.RefundGrosz)
		case "blik":
			add(&t.Blik, r.RefundGrosz)
		case "wallet":
			add(&t.Wallet, r.RefundGrosz)
		default: // «не возвращали» и способы из старых данных — денег не отдавали
			add(&t.None, r.PaidGrosz)
		}
		switch r.Method {
		case "cash", "card", "blik":
			add(&t.Live, r.RefundGrosz)
		}
		if r.PaidAt.Before(earlierThan) {
			add(&t.Earlier, r.RefundGrosz)
		}
	}
	return t
}

func (t returnsTotals) out() gin.H {
	pln := models.PLNFromGrosz
	return gin.H{
		"count":    t.Count,
		"live_pln": pln(t.Live[0]), "live_count": t.Live[1],
		"cash_pln": pln(t.Cash[0]), "cash_count": t.Cash[1],
		"card_pln": pln(t.Card[0]), "card_count": t.Card[1],
		"blik_pln": pln(t.Blik[0]), "blik_count": t.Blik[1],
		"wallet_pln": pln(t.Wallet[0]), "wallet_count": t.Wallet[1],
		"none_paid_pln": pln(t.None[0]), "none_count": t.None[1],
		"earlier_pln": pln(t.Earlier[0]), "earlier_count": t.Earlier[1],
	}
}

// cashRefunds — возвраты из ящика: наличными и не нулём, по порядку (чистая,
// тест). Строки returnsIn — свежие сверху, книге нужен порядок дня.
func cashRefunds(rows []returnRow) []returnRow {
	out := []returnRow{}
	for i := len(rows) - 1; i >= 0; i-- {
		if rows[i].Method == "cash" && rows[i].RefundGrosz > 0 {
			out = append(out, rows[i])
		}
	}
	return out
}

// returnUsers — гости и сотрудники строк: для nicknamesByID.
func returnUsers(rows []returnRow) []string {
	ids := []string{}
	for _, r := range rows {
		for _, u := range []*uuid.UUID{r.UserID, r.VoidedBy} {
			if u != nil {
				ids = append(ids, u.String())
			}
		}
	}
	return ids
}

// returnOut — строка отмены для экрана; earlierThan — «оплачено раньше».
func returnOut(r returnRow, nick map[string]string, earlierThan time.Time) gin.H {
	who := func(u *uuid.UUID) string {
		if u == nil {
			return ""
		}
		return nick[u.String()]
	}
	return gin.H{
		"kind": r.Kind, "id": r.ID, "name": r.Name, "qty": r.Qty, "receipt_id": r.ReceiptID,
		"user_id": r.UserID, "guest": who(r.UserID),
		"paid_pln": models.PLNFromGrosz(r.PaidGrosz), "paid_method": r.PaidMethod, "paid_at": r.PaidAt,
		"refund_pln": models.PLNFromGrosz(r.RefundGrosz), "refund_method": r.Method,
		"voided_at": r.VoidedAt, "voided_by": who(r.VoidedBy), "earlier": r.PaidAt.Before(earlierThan),
	}
}

// returnsMonth — месяц блока (чистая, тест): "" — прошлый месяц по клубным
// суткам, «ГГГГ-ММ» — этот месяц, не позже текущего. Окно — клубные сутки
// месяца: с 1-го числа report_hour до 1-го числа следующего report_hour.
func returnsMonth(s string, reportHour int, now time.Time) (period, bool) {
	loc := now.Location()
	cur := clubDayOf(now, reportHour)
	this := time.Date(cur.Year(), cur.Month(), 1, 0, 0, 0, 0, loc)
	if s == "" {
		first := this.AddDate(0, -1, 0)
		return makePeriod(first, first.AddDate(0, 1, -1), reportHour, "prev_month"), true
	}
	m, err := time.ParseInLocation("2006-01", s, loc)
	if err != nil || m.After(this) || m.Year() < 2000 {
		return period{}, false
	}
	return makePeriod(m, m.AddDate(0, 1, -1), reportHour, "custom"), true
}

// GET /admin/reports/returns?month=ГГГГ-ММ — возвраты месяца по дню отмены
// (owner); без month — прошлый месяц.
func handleReportReturns(c *gin.Context) {
	reportHour := int(settingInt64("report_hour", 8))
	p, ok := returnsMonth(c.Query("month"), reportHour, time.Now())
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"code": "bad_month", "message": "Месяц — в формате ГГГГ-ММ и не позже текущего"})
		return
	}
	rows := returnsIn(db, p.From, p.To)
	shown := rows[:min(len(rows), maxReturnItems)]
	nick := nicknamesByID(returnUsers(shown))
	items := make([]gin.H, 0, len(shown))
	for _, r := range shown {
		items = append(items, returnOut(r, nick, p.From))
	}
	c.JSON(http.StatusOK, gin.H{
		"month": p.FromDay.Format("2006-01"), "period": p.out(),
		"totals": sumReturns(rows, p.From).out(), "items": items, "items_total": len(rows),
	})
}
