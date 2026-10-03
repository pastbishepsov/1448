package main

// З8-и3 «Кассовая книга» (OWNER.md, трек З; №78): ящик клуба по клубным
// дням — только владельцу. На каждый день: остаток на начало, пришло
// наличными по видам (пополнения, товары и кухня, пакеты), возвраты,
// инкассации, коррекции, расхождения пересчётов, остаток на конец; ниже —
// сдачи дня (кто, когда, должно, пересчитано, расхождение) и события ящика
// (инкассации и коррекции, с отменёнными — пометкой). З9-и3а — расходы из кассы,
// З9-и3б — возвраты владельцу из кассы (с отменёнными — пометкой). З11-и3 —
// возвраты наличными: кто, кому, за что и сколько отдал из ящика (returns.go).
//
// Остаток на момент t считается так же, как «должно быть» в сдаче: остаток
// последней сдачи до t (пересчитанный, если пересчитывали) + движение с неё
// (drawer.go). Конец дня — начало следующего, поэтому дни сшиты без щелей.
// Расхождения дня — то, что осталось после движения: на конец − (на начало +
// пришло − ушло ± коррекции); так строка дня сходится всегда, даже если
// событие ящика отменили уже после сдачи. День, в котором ящик заведён
// (нулевая точка), начинается с «ящика ещё не было» и начального остатка.

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

const maxBookDays = 92 // книга считается по дню; квартал — с запасом

// drawerBalanceAt — сколько было в ящике в момент t: остаток последней сдачи
// до t + движение с неё; nil — ящика ещё не было.
func drawerBalanceAt(q *gorm.DB, t time.Time) *int64 {
	var co models.ShiftCloseout
	if q.Where("opening_grosz IS NOT NULL AND created_at < ?", t).Order("created_at DESC").First(&co).Error != nil {
		return nil
	}
	b := drawerExpected(drawerClosing(co.ExpectedGrosz, co.CountedGrosz), drawerFlowFor(q, nil, co.CreatedAt, t))
	return &b
}

// bookWindow — клубные сутки книги: [From, To), у сегодняшних To — «сейчас».
type bookWindow struct {
	Day      time.Time
	From, To time.Time
}

// bookWindows — дни периода по часам клуба (чистая, тест): начало — report_hour
// по стенным часам (в дни перевода часов сутки длиной 23 или 25 часов), дни
// после «сейчас» не входят, сегодняшний обрезан «сейчас».
func bookWindows(p period, reportHour int, now time.Time) []bookWindow {
	loc := p.FromDay.Location()
	out := make([]bookWindow, 0, p.Days)
	for d := p.FromDay; !d.After(p.ToDay); d = d.AddDate(0, 0, 1) {
		from := time.Date(d.Year(), d.Month(), d.Day(), reportHour, 0, 0, 0, loc)
		if !from.Before(now) {
			break
		}
		to := time.Date(d.Year(), d.Month(), d.Day()+1, reportHour, 0, 0, 0, loc)
		if to.After(now) {
			to = now
		}
		out = append(out, bookWindow{Day: d, From: from, To: to})
	}
	return out
}

// bookDiff — расхождения пересчётов за день (чистая, тест): остаток на конец
// минус то, что дало бы одно движение.
func bookDiff(opening, closing int64, f drawerFlow) int64 {
	return closing - drawerExpected(opening, f)
}

func plnPtr(g *int64) any {
	if g == nil {
		return nil
	}
	return models.PLNFromGrosz(*g)
}

