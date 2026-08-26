package main

// Табель: фактически отработанное время (спринт В3, этап 4; миграция 026).
// График (Б11) — это план: кто на какой смене стоит. Здесь факт: сотрудник
// отмечает приход и уход сам, владелец может исправить. Без факта ставка из
// кадровой карточки бесполезна — зарплату не из чего считать.
//
// Дата записи — клубные сутки (день начала смены), как и везде в отчётах:
// ночная смена целиком принадлежит дню, в котором началась.

import (
	"fmt"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

// workMinutes — длительность записи в минутах. Чистая функция (тест).
// Отрицательная и нулевая длительность превращается в ноль: часы задним
// числом придумывать нельзя, пусть владелец увидит ноль и поправит.
func workMinutes(start, end time.Time) int {
	m := int(end.Sub(start).Minutes())
	if m < 0 {
		return 0
	}
	return m
}

// shiftEndAt — момент окончания смены sh, начавшейся в день day.
// Ночная (end <= start) заканчивается на следующие календарные сутки.
// Чистая функция (тест).
func shiftEndAt(day time.Time, startMin, endMin int) time.Time {
	base := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, day.Location())
	end := base.Add(time.Duration(endMin) * time.Minute)
	if endMin <= startMin {
		end = end.AddDate(0, 0, 1)
	}
	return end
}

// parseHM — «20:30» → 1230 минут от полуночи. Чистая функция (тест).
func parseHM(s string) (int, bool) {
	parts := strings.Split(strings.TrimSpace(s), ":")
	if len(parts) != 2 {
		return 0, false
	}
	h, err1 := strconv.Atoi(parts[0])
	m, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil || h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, false
	}
	return h*60 + m, true
}

// payoutFor — сколько причитается за период. Чистая функция (тест).
// Оклад НЕ делим пропорционально периоду: это была бы выдумка — отдаём сумму
// как есть и подписываем «оклад», чтобы владелец не принял её за расчёт.
func payoutFor(rateType string, rate float64, factMinutes int64, factShifts int64) (float64, string) {
	switch rateType {
	case "hour":
		return math.Round(rate*float64(factMinutes)/60*100) / 100, "hour"
	case "shift":
		return math.Round(rate*float64(factShifts)*100) / 100, "shift"
	case "month":
		return math.Round(rate*100) / 100, "month"
	}
	return 0, ""
}

func activeShifts() []models.Shift {
	var shifts []models.Shift
	db.Where("active = ?", true).Order("sort, start_min").Find(&shifts)
	return shifts
}

// shiftNow — какая смена идёт в момент t и к какому дню она относится.
func shiftNow(t time.Time) (*models.Shift, time.Time) {
	for _, s := range activeShifts() {
		if on, day := shiftActiveAt(s.StartMin, s.EndMin, s.DaysMask, t); on {
			sh := s
			return &sh, day
		}
	}
	return nil, time.Time{}
}

func workOut(w *models.WorkEntry, shiftName string) gin.H {
	row := gin.H{
		"id": w.ID, "date": w.Date.Format("2006-01-02"),
		"started_at": w.StartedAt, "minutes": w.Minutes,
		"auto_closed": w.AutoClosed, "note": w.Note, "shift": shiftName,
		"open": w.EndedAt == nil,
	}
	if w.EndedAt != nil {
		row["ended_at"] = w.EndedAt
	}
	return row
}

// openEntryFor — незакрытая запись сотрудника, если есть.
func openEntryFor(userID uuid.UUID) *models.WorkEntry {
	var w models.WorkEntry
	if err := db.Where("user_id = ? AND ended_at IS NULL", userID).First(&w).Error; err != nil {
		return nil
	}
	return &w
}

// closeForgotten — закрыть запись, в которой человек не отметил уход.
// Часы берём по расписанию смены; если смена неизвестна — ноль. Пометка
// auto_closed поднимет запись в табеле, чтобы владелец её поправил.
func closeForgotten(w *models.WorkEntry) {
	end := w.StartedAt
	if w.ShiftID != nil {
		var sh models.Shift
		if db.First(&sh, "id = ?", *w.ShiftID).Error == nil {
			end = shiftEndAt(w.Date, sh.StartMin, sh.EndMin)
		}
	}
	db.Model(&models.WorkEntry{}).Where("id = ?", w.ID).Updates(map[string]any{
		"ended_at": end, "minutes": workMinutes(w.StartedAt, end),
		"auto_closed": true, "note": "не отметил уход", "updated_at": time.Now(),
	})
}

