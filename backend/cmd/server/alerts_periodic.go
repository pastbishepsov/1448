package main

// З22-и3 (OWNER.md, трек З; №59, З-Р28): периодические алерты — фоновый прогон раз в 15 минут, как напоминания о
// расходах (З9-и5): долг гостя больше N zł дольше суток, доля отмен у админа за 7 дней выше N %, выручка сегодня ниже
// нормы на N % (пульс, З21-и1). Все — владельцу (деньги и люди), каждый — раз за клубные сутки на свой предмет: гостя,
// админа, сутки. Пороги — «Экономика» → «Алерты»; 0 — вид выключен. Проверить сейчас, не дожидаясь фона, —
// POST /admin/alerts/check: та же логика, что у джоба.

import (
	"fmt"
	"log"
	"math"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

// Пороги по умолчанию (settingDefaults): долг — 50 zł, доля отмен — 20 %, выручка ниже нормы — на 30 %.
const (
	alertDebtDef       = 50
	alertVoidShareDef  = 20
	alertRevenueLowDef = 30
)

const (
	alertDebtAge      = 24 * time.Hour // долг висит дольше суток
	alertVoidDays     = 7              // окно доли отмен
	alertVoidMin      = 3              // меньше трёх отмен — не повод: 1 из 2 — это 50 %, но не система
	alertRevenueWeeks = 4              // норма выручки — хотя бы из четырёх прошлых таких же дней
	alertRevenueNorm  = 100.0          // к этому часу обычно меньше 100 zł — отклонение ни о чём не говорит
)

// debtsToAlert — долги для алерта (чистая функция, тест): не меньше minGrosz и висят дольше суток на момент now.
func debtsToAlert(rows []receivable, now time.Time, minGrosz int64) []receivable {
	var out []receivable
	for _, r := range rows {
		if r.total() >= minGrosz && now.Sub(r.Since) >= alertDebtAge {
			out = append(out, r)
		}
	}
	return out
}

// voidShareHit — доля отмен выше порога (чистая функция, тест): отмен не меньше alertVoidMin и voided/ops ≥ pct %.
func voidShareHit(ops, voided, pct int64) bool {
	return ops > 0 && voided >= alertVoidMin && voided*100 >= pct*ops
}

// revenueLow — выручка до этого часа ниже нормы на pct % и больше (чистая функция, тест): норма — из alertRevenueWeeks
// недель и больше и не меньше alertRevenueNorm zł.
func revenueLow(today, norm float64, weeks int, pct int64) bool {
	return weeks >= alertRevenueWeeks && norm >= alertRevenueNorm && today*100 <= norm*float64(100-pct)
}

// alertDebts — «долг больше N zł дольше суток»: раз за сутки на гостя.
func alertDebts(now time.Time, rh int, minPLN int64) int {
	var rows []receivable
	db.Raw(receivablesSQL).Scan(&rows)
	hit := debtsToAlert(rows, now, minPLN*100)
	ids := make([]string, 0, len(hit))
	for _, r := range hit {
		ids = append(ids, r.UserID.String())
	}
	nick := nicknamesByID(ids)
	n := 0
	for _, r := range hit {
		text := fmt.Sprintf("💸 %s должен клубу %s zł дольше суток — с %s (бар и кухня в долг); список — «Отчёты» → «Деньги» → "+
			"«Дебиторка»", nick[r.UserID.String()], plnRU(models.PLNFromGrosz(r.total())), r.Since.In(now.Location()).Format("02.01 15:04"))
		if alertSend("debt", alertDayKey(now, rh, "debt/"+r.UserID.String()), text) {
			n++
		}
	}
	return n
}

// alertVoidShares — «доля отмен у админа за 7 дней выше нормы»: его пополнения и продажи за 7 дней и сколько из них
// отменено; раз за сутки на админа. Владелец — не в счёт.
func alertVoidShares(now time.Time, rh int, pct int64) int {
	var rows []struct {
		UserID uuid.UUID
		Ops    int64
		Voided int64
	}
	from := now.AddDate(0, 0, -alertVoidDays)
	db.Raw(`SELECT x.author AS user_id, COUNT(*) AS ops, COUNT(*) FILTER (WHERE x.voided) AS voided FROM (
	          SELECT created_by AS author, voided_at IS NOT NULL AS voided FROM deposits WHERE created_at >= ? AND created_at < ?
	          UNION ALL SELECT created_by, voided_at IS NOT NULL FROM sales WHERE created_at >= ? AND created_at < ?) x
	        JOIN users u ON u.id = x.author WHERE u.role = ? GROUP BY x.author`,
		from, now, from, now, models.UserRoleAdmin).Scan(&rows)
	ids := make([]string, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.UserID.String())
	}
	nick := nicknamesByID(ids)
	n := 0
	for _, r := range rows {
		if !voidShareHit(r.Ops, r.Voided, pct) {
			continue
		}
		text := fmt.Sprintf("⚠ %s: за 7 дней отменено %d из %d его пополнений и продаж (%d%%) — выше порога %d%%",
			nick[r.UserID.String()], r.Voided, r.Ops, r.Voided*100/r.Ops, pct)
		if alertSend("void_share", alertDayKey(now, rh, "void_share/"+r.UserID.String()), text) {
			n++
		}
	}
	return n
}

