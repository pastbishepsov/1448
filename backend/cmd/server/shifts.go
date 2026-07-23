package main

// Смены и график персонала (спринт Б11, ADMIN.md; миграция 023).
// Запрос владельца 2026-07-21: смены, их время и кто работает, управляются
// из админки. Шаблоны и назначения правит owner; персонал читает график
// (сетка недели в «Персонале», «на смене: …» в Зале — и2/и3). Дата
// назначения — день НАЧАЛА смены: ночная пятничная утром субботы — пятничная.

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

// validateShiftTemplate — чистая проверка шаблона смены (тест в
// shifts_test.go): имя 1..32 после трима; минуты 0..1439; start != end
// (end < start — валидная смена через полночь); маска дней 1..127.
func validateShiftTemplate(name string, startMin, endMin, daysMask int) (ok bool, code string) {
	n := strings.TrimSpace(name)
	if n == "" || len([]rune(n)) > 32 {
		return false, "bad_name"
	}
	if startMin < 0 || startMin > 1439 || endMin < 0 || endMin > 1439 {
		return false, "bad_time"
	}
	if startMin == endMin {
		return false, "zero_length"
	}
	if daysMask < 1 || daysMask > 127 {
		return false, "bad_days"
	}
	return true, ""
}

// dowMonday — день недели с понедельником в нуле (для days_mask).
func dowMonday(t time.Time) int { return (int(t.Weekday()) + 6) % 7 }

// shiftActiveAt — чистая (тест в shifts_test.go): идёт ли смена в момент t;
// вернёт и дату смены (день её начала). Ночная смена (end < start) утром
// принадлежит вчерашнему дню — и маска дней проверяется по дню начала.
func shiftActiveAt(startMin, endMin, daysMask int, t time.Time) (bool, time.Time) {
	day := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
	min := t.Hour()*60 + t.Minute()
	if endMin > startMin { // дневная
		if min >= startMin && min < endMin && daysMask&(1<<dowMonday(day)) != 0 {
			return true, day
		}
		return false, time.Time{}
	}
	// через полночь: вечерняя половина — сегодняшняя смена
	if min >= startMin && daysMask&(1<<dowMonday(day)) != 0 {
		return true, day
	}
	// утренняя половина — смена, начатая вчера
	if min < endMin {
		prev := day.AddDate(0, 0, -1)
		if daysMask&(1<<dowMonday(prev)) != 0 {
			return true, prev
		}
	}
	return false, time.Time{}
}

// fmtShiftTime — 480 → «8:00» (для аудита и ответов).
func fmtShiftTime(min int) string {
	return time.Date(2000, 1, 1, min/60, min%60, 0, 0, time.UTC).Format("15:04")
}

func shiftOut(s *models.Shift) gin.H {
	return gin.H{
		"id": s.ID, "name": s.Name, "start_min": s.StartMin, "end_min": s.EndMin,
		"days_mask": s.DaysMask, "sort": s.Sort, "active": s.Active,
	}
}

// GET /admin/shifts — шаблоны смен (staff: нужны для сетки графика).
func handleAdminShifts(c *gin.Context) {
	var shifts []models.Shift
	db.Order("sort, start_min").Find(&shifts)
	out := make([]gin.H, 0, len(shifts))
	for i := range shifts {
		out = append(out, shiftOut(&shifts[i]))
	}
	c.JSON(http.StatusOK, gin.H{"count": len(out), "shifts": out})
}

type shiftTemplateRequest struct {
	Name     string `json:"name"`
	StartMin *int   `json:"start_min"`
	EndMin   *int   `json:"end_min"`
	DaysMask *int   `json:"days_mask"`
	Sort     int    `json:"sort"`
	Active   *bool  `json:"active"`
}

var shiftTemplateErrors = map[string]string{
	"bad_name":    "Имя смены: 1–32 символа",
	"bad_time":    "Время — минуты от полуночи, 0–1439",
	"zero_length": "Начало и конец совпадают — смена нулевой длины",
	"bad_days":    "Дни недели: маска от 1 до 127",
}