// POST /admin/work/start — «пришёл» (staff, за себя).
func handleWorkStart(c *gin.Context) {
	userID, err := uuid.Parse(c.GetString("user_id"))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": "bad_token", "message": "Не разобрал токен"})
		return
	}
	club, ok := defaultClub()
	if !ok {
		c.JSON(http.StatusConflict, gin.H{"code": "no_club", "message": "Клуб не найден"})
		return
	}
	now := time.Now()
	reportHour := int(settingInt64("report_hour", 8))
	sh, day := shiftNow(now)
	if sh == nil { // пришёл вне смены — тоже фиксируем, день по клубным суткам
		day = clubDayOf(now, reportHour)
	}

	autoClosed := false
	if open := openEntryFor(userID); open != nil {
		if open.Date.Format("2006-01-02") == day.Format("2006-01-02") {
			c.JSON(http.StatusConflict, gin.H{"code": "already_open",
				"message": "Ты уже на смене — сначала отметь уход"})
			return
		}
		closeForgotten(open) // прошлая смена осталась незакрытой
		autoClosed = true
	}

	entry := models.WorkEntry{
		ClubID: club.ID, UserID: userID, Date: day, StartedAt: now, CreatedBy: &userID,
	}
	if sh != nil {
		entry.ShiftID = &sh.ID
	}
	if err := db.Create(&entry).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "db_error", "message": err.Error()})
		return
	}
	name := ""
	if sh != nil {
		name = sh.Name
	}
	logAdminAction(c, "work_start", nil, "пришёл"+shiftSuffix(name))
	c.JSON(http.StatusCreated, gin.H{"entry": workOut(&entry, name), "auto_closed_previous": autoClosed})
}

func shiftSuffix(name string) string {
	if name == "" {
		return " (вне смены)"
	}
	return " · " + name
}

// POST /admin/work/stop — «ушёл» (staff, за себя).
func handleWorkStop(c *gin.Context) {
	userID, err := uuid.Parse(c.GetString("user_id"))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": "bad_token", "message": "Не разобрал токен"})
		return
	}
	open := openEntryFor(userID)
	if open == nil {
		c.JSON(http.StatusConflict, gin.H{"code": "not_open", "message": "Смена не открыта — сначала отметь приход"})
		return
	}
	now := time.Now()
	mins := workMinutes(open.StartedAt, now)
	if err := db.Model(&models.WorkEntry{}).Where("id = ?", open.ID).Updates(map[string]any{
		"ended_at": now, "minutes": mins, "edited_by": userID, "updated_at": now,
	}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "db_error", "message": err.Error()})
		return
	}
	logAdminAction(c, "work_stop", nil, fmt.Sprintf("ушёл · %s", hoursRU(int64(mins))))
	open.EndedAt, open.Minutes = &now, mins
	c.JSON(http.StatusOK, gin.H{"entry": workOut(open, "")})
}

func hoursRU(minutes int64) string {
	return fmt.Sprintf("%d ч %02d м", minutes/60, minutes%60)
}

// GET /admin/work/me — моё состояние: на смене или нет (staff).
func handleWorkMe(c *gin.Context) {
	userID, _ := uuid.Parse(c.GetString("user_id"))
	reportHour := int(settingInt64("report_hour", 8))
	day := clubDayOf(time.Now(), reportHour)

	out := gin.H{"on_shift": false, "date": day.Format("2006-01-02")}
	if open := openEntryFor(userID); open != nil {
		name := ""
		if open.ShiftID != nil {
			var sh models.Shift
			if db.First(&sh, "id = ?", *open.ShiftID).Error == nil {
				name = sh.Name
			}
		}
		out["on_shift"] = true
		out["entry"] = workOut(open, name)
		out["minutes_so_far"] = workMinutes(open.StartedAt, time.Now())
	}
	var todayMin int64
	db.Model(&models.WorkEntry{}).Select("COALESCE(SUM(minutes),0)").
		Where("user_id = ? AND date = ?", userID, day.Format("2006-01-02")).Scan(&todayMin)
	out["minutes_today"] = todayMin
	c.JSON(http.StatusOK, out)
}

// ── Табель владельца ──────────────────────────────────────────────────

// factByUser — факт за период: минуты и число смен по каждому сотруднику.
func factByUser(p period) map[string][2]int64 {
	var rows []struct {
		UserID string
		Mins   int64
		Cnt    int64
	}
	db.Model(&models.WorkEntry{}).
		Select("user_id, COALESCE(SUM(minutes),0) AS mins, COUNT(*) AS cnt").
		Where("date >= ? AND date <= ?", p.FromDay, p.ToDay).
		Group("user_id").Scan(&rows)
	out := make(map[string][2]int64, len(rows))
	for _, r := range rows {
		out[r.UserID] = [2]int64{r.Mins, r.Cnt}
	}
	return out
}

