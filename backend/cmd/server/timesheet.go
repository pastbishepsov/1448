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

// ── З15-и1 (OWNER.md, №22; З-Р16): смена при приходе — по назначению ─────────────────────────────────────────────
//
// Смену записи табеля брал шаблон, который шёл в минуту входа: назначенный на «День 08–20» и вошедший в 07:50 попадал
// в хвост «Ночи» вчерашнего дня (или «вне смены», если ночного шаблона нет), а в сводке — в «вне графика»; при
// пересекающихся шаблонах брался первый по порядку. Теперь сначала — назначение человека: смена, которая у него идёт
// сейчас, иначе та, что начнётся не позже чем через shiftEarly; нет назначения — как раньше, шаблон по времени.

// shiftEarly — насколько раньше начала назначенной смены приход ещё относится к ней.
const shiftEarly = 3 * time.Hour

// assignedWindow — назначенная смена человека: день (день НАЧАЛА смены), начало и конец.
type assignedWindow struct {
	Day, Start, End time.Time
}

// shiftSpan — начало и конец смены, назначенной на день day (чистая, тест).
func shiftSpan(day time.Time, startMin, endMin int) (time.Time, time.Time) {
	base := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, day.Location())
	return base.Add(time.Duration(startMin) * time.Minute), shiftEndAt(day, startMin, endMin)
}

// pickAssigned — какое из назначений относится к моменту t (чистая, тест): сначала смена, которая идёт сейчас (из
// нескольких — начавшаяся последней), потом ближайшая, что начнётся не позже чем через early; ни одной — -1.
func pickAssigned(c []assignedWindow, t time.Time, early time.Duration) int {
	best := -1
	for i, a := range c {
		if !t.Before(a.Start) && t.Before(a.End) && (best < 0 || a.Start.After(c[best].Start)) {
			best = i
		}
	}
	if best >= 0 {
		return best
	}
	for i, a := range c {
		if t.Before(a.Start) && a.Start.Sub(t) <= early && (best < 0 || a.Start.Before(c[best].Start)) {
			best = i
		}
	}
	return best
}