// POST /admin/shifts — новый шаблон смены (owner).
func handleAdminShiftCreate(c *gin.Context) {
	var req shiftTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.StartMin == nil || req.EndMin == nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_request", "message": "Нужны name, start_min, end_min"})
		return
	}
	mask := 127
	if req.DaysMask != nil {
		mask = *req.DaysMask
	}
	if ok, code := validateShiftTemplate(req.Name, *req.StartMin, *req.EndMin, mask); !ok {
		c.JSON(http.StatusBadRequest, gin.H{"code": code, "message": shiftTemplateErrors[code]})
		return
	}
	club, ok := defaultClub()
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "club_missing", "message": "Клуб не найден"})
		return
	}
	s := models.Shift{
		ClubID: club.ID, Name: strings.TrimSpace(req.Name),
		StartMin: *req.StartMin, EndMin: *req.EndMin, DaysMask: mask, Sort: req.Sort, Active: true,
	}
	if err := db.Create(&s).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "db_error", "message": err.Error()})
		return
	}
	logAdminAction(c, "shift_tpl_create", nil,
		s.Name+" · "+fmtShiftTime(s.StartMin)+"–"+fmtShiftTime(s.EndMin))
	hub.AdminBroadcast("shifts", map[string]any{"kind": "tpl"})
	c.JSON(http.StatusCreated, shiftOut(&s))
}

// PATCH /admin/shifts/:id — правка шаблона (owner); поля опциональны.
func handleAdminShiftUpdate(c *gin.Context) {
	var s models.Shift
	if err := db.First(&s, "id = ?", c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "shift_not_found", "message": "Смена не найдена"})
		return
	}
	var req shiftTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_request", "message": err.Error()})
		return
	}
	if req.Name != "" {
		s.Name = strings.TrimSpace(req.Name)
	}
	if req.StartMin != nil {
		s.StartMin = *req.StartMin
	}
	if req.EndMin != nil {
		s.EndMin = *req.EndMin
	}
	if req.DaysMask != nil {
		s.DaysMask = *req.DaysMask
	}
	if req.Sort != 0 {
		s.Sort = req.Sort
	}
	if req.Active != nil {
		s.Active = *req.Active
	}
	if ok, code := validateShiftTemplate(s.Name, s.StartMin, s.EndMin, s.DaysMask); !ok {
		c.JSON(http.StatusBadRequest, gin.H{"code": code, "message": shiftTemplateErrors[code]})
		return
	}
	if err := db.Save(&s).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "db_error", "message": err.Error()})
		return
	}
	logAdminAction(c, "shift_tpl_update", nil,
		s.Name+" · "+fmtShiftTime(s.StartMin)+"–"+fmtShiftTime(s.EndMin))
	hub.AdminBroadcast("shifts", map[string]any{"kind": "tpl"})
	c.JSON(http.StatusOK, shiftOut(&s))
}

// DELETE /admin/shifts/:id — удалить шаблон (owner). Назначения уходят
// каскадом (история графика по этой смене исчезает — честное удаление;
// чтобы просто спрятать, есть active=false).
func handleAdminShiftDelete(c *gin.Context) {
	var s models.Shift
	if err := db.First(&s, "id = ?", c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "shift_not_found", "message": "Смена не найдена"})
		return
	}
	if err := db.Delete(&s).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "db_error", "message": err.Error()})
		return
	}
	logAdminAction(c, "shift_tpl_delete", nil, s.Name)
	hub.AdminBroadcast("shifts", map[string]any{"kind": "tpl"})
	c.JSON(http.StatusOK, gin.H{"id": s.ID, "deleted": true})
}

// GET /admin/shifts/schedule?from=YYYY-MM-DD&days=7 — график (staff).
func handleAdminShiftSchedule(c *gin.Context) {
	now := time.Now()
	from := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	if q := c.Query("from"); q != "" {
		d, err := time.ParseInLocation("2006-01-02", q, now.Location())
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": "bad_date", "message": "from — в формате YYYY-MM-DD"})
			return
		}
		from = d
	}
	days := 7
	if v, err := strconv.Atoi(c.DefaultQuery("days", "7")); err == nil && v >= 1 && v <= 31 {
		days = v
	}
	to := from.AddDate(0, 0, days)

	var items []models.ShiftAssignment
	db.Preload("User").
		Where("date >= ? AND date < ?", from.Format("2006-01-02"), to.Format("2006-01-02")).
		Order("date, created_at").Find(&items)

	byDate := map[string][]gin.H{}
	for _, a := range items {
		key := a.Date.Format("2006-01-02")
		byDate[key] = append(byDate[key], gin.H{
			"id": a.ID, "shift_id": a.ShiftID, "user_id": a.UserID,
			"nickname": nicknameOf(a.User),
		})
	}
	out := make([]gin.H, 0, days)
	for i := 0; i < days; i++ {
		d := from.AddDate(0, 0, i)
		key := d.Format("2006-01-02")
		out = append(out, gin.H{"date": key, "dow": dowMonday(d), "items": byDate[key]})
	}
	c.JSON(http.StatusOK, gin.H{"from": from.Format("2006-01-02"), "days": out})
}