// GET /admin/work — табель за период (owner): записи и сводка по людям.
func handleWorkList(c *gin.Context) {
	p, _, _, ok := periodFromQuery(c)
	if !ok {
		return
	}
	var entries []models.WorkEntry
	db.Where("date >= ? AND date <= ?", p.FromDay, p.ToDay).
		Order("date DESC, started_at DESC").Limit(500).Find(&entries)

	shiftNames := map[string]string{}
	for _, s := range activeShifts() {
		shiftNames[s.ID.String()] = s.Name
	}
	ids := make([]string, 0, len(entries))
	for _, w := range entries {
		ids = append(ids, w.UserID.String())
	}
	nick := nicknamesByID(ids)

	items := make([]gin.H, 0, len(entries))
	openCnt, autoCnt := 0, 0
	for i := range entries {
		w := entries[i]
		name := ""
		if w.ShiftID != nil {
			name = shiftNames[w.ShiftID.String()]
		}
		row := workOut(&w, name)
		row["nickname"] = nick[w.UserID.String()]
		row["user_id"] = w.UserID
		items = append(items, row)
		if w.EndedAt == nil {
			openCnt++
		}
		if w.AutoClosed {
			autoCnt++
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"period": p.out(), "count": len(items), "entries": items,
		"open": openCnt, "auto_closed": autoCnt,
	})
}

type workRequest struct {
	Nickname string `json:"nickname"`
	Date     string `json:"date"`
	Start    string `json:"start"`
	End      string `json:"end"`
	Note     string `json:"note"`
}

var workErrors = map[string]string{
	"bad_date":  "Дата — в формате ГГГГ-ММ-ДД",
	"bad_time":  "Время — в формате ЧЧ:ММ",
	"need_time": "Нужны начало и конец",
	"too_long":  "Смена длиннее суток — проверь время",
}

// composeWorkTimes — из даты и «ЧЧ:ММ» собрать моменты начала и конца.
// Конец не позже начала = смена через полночь. Чистая функция (тест).
func composeWorkTimes(day time.Time, startMin, endMin int) (time.Time, time.Time) {
	base := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, day.Location())
	start := base.Add(time.Duration(startMin) * time.Minute)
	end := base.Add(time.Duration(endMin) * time.Minute)
	if endMin <= startMin {
		end = end.AddDate(0, 0, 1)
	}
	return start, end
}

// POST /admin/work — добавить запись табеля руками (owner).
func handleWorkCreate(c *gin.Context) {
	var req workRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_request", "message": "Не разобрал запись"})
		return
	}
	var user models.User
	if err := db.First(&user, "nickname = ?", strings.TrimSpace(req.Nickname)).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "user_not_found", "message": "Сотрудник с таким ником не найден"})
		return
	}
	day, ok := parseDateOpt(req.Date)
	if !ok || day == nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "bad_date", "message": workErrors["bad_date"]})
		return
	}
	sm, ok1 := parseHM(req.Start)
	em, ok2 := parseHM(req.End)
	if !ok1 || !ok2 {
		c.JSON(http.StatusBadRequest, gin.H{"code": "bad_time", "message": workErrors["bad_time"]})
		return
	}
	club, okc := defaultClub()
	if !okc {
		c.JSON(http.StatusConflict, gin.H{"code": "no_club", "message": "Клуб не найден"})
		return
	}
	start, end := composeWorkTimes(*day, sm, em)
	admin, _ := uuid.Parse(c.GetString("user_id"))
	entry := models.WorkEntry{
		ClubID: club.ID, UserID: user.ID, Date: *day, StartedAt: start, EndedAt: &end,
		Minutes: workMinutes(start, end), Note: strings.TrimSpace(req.Note),
		CreatedBy: &admin, EditedBy: &admin,
	}
	if sh, _ := shiftNow(start); sh != nil {
		entry.ShiftID = &sh.ID
	}
	if err := db.Create(&entry).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "db_error", "message": err.Error()})
		return
	}
	target := user.ID
	logAdminAction(c, "work_edit", &target, fmt.Sprintf("табель: добавлено %s %s–%s (%s)",
		entry.Date.Format("02.01"), req.Start, req.End, hoursRU(int64(entry.Minutes))))
	c.JSON(http.StatusCreated, gin.H{"entry": workOut(&entry, "")})
}

