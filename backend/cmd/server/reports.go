package main

// Отчёты владельца за произвольный период (спринт В1, ADMIN.md).
// Решение основателя 2026-08-18: владельческая часть была «подбитой» —
// выручка и загрузка считались только за жёстко зашитые 14 дней, сегменты
// гостей вообще игнорировали период. Здесь один общий слой периода
// (пресеты + произвольный диапазон «с…по…»), автоматическое сравнение с
// предыдущим периодом и четыре разреза: Деньги · Гости · Загрузка · Персонал.
//
// Все окна считаются КЛУБНЫМИ СУТКАМИ от report_hour (настройка владельца):
// ночная смена целиком принадлежит дню своего начала, иначе выручка ночи
// размазывается на два календарных дня и цифры врут.
//
// Часы работы клуба для процента занятости берутся из активных шаблонов
// смен (миграция 023), а не из отдельной настройки: владелец и так ведёт
// смены, второй источник правды разошёлся бы с первым.

import (
	"fmt"
	"math"
	"net/http"
	"sort"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

// ── Период ────────────────────────────────────────────────────────────

const maxPeriodDays = 730 // два года: защита от отчёта «за всё время» на проде

// period — окно отчёта в клубных сутках.
type period struct {
	FromDay time.Time // первый день периода (полночь календарной даты)
	ToDay   time.Time // последний день периода, включительно
	Days    int
	From    time.Time // начало окна: FromDay + report_hour
	To      time.Time // конец окна, исключительно
	Preset  string
}

func (p period) out() gin.H {
	return gin.H{
		"from":   p.FromDay.Format("2006-01-02"),
		"to":     p.ToDay.Format("2006-01-02"),
		"days":   p.Days,
		"preset": p.Preset,
	}
}

// clubDayOf — клубные сутки, которым принадлежит момент t при границе reportHour.
func clubDayOf(t time.Time, reportHour int) time.Time {
	d := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
	if t.Hour() < reportHour {
		d = d.AddDate(0, 0, -1)
	}
	return d
}

// daysBetween — сколько календарных дней в [a, b] включительно (через AddDate,
// а не деление на 24 часа: перевод часов сломал бы арифметику).
func daysBetween(a, b time.Time) int {
	n := 1
	for d := a; d.Before(b); d = d.AddDate(0, 0, 1) {
		n++
	}
	return n
}

// makePeriod — период по двум датам включительно. Чистая функция (тест).
func makePeriod(fromDay, toDay time.Time, reportHour int, preset string) period {
	loc := fromDay.Location()
	return period{
		FromDay: fromDay,
		ToDay:   toDay,
		Days:    daysBetween(fromDay, toDay),
		Preset:  preset,
		From:    time.Date(fromDay.Year(), fromDay.Month(), fromDay.Day(), reportHour, 0, 0, 0, loc),
		To:      time.Date(toDay.Year(), toDay.Month(), toDay.Day()+1, reportHour, 0, 0, 0, loc),
	}
}

// resolvePeriod — разбор запроса в период. Чистая функция (тест в reports_test.go).
// Приоритет у произвольного диапазона: если пришёл from — пресет игнорируется.
func resolvePeriod(preset, fromStr, toStr string, reportHour int, now time.Time) (period, string) {
	loc := now.Location()
	cur := clubDayOf(now, reportHour)
	parse := func(s string) (time.Time, bool) {
		d, err := time.ParseInLocation("2006-01-02", s, loc)
		return d, err == nil
	}

	if fromStr != "" || toStr != "" {
		from, ok := parse(fromStr)
		if !ok {
			return period{}, "bad_from"
		}
		to := cur
		if toStr != "" {
			if to, ok = parse(toStr); !ok {
				return period{}, "bad_to"
			}
		}
		if to.Before(from) {
			return period{}, "bad_range"
		}
		p := makePeriod(from, to, reportHour, "custom")
		if p.Days > maxPeriodDays {
			return period{}, "too_long"
		}
		return p, ""
	}

	switch preset {
	case "", "d7":
		return makePeriod(cur.AddDate(0, 0, -6), cur, reportHour, "d7"), ""
	case "today":
		return makePeriod(cur, cur, reportHour, "today"), ""
	case "yesterday":
		d := cur.AddDate(0, 0, -1)
		return makePeriod(d, d, reportHour, "yesterday"), ""
	case "d30":
		return makePeriod(cur.AddDate(0, 0, -29), cur, reportHour, "d30"), ""
	case "d90":
		return makePeriod(cur.AddDate(0, 0, -89), cur, reportHour, "d90"), ""
	case "month":
		return makePeriod(time.Date(cur.Year(), cur.Month(), 1, 0, 0, 0, 0, loc), cur, reportHour, "month"), ""
	case "prev_month":
		first := time.Date(cur.Year(), cur.Month(), 1, 0, 0, 0, 0, loc).AddDate(0, -1, 0)
		return makePeriod(first, first.AddDate(0, 1, -1), reportHour, "prev_month"), ""
	case "year":
		return makePeriod(time.Date(cur.Year(), 1, 1, 0, 0, 0, 0, loc), cur, reportHour, "year"), ""
	}
	return period{}, "bad_preset"
}

// prevPeriod — с чем сравниваем. Для календарных пресетов это тот же отрезок
// прошлого месяца/года (владелец сравнивает «1–18 августа» с «1–18 июля»,
// а не с концом июля), для остальных — примыкающее окно той же длины.
func prevPeriod(p period, reportHour int) period {
	switch p.Preset {
	case "month":
		first := p.FromDay.AddDate(0, -1, 0)
		last := first.AddDate(0, 0, p.Days-1)
		if endOfMonth := first.AddDate(0, 1, -1); last.After(endOfMonth) {
			last = endOfMonth
		}
		return makePeriod(first, last, reportHour, "prev")
	case "prev_month":
		first := p.FromDay.AddDate(0, -1, 0)
		return makePeriod(first, first.AddDate(0, 1, -1), reportHour, "prev")
	case "year":
		return makePeriod(p.FromDay.AddDate(-1, 0, 0), p.ToDay.AddDate(-1, 0, 0), reportHour, "prev")
	}
	to := p.FromDay.AddDate(0, 0, -1)
	return makePeriod(to.AddDate(0, 0, -(p.Days-1)), to, reportHour, "prev")
}

var periodErrors = map[string]string{
	"bad_from":   "Дата начала — в формате ГГГГ-ММ-ДД",
	"bad_to":     "Дата конца — в формате ГГГГ-ММ-ДД",
	"bad_range":  "Конец периода раньше начала",
	"too_long":   fmt.Sprintf("Период длиннее %d дней", maxPeriodDays),
	"bad_preset": "Неизвестный период",
}

// periodFromQuery — общий разбор для всех отчётов: период, предыдущий период
// и граница клубных суток. При ошибке сам отвечает 400 и возвращает false.
func periodFromQuery(c *gin.Context) (period, period, int, bool) {
	reportHour := int(settingInt64("report_hour", 8))
	p, code := resolvePeriod(c.Query("preset"), c.Query("from"), c.Query("to"), reportHour, time.Now())
	if code != "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": code, "message": periodErrors[code]})
		return period{}, period{}, 0, false
	}
	return p, prevPeriod(p, reportHour), reportHour, true
}

