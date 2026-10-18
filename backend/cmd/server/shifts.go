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
	"gorm.io/gorm"

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

// shiftsOverlap — пересекаются ли два шаблона хоть в один день недели (чистая, тест; З15-и1, №22). Смена через полночь
// захватывает утро следующего дня (ночь воскресенья — утро понедельника); стык «конец 20:00 — начало 20:00» — не
// пересечение.
func shiftsOverlap(aStart, aEnd, aMask, bStart, bEnd, bMask int) bool {
	spans := func(start, end, mask int) [][2]int {
		length := end - start
		if end <= start {
			length += 1440
		}
		out := [][2]int{}
		for w := 0; w < 2; w++ { // две недели подряд — чтобы ночь воскресенья встретилась с утром понедельника
			for d := 0; d < 7; d++ {
				if mask&(1<<d) != 0 {
					s := (w*7+d)*1440 + start
					out = append(out, [2]int{s, s + length})
				}
			}
		}
		return out
	}
	for _, x := range spans(aStart, aEnd, aMask) {
		for _, y := range spans(bStart, bEnd, bMask) {
			if x[0] < y[1] && y[0] < x[1] {
				return true
			}
		}
	}
	return false
}

// shiftOverlaps — активные шаблоны, с которыми пересекается s (кроме него самого).
func shiftOverlaps(s *models.Shift) []models.Shift {
	out := []models.Shift{}
	for _, o := range activeShifts() {
		if o.ID != s.ID && shiftsOverlap(s.StartMin, s.EndMin, s.DaysMask, o.StartMin, o.EndMin, o.DaysMask) {
			out = append(out, o)
		}
	}
	return out
}

// overlapText — «со сменой «День» 08:00–20:00» / «со сменами …» (чистая, тест).
func overlapText(with []models.Shift) string {
	parts := make([]string, 0, len(with))
	for _, o := range with {
		parts = append(parts, "«"+o.Name+"» "+fmtShiftTime(o.StartMin)+"–"+fmtShiftTime(o.EndMin))
	}
	if len(parts) == 1 {
		return "со сменой " + parts[0]
	}
	return "со сменами " + strings.Join(parts, ", ")
}

// respondOverlap — 409 overlap: с кем пересекается; «всё равно» — тот же запрос с force.
func respondOverlap(c *gin.Context, with []models.Shift) {
	list := make([]gin.H, 0, len(with))
	for i := range with {
		list = append(list, shiftOut(&with[i]))
	}
	c.JSON(http.StatusConflict, gin.H{"code": "overlap", "message": "Пересекается " + overlapText(with), "with": list})
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

// ── З15-и4 (OWNER.md, №30; З-Р20): сужение дней шаблона — не молча ─────────────────────────────────────────────────
//
// Убрал из шаблона день недели — назначения на этот день оставались в базе, но сетка их больше не рисовала: невидимые,
// они всё равно давали смену записи табеля при входе, «по графику» в сводке и плановые часы в отчёте. Теперь сохранить
// такую правку с будущими назначениями в убранные дни — 409 days_assigned со списком; «снять и сохранить» — тот же
// запрос с drop_assignments: шаблон и снятие назначений — одной транзакцией, в журнале — кого сняли. Прошедшие и уже
// начавшиеся смены остаются в истории графика.

// daysRU — дни недели с понедельника, как в сетке графика.
var daysRU = [7]string{"пн", "вт", "ср", "чт", "пт", "сб", "вс"}

// dropOnNarrow — какие назначения уносит сужение дней шаблона (чистая, тест): день недели назначения убран (removed —
// биты убранных дней), а смена по времени шаблона до правки ещё не началась. days — дни назначений (день НАЧАЛА
// смены); ответ — их номера в days.
func dropOnNarrow(days []time.Time, removed, startMin, endMin int, now time.Time) []int {
	out := []int{}
	for i, d := range days {
		day := time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, models.ClubLocation)
		if removed&(1<<dowMonday(day)) == 0 {
			continue
		}
		if start, _ := shiftSpan(day, startMin, endMin); start.After(now) {
			out = append(out, i)
		}
	}
	return out
}

// droppedAssignments — будущие назначения шаблона was в дни, которых нет в новой маске.
func droppedAssignments(was *models.Shift, newMask int, now time.Time) []models.ShiftAssignment {
	removed := was.DaysMask &^ newMask
	if removed == 0 {
		return nil
	}
	nc := now.In(models.ClubLocation)
	var list []models.ShiftAssignment
	db.Preload("User").Where("shift_id = ? AND date >= ?", was.ID, nc.Format("2006-01-02")).
		Order("date, created_at").Find(&list)
	days := make([]time.Time, len(list))
	for i := range list {
		days[i] = list[i].Date
	}
	out := []models.ShiftAssignment{}
	for _, i := range dropOnNarrow(days, removed, was.StartMin, was.EndMin, now) {
		out = append(out, list[i])
	}
	return out
}

// assignmentsText — «пн 19.10 admin1, пт 23.10 admin2» (чистая, тест; для отказа и журнала).
func assignmentsText(list []models.ShiftAssignment) string {
	parts := make([]string, 0, len(list))
	for _, a := range list {
		d := time.Date(a.Date.Year(), a.Date.Month(), a.Date.Day(), 0, 0, 0, 0, models.ClubLocation)
		parts = append(parts, daysRU[dowMonday(d)]+" "+d.Format("02.01")+" "+nicknameOf(a.User))
	}
	return strings.Join(parts, ", ")
}