// PATCH /admin/work/:id — исправить запись табеля (owner).
func handleWorkUpdate(c *gin.Context) {
	var w models.WorkEntry
	if err := db.First(&w, "id = ?", c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "entry_not_found", "message": "Запись табеля не найдена"})
		return
	}
	var req workRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_request", "message": "Не разобрал запись"})
		return
	}
	sm, ok1 := parseHM(req.Start)
	em, ok2 := parseHM(req.End)
	if !ok1 || !ok2 {
		c.JSON(http.StatusBadRequest, gin.H{"code": "bad_time", "message": workErrors["bad_time"]})
		return
	}
	day := w.Date
	if req.Date != "" {
		d, ok := parseDateOpt(req.Date)
		if !ok || d == nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": "bad_date", "message": workErrors["bad_date"]})
			return
		}
		day = *d
	}
	start, end := composeWorkTimes(day, sm, em)
	admin, _ := uuid.Parse(c.GetString("user_id"))
	mins := workMinutes(start, end)
	if err := db.Model(&models.WorkEntry{}).Where("id = ?", w.ID).Updates(map[string]any{
		"date": day, "started_at": start, "ended_at": end, "minutes": mins,
		"auto_closed": false, "note": strings.TrimSpace(req.Note),
		"edited_by": admin, "updated_at": time.Now(),
	}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "db_error", "message": err.Error()})
		return
	}
	target := w.UserID
	logAdminAction(c, "work_edit", &target, fmt.Sprintf("табель: правка %s %s–%s (%s было %s)",
		day.Format("02.01"), req.Start, req.End, hoursRU(int64(mins)), hoursRU(int64(w.Minutes))))
	c.JSON(http.StatusOK, gin.H{"id": w.ID, "minutes": mins})
}

// DELETE /admin/work/:id — удалить запись табеля (owner).
func handleWorkDelete(c *gin.Context) {
	var w models.WorkEntry
	if err := db.First(&w, "id = ?", c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "entry_not_found", "message": "Запись табеля не найдена"})
		return
	}
	if err := db.Delete(&models.WorkEntry{}, "id = ?", w.ID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "db_error", "message": err.Error()})
		return
	}
	target := w.UserID
	logAdminAction(c, "work_edit", &target, fmt.Sprintf("табель: удалена запись %s (%s)",
		w.Date.Format("02.01"), hoursRU(int64(w.Minutes))))
	c.JSON(http.StatusOK, gin.H{"deleted": w.ID})
}

// ── Е3-и1: смена открывается ВХОДОМ в систему (решение Р8) ────────────
//
// «Начало работы только тогда, когда он зашёл под своим логином» — прямая
// формулировка основателя. До этого табель был ручной кнопкой: можно было
// залогиниться и не отметиться (и работать «бесплатно» для отчёта), можно
// было отметиться и уйти пить кофе. Вход — единственное событие, которое
// админ физически не может пропустить: без него он ничего не сделает.
//
// Кнопка «пришёл» остаётся (IV.2): вход бывает и до фактического начала
// работы, и человек может открыть смену руками, если система его не узнала.

// openWorkOnLogin — открыть запись табеля при входе, если её ещё нет.
// Молча ничего не делает, когда открывать нечего: вход — не то место, где
// уместно ругаться на состояние табеля.
//
// Владельца НЕ трогаем сознательно: он заходит смотреть отчёты из дома, и
// каждый такой вход создавал бы в табеле час работы, которого не было.
// Владелец, вставший на смену, отмечается кнопкой — как и раньше.
func openWorkOnLogin(user *models.User, now time.Time) (opened bool, autoClosedPrev bool) {
	if user == nil || user.Role != models.UserRoleAdmin {
		return false, false
	}
	club, ok := defaultClub()
	if !ok {
		return false, false
	}
	reportHour := int(settingInt64("report_hour", 8))
	sh, day := shiftNow(now)
	if sh == nil {
		day = clubDayOf(now, reportHour)
	}
	if open := openEntryFor(user.ID); open != nil {
		// Смена этих же клубных суток уже открыта — повторный вход (второй
		// браузер, перезагрузка шелла) не должен плодить записи.
		if open.Date.Format("2006-01-02") == day.Format("2006-01-02") {
			return false, false
		}
		closeForgotten(open)
		autoClosedPrev = true
	}
	entry := models.WorkEntry{
		ClubID: club.ID, UserID: user.ID, Date: day, StartedAt: now,
		CreatedBy: &user.ID, Note: "открыта входом в систему",
	}
	if sh != nil {
		entry.ShiftID = &sh.ID
	}
	if err := db.Create(&entry).Error; err != nil {
		log.Printf("табель: смена по входу для %s не открылась: %v", user.Nickname, err)
		return false, autoClosedPrev
	}
	name := ""
	if sh != nil {
		name = sh.Name
	}
	logAdminActionAs(user.ID, "work_start", nil, "вход в систему"+shiftSuffix(name))
	return true, autoClosedPrev
}