// pctDelta — рост в процентах к предыдущему периоду. nil (JSON null) означает
// «не с чем сравнивать»: в прошлом периоде был ноль, процент бессмыслен.
func pctDelta(now, prev float64) *float64 {
	if prev == 0 {
		if now == 0 {
			z := 0.0
			return &z
		}
		return nil
	}
	v := math.Round((now-prev)/prev*1000) / 10
	return &v
}

// dayExpr — выражение «клубные сутки» для GROUP BY по колонке ts.
func dayExpr(col string, reportHour int) string {
	return fmt.Sprintf("(%s - INTERVAL '%d hours')::date", col, reportHour)
}

// fillDays — ряд по дням без дыр: дни без данных дают ноль, иначе график врёт.
func fillDays(p period, byDay map[string][2]float64) []gin.H {
	out := make([]gin.H, 0, p.Days)
	for d := p.FromDay; !d.After(p.ToDay); d = d.AddDate(0, 0, 1) {
		key := d.Format("2006-01-02")
		v := byDay[key]
		out = append(out, gin.H{"date": key, "value": v[0], "count": int64(v[1])})
	}
	return out
}

// ── Разрез «Деньги» ───────────────────────────────────────────────────

type moneyAgg struct {
	Revenue     float64 `json:"revenue_pln"` // итого: пополнения + товары
	DepositsPLN float64 `json:"deposits_pln"`
	Deposits    int64   `json:"deposits"`
	GoodsPLN    float64 `json:"goods_pln"`
	GoodsSales  int64   `json:"goods_sales"`
	GoodsItems  int64   `json:"goods_items"`
	Guests      int64   `json:"guests"`
	AvgCheck    float64 `json:"avg_check_pln"` // средний чек пополнения
	Cash        float64 `json:"cash_pln"`
	Card        float64 `json:"card_pln"`
	Blik        float64 `json:"blik_pln"`
}

// aggMoney — деньги за период. Выручка клуба = пополнения + продажи товаров
// (решение основателя 2026-08-18); отменённые продажи в выручку не идут.
func aggMoney(p period) moneyAgg {
	var a moneyAgg
	var head struct {
		Revenue  float64
		Deposits int64
		Guests   int64
	}
	db.Model(&models.Deposit{}).
		Select("COALESCE(SUM(amount_pln),0) AS revenue, COUNT(*) AS deposits, COUNT(DISTINCT user_id) AS guests").
		Where("created_at >= ? AND created_at < ?", p.From, p.To).Scan(&head)
	a.DepositsPLN, a.Deposits, a.Guests = head.Revenue, head.Deposits, head.Guests
	if a.Deposits > 0 {
		a.AvgCheck = math.Round(a.DepositsPLN/float64(a.Deposits)*100) / 100
	}

	var goods struct {
		Pln   float64
		Cnt   int64
		Items int64
	}
	db.Model(&models.Sale{}).
		Select("COALESCE(SUM(total_pln),0) AS pln, COUNT(*) AS cnt, COALESCE(SUM(qty),0) AS items").
		Where("created_at >= ? AND created_at < ? AND voided_at IS NULL", p.From, p.To).Scan(&goods)
	a.GoodsPLN, a.GoodsSales, a.GoodsItems = goods.Pln, goods.Cnt, goods.Items
	a.Revenue = math.Round((a.DepositsPLN+a.GoodsPLN)*100) / 100

	// разбивка по способам оплаты — по обоим каналам сразу
	add := func(method string, pln float64) {
		switch method {
		case "cash":
			a.Cash += pln
		case "card":
			a.Card += pln
		case "blik":
			a.Blik += pln
		}
	}
	var rows []struct {
		Method string
		Pln    float64
	}
	db.Model(&models.Deposit{}).Select("method, COALESCE(SUM(amount_pln),0) AS pln").
		Where("created_at >= ? AND created_at < ?", p.From, p.To).Group("method").Scan(&rows)
	for _, r := range rows {
		add(r.Method, r.Pln)
	}
	rows = nil
	db.Model(&models.Sale{}).Select("method, COALESCE(SUM(total_pln),0) AS pln").
		Where("created_at >= ? AND created_at < ? AND voided_at IS NULL", p.From, p.To).
		Group("method").Scan(&rows)
	for _, r := range rows {
		add(r.Method, r.Pln)
	}
	return a
}