// respondDaysAssigned — 409 days_assigned: в убранные дни уже стоят назначения. «Снять и сохранить» — тот же запрос с
// drop_assignments; force тут не помогает — «всё равно» про пересечение людей с графика не снимает.
func respondDaysAssigned(c *gin.Context, s *models.Shift, list []models.ShiftAssignment) {
	items := make([]gin.H, 0, len(list))
	for _, a := range list {
		items = append(items, gin.H{"id": a.ID, "date": a.Date.Format("2006-01-02"), "nickname": nicknameOf(a.User)})
	}
	c.JSON(http.StatusConflict, gin.H{"code": "days_assigned",
		"message": "В убранные дни на «" + s.Name + "» уже назначены: " + assignmentsText(list), "assignments": items})
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
	Sort     *int   `json:"sort"` // З15-и4: указатель — порядок можно вернуть в 0
	Active   *bool  `json:"active"`
	Force    bool   `json:"force"` // З15-и1: создать или сохранить, хоть и пересекается с другими
	// З15-и4: сохранить сужение дней и снять будущие назначения в убранные дни
	DropAssignments bool `json:"drop_assignments"`
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
	if !validSort(req.Sort) { // З6-и3: 3·10⁹ падало на int4 базы ошибкой сервера
		c.JSON(http.StatusBadRequest, gin.H{"code": "bad_sort", "message": sortMessage})
		return
	}
	order := 0
	if req.Sort != nil {
		order = *req.Sort
	}
	club, ok := defaultClub()
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "club_missing", "message": "Клуб не найден"})
		return
	}
	s := models.Shift{
		ClubID: club.ID, Name: strings.TrimSpace(req.Name),
		StartMin: *req.StartMin, EndMin: *req.EndMin, DaysMask: mask, Sort: order, Active: true,
	}
	with := shiftOverlaps(&s) // З15-и1 (№22; З-Р16): пересечение — предупреждение с «всё равно создать»
	if len(with) > 0 && !req.Force {
		respondOverlap(c, with)
		return
	}
	if err := db.Create(&s).Error; err != nil {
		respondDBError(c, err)
		return
	}
	details := s.Name + " · " + fmtShiftTime(s.StartMin) + "–" + fmtShiftTime(s.EndMin)
	if len(with) > 0 {
		details += " · пересекается " + overlapText(with)
	}
	logAdminAction(c, "shift_tpl_create", nil, details)
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
		respondBindError(c, err)
		return
	}
	was := s // З15-и4: дни и время до правки — по ним видно, какие назначения уносит сужение дней
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
	if !validSort(req.Sort) { // З6-и3
		c.JSON(http.StatusBadRequest, gin.H{"code": "bad_sort", "message": sortMessage})
		return
	}
	if req.Sort != nil { // З15-и4 (№30): ноль — тоже порядок; раньше «0» молча не сохранялся
		s.Sort = *req.Sort
	}
	if req.Active != nil {
		s.Active = *req.Active
	}
	if ok, code := validateShiftTemplate(s.Name, s.StartMin, s.EndMin, s.DaysMask); !ok {
		c.JSON(http.StatusBadRequest, gin.H{"code": code, "message": shiftTemplateErrors[code]})
		return
	}
	var with []models.Shift
	if s.Active { // З15-и1: выключенная смена в сетке не идёт — ей пересекаться не с чем
		with = shiftOverlaps(&s)
	}
	if len(with) > 0 && !req.Force {
		respondOverlap(c, with)
		return
	}
	drop := droppedAssignments(&was, s.DaysMask, time.Now()) // З15-и4: сужение дней — не молча
	if len(drop) > 0 && !req.DropAssignments {
		respondDaysAssigned(c, &was, drop)
		return
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&s).Error; err != nil {
			return err
		}
		if len(drop) == 0 {
			return nil
		}
		ids := make([]uuid.UUID, 0, len(drop))
		for _, a := range drop {
			ids = append(ids, a.ID)
		}
		return tx.Where("id IN ?", ids).Delete(&models.ShiftAssignment{}).Error
	}); err != nil {
		respondDBError(c, err)
		return
	}
	details := s.Name + " · " + fmtShiftTime(s.StartMin) + "–" + fmtShiftTime(s.EndMin)
	if len(with) > 0 {
		details += " · пересекается " + overlapText(with)
	}
	if len(drop) > 0 {
		details += " · сняты с графика в убранные дни: " + assignmentsText(drop)
	}
	logAdminAction(c, "shift_tpl_update", nil, details)
	hub.AdminBroadcast("shifts", map[string]any{"kind": "tpl"})
	out := shiftOut(&s)
	if len(drop) > 0 {
		out["unassigned"] = len(drop)
	}
	c.JSON(http.StatusOK, out)
}

// DELETE /admin/shifts/:id — удалить шаблон (owner). Назначения уходят
// каскадом (история графика по этой смене исчезает — честное удаление;
// чтобы просто спрятать, есть active=false). Имя смены в табеле и отчёте
// остаётся — оно снимком в записи табеля (З15-и4).
func handleAdminShiftDelete(c *gin.Context) {
	var s models.Shift
	if err := db.First(&s, "id = ?", c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "shift_not_found", "message": "Смена не найдена"})
		return
	}
	if err := db.Delete(&s).Error; err != nil {
		respondDBError(c, err)
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
		respondDBError(c, err)
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
		respondDBError(c, err)
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
