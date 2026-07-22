package main

// Статистика владельца (спринт А5, ADMIN.md): выручка по дням из deposits,
// загрузка по часам суток и топ гостей из sessions. Только чтение.

import (
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

func statsDays(c *gin.Context) int {
	days, err := strconv.Atoi(c.DefaultQuery("days", "14"))
	if err != nil || days < 1 || days > 90 {
		days = 14
	}
	return days
}

// ── Б10-и1: сводка смены ─────────────────────────────────────────────

// shiftWindow — чистая функция (тест в stats_test.go): окно «клубных суток»
// смены date (YYYY-MM-DD) при границе в reportHour часов. Пустая date —
// текущая смена относительно now (до границы идёт ещё вчерашняя).
func shiftWindow(dateStr string, reportHour int, now time.Time) (from, to time.Time, key string, ok bool) {
	loc := now.Location()
	var day time.Time
	if dateStr == "" {
		day = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
		if now.Hour() < reportHour {
			day = day.AddDate(0, 0, -1)
		}
	} else {
		d, err := time.ParseInLocation("2006-01-02", dateStr, loc)
		if err != nil {
			return from, to, "", false
		}
		day = d
	}
	from = day.Add(time.Duration(reportHour) * time.Hour)
	return from, from.Add(24 * time.Hour), day.Format("2006-01-02"), true
}

// GET /admin/stats/shift?date=YYYY-MM-DD — сводка смены (owner, Б10-и1).
// «Деньги» отвечают владельцу, как прошёл день: выручка по методам, сессии
// и часы, новые гости, брони, инциденты. Всё в одном месте — решение
// 2026-07-21, Telegram-доставка в бэклоге.
func handleAdminStatsShift(c *gin.Context) {
	reportHour := int(settingInt64("report_hour", 8))
	from, to, key, ok := shiftWindow(c.Query("date"), reportHour, time.Now())
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"code": "bad_date", "message": "date — в формате YYYY-MM-DD"})
		return
	}

	// выручка по методам оплаты
	var depRows []struct {
		Method string
		Pln    float64
		Cnt    int64
	}
	db.Model(&models.Deposit{}).
		Select("method, COALESCE(SUM(amount_pln),0) AS pln, COUNT(*) AS cnt").
		Where("created_at >= ? AND created_at < ?", from, to).
		Group("method").Scan(&depRows)
	byMethod := gin.H{"cash": 0.0, "card": 0.0, "blik": 0.0}
	var totalPLN float64
	var depCount int64
	for _, r := range depRows {
		byMethod[r.Method] = r.Pln
		totalPLN += r.Pln
		depCount += r.Cnt
	}

	// сессии: начатые в окне; часы — по завершённым в окне (начисления
	// привязаны к завершению, активные ещё не отыграли своё)
	var sessStarted int64
	db.Model(&models.Session{}).
		Where("started_at >= ? AND started_at < ?", from, to).Count(&sessStarted)
	var minutes int64
	db.Model(&models.Session{}).
		Select("COALESCE(SUM(minutes_used),0)").
		Where("status = ? AND ended_at >= ? AND ended_at < ?",
			models.SessionStatusCompleted, from, to).
		Scan(&minutes)

	var newGuests int64
	db.Model(&models.User{}).
		Where("role = ? AND registered_at >= ? AND registered_at < ?",
			models.UserRolePlayer, from, to).Count(&newGuests)

	var bkCreated, bkCancelled int64
	db.Model(&models.Booking{}).
		Where("created_at >= ? AND created_at < ?", from, to).Count(&bkCreated)
	db.Model(&models.Booking{}).
		Where("status = ? AND updated_at >= ? AND updated_at < ?",
			models.BookingStatusCancelled, from, to).Count(&bkCancelled)

	var bans, repairs int64
	db.Model(&models.AdminAction{}).
		Where("action = ? AND created_at >= ? AND created_at < ?", "ban", from, to).Count(&bans)
	db.Model(&models.AdminAction{}).
		Where("action = ? AND created_at >= ? AND created_at < ?", "pc_maintenance", from, to).Count(&repairs)
	var calls int64
	db.Model(&models.ChatMessage{}).
		Where("kind = ? AND created_at >= ? AND created_at < ?", models.ChatKindCall, from, to).Count(&calls)

	// кто работал (Б11-и3): график на день смены (дата — строкой, колонка date)
	var worked []models.ShiftAssignment
	db.Preload("User").Preload("Shift").
		Where("date = ?", key).Order("created_at").Find(&worked)
	staffOut := make([]gin.H, 0, len(worked))
	for _, a := range worked {
		sn := ""
		if a.Shift != nil {
			sn = a.Shift.Name
		}
		staffOut = append(staffOut, gin.H{"nickname": nicknameOf(a.User), "shift": sn})
	}

	c.JSON(http.StatusOK, gin.H{
		"date": key, "from": from, "to": to, "report_hour": reportHour,
		"staff":      staffOut,
		"revenue":    gin.H{"total_pln": totalPLN, "by_method": byMethod, "deposits": depCount},
		"sessions":   gin.H{"started": sessStarted, "minutes": minutes},
		"new_guests": newGuests,
		"bookings":   gin.H{"created": bkCreated, "cancelled": bkCancelled},
		"incidents":  gin.H{"bans": bans, "repairs": repairs, "calls": calls},
	})
}

