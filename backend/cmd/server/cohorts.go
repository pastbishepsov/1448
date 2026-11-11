package main

// З21-и2а (OWNER.md, трек З; №47, З-Р28): когорты и LTV — гости по месяцу первого визита: сколько их, сколько вернулись
// через 1, 2 и 3 месяца и сколько живых денег гость такой когорты принёс клубу в среднем (LTV: пополнения + товары +
// пакеты — те же каналы и фильтры, что у выручки, только его). Ответ на вопрос из ревью — работает ли геймификация:
// возвращаются ли новые гости. Экран — З21-и2б.

import (
	"math"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	cohortMonths = 12 // когорт в отчёте: этот месяц и 11 прошлых
	cohortBack   = 3  // «вернулись через 1, 2, 3 месяца»
)

// monthAdd — «ГГГГ-ММ» на k месяцев позже (k < 0 — раньше); кривой месяц — "". Чистая функция (тест).
func monthAdd(m string, k int) string {
	t, err := time.Parse("2006-01", m)
	if err != nil {
		return ""
	}
	return t.AddDate(0, k, 0).Format("2006-01")
}

// cohortState — месяц m относительно текущего cur: "done" — прошёл, "going" — идёт (число ещё растёт), "future" — не
// начался (числа нет).
func cohortState(m, cur string) string {
	switch {
	case m < cur:
		return "done"
	case m == cur:
		return "going"
	}
	return "future"
}

// cohortGuest — гость в когортах: месяц первого визита, месяцы визитов, живые деньги за всё время.
type cohortGuest struct {
	First  string
	Months map[string]bool
	Money  float64
}

// cohortsOf — строки когорт по месяцам months (старый первым) при текущем месяце cur. Чистая функция (тест): размер,
// вернулись в M+1…M+3 (число и доля; месяц не начался — числа нет), выручка когорты за всё время и LTV — она же на
// гостя.
func cohortsOf(guests map[string]*cohortGuest, months []string, cur string) []gin.H {
	out := make([]gin.H, 0, len(months))
	for _, m := range months {
		size, money := 0, 0.0
		var back [cohortBack]int
		for _, g := range guests {
			if g.First != m {
				continue
			}
			size++
			money += g.Money
			for k := 1; k <= cohortBack; k++ {
				if g.Months[monthAdd(m, k)] {
					back[k-1]++
				}
			}
		}
		row := gin.H{"month": m, "size": size, "revenue_pln": round2(money), "ltv_pln": nil}
		if size > 0 {
			row["ltv_pln"] = round2(money / float64(size))
		}
		bs := make([]gin.H, cohortBack)
		for k := 1; k <= cohortBack; k++ {
			mk := monthAdd(m, k)
			b := gin.H{"month": mk, "state": cohortState(mk, cur), "n": nil, "pct": nil}
			if b["state"] != "future" {
				b["n"] = back[k-1]
				if size > 0 {
					b["pct"] = math.Round(float64(back[k-1])/float64(size)*1000) / 10
				}
			}
			bs[k-1] = b
		}
		row["back"] = bs
		out = append(out, row)
	}
	return out
}

// cohortsAt — когорты на момент now: месяц — клубных суток (первая сессия ночью до границы — в месяце вчерашних суток).
// Гость — роль player, визит — засчитанная сессия (как «Гости» и «новые»); деньги — его пополнения, товары и пакеты
// живыми деньгами без отменённых, за всё время до now.
func cohortsAt(now time.Time, reportHour int) gin.H {
	cur := clubDayOf(now, reportHour).Format("2006-01")
	months := make([]string, cohortMonths)
	for i := range months {
		months[i] = monthAdd(cur, i-(cohortMonths-1))
	}
	month := func(col string) string { return "to_char(" + dayExpr(col, reportHour) + ", 'YYYY-MM')" }

	guests := map[string]*cohortGuest{}
	var firsts []struct{ UserID, M string }
	guestSessions().Where("sessions.started_at < ?", now).
		Select("CAST(sessions.user_id AS text) AS user_id, " + month("MIN(sessions.started_at)") + " AS m").
		Group("sessions.user_id").Scan(&firsts)
	for _, f := range firsts {
		if f.M >= months[0] {
			guests[f.UserID] = &cohortGuest{First: f.M, Months: map[string]bool{}}
		}
	}
	if len(guests) > 0 {
		t0, _ := time.ParseInLocation("2006-01", months[0], now.Location())
		var acts []struct{ UserID, M string }
		guestSessions().Where("sessions.started_at >= ? AND sessions.started_at < ?", makePeriod(t0, t0, reportHour, "").From, now).
			Select("DISTINCT CAST(sessions.user_id AS text) AS user_id, " + month("sessions.started_at") + " AS m").Scan(&acts)
		for _, a := range acts {
			if g := guests[a.UserID]; g != nil {
				g.Months[a.M] = true
			}
		}
		for _, s := range moneySources {
			var rows []struct {
				UserID string
				Pln    float64
			}
			db.Table(s.table).Where(s.at+" < ? AND "+s.filter+" AND user_id IS NOT NULL", now).
				Select("CAST(user_id AS text) AS user_id, COALESCE(SUM(" + s.pln + "), 0) AS pln").Group("user_id").Scan(&rows)
			for _, r := range rows {
				if g := guests[r.UserID]; g != nil {
					g.Money += r.Pln
				}
			}
		}
	}
	return gin.H{"month": cur, "as_of": now, "cohorts": cohortsOf(guests, months, cur)}
}

// GET /admin/reports/cohorts — когорты по месяцу первого визита и LTV (owner, З21-и2а).
func handleReportCohorts(c *gin.Context) {
	c.JSON(http.StatusOK, cohortsAt(time.Now().Truncate(time.Microsecond), clubDayHour()))
}
