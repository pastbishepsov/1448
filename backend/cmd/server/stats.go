package main

// Сводка смены за конкретный день (спринт Б10-и1): «как прошёл день» одним
// экраном. Отчёты за произвольный период — в reports.go (спринт В1).

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

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
	var depPLN float64
	var depCount int64
	for _, r := range depRows {
		byMethod[r.Method] = r.Pln
		depPLN += r.Pln
		depCount += r.Cnt
	}

	// продажи товаров (В2) — вторая половина выручки смены.
	// Р7: продажи «кошельком» в кассу НЕ идут — эти деньги уже посчитаны
	// депозитом, которым гость пополнял кошелёк. Считать их второй раз значит
	// показать владельцу выручку больше, чем реально в кассе (ревью 26.08:
	// /admin/sales и aggMoney это правило соблюдают, сводка смены — нет).
	depRows = nil
	db.Model(&models.Sale{}).
		Select("method, COALESCE(SUM(total_pln),0) AS pln, COUNT(*) AS cnt").
		Where("created_at >= ? AND created_at < ? AND voided_at IS NULL AND method <> ?", from, to, "wallet").
		Group("method").Scan(&depRows)
	var goodsPLN float64
	var saleCount int64
	for _, r := range depRows {
		if v, ok := byMethod[r.Method].(float64); ok {
			byMethod[r.Method] = v + r.Pln
		}
		goodsPLN += r.Pln
		saleCount += r.Cnt
	}
	// Кошельковые продажи показываем отдельной строкой — как оборот, не выручку.
	var walletPLN float64
	var walletCount int64
	db.Model(&models.Sale{}).
		Select("COALESCE(SUM(total_pln),0)").
		Where("created_at >= ? AND created_at < ? AND voided_at IS NULL AND method = ?", from, to, "wallet").
		Scan(&walletPLN)
	db.Model(&models.Sale{}).
		Where("created_at >= ? AND created_at < ? AND voided_at IS NULL AND method = ?", from, to, "wallet").
		Count(&walletCount)
	saleCount += walletCount
	totalPLN := depPLN + goodsPLN

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

	// Е3-и3: ФАКТ против графика. График — план, табель — что было на самом
	// деле; сводка смены обязана показывать оба и называть расхождение. Пока
	// смену открывала кнопка, «нет записи» означало «забыл нажать»; с Р8 вход
	// открывает её сам, и пустая строка стала настоящим сигналом: человека на
	// смене не было, а зал работал.
	var entries []models.WorkEntry
	db.Where("date = ?", key).Order("started_at").Find(&entries)
	byUser := map[uuid.UUID]bool{}
	workedOut := make([]gin.H, 0, len(entries))
	offSchedule := make([]string, 0)
	for i := range entries {
		e := entries[i]
		byUser[e.UserID] = true
		var u models.User
		db.Select("nickname").First(&u, "id = ?", e.UserID)
		row := gin.H{
			"nickname": u.Nickname, "started_at": e.StartedAt, "ended_at": e.EndedAt,
			"minutes": e.Minutes, "open": e.EndedAt == nil,
			"in_schedule": e.ShiftID != nil, "auto_closed": e.AutoClosed,
		}
		if e.EndedAt == nil {
			row["minutes"] = workMinutes(e.StartedAt, time.Now())
		}
		workedOut = append(workedOut, row)
		if e.ShiftID == nil {
			offSchedule = append(offSchedule, u.Nickname)
		}
	}
	// Обратное расхождение: в графике стоял, а табеля нет. Это не «забыл
	// нажать» — это «не пришёл», и владелец должен увидеть именно так.
	noShow := make([]string, 0)
	for _, a := range worked {
		if a.User != nil && !byUser[a.User.ID] {
			noShow = append(noShow, nicknameOf(a.User))
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"date": key, "from": from, "to": to, "report_hour": reportHour,
		"staff":  staffOut,
		"worked": workedOut,
		"mismatch": gin.H{
			"off_schedule": offSchedule, // работал, а смены в графике нет
			"no_show":      noShow,      // стоял в графике, а табеля нет
		},
		"revenue": gin.H{"total_pln": totalPLN, "by_method": byMethod, "deposits": depCount,
			"deposits_pln": depPLN, "goods_pln": goodsPLN, "sales": saleCount,
			// оборот кошельком — уже учтён в депозитах, в total_pln не входит
			"wallet_pln": walletPLN, "wallet_sales": walletCount},
		"sessions":   gin.H{"started": sessStarted, "minutes": minutes},
		"new_guests": newGuests,
		"bookings":   gin.H{"created": bkCreated, "cancelled": bkCancelled},
		"incidents":  gin.H{"bans": bans, "repairs": repairs, "calls": calls},
	})
}