// ── Б10-и2: сегменты гостей ──────────────────────────────────────────

// classifySegment — чистая классификация гостя (тест в stats_test.go):
// new — первый визит (или регистрация, если сессий нет) в окне newDays;
// returned — визит в окне newDays после паузы ≥ lostDays (prevOut —
// последний визит СТАРШЕ окна новых); lost — активности нет ≥ lostDays;
// остальное — regular (в списки не попадает).
func classifySegment(first, last, prevOut *time.Time, registered, now time.Time, newDays, lostDays int) string {
	newEdge := now.AddDate(0, 0, -newDays)
	lostEdge := now.AddDate(0, 0, -lostDays)
	firstV, lastV := registered, registered
	if first != nil {
		firstV = *first
	}
	if last != nil {
		lastV = *last
	}
	switch {
	case !firstV.Before(newEdge):
		return "new"
	case last != nil && !lastV.Before(newEdge) && prevOut != nil && !prevOut.After(lostEdge):
		return "returned"
	case lastV.Before(lostEdge):
		return "lost"
	}
	return "regular"
}

// GET /admin/stats/segments — сегменты гостей в «Деньгах» (owner, Б10-и2):
// новые / вернувшиеся / пропавшие; пороги — настройки owner (seg_new_days,
// seg_lost_days). Основа будущих win-back-механик (RESEARCH §3).
func handleAdminStatsSegments(c *gin.Context) {
	newDays := int(settingInt64("seg_new_days", 14))
	lostDays := int(settingInt64("seg_lost_days", 21))
	now := time.Now()

	type row struct {
		ID         string
		Nickname   string
		Registered time.Time
		Visits     int64
		FirstV     *time.Time
		LastV      *time.Time
		PrevOut    *time.Time
		Dep        float64
	}
	var rows []row
	db.Raw(`SELECT u.id, u.nickname, u.registered_at AS registered,
	        COUNT(s.id) AS visits,
	        MIN(s.started_at) AS first_v, MAX(s.started_at) AS last_v,
	        MAX(s.started_at) FILTER (WHERE s.started_at < ?) AS prev_out,
	        COALESCE(d.pln, 0) AS dep
	 FROM users u
	 LEFT JOIN sessions s ON s.user_id = u.id
	 LEFT JOIN (SELECT user_id, SUM(amount_pln) AS pln FROM deposits GROUP BY user_id) d
	        ON d.user_id = u.id
	 WHERE u.role = 'player'
	 GROUP BY u.id, u.nickname, u.registered_at, d.pln`,
		now.AddDate(0, 0, -newDays)).Scan(&rows)

	type item struct {
		out  gin.H
		last time.Time
	}
	segItems := map[string][]item{}
	counts := gin.H{"new": 0, "returned": 0, "lost": 0, "regular": 0}
	for _, r := range rows {
		seg := classifySegment(r.FirstV, r.LastV, r.PrevOut, r.Registered, now, newDays, lostDays)
		counts[seg] = counts[seg].(int) + 1
		if seg == "regular" {
			continue
		}
		lastAct := r.Registered
		if r.LastV != nil {
			lastAct = *r.LastV
		}
		segItems[seg] = append(segItems[seg], item{gin.H{
			"user_id": r.ID, "nickname": r.Nickname, "visits": r.Visits,
			"last_visit": lastAct, "deposited_pln": r.Dep,
		}, lastAct})
	}
	segments := gin.H{}
	for _, key := range []string{"new", "returned", "lost"} {
		items := segItems[key]
		sort.Slice(items, func(i, j int) bool { return items[i].last.After(items[j].last) })
		if len(items) > 30 {
			items = items[:30]
		}
		out := make([]gin.H, 0, len(items))
		for _, it := range items {
			out = append(out, it.out)
		}
		segments[key] = out
	}
	c.JSON(http.StatusOK, gin.H{
		"thresholds": gin.H{"new_days": newDays, "lost_days": lostDays},
		"totals":     counts, "players": len(rows),
		"segments": segments,
	})
}

