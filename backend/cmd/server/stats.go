package main

// Статистика владельца (спринт А5, ADMIN.md): выручка по дням из deposits,
// загрузка по часам суток и топ гостей из sessions. Только чтение.

import (
	"net/http"
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
