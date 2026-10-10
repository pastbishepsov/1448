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

// bookDay — день кассовой книги: окно, движение наличных, остатки, расхождение пересчётов и то, из чего день
// сложился. Opening/Closing nil — ящика ещё не было; Start — нулевая точка, если ящик заведён в этот день (Base —
// её остаток, от него считается расхождение дня).
type bookDay struct {
	bookWindow
	Flow             drawerFlow
	Opening, Closing *int64
	Diff             int64
	Today            bool
	Start            *models.ShiftCloseout
	Base             int64
	Closeouts        []*models.ShiftCloseout
	Events           []*models.CashEvent      // с отменёнными — их строка с пометкой
	Tills            []*models.Expense        // расходы из кассы — по моменту, когда ушли из ящика
	Owners           []*models.OwnerRepayment // возвраты владельцу из кассы, с отменёнными
	Refunds          []returnRow              // возвраты наличными — по моменту отмены
}

// cashBook — кассовая книга периода: дни, итоги и остатки на начало и конец. Одна функция для экрана (З8-и3) и
// для листов выгрузки «Касса» и «Сдачи касс» (З13-и5): цифры сходятся.
type cashBook struct {
	Days        []bookDay
	Total       drawerFlow
	Diff        int64
	First, Last *int64              // остаток на начало периода и на конец последнего дня
	Explained   map[uuid.UUID]int64 // сдача → расходы, внесённые позже и объяснившие её расхождение
	Nick        map[string]string   // ники всех, кто в книге (и кто вносил расходы из кассы)
	Cats        map[string]string   // категории расходов
}