// GET /admin/reports/money — выручка за период (owner).
func handleReportMoney(c *gin.Context) {
	p, prev, reportHour, ok := periodFromQuery(c)
	if !ok {
		return
	}
	cur, old := aggMoney(p), aggMoney(prev)

	// ряд по дням: столбик = пополнения + товары, в подсказке разбивка
	type dayRow struct {
		deposits float64
		goods    float64
		count    int64
	}
	perDay := map[string]*dayRow{}
	get := func(k string) *dayRow {
		if perDay[k] == nil {
			perDay[k] = &dayRow{}
		}
		return perDay[k]
	}
	var rows []struct {
		D   time.Time
		Pln float64
		Cnt int64
	}
	db.Model(&models.Deposit{}).
		Select(dayExpr("created_at", reportHour)+" AS d, COALESCE(SUM(amount_pln),0) AS pln, COUNT(*) AS cnt").
		Where("created_at >= ? AND created_at < ?", p.From, p.To).
		Group("d").Order("d").Scan(&rows)
	for _, r := range rows {
		v := get(r.D.Format("2006-01-02"))
		v.deposits, v.count = r.Pln, r.Cnt
	}
	rows = nil
	db.Model(&models.Sale{}).
		Select(dayExpr("created_at", reportHour)+" AS d, COALESCE(SUM(total_pln),0) AS pln, COUNT(*) AS cnt").
		Where("created_at >= ? AND created_at < ? AND voided_at IS NULL", p.From, p.To).
		Group("d").Order("d").Scan(&rows)
	for _, r := range rows {
		v := get(r.D.Format("2006-01-02"))
		v.goods, v.count = r.Pln, v.count+r.Cnt
	}
	days := make([]gin.H, 0, p.Days)
	for d := p.FromDay; !d.After(p.ToDay); d = d.AddDate(0, 0, 1) {
		key := d.Format("2006-01-02")
		v := perDay[key]
		if v == nil {
			v = &dayRow{}
		}
		days = append(days, gin.H{"date": key, "value": math.Round((v.deposits+v.goods)*100) / 100,
			"deposits_pln": v.deposits, "goods_pln": v.goods, "count": v.count})
	}

	var admRows []struct {
		AdminID string
		Pln     float64
		Cnt     int64
	}
	db.Model(&models.Deposit{}).
		Select("created_by AS admin_id, COALESCE(SUM(amount_pln),0) AS pln, COUNT(*) AS cnt").
		Where("created_at >= ? AND created_at < ? AND created_by IS NOT NULL", p.From, p.To).
		Group("created_by").Scan(&admRows)
	type byAdminRow struct {
		deposits float64
		cnt      int64
		goods    float64
		sales    int64
	}
	admins := map[string]*byAdminRow{}
	getAdmin := func(id string) *byAdminRow {
		if admins[id] == nil {
			admins[id] = &byAdminRow{}
		}
		return admins[id]
	}
	for _, r := range admRows {
		a := getAdmin(r.AdminID)
		a.deposits, a.cnt = r.Pln, r.Cnt
	}
	admRows = nil
	db.Model(&models.Sale{}).
		Select("created_by AS admin_id, COALESCE(SUM(total_pln),0) AS pln, COUNT(*) AS cnt").
		Where("created_at >= ? AND created_at < ? AND voided_at IS NULL", p.From, p.To).
		Group("created_by").Scan(&admRows)
	for _, r := range admRows {
		a := getAdmin(r.AdminID)
		a.goods, a.sales = r.Pln, r.Cnt
	}
	ids := make([]string, 0, len(admins))
	for id := range admins {
		ids = append(ids, id)
	}
	nick := nicknamesByID(ids)
	byAdmin := make([]gin.H, 0, len(admins))
	for id, a := range admins {
		byAdmin = append(byAdmin, gin.H{"nickname": nick[id],
			"revenue_pln": math.Round((a.deposits+a.goods)*100) / 100,
			"deposits":    a.cnt, "goods_pln": a.goods, "sales": a.sales})
	}
	sort.Slice(byAdmin, func(i, j int) bool {
		return byAdmin[i]["revenue_pln"].(float64) > byAdmin[j]["revenue_pln"].(float64)
	})

	// топ позиций ценника
	var goodRows []struct {
		Name  string
		Pln   float64
		Items int64
	}
	db.Model(&models.Sale{}).
		Select("name, COALESCE(SUM(total_pln),0) AS pln, COALESCE(SUM(qty),0) AS items").
		Where("created_at >= ? AND created_at < ? AND voided_at IS NULL", p.From, p.To).
		Group("name").Order("pln DESC").Limit(15).Scan(&goodRows)
	topGoods := make([]gin.H, 0, len(goodRows))
	for _, r := range goodRows {
		topGoods = append(topGoods, gin.H{"name": r.Name, "revenue_pln": r.Pln, "items": r.Items})
	}

	// потери: всё, что ушло со склада корректировкой, а не через кассу
	var loss struct {
		Units int64
		Pln   float64
		Moves int64
	}
	db.Model(&models.StockMove{}).
		Select("COALESCE(-SUM(stock_moves.delta),0) AS units, COUNT(*) AS moves, COALESCE(-SUM(stock_moves.delta * goods.price_pln),0) AS pln").
		Joins("JOIN goods ON goods.id = stock_moves.good_id").
		Where("stock_moves.created_at >= ? AND stock_moves.created_at < ? AND stock_moves.reason = ? AND stock_moves.delta < 0",
			p.From, p.To, "adjust").Scan(&loss)

	c.JSON(http.StatusOK, gin.H{
		"period": p.out(), "prev_period": prev.out(),
		"totals": cur, "prev": old,
		"delta": gin.H{
			"revenue_pln":   pctDelta(cur.Revenue, old.Revenue),
			"deposits_pln":  pctDelta(cur.DepositsPLN, old.DepositsPLN),
			"goods_pln":     pctDelta(cur.GoodsPLN, old.GoodsPLN),
			"deposits":      pctDelta(float64(cur.Deposits), float64(old.Deposits)),
			"guests":        pctDelta(float64(cur.Guests), float64(old.Guests)),
			"avg_check_pln": pctDelta(cur.AvgCheck, old.AvgCheck),
		},
		"days": days, "by_admin": byAdmin, "top_goods": topGoods,
		"losses": gin.H{"units": loss.Units, "pln": math.Round(loss.Pln*100) / 100, "moves": loss.Moves},
	})
}