// alertRevenueLow — «выручка ниже нормы»: пульс (З21-и1) на сейчас; раз за сутки.
func alertRevenueLow(now time.Time, rh int, pct int64) int {
	p := pulseAt(now, rh)
	rev, _ := p["revenue"].(gin.H)
	today, _ := rev["today"].(float64)
	norm, _ := rev["norm"].(float64)
	weeks, _ := p["weeks"].(int)
	if !revenueLow(today, norm, weeks, pct) {
		return 0
	}
	text := fmt.Sprintf("📉 Выручка сегодня до %s — %s zł, а обычно к этому часу %s zł: ниже нормы на %d%%; пульс — "+
		"сверху «Отчётов»", p["at"], plnRU(today), plnRU(norm), int64(math.Round((norm-today)/norm*100)))
	if alertSend("revenue_low", alertDayKey(now, rh, "revenue"), text) {
		return 1
	}
	return 0
}

// alertsCheck — один прогон периодических алертов на момент now; что ушло — по видам.
func alertsCheck(now time.Time) gin.H {
	rh := clubDayHour()
	out := gin.H{"debt": 0, "void_share": 0, "revenue_low": 0}
	if v := settingInt64("alert_debt_pln", alertDebtDef); v > 0 {
		out["debt"] = alertDebts(now, rh, v)
	}
	if v := settingInt64("alert_void_share_pct", alertVoidShareDef); v > 0 {
		out["void_share"] = alertVoidShares(now, rh, v)
	}
	if v := settingInt64("alert_revenue_low_pct", alertRevenueLowDef); v > 0 {
		out["revenue_low"] = alertRevenueLow(now, rh, v)
	}
	return out
}

// startAlertsJob — фоновый прогон раз в 15 минут, первый через минуту после старта.
func startAlertsJob() {
	go func() {
		time.Sleep(time.Minute)
		for {
			safely("alerts", func() {
				if settingInt64("alerts_on", 1) != 0 {
					r := alertsCheck(time.Now())
					if r["debt"] != 0 || r["void_share"] != 0 || r["revenue_low"] != 0 {
						log.Printf("алерты: %v", r)
					}
				}
			})
			time.Sleep(15 * time.Minute)
		}
	}()
}

// POST /admin/alerts/check — периодические алерты сейчас, не дожидаясь фона (owner, З22-и3); sent — сколько ушло по
// видам. Выключены — ничего не уходит.
func handleAlertsCheck(c *gin.Context) {
	if settingInt64("alerts_on", 1) == 0 {
		c.JSON(http.StatusOK, gin.H{"sent": gin.H{}, "reason": "off"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"sent": alertsCheck(time.Now())})
}