// shiftFor — смена и её день для прихода человека в момент t: сначала его назначение (идёт сейчас или вот-вот
// начнётся), потом — шаблон, который идёт в эту минуту. assigned — смена взята из назначения.
func shiftFor(userID uuid.UUID, t time.Time) (sh *models.Shift, day time.Time, assigned bool) {
	tc := t.In(models.ClubLocation)
	today := time.Date(tc.Year(), tc.Month(), tc.Day(), 0, 0, 0, 0, models.ClubLocation)
	var as []models.ShiftAssignment
	db.Preload("Shift").Where("user_id = ? AND date BETWEEN ? AND ?", userID,
		today.AddDate(0, 0, -1).Format("2006-01-02"), today.AddDate(0, 0, 1).Format("2006-01-02")).Find(&as)
	wins := make([]assignedWindow, 0, len(as))
	shifts := make([]*models.Shift, 0, len(as))
	for i := range as {
		if a := as[i]; a.Shift != nil && a.Shift.Active {
			s, e := shiftSpan(a.Date, a.Shift.StartMin, a.Shift.EndMin)
			wins = append(wins, assignedWindow{Day: a.Date, Start: s, End: e})
			shifts = append(shifts, a.Shift)
		}
	}
	if i := pickAssigned(wins, tc, shiftEarly); i >= 0 {
		return shifts[i], wins[i].Day, true
	}
	sh, day = shiftNow(t)
	return sh, day, false
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

// workOut — запись табеля для ответа. З15-и4 (№30): имя смены — снимок в самой записи; раньше его искали по шаблону, и
// удалённый (а в табеле — и выключенный) шаблон уносил имя, переименованный — переписывал.
func workOut(w *models.WorkEntry) gin.H {
	row := gin.H{
		"id": w.ID, "date": w.Date.Format("2006-01-02"),
		"started_at": w.StartedAt, "minutes": w.Minutes,
		"auto_closed": w.AutoClosed, "note": w.Note, "shift": w.ShiftName,
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
	// З15-и1: сначала назначение, потом шаблон по времени
	sh, day, _ := shiftFor(userID, now)
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
		entry.ShiftID, entry.ShiftName = &sh.ID, sh.Name // З15-и4: имя — снимком
	}
	if err := db.Create(&entry).Error; err != nil {
		if isDuplicate(err) { // З3-и3: два «Я пришёл» разом — вторую открытую запись не пускает индекс
			c.JSON(http.StatusConflict, gin.H{"code": "already_open", "message": "Ты уже на смене — сначала отметь уход"})
			return
		}
		respondDBError(c, err)
		return
	}
	logAdminAction(c, "work_start", nil, "пришёл"+shiftSuffix(entry.ShiftName))
	c.JSON(http.StatusCreated, gin.H{"entry": workOut(&entry), "auto_closed_previous": autoClosed})
}

func shiftSuffix(name string) string {
	if name == "" {
		return " (вне смены)"
	}
	return " · " + name
}

// POST /admin/work/stop {counted_pln?, note?} — «ушёл» (staff, за себя).
//
// Ж6-и4 (№32): уход — это и сдача кассы. counted_pln — пересчитанные
// наличные в ящике: сервер сравнит с ожидаемой суммой по операциям админа за
// клубные сутки (closeout.go), запишет расхождение и сообщит владельцу;
// без counted_pln — «ушёл без пересчёта», запись с пометкой skipped. Тело
// необязательно: старый вызов без него — уход без пересчёта.
func handleWorkStop(c *gin.Context) {
	userID, err := uuid.Parse(c.GetString("user_id"))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": "bad_token", "message": "Не разобрал токен"})
		return
	}
	var req struct {
		CountedPLN *float64 `json:"counted_pln"`
		Note       string   `json:"note"`
	}
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_request", "message": "counted_pln — сумма в злотых, note — текст"})
			return
		}
	}
	if req.CountedPLN != nil && (*req.CountedPLN < 0 || *req.CountedPLN > 1_000_000) {
		c.JSON(http.StatusBadRequest, gin.H{"code": "bad_amount", "message": "Пересчитанная сумма — от 0 до 1 000 000 zł"})
		return
	}
	if len([]rune(req.Note)) > 500 {
		c.JSON(http.StatusBadRequest, gin.H{"code": "too_long", "message": "Заметка — до 500 символов"})
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
		respondDBError(c, err)
		return
	}
	logAdminAction(c, "work_stop", nil, fmt.Sprintf("ушёл · %s", hoursRU(int64(mins))))
	open.EndedAt, open.Minutes = &now, mins
	resp := gin.H{"entry": workOut(open)}
	if co := recordCloseout(c, userID, open, req.CountedPLN, req.Note, now); co != nil {
		resp["closeout"] = closeoutOut(co)
	}
	c.JSON(http.StatusOK, resp)
}

func hoursRU(minutes int64) string {
	return fmt.Sprintf("%d ч %02d м", minutes/60, minutes%60)
}

// GET /admin/work/me — моё состояние: на смене или нет (staff).
func handleWorkMe(c *gin.Context) {
	userID, _ := uuid.Parse(c.GetString("user_id"))
	reportHour := int(settingInt64("report_hour", 8))
	now := time.Now()
	day := clubDayOf(now, reportHour)

	out := gin.H{"on_shift": false, "date": day.Format("2006-01-02")}
	if open := openEntryFor(userID); open != nil {
		out["on_shift"] = true
		out["entry"] = workOut(open)
		out["minutes_so_far"] = workMinutes(open.StartedAt, now)
	}
	// З15-и6 (№30): «Сегодня отработано» — записи этих клубных суток и те, что в них закончились. Ночная смена
	// записана днём начала (20:00 вчера — 08:00 сегодня), и после ухода утром здесь стоял ноль.
	var todayMin int64
	db.Model(&models.WorkEntry{}).Select("COALESCE(SUM(minutes),0)").
		Where("user_id = ? AND (date = ? OR ended_at >= ?)", userID, day.Format("2006-01-02"), clubDayStartAt(reportHour, now)).
		Scan(&todayMin)
	out["minutes_today"] = todayMin
	c.JSON(http.StatusOK, out)
}

// ── Табель владельца ──────────────────────────────────────────────────