// ── Разрез «Гости» ────────────────────────────────────────────────────

type guestsAgg struct {
	Unique      int64   `json:"unique"`
	Sessions    int64   `json:"sessions"`
	Hours       float64 `json:"hours"`
	AvgSessions float64 `json:"avg_sessions"`
	New         int64   `json:"new"`
	Returning   int64   `json:"returning"`
}

// activeUsers — кто играл в окне (id → минуты, сессии).
func activeUsers(p period) map[string][2]int64 {
	var rows []struct {
		UserID  string
		Minutes int64
		Cnt     int64
	}
	db.Model(&models.Session{}).
		Select("user_id, COALESCE(SUM(minutes_used),0) AS minutes, COUNT(*) AS cnt").
		Where("started_at >= ? AND started_at < ?", p.From, p.To).
		Group("user_id").Scan(&rows)
	out := make(map[string][2]int64, len(rows))
	for _, r := range rows {
		out[r.UserID] = [2]int64{r.Minutes, r.Cnt}
	}
	return out
}

// firstSessionIn — у кого первая в жизни сессия попала в окно (новые гости).
func firstSessionIn(p period) map[string]time.Time {
	var rows []struct {
		UserID  string
		FirstAt time.Time
	}
	db.Model(&models.Session{}).Select("user_id, MIN(started_at) AS first_at").
		Group("user_id").Having("MIN(started_at) >= ? AND MIN(started_at) < ?", p.From, p.To).Scan(&rows)
	out := make(map[string]time.Time, len(rows))
	for _, r := range rows {
		out[r.UserID] = r.FirstAt
	}
	return out
}

func aggGuests(p period, lostDays int) (guestsAgg, map[string][2]int64) {
	act := activeUsers(p)
	var a guestsAgg
	for _, v := range act {
		a.Sessions += v[1]
		a.Hours += float64(v[0])
	}
	a.Hours = math.Round(a.Hours/60*10) / 10
	a.Unique = int64(len(act))
	if a.Unique > 0 {
		a.AvgSessions = math.Round(float64(a.Sessions)/float64(a.Unique)*10) / 10
	}
	newbies := firstSessionIn(p)
	a.New = int64(len(newbies))

	// вернувшиеся: играли в окне, а до этого пропадали на lost_days и дольше
	var lastBefore []struct {
		UserID string
		LastAt time.Time
	}
	db.Model(&models.Session{}).Select("user_id, MAX(started_at) AS last_at").
		Where("started_at < ?", p.From).Group("user_id").Scan(&lastBefore)
	gap := time.Duration(lostDays) * 24 * time.Hour
	for _, r := range lastBefore {
		if _, played := act[r.UserID]; played && p.From.Sub(r.LastAt) >= gap {
			a.Returning++
		}
	}
	return a, act
}