// cashBookIn — кассовая книга периода p по клубным дням до «сейчас».
func cashBookIn(p period, reportHour int, now time.Time) cashBook {
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
	b := cashBook{Explained: map[uuid.UUID]int64{}, Cats: categoryNames()}
	for i := range tills {
		if x := tills[i].ExplainsCloseout; x != nil {
			b.Explained[*x] += tills[i].GrossGrosz
		}
	}
	ids := []string{}
	add := func(us ...*uuid.UUID) {
		for _, u := range us {
			if u != nil {
				ids = append(ids, u.String())
			}
		}
	}
	for i := range closeouts {
		ids = append(ids, closeouts[i].UserID.String())
	}
	for i := range events {
		add(events[i].CreatedBy, events[i].VoidedBy)
	}
	for i := range owners {
		add(owners[i].CreatedBy, owners[i].VoidedBy)
	}
	for i := range tills {
		add(tills[i].CreatedBy)
	}
	b.Nick = nicknamesByID(append(ids, returnUsers(refunds)...))

	opening := drawerBalanceAt(db, p.From) // начало первого дня
	b.First, b.Last = opening, opening
	for wi, w := range wins {
		d := bookDay{bookWindow: w, Opening: opening, Flow: drawerFlowFor(db, nil, w.From, w.To), Closing: drawerBalanceAt(db, w.To)}
		d.Today = wi == len(wins)-1 && !w.To.Before(now)
		in := func(t time.Time) bool { return !t.Before(w.From) && t.Before(w.To) }
		var start *models.ShiftCloseout // нулевая точка этого дня
		for i := range closeouts {
			co := &closeouts[i]
			if !in(co.CreatedAt) {
				continue
			}
			d.Closeouts = append(d.Closeouts, co)
			if start == nil && opening == nil && co.OpeningGrosz != nil {
				start = co
			}
		}
		for i := range events {
			if in(events[i].CreatedAt) {
				d.Events = append(d.Events, &events[i])
			}
		}
		for i := range tills {
			if in(*tills[i].DrawerAt) {
				d.Tills = append(d.Tills, &tills[i])
			}
		}
		for i := range owners {
			if in(owners[i].CreatedAt) {
				d.Owners = append(d.Owners, &owners[i])
			}
		}
		for i := range refunds { // З11-и3: Σ — «возвраты» дня; «оплачено раньше» — до начала этого дня
			if in(refunds[i].VoidedAt) {
				d.Refunds = append(d.Refunds, refunds[i])
			}
		}
		switch {
		case opening != nil && d.Closing != nil:
			d.Diff = bookDiff(*opening, *d.Closing, d.Flow)
		case start != nil && d.Closing != nil: // ящик заведён сегодня: считаем от начального остатка
			d.Start, d.Base = start, drawerClosing(start.ExpectedGrosz, start.CountedGrosz)
			d.Diff = bookDiff(d.Base, *d.Closing, drawerFlowFor(db, nil, start.CreatedAt, w.To))
		}
		b.Days = append(b.Days, d)

		b.Total.Deposits += d.Flow.Deposits
		b.Total.Goods += d.Flow.Goods
		b.Total.Packs += d.Flow.Packs
		b.Total.Refunds += d.Flow.Refunds
		b.Total.Collections += d.Flow.Collections
		b.Total.Expenses += d.Flow.Expenses
		b.Total.Owner += d.Flow.Owner
		b.Total.Adjust += d.Flow.Adjust
		b.Diff += d.Diff
		opening, b.Last = d.Closing, d.Closing
	}
	return b
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
	b := cashBookIn(p, reportHour, now)
	who := func(u *uuid.UUID) string {
		if u == nil {
			return ""
		}
		return b.Nick[u.String()]
	}

	days := make([]gin.H, 0, len(b.Days))
	for _, d := range b.Days {
		row := d.Flow.out()
		row["date"], row["from"], row["to"] = d.Day.Format("2006-01-02"), d.From, d.To
		row["today"] = d.Today
		row["opening_pln"], row["closing_pln"] = plnPtr(d.Opening), plnPtr(d.Closing)

		dayCO := []gin.H{}
		for _, co := range d.Closeouts {
			r := closeoutOut(co)
			r["nickname"] = b.Nick[co.UserID.String()]
			if x := b.Explained[co.ID]; x > 0 {
				r["explained_pln"] = models.PLNFromGrosz(x)
			}
			dayCO = append(dayCO, r)
		}
		dayEv := []gin.H{}
		for _, e := range d.Events {
			r := cashEventOut(e)
			r["nickname"] = who(e.CreatedBy)
			if e.VoidedBy != nil {
				r["voided_by_nickname"] = who(e.VoidedBy)
			}
			dayEv = append(dayEv, r)
		}
		dayExp := []gin.H{}
		for _, e := range d.Tills {
			r := gin.H{"id": e.ID, "category": b.Cats[e.CategoryID.String()], "amount_pln": models.PLNFromGrosz(e.GrossGrosz),
				"note": e.Note, "doc_no": e.DocNo, "op_date": e.OpDate.Format("2006-01-02"), "created_at": e.CreatedAt, "drawer_at": e.DrawerAt}
			if e.ExplainsCloseout != nil {
				r["explains_closeout"] = *e.ExplainsCloseout
			}
			dayExp = append(dayExp, r)
		}
		dayOwn := []gin.H{}
		for _, r := range d.Owners {
			dayOwn = append(dayOwn, repaymentOut(r, b.Nick))
		}
		dayRef := []gin.H{} // З11-и3: Σ — «возвраты» дня; «оплачено раньше» — до начала этого дня
		for _, r := range d.Refunds {
			dayRef = append(dayRef, returnOut(r, b.Nick, d.From))
		}
		row["closeouts"], row["events"], row["expenses"], row["owner"] = dayCO, dayEv, dayExp, dayOwn
		row["refunds"] = dayRef
		if d.Start != nil {
			row["baseline"] = gin.H{"pln": models.PLNFromGrosz(d.Base), "by": b.Nick[d.Start.UserID.String()], "at": d.Start.CreatedAt}
		}
		row["diff_pln"] = models.PLNFromGrosz(d.Diff)
		days = append(days, row)
	}
	tot := b.Total.out()
	tot["diff_pln"] = models.PLNFromGrosz(b.Diff)
	st := drawerAt(db, now)
	c.JSON(http.StatusOK, gin.H{
		"period": p.out(), "days": days, "totals": tot,
		"opening_pln": plnPtr(b.First), "closing_pln": plnPtr(b.Last),
		"now": gin.H{"expected_pln": models.PLNFromGrosz(st.Expected), "baseline": st.Point == nil},
		// З12-и1 (№49): рядом с ящиком — сколько клуб должен гостям (снимок) и что сгорело за период книги
		"liabilities": liabilitiesNow(), "burned": burnedIn(p.From, p.To),
	})
}