// GET /admin/stats/revenue?days=14 — выручка по дням + сводка (owner).
func handleAdminStatsRevenue(c *gin.Context) {
	days := statsDays(c)
	now := time.Now()
	from := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, -(days - 1))

	var rows []struct {
		D   time.Time
		Pln float64
		Cnt int64
	}
	db.Model(&models.Deposit{}).
		Select("DATE(created_at) AS d, COALESCE(SUM(amount_pln),0) AS pln, COUNT(*) AS cnt").
		Where("created_at >= ?", from).
		Group("d").Order("d").Scan(&rows)

	type dayAgg struct {
		pln float64
		cnt int64
	}
	byDay := map[string]dayAgg{}
	for _, r := range rows {
		byDay[r.D.Format("2006-01-02")] = dayAgg{r.Pln, r.Cnt}
	}

	out := make([]gin.H, 0, days)
	var total, today, week float64
	todayKey := now.Format("2006-01-02")
	for i := 0; i < days; i++ {
		d := from.AddDate(0, 0, i)
		key := d.Format("2006-01-02")
		v := byDay[key]
		out = append(out, gin.H{"date": key, "pln": v.pln, "count": v.cnt})
		total += v.pln
		if key == todayKey {
			today = v.pln
		}
		if i >= days-7 {
			week += v.pln
		}
	}
	c.JSON(http.StatusOK, gin.H{"days": out, "total_pln": total, "today_pln": today, "week_pln": week})
}

// GET /admin/stats/load?days=14 — загрузка по часам суток + топ гостей (owner).
func handleAdminStatsLoad(c *gin.Context) {
	days := statsDays(c)
	from := time.Now().AddDate(0, 0, -days)

	var hourRows []struct {
		H       int
		Minutes int64
		Cnt     int64
	}
	db.Model(&models.Session{}).
		Select("EXTRACT(HOUR FROM started_at)::int AS h, COALESCE(SUM(minutes_used),0) AS minutes, COUNT(*) AS cnt").
		Where("started_at >= ?", from).
		Group("h").Order("h").Scan(&hourRows)
	byHour := map[int][2]int64{}
	for _, r := range hourRows {
		byHour[r.H] = [2]int64{r.Minutes, r.Cnt}
	}
	hoursOut := make([]gin.H, 0, 24)
	for h := 0; h < 24; h++ {
		v := byHour[h]
		hoursOut = append(hoursOut, gin.H{"hour": h, "minutes": v[0], "sessions": v[1]})
	}

	var top []struct {
		UserID  string
		Minutes int64
	}
	db.Model(&models.Session{}).
		Select("user_id, COALESCE(SUM(minutes_used),0) AS minutes").
		Where("started_at >= ?", from).
		Group("user_id").Order("minutes DESC").Limit(10).Scan(&top)

	nick := map[string]string{}
	dep := map[string]float64{}
	if len(top) > 0 {
		ids := make([]string, 0, len(top))
		for _, t := range top {
			ids = append(ids, t.UserID)
		}
		var users []models.User
		db.Where("id IN ?", ids).Find(&users)
		for _, u := range users {
			nick[u.ID.String()] = u.Nickname
		}
		var depRows []struct {
			UserID string
			Pln    float64
		}
		db.Model(&models.Deposit{}).
			Select("user_id, COALESCE(SUM(amount_pln),0) AS pln").
			Where("user_id IN ? AND created_at >= ?", ids, from).
			Group("user_id").Scan(&depRows)
		for _, r := range depRows {
			dep[r.UserID] = r.Pln
		}
	}
	topOut := make([]gin.H, 0, len(top))
	for _, t := range top {
		topOut = append(topOut, gin.H{
			"user_id": t.UserID, "nickname": nick[t.UserID],
			"minutes": t.Minutes, "deposited_pln": dep[t.UserID],
		})
	}
	c.JSON(http.StatusOK, gin.H{"hours": hoursOut, "top": topOut, "days": days})
}