// GET /admin/reports/guests — гости за период (owner).
func handleReportGuests(c *gin.Context) {
	p, prev, _, ok := periodFromQuery(c)
	if !ok {
		return
	}
	lostDays := int(settingInt64("seg_lost_days", 21))
	newDays := int(settingInt64("seg_new_days", 14))

	cur, act := aggGuests(p, lostDays)
	old, prevAct := aggGuests(prev, lostDays)

	// удержание: сколько гостей прошлого периода вернулись в этот
	var retention *float64
	if len(prevAct) > 0 {
		back := 0
		for id := range prevAct {
			if _, ok := act[id]; ok {
				back++
			}
		}
		v := math.Round(float64(back)/float64(len(prevAct))*1000) / 10
		retention = &v
	}

	// пропавшие — состояние НА КОНЕЦ периода: визиты были, но последний
	// раньше, чем lost_days до конца окна. Забаненных не считаем: они не
	// «пропали», их увели (открытый вопрос QA Б9–Б11 закрыт здесь).
	var lostRows []struct {
		UserID string
		LastAt time.Time
	}
	db.Model(&models.Session{}).Select("sessions.user_id, MAX(sessions.started_at) AS last_at").
		Joins("JOIN users ON users.id = sessions.user_id").
		Where("users.status <> ?", models.UserStatusBanned).
		Group("sessions.user_id").
		Having("MAX(sessions.started_at) < ?", p.To.Add(-time.Duration(lostDays)*24*time.Hour)).
		Scan(&lostRows)

	ids := make([]string, 0, len(act)+len(lostRows))
	for id := range act {
		ids = append(ids, id)
	}
	for _, r := range lostRows {
		ids = append(ids, r.UserID)
	}
	nick := nicknamesByID(ids)

	// топ по деньгам за период
	var moneyRows []struct {
		UserID string
		Pln    float64
	}
	db.Model(&models.Deposit{}).Select("user_id, COALESCE(SUM(amount_pln),0) AS pln").
		Where("created_at >= ? AND created_at < ?", p.From, p.To).
		Group("user_id").Order("pln DESC").Limit(10).Scan(&moneyRows)
	moneyIDs := make([]string, 0, len(moneyRows))
	for _, r := range moneyRows {
		moneyIDs = append(moneyIDs, r.UserID)
	}
	for id, n := range nicknamesByID(moneyIDs) {
		nick[id] = n
	}
	topMoney := make([]gin.H, 0, len(moneyRows))
	for _, r := range moneyRows {
		v := act[r.UserID]
		topMoney = append(topMoney, gin.H{"user_id": r.UserID, "nickname": nick[r.UserID],
			"deposited_pln": r.Pln, "sessions": v[1], "hours": math.Round(float64(v[0])/60*10) / 10})
	}

	// топ по часам за период
	type hourItem struct {
		id string
		m  int64
		s  int64
	}
	hrs := make([]hourItem, 0, len(act))
	for id, v := range act {
		hrs = append(hrs, hourItem{id, v[0], v[1]})
	}
	sort.Slice(hrs, func(i, j int) bool { return hrs[i].m > hrs[j].m })
	if len(hrs) > 10 {
		hrs = hrs[:10]
	}
	topHours := make([]gin.H, 0, len(hrs))
	for _, h := range hrs {
		topHours = append(topHours, gin.H{"user_id": h.id, "nickname": nick[h.id],
			"hours": math.Round(float64(h.m)/60*10) / 10, "sessions": h.s})
	}

	lostList := make([]gin.H, 0, len(lostRows))
	sort.Slice(lostRows, func(i, j int) bool { return lostRows[i].LastAt.After(lostRows[j].LastAt) })
	for i, r := range lostRows {
		if i >= 100 {
			break
		}
		lostList = append(lostList, gin.H{"user_id": r.UserID, "nickname": nick[r.UserID], "last_visit": r.LastAt})
	}

	c.JSON(http.StatusOK, gin.H{
		"period": p.out(), "prev_period": prev.out(),
		"totals": cur, "prev": old,
		"delta": gin.H{
			"unique":    pctDelta(float64(cur.Unique), float64(old.Unique)),
			"sessions":  pctDelta(float64(cur.Sessions), float64(old.Sessions)),
			"hours":     pctDelta(cur.Hours, old.Hours),
			"new":       pctDelta(float64(cur.New), float64(old.New)),
			"returning": pctDelta(float64(cur.Returning), float64(old.Returning)),
		},
		"retention_pct": retention,
		"thresholds":    gin.H{"new_days": newDays, "lost_days": lostDays},
		"lost":          gin.H{"count": len(lostRows), "items": lostList},
		"top_money":     topMoney, "top_hours": topHours,
	})
}

// ── Разрез «Загрузка зала» ────────────────────────────────────────────

// openMinutesPerDay — сколько минут клуб открыт в календарный день d по
// активным шаблонам смен: минуты, покрытые сменой этого дня, плюс утренний
// хвост ночной смены, начатой накануне. Пересечения смен не удваиваются.
// Смен нет вовсе — считаем круглосуточно. Чистая функция (тест).
func openMinutesPerDay(shifts []models.Shift, d time.Time) int {
	active := false
	for _, s := range shifts {
		if s.Active {
			active = true
			break
		}
	}
	if !active {
		return 1440
	}
	var covered [1440]bool
	mark := func(from, to int) {
		if from < 0 {
			from = 0
		}
		if to > 1440 {
			to = 1440
		}
		for i := from; i < to; i++ {
			covered[i] = true
		}
	}
	today, yesterday := dowMonday(d), dowMonday(d.AddDate(0, 0, -1))
	for _, s := range shifts {
		if !s.Active {
			continue
		}
		if s.DaysMask&(1<<today) != 0 {
			if s.EndMin > s.StartMin {
				mark(s.StartMin, s.EndMin)
			} else {
				mark(s.StartMin, 1440)
			}
		}
		if s.EndMin <= s.StartMin && s.DaysMask&(1<<yesterday) != 0 {
			mark(0, s.EndMin)
		}
	}
	n := 0
	for _, v := range covered {
		if v {
			n++
		}
	}
	return n
}