// workFact — факт табеля за период по человеку (З3-и2, OWNER.md №20 «а»).
// Платим по подтверждённому: уход отметил сам человек или запись поправил
// владелец. Автозакрытая запись (ушёл, не отметившись, — часы поставлены по
// графику) в выплату не входит и показывается отдельно, пока владелец её не
// поправит. Смена — день с подтверждёнными минутами: «ушёл — пришёл» днём
// раньше давал две записи и две ставки за смену.
type workFact struct {
	Minutes     int64 // подтверждённые минуты
	Shifts      int64 // дни с подтверждёнными минутами
	AutoMinutes int64 // автозакрытые минуты — вне выплаты
	AutoEntries int64 // автозакрытых записей
}

// factByUser — факт за период по каждому сотруднику.
func factByUser(p period) map[string]workFact {
	var rows []struct {
		UserID                        string
		Mins, Days, AutoMins, AutoCnt int64
	}
	db.Model(&models.WorkEntry{}).
		Select(`user_id,
			COALESCE(SUM(minutes) FILTER (WHERE NOT auto_closed), 0) AS mins,
			COUNT(DISTINCT date) FILTER (WHERE NOT auto_closed AND minutes > 0) AS days,
			COALESCE(SUM(minutes) FILTER (WHERE auto_closed), 0) AS auto_mins,
			COUNT(*) FILTER (WHERE auto_closed) AS auto_cnt`).
		Where("date >= ? AND date <= ?", p.FromDay, p.ToDay).
		Group("user_id").Scan(&rows)
	out := make(map[string]workFact, len(rows))
	for _, r := range rows {
		out[r.UserID] = workFact{Minutes: r.Mins, Shifts: r.Days, AutoMinutes: r.AutoMins, AutoEntries: r.AutoCnt}
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

	ids := make([]string, 0, len(entries))
	for _, w := range entries {
		ids = append(ids, w.UserID.String())
	}
	nick := nicknamesByID(ids)

	closeouts := closeoutsByEntry(entries) // Ж6-и4: сдача кассы у записи табеля
	items := make([]gin.H, 0, len(entries))
	openCnt, autoCnt := 0, 0
	for i := range entries {
		w := entries[i]
		row := workOut(&w)
		row["nickname"] = nick[w.UserID.String()]
		row["user_id"] = w.UserID
		if co, ok := closeouts[w.ID.String()]; ok {
			row["closeout"] = co
		}
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
	"not_staff": "Табель ведётся только на персонал: у этого ника нет ни роли, ни кадровой карточки",
}

// workTargetOK — кому владелец ведёт табель руками (З3-и3, №21; Д12): персоналу
// или бывшему персоналу с кадровой карточкой — дописать последнюю смену
// уволенного. Гость табеля не получает: раньше запись на него молча проходила.
func workTargetOK(u *models.User) bool {
	if roleIsStaff(string(u.Role)) {
		return true
	}
	var n int64
	db.Model(&models.StaffProfile{}).Where("user_id = ?", u.ID).Count(&n)
	return n > 0
}

// workOverlap — запись того же человека, с которой пересекается [start, end), или
// nil (З3-и3). Открытая запись тянется до бесконечности — человек ещё на смене;
// встык — не пересечение; except — сама правимая запись.
func workOverlap(userID uuid.UUID, start, end time.Time, except *uuid.UUID) *models.WorkEntry {
	q := db.Where("user_id = ? AND started_at < ? AND COALESCE(ended_at, 'infinity'::timestamptz) > ?", userID, end, start)
	if except != nil {
		q = q.Where("id <> ?", *except)
	}
	var w models.WorkEntry
	if q.Order("started_at").First(&w).Error != nil {
		return nil
	}
	return &w
}

// overlapMessage — отказ с указанием, с какой записью пересеклись (чистая, тест).
func overlapMessage(w *models.WorkEntry) string {
	from := w.StartedAt.Format("02.01 15:04")
	if w.EndedAt == nil {
		return "Время пересекается с открытой сменой этого человека: с " + from
	}
	to := w.EndedAt.Format("15:04")
	if w.EndedAt.Format("2006-01-02") != w.StartedAt.Format("2006-01-02") {
		to = w.EndedAt.Format("02.01 15:04")
	}
	return "Время пересекается с другой записью этого человека: " + from + "–" + to
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

// POST /admin/work — добавить запись табеля руками (owner). З3-и3: только
// персоналу или бывшему персоналу с карточкой (Д12) и без пересечений с другими
// записями того же человека.
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
	if !workTargetOK(&user) {
		c.JSON(http.StatusConflict, gin.H{"code": "not_staff", "message": workErrors["not_staff"]})
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
	if other := workOverlap(user.ID, start, end, nil); other != nil {
		c.JSON(http.StatusConflict, gin.H{"code": "overlap", "message": overlapMessage(other)})
		return
	}
	admin, _ := uuid.Parse(c.GetString("user_id"))
	entry := models.WorkEntry{
		ClubID: club.ID, UserID: user.ID, Date: *day, StartedAt: start, EndedAt: &end,
		Minutes: workMinutes(start, end), Note: strings.TrimSpace(req.Note),
		CreatedBy: &admin, EditedBy: &admin,
	}
	if sh, _, _ := shiftFor(user.ID, start); sh != nil { // З15-и1: смена — по назначению человека
		entry.ShiftID, entry.ShiftName = &sh.ID, sh.Name // З15-и4: имя — снимком
	}
	if err := db.Create(&entry).Error; err != nil {
		respondDBError(c, err)
		return
	}
	target := user.ID
	logAdminAction(c, "work_edit", &target, fmt.Sprintf("табель: добавлено %s %s–%s (%s)",
		entry.Date.Format("02.01"), req.Start, req.End, hoursRU(int64(entry.Minutes))))
	c.JSON(http.StatusCreated, gin.H{"entry": workOut(&entry)})
}

// PATCH /admin/work/:id — исправить запись табеля (owner). З3-и3: без
// пересечений с другими записями того же человека.
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
	var owner models.User // чья запись — ник из тела цель не меняет
	if db.First(&owner, "id = ?", w.UserID).Error != nil || !workTargetOK(&owner) {
		c.JSON(http.StatusConflict, gin.H{"code": "not_staff", "message": workErrors["not_staff"]})
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
	if other := workOverlap(w.UserID, start, end, &w.ID); other != nil {
		c.JSON(http.StatusConflict, gin.H{"code": "overlap", "message": overlapMessage(other)})
		return
	}
	admin, _ := uuid.Parse(c.GetString("user_id"))
	mins := workMinutes(start, end)
	if err := db.Model(&models.WorkEntry{}).Where("id = ?", w.ID).Updates(map[string]any{
		"date": day, "started_at": start, "ended_at": end, "minutes": mins,
		"auto_closed": false, "note": strings.TrimSpace(req.Note),
		"edited_by": admin, "updated_at": time.Now(),
	}).Error; err != nil {
		respondDBError(c, err)
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
		respondDBError(c, err)
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
//
// З3-и1 (OWNER.md, №19): смену открывает только вход в админку (client:
// "admin" в теле /auth/login). Киоск и PWA входят тем же /auth/login, и админ,
// зашедший из дома в PWA в 09:00, становился «на смене» до ухода в 24:00.

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
	if staffLeft(user.ID.String(), now) { // З14-и2: последний рабочий день прошёл — смены больше нет
		return false, false
	}
	club, ok := defaultClub()
	if !ok {
		return false, false
	}
	reportHour := int(settingInt64("report_hour", 8))
	sh, day, _ := shiftFor(user.ID, now) // З15-и1: сначала назначение, потом шаблон по времени
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
		CreatedBy: &user.ID, Note: "открыта входом в админку",
	}
	if sh != nil {
		entry.ShiftID, entry.ShiftName = &sh.ID, sh.Name // З15-и4: имя — снимком
	}
	if err := db.Create(&entry).Error; err != nil {
		log.Printf("табель: смена по входу для %s не открылась: %v", user.Nickname, err)
		return false, autoClosedPrev
	}
	logAdminActionAs(user.ID, "work_start", nil, "вход в админку"+shiftSuffix(entry.ShiftName))
	return true, autoClosedPrev
}
