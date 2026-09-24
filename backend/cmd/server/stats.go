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
	// З4-и2: окно — по стенным часам клуба, как у отчётов и шапки (makePeriod):
	// «полночь + 8 ч» в дни перевода часов — это 07:00 (25.10) или 09:00 (28.03),
	// и сводка смены, потолки и отмены жили бы в другом окне, чем отчёт «Деньги».
	p := makePeriod(day, day, reportHour, "")
	return p.From, p.To, day.Format("2006-01-02"), true
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

	// З4-и2 (З-Р4): выручка смены — та же функция, что шапка и отчёт «Деньги»
	// (aggMoney): пополнения + товары + пакеты, без отменённых и без оплат
	// кошельком (Р7: те деньги пришли депозитом); пакеты — и в разбивке по
	// способам. Раньше сводка складывала только пополнения и товары и
	// расходилась с шапкой на цену пакетов. Товар — по paid_at (Ж6).
	money := aggMoney(period{From: from, To: to})
	byMethod := gin.H{"cash": money.Cash, "card": money.Card, "blik": money.Blik}
	// Кошельковые продажи показываем отдельной строкой — как оборот, не выручку.
	var walletPLN float64
	var walletCount int64
	db.Model(&models.Sale{}).
		Select("COALESCE(SUM(total_pln),0)").
		Where("paid_at >= ? AND paid_at < ? AND voided_at IS NULL AND method = ?", from, to, "wallet").
		Scan(&walletPLN)
	db.Model(&models.Sale{}).
		Where("paid_at >= ? AND paid_at < ? AND voided_at IS NULL AND method = ?", from, to, "wallet").
		Count(&walletCount)
	saleCount := money.GoodsSales + walletCount
	// Ж6: долг, выданный в эту смену и ещё не рассчитанный — не выручка, но
	// владелец должен видеть, сколько «висит» (бар + кухня)
	var unpaidPLN float64
	var unpaidCount int64
	db.Model(&models.Sale{}).Select("COALESCE(SUM(total_pln),0)").
		Where("created_at >= ? AND created_at < ? AND paid_at IS NULL AND voided_at IS NULL", from, to).Scan(&unpaidPLN)
	db.Model(&models.Sale{}).
		Where("created_at >= ? AND created_at < ? AND paid_at IS NULL AND voided_at IS NULL", from, to).Count(&unpaidCount)
	var kUnpaid struct {
		Cnt int64
		Sum float64
	}
	db.Model(&models.KitchenOrder{}).Select("COUNT(*) AS cnt, COALESCE(SUM(total_pln),0) AS sum").
		Where("created_at >= ? AND created_at < ? AND status = ? AND paid_at IS NULL", from, to, models.KitchenDone).Scan(&kUnpaid)
	unpaidPLN += kUnpaid.Sum
	unpaidCount += kUnpaid.Cnt

	// сессии и «наиграно» — начатые в окне, как в отчётах (З4-и3, №16): идущая
	// сессия — «на сейчас» (sessionMinutes), ночная через report_hour — в сутках
	// начала. Раньше часы брались по завершённым в окне: идущие давали 0, а
	// ночная сессия попадала в сводку следующего дня. Снятая посадка — не
	// сессия (З4-и1).
	var sess struct {
		Started int64
		Minutes int64
	}
	countedSessions().
		Select("COUNT(*) AS started, COALESCE(SUM("+sessionMinutes+"),0) AS minutes").
		Where("started_at >= ? AND started_at < ?", from, to).Scan(&sess)
	sessStarted, minutes := sess.Started, sess.Minutes

	// З5-и1 (З-Р10): новые — игравшие в эту смену, чьё первое появление не
	// раньше seg_new_days до её конца, тем же определением, что отчёт «Гости».
	// Раньше — регистрации за смену: сводка и отчёт за один день расходились,
	// а гость, зарегистрированный в PWA накануне, новым не был нигде.
	newGuests := newGuestsIn(from, to)

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
		"revenue": gin.H{"total_pln": money.Revenue, "by_method": byMethod, "deposits": money.Deposits,
			"deposits_pln": money.DepositsPLN, "goods_pln": money.GoodsPLN, "sales": saleCount,
			"packs_pln": money.PacksPLN, "packs": money.Packs, // З4-и2: пакеты за живые деньги
			// оборот кошельком — уже учтён в депозитах, в total_pln не входит
			"wallet_pln": walletPLN, "wallet_sales": walletCount,
			// Ж6: выдано в долг за смену и ещё не рассчитано (бар + кухня) — не выручка
			"unpaid_pln": roundPLN(unpaidPLN), "unpaid": unpaidCount},
		"sessions":   gin.H{"started": sessStarted, "minutes": minutes},
		"new_guests": newGuests,
		"new_days":   segNewDays(), // З5-и1: окно «новых» — в подпись KPI
		"bookings":   gin.H{"created": bkCreated, "cancelled": bkCancelled},
		"incidents":  gin.H{"bans": bans, "repairs": repairs, "calls": calls},
		"closeouts":  closeoutsForDay(key), // Ж6-и4: сдачи касс за смену — ожидалось / пересчитано / расхождение
	})
}