func openMinutesInPeriod(shifts []models.Shift, p period) int64 {
	var total int64
	for d := p.FromDay; !d.After(p.ToDay); d = d.AddDate(0, 0, 1) {
		total += int64(openMinutesPerDay(shifts, d))
	}
	return total
}

type loadAgg struct {
	Hours     float64 `json:"hours"`
	Sessions  int64   `json:"sessions"`
	AvgMin    float64 `json:"avg_minutes"`
	Occupancy float64 `json:"occupancy_pct"`
}

func aggLoad(p period, shifts []models.Shift, computers int64) loadAgg {
	var a loadAgg
	var head struct {
		Minutes int64
		Cnt     int64
	}
	db.Model(&models.Session{}).
		Select("COALESCE(SUM(minutes_used),0) AS minutes, COUNT(*) AS cnt").
		Where("started_at >= ? AND started_at < ?", p.From, p.To).Scan(&head)
	a.Sessions = head.Cnt
	a.Hours = math.Round(float64(head.Minutes)/60*10) / 10
	if head.Cnt > 0 {
		a.AvgMin = math.Round(float64(head.Minutes) / float64(head.Cnt))
	}
	if capacity := openMinutesInPeriod(shifts, p) * computers; capacity > 0 {
		a.Occupancy = math.Round(float64(head.Minutes)/float64(capacity)*1000) / 10
	}
	return a
}

// GET /admin/reports/load — загрузка зала за период (owner).
func handleReportLoad(c *gin.Context) {
	p, prev, reportHour, ok := periodFromQuery(c)
	if !ok {
		return
	}
	var shifts []models.Shift
	db.Order("sort, start_min").Find(&shifts)
	var computers int64
	db.Model(&models.Computer{}).Count(&computers)

	cur, old := aggLoad(p, shifts, computers), aggLoad(prev, shifts, computers)

	var dayRows []struct {
		D       time.Time
		Minutes int64
		Cnt     int64
	}
	db.Model(&models.Session{}).
		Select(dayExpr("started_at", reportHour)+" AS d, COALESCE(SUM(minutes_used),0) AS minutes, COUNT(*) AS cnt").
		Where("started_at >= ? AND started_at < ?", p.From, p.To).
		Group("d").Order("d").Scan(&dayRows)
	byDay := map[string][2]float64{}
	for _, r := range dayRows {
		byDay[r.D.Format("2006-01-02")] = [2]float64{math.Round(float64(r.Minutes)/60*10) / 10, float64(r.Cnt)}
	}

	var hourRows []struct {
		H       int
		Minutes int64
		Cnt     int64
	}
	db.Model(&models.Session{}).
		Select("EXTRACT(HOUR FROM started_at)::int AS h, COALESCE(SUM(minutes_used),0) AS minutes, COUNT(*) AS cnt").
		Where("started_at >= ? AND started_at < ?", p.From, p.To).
		Group("h").Order("h").Scan(&hourRows)
	byHour := map[int][2]int64{}
	peakHour, peakVal := -1, int64(-1)
	for _, r := range hourRows {
		byHour[r.H] = [2]int64{r.Minutes, r.Cnt}
		if r.Minutes > peakVal {
			peakHour, peakVal = r.H, r.Minutes
		}
	}
	hours := make([]gin.H, 0, 24)
	for h := 0; h < 24; h++ {
		v := byHour[h]
		hours = append(hours, gin.H{"hour": h, "hours": math.Round(float64(v[0])/60*10) / 10, "sessions": v[1]})
	}

	var zoneRows []struct {
		Zone    string
		Minutes int64
		Cnt     int64
		Pcs     int64
	}
	db.Model(&models.Session{}).
		Select("COALESCE(computers.zone,'—') AS zone, COALESCE(SUM(sessions.minutes_used),0) AS minutes, COUNT(*) AS cnt, COUNT(DISTINCT computers.id) AS pcs").
		Joins("JOIN computers ON computers.id = sessions.computer_id").
		Where("sessions.started_at >= ? AND sessions.started_at < ?", p.From, p.To).
		Group("zone").Order("minutes DESC").Scan(&zoneRows)
	openMin := openMinutesInPeriod(shifts, p)
	zones := make([]gin.H, 0, len(zoneRows))
	for _, r := range zoneRows {
		occ := 0.0
		if capacity := openMin * r.Pcs; capacity > 0 {
			occ = math.Round(float64(r.Minutes)/float64(capacity)*1000) / 10
		}
		zones = append(zones, gin.H{"zone": r.Zone, "hours": math.Round(float64(r.Minutes)/60*10) / 10,
			"sessions": r.Cnt, "computers": r.Pcs, "occupancy_pct": occ})
	}

	var pcRows []struct {
		Name    string
		Zone    string
		Minutes int64
		Cnt     int64
	}
	db.Model(&models.Session{}).
		Select("computers.name AS name, COALESCE(computers.zone,'—') AS zone, COALESCE(SUM(sessions.minutes_used),0) AS minutes, COUNT(*) AS cnt").
		Joins("JOIN computers ON computers.id = sessions.computer_id").
		Where("sessions.started_at >= ? AND sessions.started_at < ?", p.From, p.To).
		Group("computers.name, computers.zone").Order("minutes DESC").Scan(&pcRows)
	pcs := make([]gin.H, 0, len(pcRows))
	for _, r := range pcRows {
		occ := 0.0
		if openMin > 0 {
			occ = math.Round(float64(r.Minutes)/float64(openMin)*1000) / 10
		}
		pcs = append(pcs, gin.H{"name": r.Name, "zone": r.Zone, "sessions": r.Cnt,
			"hours": math.Round(float64(r.Minutes)/60*10) / 10, "occupancy_pct": occ})
	}

	c.JSON(http.StatusOK, gin.H{
		"period": p.out(), "prev_period": prev.out(),
		"totals": cur, "prev": old,
		"delta": gin.H{
			"hours":         pctDelta(cur.Hours, old.Hours),
			"sessions":      pctDelta(float64(cur.Sessions), float64(old.Sessions)),
			"occupancy_pct": pctDelta(cur.Occupancy, old.Occupancy),
			"avg_minutes":   pctDelta(cur.AvgMin, old.AvgMin),
		},
		"capacity": gin.H{"computers": computers, "open_minutes": openMin,
			"open_hours_per_day": math.Round(float64(openMin)/float64(p.Days)/60*10) / 10},
		"days": fillDays(p, byDay), "hours": hours, "zones": zones, "computers": pcs,
		"peak_hour": peakHour,
	})
}