// GET /admin/shifts/now — кто сейчас на смене (staff; строка в Зале, Б11-и3).
// Активные шаблоны прогоняются через shiftActiveAt: ночная утром смотрит
// назначения вчерашней даты. Даты в запросах — строками (колонка date;
// time.Time через TZ-приведение может уехать на сутки).
func handleAdminShiftsNow(c *gin.Context) {
	now := time.Now()
	var shifts []models.Shift
	db.Where("active = ?", true).Order("sort, start_min").Find(&shifts)
	out := []gin.H{}
	for i := range shifts {
		s := &shifts[i]
		active, day := shiftActiveAt(s.StartMin, s.EndMin, s.DaysMask, now)
		if !active {
			continue
		}
		var items []models.ShiftAssignment
		db.Preload("User").
			Where("shift_id = ? AND date = ?", s.ID, day.Format("2006-01-02")).
			Order("created_at").Find(&items)
		for _, a := range items {
			out = append(out, gin.H{"shift": s.Name, "nickname": nicknameOf(a.User)})
		}
	}
	c.JSON(http.StatusOK, gin.H{"count": len(out), "items": out})
}

// POST /admin/shifts/:id/assign {date, nickname} — сотрудник на смену (owner).
// Цель — только персонал (admin/owner): график не для гостей.
func handleAdminShiftAssign(c *gin.Context) {
	var s models.Shift
	if err := db.First(&s, "id = ?", c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "shift_not_found", "message": "Смена не найдена"})
		return
	}
	if !s.Active { // QA Б11: назначение на выключенную смену ушло бы в невидимую сетку
		c.JSON(http.StatusConflict, gin.H{"code": "shift_inactive",
			"message": "«" + s.Name + "» выключена — включи её в шаблоне, потом назначай"})
		return
	}
	var req struct {
		Date     string `json:"date" binding:"required"`
		Nickname string `json:"nickname" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_request", "message": "Нужны date и nickname"})
		return
	}
	date, err := time.ParseInLocation("2006-01-02", req.Date, time.Now().Location())
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "bad_date", "message": "date — в формате YYYY-MM-DD"})
		return
	}
	if s.DaysMask&(1<<dowMonday(date)) == 0 {
		c.JSON(http.StatusConflict, gin.H{"code": "day_off",
			"message": "«" + s.Name + "» в этот день недели не работает (дни — в шаблоне смены)"})
		return
	}
	var user models.User
	if err := db.First(&user, "nickname = ?", req.Nickname).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "user_not_found", "message": "Сотрудник с таким ником не найден"})
		return
	}
	if !roleIsStaff(string(user.Role)) {
		c.JSON(http.StatusConflict, gin.H{"code": "not_staff",
			"message": "В график попадает только персонал — сначала назначь админом («Персонал»)"})
		return
	}
	adminID, _ := uuid.Parse(c.GetString("user_id"))
	a := models.ShiftAssignment{
		ClubID: s.ClubID, Date: date, ShiftID: s.ID, UserID: user.ID, CreatedBy: &adminID,
	}
	if err := db.Create(&a).Error; err != nil {
		if isDuplicate(err) {
			c.JSON(http.StatusConflict, gin.H{"code": "already_assigned", "message": "Уже в графике на эту смену"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"code": "db_error", "message": err.Error()})
		return
	}
	target := user.ID
	logAdminAction(c, "shift_assign", &target,
		user.Nickname+" · "+s.Name+" · "+date.Format("02.01"))
	hub.AdminBroadcast("shifts", map[string]any{"kind": "assign", "nickname": user.Nickname, "shift": s.Name, "date": req.Date})
	c.JSON(http.StatusCreated, gin.H{
		"id": a.ID, "date": req.Date, "shift_id": s.ID, "nickname": user.Nickname,
	})
}

// DELETE /admin/shift-assignments/:id — снять с графика (owner).
func handleAdminShiftUnassign(c *gin.Context) {
	var a models.ShiftAssignment
	if err := db.Preload("User").Preload("Shift").
		First(&a, "id = ?", c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "assignment_not_found", "message": "Запись графика не найдена"})
		return
	}
	if err := db.Delete(&a).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "db_error", "message": err.Error()})
		return
	}
	shiftName := ""
	if a.Shift != nil {
		shiftName = a.Shift.Name
	}
	target := a.UserID
	logAdminAction(c, "shift_unassign", &target,
		nicknameOf(a.User)+" · "+shiftName+" · "+a.Date.Format("02.01"))
	hub.AdminBroadcast("shifts", map[string]any{"kind": "unassign", "nickname": nicknameOf(a.User)})
	c.JSON(http.StatusOK, gin.H{"id": a.ID, "deleted": true})
}