// GET /admin/cash/book?preset=…|from=…&to=… — кассовая книга по клубным дням (owner).
func handleCashBook(c *gin.Context) {
	p, _, reportHour, ok := periodFromQuery(c)
	if !ok {
		return
	}
	if p.Days > maxBookDays {
		c.JSON(http.StatusBadRequest, gin.H{"code": "too_long", "message": "Кассовая книга — не длиннее 92 дней"})
		return
	}
	now := time.Now()
	wins := bookWindows(p, reportHour, now)

	var closeouts []models.ShiftCloseout
	var events []models.CashEvent
	var tills []models.Expense         // З9-и3а: расходы из кассы — по моменту, когда ушли из ящика
	var owners []models.OwnerRepayment // З9-и3б: возвраты владельцу из кассы — по минуте записи
	var refunds []returnRow            // З11-и3: возвраты наличными — по моменту отмены
	if len(wins) > 0 {
		from, to := wins[0].From, wins[len(wins)-1].To
		db.Where("created_at >= ? AND created_at < ?", from, to).Order("created_at").Find(&closeouts)
		db.Where("created_at >= ? AND created_at < ?", from, to).Order("created_at").Find(&events)
		expenseQ().Where("drawer_at >= ? AND drawer_at < ? AND storno_at IS NULL", from, to).Order("drawer_at").Find(&tills)
		db.Where("method = ? AND created_at >= ? AND created_at < ?", models.RepayTill, from, to).Order("created_at").Find(&owners)
		refunds = cashRefunds(returnsIn(db, from, to))
	}
	explained := map[uuid.UUID]int64{} // сдача → расходы, внесённые позже и объяснившие её расхождение
	for i := range tills {
		if x := tills[i].ExplainsCloseout; x != nil {
			explained[*x] += tills[i].GrossGrosz
		}
	}
	catNames := categoryNames()
	ids := []string{}
	for i := range closeouts {
		ids = append(ids, closeouts[i].UserID.String())
	}
	for i := range events {
		for _, u := range []*uuid.UUID{events[i].CreatedBy, events[i].VoidedBy} {
			if u != nil {
				ids = append(ids, u.String())
			}
		}
	}
	for i := range owners {
		for _, u := range []*uuid.UUID{owners[i].CreatedBy, owners[i].VoidedBy} {
			if u != nil {
				ids = append(ids, u.String())
			}
		}
	}
	nick := nicknamesByID(append(ids, returnUsers(refunds)...))
	who := func(u *uuid.UUID) string {
		if u == nil {
			return ""
		}
		return nick[u.String()]
	}

	days := make([]gin.H, 0, len(wins))
	var total drawerFlow
	var totalDiff int64
	opening := drawerBalanceAt(db, p.From) // начало первого дня
	first, last := opening, opening
	for wi, w := range wins {
		f := drawerFlowFor(db, nil, w.From, w.To)
		closing := drawerBalanceAt(db, w.To)
		row := f.out()
		row["date"], row["from"], row["to"] = w.Day.Format("2006-01-02"), w.From, w.To
		row["today"] = wi == len(wins)-1 && !w.To.Before(now)
		row["opening_pln"], row["closing_pln"] = plnPtr(opening), plnPtr(closing)

		dayCO := []gin.H{}
		var start *models.ShiftCloseout // нулевая точка этого дня
		for i := range closeouts {
			co := &closeouts[i]
			if co.CreatedAt.Before(w.From) || !co.CreatedAt.Before(w.To) {
				continue
			}
			r := closeoutOut(co)
			r["nickname"] = nick[co.UserID.String()]
			if x := explained[co.ID]; x > 0 {
				r["explained_pln"] = models.PLNFromGrosz(x)
			}
			dayCO = append(dayCO, r)
			if start == nil && opening == nil && co.OpeningGrosz != nil {
				start = co
			}
		}
		dayEv := []gin.H{}
		for i := range events {
			e := &events[i]
			if e.CreatedAt.Before(w.From) || !e.CreatedAt.Before(w.To) {
				continue
			}
			r := cashEventOut(e)
			r["nickname"] = who(e.CreatedBy)
			if e.VoidedBy != nil {
				r["voided_by_nickname"] = who(e.VoidedBy)
			}
			dayEv = append(dayEv, r)
		}
		dayExp := []gin.H{}
		for i := range tills {
			e := &tills[i]
			if e.DrawerAt.Before(w.From) || !e.DrawerAt.Before(w.To) {
				continue
			}
			r := gin.H{"id": e.ID, "category": catNames[e.CategoryID.String()], "amount_pln": models.PLNFromGrosz(e.GrossGrosz),
				"note": e.Note, "doc_no": e.DocNo, "op_date": e.OpDate.Format("2006-01-02"), "created_at": e.CreatedAt, "drawer_at": e.DrawerAt}
			if e.ExplainsCloseout != nil {
				r["explains_closeout"] = *e.ExplainsCloseout
			}
			dayExp = append(dayExp, r)
		}
		dayOwn := []gin.H{}
		for i := range owners {
			r := &owners[i]
			if r.CreatedAt.Before(w.From) || !r.CreatedAt.Before(w.To) {
				continue
			}
			dayOwn = append(dayOwn, repaymentOut(r, nick))
		}
		dayRef := []gin.H{} // З11-и3: Σ — «возвраты» дня; «оплачено раньше» — до начала этого дня
		for i := range refunds {
			r := &refunds[i]
			if r.VoidedAt.Before(w.From) || !r.VoidedAt.Before(w.To) {
				continue
			}
			dayRef = append(dayRef, returnOut(*r, nick, w.From))
		}
		row["closeouts"], row["events"], row["expenses"], row["owner"] = dayCO, dayEv, dayExp, dayOwn
		row["refunds"] = dayRef

		var diff int64
		switch {
		case opening != nil && closing != nil:
			diff = bookDiff(*opening, *closing, f)
		case start != nil && closing != nil: // ящик заведён сегодня: считаем от начального остатка
			base := drawerClosing(start.ExpectedGrosz, start.CountedGrosz)
			diff = bookDiff(base, *closing, drawerFlowFor(db, nil, start.CreatedAt, w.To))
			row["baseline"] = gin.H{"pln": models.PLNFromGrosz(base), "by": nick[start.UserID.String()], "at": start.CreatedAt}
		}
		row["diff_pln"] = models.PLNFromGrosz(diff)
		days = append(days, row)

		total.Deposits += f.Deposits
		total.Goods += f.Goods
		total.Packs += f.Packs
		total.Refunds += f.Refunds
		total.Collections += f.Collections
		total.Expenses += f.Expenses
		total.Owner += f.Owner
		total.Adjust += f.Adjust
		totalDiff += diff
		opening, last = closing, closing
	}
	tot := total.out()
	tot["diff_pln"] = models.PLNFromGrosz(totalDiff)
	st := drawerAt(db, now)
	c.JSON(http.StatusOK, gin.H{
		"period": p.out(), "days": days, "totals": tot,
		"opening_pln": plnPtr(first), "closing_pln": plnPtr(last),
		"now": gin.H{"expected_pln": models.PLNFromGrosz(st.Expected), "baseline": st.Point == nil},
		// З12-и1 (№49): рядом с ящиком — сколько клуб должен гостям (снимок) и что сгорело за период книги
		"liabilities": liabilitiesNow(), "burned": burnedIn(p.From, p.To),
	})
}