// ── Разрез «Персонал и смены» ─────────────────────────────────────────

// GET /admin/reports/staff — кто работал и что принесла каждая смена (owner).
func handleReportStaff(c *gin.Context) {
	p, prev, _, ok := periodFromQuery(c)
	if !ok {
		return
	}
	var shifts []models.Shift
	db.Order("sort, start_min").Find(&shifts)

	// выручка и сессии по сменам: момент операции раскладываем по шаблонам
	type bucket struct {
		revenue  float64 // пополнения
		deposits int64
		goods    float64 // продажи товаров
		sales    int64
		sessions int64
		minutes  int64
	}
	buckets := map[string]*bucket{}
	get := func(name string) *bucket {
		if buckets[name] == nil {
			buckets[name] = &bucket{}
		}
		return buckets[name]
	}
	const outside = "вне смен"

	shiftOf := func(at time.Time) string {
		for _, s := range shifts {
			if !s.Active {
				continue
			}
			if on, _ := shiftActiveAt(s.StartMin, s.EndMin, s.DaysMask, at); on {
				return s.Name
			}
		}
		return outside
	}

	var deps []models.Deposit
	db.Where("created_at >= ? AND created_at < ?", p.From, p.To).Find(&deps)
	for _, d := range deps {
		b := get(shiftOf(d.CreatedAt))
		b.revenue += d.AmountPLN
		b.deposits++
	}

	var sales []models.Sale
	db.Where("created_at >= ? AND created_at < ? AND voided_at IS NULL", p.From, p.To).Find(&sales)
	for _, sl := range sales {
		b := get(shiftOf(sl.CreatedAt))
		b.goods += sl.TotalPLN
		b.sales++
	}

	var sess []models.Session
	db.Select("id, started_at, minutes_used").
		Where("started_at >= ? AND started_at < ?", p.From, p.To).Find(&sess)
	for _, s := range sess {
		b := get(shiftOf(s.StartedAt))
		b.sessions++
		b.minutes += int64(s.MinutesUsed)
	}

	byShift := make([]gin.H, 0, len(buckets))
	add := func(name string) {
		b := buckets[name]
		if b == nil {
			b = &bucket{}
		}
		avg := 0.0
		if b.deposits > 0 {
			avg = math.Round(b.revenue/float64(b.deposits)*100) / 100
		}
		byShift = append(byShift, gin.H{"shift": name,
			"revenue_pln":  math.Round((b.revenue+b.goods)*100) / 100,
			"deposits_pln": b.revenue, "deposits": b.deposits,
			"goods_pln": b.goods, "sales": b.sales,
			"sessions": b.sessions, "hours": math.Round(float64(b.minutes)/60*10) / 10, "avg_check_pln": avg})
	}
	for _, s := range shifts {
		if s.Active {
			add(s.Name)
		}
	}
	if buckets[outside] != nil {
		add(outside)
	}

	// по людям: смены в графике, выручка, операции
	shiftLen := map[string]int{}
	for _, s := range shifts {
		l := s.EndMin - s.StartMin
		if l <= 0 {
			l += 1440
		}
		shiftLen[s.ID.String()] = l
	}
	var asg []models.ShiftAssignment
	db.Where("date >= ? AND date <= ?", p.FromDay, p.ToDay).Find(&asg)
	type person struct {
		shifts   int64
		schedMin int64
		factMin  int64 // В3-4: факт из табеля
		factCnt  int64
		revenue  float64 // пополнения + товары
		deposits int64
		sales    int64
		actions  int64
		grants   int64
	}
	people := map[string]*person{}
	who := func(id string) *person {
		if people[id] == nil {
			people[id] = &person{}
		}
		return people[id]
	}
	for _, a := range asg {
		w := who(a.UserID.String())
		w.shifts++
		w.schedMin += int64(shiftLen[a.ShiftID.String()])
	}
	var depRows []struct {
		AdminID string
		Pln     float64
		Cnt     int64
	}
	db.Model(&models.Deposit{}).
		Select("created_by AS admin_id, COALESCE(SUM(amount_pln),0) AS pln, COUNT(*) AS cnt").
		Where("created_at >= ? AND created_at < ? AND created_by IS NOT NULL", p.From, p.To).
		Group("created_by").Scan(&depRows)
	for _, r := range depRows {
		w := who(r.AdminID)
		w.revenue, w.deposits = r.Pln, r.Cnt
	}
	depRows = nil
	db.Model(&models.Sale{}).
		Select("created_by AS admin_id, COALESCE(SUM(total_pln),0) AS pln, COUNT(*) AS cnt").
		Where("created_at >= ? AND created_at < ? AND voided_at IS NULL", p.From, p.To).
		Group("created_by").Scan(&depRows)
	for _, r := range depRows {
		w := who(r.AdminID)
		w.revenue += r.Pln
		w.sales = r.Cnt
	}
	var actRows []struct {
		AdminID string
		Cnt     int64
	}
	db.Model(&models.AdminAction{}).Select("admin_id, COUNT(*) AS cnt").
		Where("created_at >= ? AND created_at < ?", p.From, p.To).Group("admin_id").Scan(&actRows)
	for _, r := range actRows {
		who(r.AdminID).actions = r.Cnt
	}
	actRows = nil
	db.Model(&models.AdminGrant{}).Select("admin_id, COUNT(*) AS cnt").
		Where("created_at >= ? AND created_at < ?", p.From, p.To).Group("admin_id").Scan(&actRows)
	for _, r := range actRows {
		who(r.AdminID).grants = r.Cnt
	}

	// факт из табеля (В3-4): без него ставка из кадровой карточки бесполезна
	for id, v := range factByUser(p) {
		w := who(id)
		w.factMin, w.factCnt = v[0], v[1]
	}

	ids := make([]string, 0, len(people))
	for id := range people {
		ids = append(ids, id)
	}
	nick := nicknamesByID(ids)
	roles := rolesByID(ids)
	cards := map[string]models.StaffProfile{}
	var profiles []models.StaffProfile
	db.Find(&profiles)
	for _, pr := range profiles {
		cards[pr.UserID.String()] = pr
	}
	byPerson := make([]gin.H, 0, len(people))
	for id, w := range people {
		row := gin.H{
			"nickname": nick[id], "role": roles[id],
			"shifts": w.shifts, "scheduled_hours": math.Round(float64(w.schedMin)/60*10) / 10,
			"fact_hours": math.Round(float64(w.factMin)/60*10) / 10, "fact_shifts": w.factCnt,
			"revenue_pln": math.Round(w.revenue*100) / 100, "deposits": w.deposits, "sales": w.sales,
			"actions": w.actions + w.grants + w.sales,
		}
		if card, ok := cards[id]; ok {
			amount, kind := payoutFor(card.RateType, card.RateAmount, w.factMin, w.factCnt)
			if kind != "" {
				row["payout_pln"] = amount
				row["payout_kind"] = kind
			}
			if card.Position != "" {
				row["position"] = card.Position
			}
		}
		byPerson = append(byPerson, row)
	}
	sort.Slice(byPerson, func(i, j int) bool {
		return byPerson[i]["revenue_pln"].(float64) > byPerson[j]["revenue_pln"].(float64)
	})

	curMoney, oldMoney := aggMoney(p), aggMoney(prev)
	c.JSON(http.StatusOK, gin.H{
		"period": p.out(), "prev_period": prev.out(),
		"totals": gin.H{"revenue_pln": curMoney.Revenue, "deposits": curMoney.Deposits,
			"goods_pln": curMoney.GoodsPLN, "people": len(people), "assignments": len(asg)},
		"prev":      gin.H{"revenue_pln": oldMoney.Revenue, "deposits": oldMoney.Deposits, "goods_pln": oldMoney.GoodsPLN},
		"delta":     gin.H{"revenue_pln": pctDelta(curMoney.Revenue, oldMoney.Revenue)},
		"by_shift":  byShift,
		"by_person": byPerson,
	})
}

// ── Мелкие помощники ──────────────────────────────────────────────────

func nicknamesByID(ids []string) map[string]string {
	out := map[string]string{}
	if len(ids) == 0 {
		return out
	}
	var users []models.User
	db.Select("id, nickname").Where("id IN ?", ids).Find(&users)
	for i := range users {
		out[users[i].ID.String()] = users[i].Nickname
	}
	return out
}

func rolesByID(ids []string) map[string]string {
	out := map[string]string{}
	if len(ids) == 0 {
		return out
	}
	var users []models.User
	db.Select("id, role").Where("id IN ?", ids).Find(&users)
	for i := range users {
		out[users[i].ID.String()] = string(users[i].Role)
	}
	return out
}

func idsOf[T any](rows []T, key func(T) string) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, key(r))
	}
	return out
}
