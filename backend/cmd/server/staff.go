package main

// Персонал (спринт Б5, ADMIN.md; решение №9 — на своём Go+Postgres).
// Owner назначает/снимает админов из админки вместо SQL. Роль вступает
// в силу после перелогина сотрудника (роль едет в JWT-клейме).
// Все роуты — за ownerMiddleware (main.go); события — в аудит staff_*
// (от роли admin они скрыты фильтром Б1 в audit.go).

import (
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

// canChangeStaffRole — чистое правило смены роли (тест в staff_test.go):
// назначить можно только гостя (player), снять — только админа;
// owner'а и самого себя не трогаем.
func canChangeStaffRole(targetRole models.UserRole, targetID, actorID string, promote bool) (ok bool, code string) {
	if targetID != "" && targetID == actorID {
		return false, "cannot_touch_self"
	}
	if targetRole == models.UserRoleOwner {
		return false, "cannot_touch_owner"
	}
	if promote && targetRole != models.UserRolePlayer {
		return false, "already_staff"
	}
	if !promote && targetRole != models.UserRoleAdmin {
		return false, "not_admin"
	}
	return true, ""
}

var staffRoleErrors = map[string]string{
	"cannot_touch_self":  "Свою роль менять нельзя",
	"cannot_touch_owner": "Владельца трогать нельзя",
	"already_staff":      "Это уже персонал",
	"not_admin":          "Снять можно только админа",
}

// GET /admin/staff — персонал клуба: ник, роль, был активен, действий сегодня.
func handleAdminStaffList(c *gin.Context) {
	var staff []models.User
	db.Where("role IN ?", []models.UserRole{models.UserRoleOwner, models.UserRoleAdmin}).
		Order("role DESC").Order("nickname").Find(&staff)

	// действий сегодня: admin_actions + ручные начисления + депозиты
	today := clubDayStart(time.Now()) // Ж3: клубные сутки
	acts := map[string]int64{}
	type cnt struct {
		AdminID string
		N       int64
	}
	var rows []cnt
	db.Model(&models.AdminAction{}).Select("admin_id, COUNT(*) AS n").
		Where("created_at >= ? AND action <> ?", today, "login_failed").Group("admin_id").Scan(&rows) // З0-и3: чужие попытки входа — не его действия
	for _, r := range rows {
		acts[r.AdminID] += r.N
	}
	rows = nil
	db.Model(&models.AdminGrant{}).Select("admin_id, COUNT(*) AS n").
		Where("created_at >= ?", today).Group("admin_id").Scan(&rows)
	for _, r := range rows {
		acts[r.AdminID] += r.N
	}
	rows = nil
	db.Model(&models.Deposit{}).Select("created_by AS admin_id, COUNT(*) AS n").
		Where("created_at >= ? AND created_by IS NOT NULL", today).Group("created_by").Scan(&rows)
	for _, r := range rows {
		acts[r.AdminID] += r.N
	}

	// карточки (В3-этап 2): в списке показываем только ФИО и должность —
	// телефон и ставка живут в самой карточке, в общей таблице им не место
	cards := map[string]models.StaffProfile{}
	var profiles []models.StaffProfile
	db.Find(&profiles)
	for _, p := range profiles {
		cards[p.UserID.String()] = p
	}

	out := make([]gin.H, 0, len(staff))
	for _, u := range staff {
		row := gin.H{
			"id": u.ID, "nickname": u.Nickname, "role": u.Role,
			"last_active_at": u.LastActiveAt,
			"actions_today":  acts[u.ID.String()],
		}
		if p, ok := cards[u.ID.String()]; ok {
			row["full_name"] = p.FullName
			row["position"] = p.Position
			row["hired_at"] = dateOut(p.HiredAt)
			row["has_card"] = true
			if p.LeaveOn != nil { // З14-и2: отложенное увольнение — «уходит 30.12» в списке
				row["leave_on"] = dateOut(p.LeaveOn)
			}
		}
		out = append(out, row)
	}
	c.JSON(http.StatusOK, gin.H{"count": len(out), "staff": out, "cash_diffs": cashDiffs(time.Now())})
}

// cashDiffs — расхождения касс за 7 клубных суток для бейджа в «Персонале»
// (З15-и2, №27). Каждая сдача пишет владельцу строку в канал, и за неделю без
// канала они тонут среди остальных — здесь они из самих сдач. Расхождение —
// пересчитанная сдача не в ноль (начальный остаток ящика — не расхождение);
// недостачу может объяснить расход «из кассы», внесённый задним числом до этой
// сдачи (З9-и3в): такая остаётся в списке с пометкой, но в «не объяснено» не
// идёт. Свежие сверху.
func cashDiffs(now time.Time) gin.H {
	p, _ := resolvePeriod("d7", "", "", clubDayHour(), now)
	var cos []models.ShiftCloseout
	db.Where("created_at >= ? AND created_at < ? AND diff_grosz IS NOT NULL AND diff_grosz <> 0 AND NOT baseline", p.From, now).
		Order("created_at DESC").Find(&cos)
	ids, users := make([]uuid.UUID, 0, len(cos)), make([]string, 0, len(cos))
	for i := range cos {
		ids, users = append(ids, cos[i].ID), append(users, cos[i].UserID.String())
	}
	explained := map[uuid.UUID]int64{}
	if len(ids) > 0 {
		var sums []struct {
			ExplainsCloseout uuid.UUID
			Sum              int64
		}
		db.Model(&models.Expense{}).Select("explains_closeout, SUM(gross_grosz) AS sum").
			Where("explains_closeout IN ? AND storno_at IS NULL", ids).Group("explains_closeout").Scan(&sums)
		for _, s := range sums {
			explained[s.ExplainsCloseout] = s.Sum
		}
	}
	names := nicknamesByID(users)
	items := make([]gin.H, 0, len(cos))
	open := 0
	var short, over int64
	for i := range cos {
		co := &cos[i]
		r := closeoutOut(co)
		r["nickname"] = names[co.UserID.String()]
		if x := explained[co.ID]; x > 0 {
			r["explained_pln"] = models.PLNFromGrosz(x)
		}
		left := cashDiffLeft(*co.DiffGrosz, explained[co.ID])
		r["open"] = left != 0
		switch {
		case left < 0:
			short, open = short-left, open+1
		case left > 0:
			over, open = over+left, open+1
		}
		items = append(items, r)
	}
	return gin.H{"from": p.FromDay.Format("2006-01-02"), "count": len(items), "open": open,
		"short_pln": models.PLNFromGrosz(short), "over_pln": models.PLNFromGrosz(over), "items": items}
}

// cashDiffLeft — сколько расхождения сдачи осталось необъяснённым (чистая,
// тест): недостачу закрывают расходы «из кассы», внесённые задним числом до
// этой сдачи, — объяснили больше недостачи, значит объяснено, а не излишек;
// излишек расходом не объяснить.
func cashDiffLeft(diff, explained int64) int64 {
	if diff >= 0 {
		return diff
	}
	return min(diff+explained, 0)
}

// POST /admin/staff {nickname} — назначить гостя админом (owner).
func handleAdminStaffPromote(c *gin.Context) {
	var req struct {
		Nickname string `json:"nickname" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_request", "message": "Нужен ник"})
		return
	}
	var user models.User
	if err := db.First(&user, "nickname = ?", strings.TrimSpace(req.Nickname)).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "user_not_found", "message": "Гость с таким ником не найден"})
		return
	}
	if ok, code := canChangeStaffRole(user.Role, user.ID.String(), c.GetString("user_id"), true); !ok {
		c.JSON(http.StatusConflict, gin.H{"code": code, "message": staffRoleErrors[code]})
		return
	}
	if user.Status == models.UserStatusBanned { // З15-и5 (№30): забаненного админка всё равно не пустит — сначала бан
		c.JSON(http.StatusConflict, gin.H{"code": "banned",
			"message": "«" + user.Nickname + "» забанен — сначала сними бан («Гости» → карточка → «Разбанить»)"})
		return
	}
	// человек мог у нас уже работать — увольнение с карточки снимаем, иначе он останется в архиве уволенных, работая
	// на смене; З15-и5: прежний период остаётся в истории, новый начинается сегодня — в клубные сутки, как и дата
	// увольнения без даты (раньше увольнение просто стиралось)
	now := time.Now()
	today := clubDayOf(now, clubDayHour())
	var p models.StaffProfile
	hasCard := db.First(&p, "user_id = ?", user.ID).Error == nil
	returned := hasCard && p.DismissedAt != nil
	start := &today
	if hasCard && !returned && p.HiredAt != nil {
		start = p.HiredAt // сняли с прав и назначили снова — период тот же
	}
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&user).Update("role", models.UserRoleAdmin).Error; err != nil {
			return err
		}
		if returned {
			if err := tx.Model(&models.StaffProfile{}).Where("user_id = ?", user.ID).
				Updates(map[string]any{"dismissed_at": nil, "hired_at": today, "updated_at": now}).Error; err != nil {
				return err
			}
		}
		return openPeriod(tx, user.ID, start)
	})
	if err != nil {
		respondDBError(c, err)
		return
	}

	target := user.ID
	note := " → admin"
	if returned {
		note = " → admin (вернулся)"
	}
	logAdminAction(c, "staff_promote", &target, user.Nickname+note)
	c.JSON(http.StatusOK, gin.H{"user_id": user.ID, "nickname": user.Nickname, "role": models.UserRoleAdmin, "returned": returned})
}

// futureShiftsFrom — с какой даты чистить график при снятии сотрудника.
// Сегодняшнюю смену не трогаем: человек её уже отработал (или дорабатывает),
// и «работали: …» в сводке смены должно остаться честным. Чистим завтра и
// дальше. Чистая функция (тест в staff_test.go).
func futureShiftsFrom(now time.Time, reportHour int) time.Time {
	return clubDayOf(now, reportHour).AddDate(0, 0, 1)
}

// clearFutureShifts — снять сотрудника со всех будущих смен. Возвращает,
// сколько записей графика убрано (владельцу важно это увидеть в тосте).
func clearFutureShifts(userID uuid.UUID) int64 {
	from := futureShiftsFrom(time.Now(), clubDayHour())
	res := db.Where("user_id = ? AND date >= ?", userID, from).Delete(&models.ShiftAssignment{})
	return res.RowsAffected
}

// DELETE /admin/staff/:id — снять админа до гостя (owner).
func handleAdminStaffDemote(c *gin.Context) {
	var user models.User
	if err := db.First(&user, "id = ?", c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "user_not_found", "message": "Пользователь не найден"})
		return
	}
	if ok, code := canChangeStaffRole(user.Role, user.ID.String(), c.GetString("user_id"), false); !ok {
		c.JSON(http.StatusConflict, gin.H{"code": code, "message": staffRoleErrors[code]})
		return
	}
	// З14-и1 (решение основателя 16.10): снятие прав посреди смены — как увольнение: смена закрывается моментом
	// ухода той же транзакцией, владельцу в канал — что осталось за человеком.
	now := time.Now()
	shift := leaveShiftOf(user.ID, now)
	actor, _ := uuid.Parse(c.GetString("user_id"))
	closed := false
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&user).Update("role", models.UserRolePlayer).Error; err != nil {
			return err
		}
		var err error
		closed, err = leaveCloseShift(tx, shift, demoteNote, actor, now)
		return err
	})
	if err != nil {
		respondDBError(c, err)
		return
	}
	dropAdminWS(user.ID) // З0-и1: роль из базы — доступ уже закрыт, рвём и открытую вкладку
	// БАГ до этого фикса: снятый админ оставался висеть в будущих сменах —
	// назначение проверяет роль только в момент постановки, а снятие график
	// не трогало, и в сетке недели стоял человек, который тут больше не работает.
	removed := clearFutureShifts(user.ID)

	target := user.ID
	details := user.Nickname + " → player"
	if closed {
		details += ", " + leaveShiftText(shift.Entry, now)
	}
	if removed > 0 {
		details += fmt.Sprintf(", снят с %d будущих смен", removed)
	}
	logAdminAction(c, "staff_demote", &target, details)
	if closed {
		leaveNotify(user.Nickname, "снят с прав", shift, actor, now)
	}
	resp := gin.H{"user_id": user.ID, "nickname": user.Nickname, "role": models.UserRolePlayer, "removed_shifts": removed}
	if closed {
		resp["closed_shift"] = workOut(shift.Entry)
	}
	c.JSON(http.StatusOK, resp)
}

// ── Кадровая карточка (В3-этап 2, миграция 025) ───────────────────────
// Карточку видит и правит ТОЛЬКО владелец: маршруты стоят за ownerMiddleware.
// В строки аудита личные данные не попадают — там остаётся ник и перечень
// изменённых полей; исключение сделано для ставки, это деньги владельца и
// история их изменения ему нужна.

var rateTypes = map[string]string{
	"none":  "не задана",
	"hour":  "за час",
	"shift": "за смену",
	"month": "оклад",
}

// validateStaffProfile — чистая проверка карточки (тест в staff_test.go).
func validateStaffProfile(fullName, phone, position, note, rateType string, rateAmount float64,
	hired, dismissed *time.Time) (bool, string) {
	if len([]rune(fullName)) > 128 {
		return false, "bad_name"
	}
	if len([]rune(phone)) > 32 {
		return false, "bad_phone"
	}
	if len([]rune(position)) > 64 {
		return false, "bad_position"
	}
	if len([]rune(note)) > 2000 {
		return false, "bad_note"
	}
	if _, ok := rateTypes[rateType]; !ok {
		return false, "bad_rate_type"
	}
	if rateAmount < 0 || rateAmount > 1000000 {
		return false, "bad_rate"
	}
	if rateType != "none" && rateAmount <= 0 {
		return false, "rate_needed"
	}
	if rateType == "none" && rateAmount != 0 {
		return false, "rate_extra"
	}
	if hired != nil && dismissed != nil && dismissed.Before(*hired) {
		return false, "bad_dates"
	}
	return true, ""
}

var profileErrors = map[string]string{
	"bad_name":      "ФИО — до 128 символов",
	"bad_phone":     "Телефон — в международном формате: + и 6–15 цифр", // З15-и5: как у анкеты гостя
	"bad_position":  "Должность — до 64 символов",
	"bad_note":      "Заметка — до 2000 символов",
	"bad_rate_type": "Ставка: не задана, за час, за смену или оклад",
	"bad_rate":      "Сумма ставки — от 0 до 1 000 000",
	"rate_needed":   "Выбран тип ставки — нужна сумма",
	"rate_extra":    "Ставка не задана — сумма должна быть пустой",
	"bad_dates":     "Дата увольнения раньше даты найма",
	"bad_date":      "Дата — в формате ГГГГ-ММ-ДД",
	// З15-и5 (№30): дату увольнения ставит «Уволить» — с причиной, сменами и периодом работы
	"dismissed_readonly": "Дату увольнения ставит «Уволить» — в карточке её не правят",
}

// staffPhone — телефон карточки (чистая, тест; З15-и5, №30): пусто — пусто; новый — как у анкеты гостя (validatePhone:
// «+» и 6–15 цифр, пробелы, скобки и дефисы срезаются); прежний, не тронутый правкой, — как есть, даже если записан до
// этой проверки свободным текстом: иначе карточку с таким телефоном нельзя было бы сохранить, не трогая телефон.
func staffPhone(raw, old string) (string, bool) {
	s := strings.TrimSpace(raw)
	if s == "" || s == old {
		return s, true
	}
	return validatePhone(s)
}

// parseDateOpt — «» → nil, «2026-08-18» → дата. Второе значение — валидность.
func parseDateOpt(s string) (*time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, true
	}
	d, err := time.ParseInLocation("2006-01-02", s, time.Now().Location())
	if err != nil {
		return nil, false
	}
	return &d, true
}

func dateOut(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format("2006-01-02")
}

func profileOut(p *models.StaffProfile) gin.H {
	return gin.H{
		"full_name": p.FullName, "phone": p.Phone, "position": p.Position,
		"hired_at": dateOut(p.HiredAt), "dismissed_at": dateOut(p.DismissedAt),
		"rate_type": p.RateType, "rate_amount": p.RateAmount,
		"rate_label": rateTypes[p.RateType], "note": p.Note,
		"leave_on": dateOut(p.LeaveOn), // З14-и2: последний рабочий день, если увольнение отложенное
	}
}

// GET /admin/staff/:id/profile — карточка сотрудника (owner).
func handleAdminStaffProfileGet(c *gin.Context) {
	var user models.User
	if err := db.First(&user, "id = ?", c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "user_not_found", "message": "Пользователь не найден"})
		return
	}
	var p models.StaffProfile
	if err := db.First(&p, "user_id = ?", user.ID).Error; err != nil {
		p = models.StaffProfile{UserID: user.ID, RateType: "none"} // карточки ещё нет — пустая
	}
	c.JSON(http.StatusOK, gin.H{
		"nickname": user.Nickname, "role": user.Role, "status": user.Status,
		"registered_at": user.RegisteredAt, "profile": profileOut(&p),
		"periods": periodsOut([]uuid.UUID{user.ID})[user.ID], // З15-и5: все периоды работы
	})
}

type staffProfileRequest struct {
	FullName    string   `json:"full_name"`
	Phone       string   `json:"phone"`
	Position    string   `json:"position"`
	HiredAt     string   `json:"hired_at"`
	DismissedAt string   `json:"dismissed_at"`
	RateType    string   `json:"rate_type"`
	RateAmount  *float64 `json:"rate_amount"`
	Note        string   `json:"note"`
}

// PUT /admin/staff/:id/profile — завести или обновить карточку (owner).
func handleAdminStaffProfilePut(c *gin.Context) {
	var user models.User
	if err := db.First(&user, "id = ?", c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "user_not_found", "message": "Пользователь не найден"})
		return
	}
	var old models.StaffProfile
	existed := db.First(&old, "user_id = ?", user.ID).Error == nil
	// З14-и4 (№26; З-Р19): карточку уволенного (он в «Уволенных») правят без смены роли — например, стереть лишнее
	archived := existed && old.DismissedAt != nil && !roleIsStaff(string(user.Role))
	if !roleIsStaff(string(user.Role)) && !archived {
		c.JSON(http.StatusConflict, gin.H{"code": "not_staff",
			"message": "Карточка заводится сотруднику — сначала назначь админом"})
		return
	}
	var req staffProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_request", "message": "Не разобрал карточку"})
		return
	}
	hired, ok := parseDateOpt(req.HiredAt)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"code": "bad_date", "message": profileErrors["bad_date"]})
		return
	}
	dismissed, ok := parseDateOpt(req.DismissedAt)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"code": "bad_date", "message": profileErrors["bad_date"]})
		return
	}
	// З15-и5 (№30): дату увольнения карточка не правит — её ставит «Уволить» (роль, смены, период работы). Раньше PUT с
	// dismissed_at отправлял работающего админа в «Уволенные», не снимая роли. Та же дата — не правка; без даты — как
	// есть (форма карточки её не шлёт, З14-и4: из архива человек не уходит).
	if dismissed != nil && dateOut(dismissed) != dateOut(old.DismissedAt) {
		c.JSON(http.StatusBadRequest, gin.H{"code": "dismissed_readonly", "message": profileErrors["dismissed_readonly"]})
		return
	}
	dismissed = old.DismissedAt
	rateType := strings.TrimSpace(req.RateType)
	if rateType == "" {
		rateType = "none"
	}
	rate := 0.0
	if req.RateAmount != nil {
		rate = *req.RateAmount
	}
	name := strings.TrimSpace(req.FullName)
	phone, okPhone := staffPhone(req.Phone, old.Phone)
	if !okPhone {
		c.JSON(http.StatusBadRequest, gin.H{"code": "bad_phone", "message": profileErrors["bad_phone"]})
		return
	}
	position := strings.TrimSpace(req.Position)
	note := strings.TrimSpace(req.Note)
	if valid, code := validateStaffProfile(name, phone, position, note, rateType, rate, hired, dismissed); !valid {
		c.JSON(http.StatusBadRequest, gin.H{"code": code, "message": profileErrors[code]})
		return
	}

	p := models.StaffProfile{
		UserID: user.ID, FullName: name, Phone: phone, Position: position,
		HiredAt: hired, DismissedAt: dismissed, RateType: rateType, RateAmount: rate, Note: note,
		UpdatedAt: time.Now(),
	}
	if existed {
		p.CreatedAt = old.CreatedAt
		p.LeaveOn, p.LeaveBy, p.LeaveReason = old.LeaveOn, old.LeaveBy, old.LeaveReason // З14-и2: дату ухода карточка не правит — в ответе как есть
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		if existed {
			if err := tx.Model(&models.StaffProfile{}).Where("user_id = ?", user.ID).
				Updates(map[string]any{
					"full_name": name, "phone": phone, "position": position,
					"hired_at": hired, "dismissed_at": dismissed,
					"rate_type": rateType, "rate_amount": rate, "note": note, "updated_at": p.UpdatedAt,
				}).Error; err != nil {
				return err
			}
		} else if err := tx.Create(&p).Error; err != nil {
			return err
		}
		return syncPeriodStart(tx, user.ID, hired, dismissed, p.UpdatedAt) // З15-и5: «Работает с» — начало последнего периода
	}); err != nil {
		respondDBError(c, err)
		return
	}

	// в аудит — только перечень изменённых полей; значения ФИО и телефона
	// не дублируем, а ставку пишем числами: это деньги владельца
	changed := []string{}
	if !existed {
		changed = append(changed, "карточка заведена")
	} else {
		if old.FullName != name {
			changed = append(changed, "ФИО")
		}
		if old.Phone != phone {
			changed = append(changed, "телефон")
		}
		if old.Position != position {
			changed = append(changed, "должность: "+position)
		}
		if dateOut(old.HiredAt) != dateOut(hired) {
			changed = append(changed, "дата найма: "+dateOut(hired))
		}
		if dateOut(old.DismissedAt) != dateOut(dismissed) {
			changed = append(changed, "дата увольнения: "+dateOut(dismissed))
		}
		if old.RateType != rateType || old.RateAmount != rate {
			changed = append(changed, fmt.Sprintf("ставка: %s %.2f → %s %.2f",
				rateTypes[old.RateType], old.RateAmount, rateTypes[rateType], rate))
		}
		if old.Note != note {
			changed = append(changed, "заметка")
		}
	}
	if len(changed) > 0 {
		target := user.ID
		logAdminAction(c, "staff_card", &target, user.Nickname+": "+strings.Join(changed, ", "))
	}
	c.JSON(http.StatusOK, gin.H{"nickname": user.Nickname, "profile": profileOut(&p)})
}

// ── Наём и увольнение (В3-этап 3) ─────────────────────────────────────
// До этого этапа нанять можно было только того, кто уже сам зарегистрировался
// гостем на киоске: владелец находил его по нику и повышал. Теперь сотрудника
// заводят из админки целиком — аккаунт и карточка одной операцией.
//
// Увольнение — событие, а не «снять роль»: пишется дата и причина, человек
// уходит из будущих смен, а его аккаунт остаётся жить гостевым (уволенный
// админ вполне может приходить играть). Уволенные видны в архиве.

// validateHire — чистая проверка данных нового сотрудника (тест в staff_test.go).
// Правила ника и пароля те же, что при обычной регистрации (main.go), плюс
// символьный инвариант ника из QA Б9–Б11.
func validateHire(nickname, password string) (bool, string) {
	n := strings.TrimSpace(nickname)
	if len([]rune(n)) < 3 || len([]rune(n)) > 32 {
		return false, "bad_nickname"
	}
	if !nicknameSafe(n) {
		return false, "nickname_charset"
	}
	if len(password) < 6 || len(password) > 72 {
		return false, "bad_password"
	}
	return true, ""
}

var hireErrors = map[string]string{
	"bad_nickname":     "Ник сотрудника — от 3 до 32 символов",
	"nickname_charset": "В нике нельзя кавычки, угловые скобки и спецсимволы",
	"bad_password":     "Пароль — от 6 до 72 символов",
	"already_exists":   "Такой ник уже занят",
}

type hireRequest struct {
	Nickname string `json:"nickname"`
	Password string `json:"password"`
	staffProfileRequest
}

// POST /admin/staff/hire — завести сотрудника с нуля (owner): аккаунт с ролью
// admin и кадровая карточка одной транзакцией.
func handleAdminStaffHire(c *gin.Context) {
	var req hireRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_request", "message": "Не разобрал данные сотрудника"})
		return
	}
	if ok, code := validateHire(req.Nickname, req.Password); !ok {
		c.JSON(http.StatusBadRequest, gin.H{"code": code, "message": hireErrors[code]})
		return
	}
	hired, ok := parseDateOpt(req.HiredAt)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"code": "bad_date", "message": profileErrors["bad_date"]})
		return
	}
	rateType := strings.TrimSpace(req.RateType)
	if rateType == "" {
		rateType = "none"
	}
	rate := 0.0
	if req.RateAmount != nil {
		rate = *req.RateAmount
	}
	name := strings.TrimSpace(req.FullName)
	phone, okPhone := staffPhone(req.Phone, "") // З15-и5: телефон — как у анкеты гостя
	if !okPhone {
		c.JSON(http.StatusBadRequest, gin.H{"code": "bad_phone", "message": profileErrors["bad_phone"]})
		return
	}
	position := strings.TrimSpace(req.Position)
	note := strings.TrimSpace(req.Note)
	if valid, code := validateStaffProfile(name, phone, position, note, rateType, rate, hired, nil); !valid {
		c.JSON(http.StatusBadRequest, gin.H{"code": code, "message": profileErrors[code]})
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "hash_error", "message": "Не удалось обработать пароль"})
		return
	}
	now := time.Now()
	user := newStaffUser(strings.TrimSpace(req.Nickname), string(hash), now)
	if hired == nil {
		today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		hired = &today
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&user).Error; err != nil {
			return err
		}
		if err := tx.Create(&models.StaffProfile{
			UserID: user.ID, FullName: name, Phone: phone, Position: position,
			HiredAt: hired, RateType: rateType, RateAmount: rate, Note: note,
		}).Error; err != nil {
			return err
		}
		return openPeriod(tx, user.ID, hired) // З15-и5: первый период работы
	})
	if err != nil {
		if isDuplicate(err) {
			c.JSON(http.StatusConflict, gin.H{"code": "already_exists", "message": hireErrors["already_exists"]})
			return
		}
		respondDBError(c, err)
		return
	}
	target := user.ID
	details := user.Nickname
	if position != "" {
		details += " · " + position
	}
	logAdminAction(c, "staff_hire", &target, details+" · с "+dateOut(hired))
	c.JSON(http.StatusCreated, gin.H{"user_id": user.ID, "nickname": user.Nickname, "role": user.Role})
}

// newStaffUser — аккаунт нанятого (чистая, тест). З14-и3 (З-Р17): пароль от найма знает владелец — при первом входе
// админка попросит задать свой (must_change_password), до того ручки админки отвечают must_change_password.
func newStaffUser(nick, hash string, now time.Time) models.User {
	return models.User{
		Nickname: nick, PasswordHash: hash,
		Status: models.UserStatusActive, Role: models.UserRoleAdmin, Level: 1,
		RegisteredAt: now, LastActiveAt: now, MustChangePassword: true,
	}
}

type dismissRequest struct {
	Date   string `json:"date"`
	Reason string `json:"reason"`
}

// POST /admin/staff/:id/dismiss — уволить сотрудника (owner).
// Роль уходит в player, аккаунт остаётся: уволенный админ может приходить
// играть гостем, и его история сессий не должна осиротеть.
func handleAdminStaffDismiss(c *gin.Context) {
	var user models.User
	if err := db.First(&user, "id = ?", c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "user_not_found", "message": "Пользователь не найден"})
		return
	}
	if ok, code := canChangeStaffRole(user.Role, user.ID.String(), c.GetString("user_id"), false); !ok {
		c.JSON(http.StatusConflict, gin.H{"code": code, "message": staffRoleErrors[code]})
		return
	}
	var req dismissRequest
	_ = c.ShouldBindJSON(&req)
	date, ok := parseDateOpt(req.Date)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"code": "bad_date", "message": profileErrors["bad_date"]})
		return
	}
	now := time.Now()
	today := clubDayOf(now, clubDayHour()) // З14-и2: «сегодня» — клубные сутки, ночью это вчера
	if date == nil {
		date = &today
	}
	var p models.StaffProfile
	hasCard := db.First(&p, "user_id = ?", user.ID).Error == nil
	if hasCard && p.HiredAt != nil && date.Before(*p.HiredAt) {
		c.JSON(http.StatusBadRequest, gin.H{"code": "bad_dates", "message": profileErrors["bad_dates"]})
		return
	}
	reason := strings.TrimSpace(req.Reason)
	if len([]rune(reason)) > 200 {
		c.JSON(http.StatusBadRequest, gin.H{"code": "bad_reason", "message": "Причина — до 200 символов"})
		return
	}
	actor, _ := uuid.Parse(c.GetString("user_id"))
	if leaveDeferred(*date, today) { // З14-и2: последний день позже сегодняшнего — увольнение отложенное
		scheduleLeave(c, &user, hasCard, *date, reason, actor, now)
		return
	}

	// З14-и1 (№24; решение основателя 16.10): открытая смена увольняемого закрывается моментом ухода той же
	// транзакцией, что снимает роль; кассу за него не сдаём — владельцу в канал, что осталось (leaveNotify).
	shift := leaveShiftOf(user.ID, now)
	closed := false
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&user).Update("role", models.UserRolePlayer).Error; err != nil {
			return err
		}
		if hasCard { // З14-и2: назначенное раньше отложенное увольнение — больше не нужно
			if err := tx.Model(&models.StaffProfile{}).Where("user_id = ?", user.ID).Updates(map[string]any{"dismissed_at": date,
				"leave_on": nil, "leave_by": nil, "leave_reason": "", "updated_at": time.Now()}).Error; err != nil {
				return err
			}
		} else {
			// карточки не было — заводим минимальную, чтобы человек попал в архив
			if err := tx.Create(&models.StaffProfile{UserID: user.ID, DismissedAt: date, RateType: "none"}).Error; err != nil {
				return err
			}
		}
		var hired *time.Time
		if hasCard {
			hired = p.HiredAt
		}
		if err := closePeriod(tx, user.ID, hired, date, reason, now); err != nil { // З15-и5: период — с датой и причиной
			return err
		}
		var err error
		closed, err = leaveCloseShift(tx, shift, dismissNote, actor, now)
		return err
	})
	if err != nil {
		respondDBError(c, err)
		return
	}
	dropAdminWS(user.ID) // З0-и1: роль из базы — доступ уже закрыт, рвём и открытую вкладку
	removed := clearFutureShifts(user.ID)

	target := user.ID
	details := user.Nickname + " · уволен " + dateOut(date)
	if reason != "" {
		details += " · " + reason
	}
	if closed {
		details += ", " + leaveShiftText(shift.Entry, now)
	}
	if removed > 0 {
		details += fmt.Sprintf(", снят с %d будущих смен", removed)
	}
	logAdminAction(c, "staff_dismiss", &target, details)
	if closed {
		leaveNotify(user.Nickname, "уволен", shift, actor, now)
	}
	resp := gin.H{"user_id": user.ID, "nickname": user.Nickname, "dismissed_at": dateOut(date), "removed_shifts": removed}
	if closed {
		resp["closed_shift"] = workOut(shift.Entry)
	}
	c.JSON(http.StatusOK, resp)
}

// ── З14-и1 (OWNER.md, №24; З-Р18, уточнено основателем 16.10): уход посреди смены ────────────────────────────
//
// Смена уволенного оставалась открытой навсегда: «открытых смен: 1» в табеле, «на смене» в шапке, минуты сводки
// росли — а сам он её уже не закроет, доступа нет. Увольнение и снятие прав теперь закрывают его открытую запись
// табеля моментом ухода — автозакрытой с пометкой: часы не подтверждены и в выплату не идут, пока владелец их не
// поправит. Кассу за него система не сдаёт: уволенного на месте подменяют, остальное доделают за него (решение
// основателя 16.10) — владельцу в канал строка, что осталось: пересчитать ящик (сколько в нём должно быть) и проверить
// часы. Шторка увольнения и карточка сотрудника показывают это заранее (GET /admin/staff/:id/shift).

// пометки записи табеля, закрытой уходом из персонала
const (
	dismissNote = "закрыта увольнением"
	demoteNote  = "закрыта снятием прав"
)

// leaveShift — открытая смена сотрудника и чем её закроет уход из персонала.
type leaveShift struct {
	Entry   *models.WorkEntry // nil — смены нет; имя смены — снимок в записи (З15-и4)
	SameDay bool              // смена этих клубных суток: уходит посреди неё
	CloseAt time.Time         // момент закрытия
}

// leaveShiftOf — открытая смена сотрудника на момент now и момент, которым её закроет уход.
func leaveShiftOf(userID uuid.UUID, now time.Time) leaveShift {
	w := openEntryFor(userID)
	if w == nil {
		return leaveShift{}
	}
	today := clubDayOf(now, clubDayHour())
	s := leaveShift{Entry: w, SameDay: w.Date.Format("2006-01-02") == today.Format("2006-01-02")}
	var sched *time.Time
	if w.ShiftID != nil {
		var sh models.Shift
		if db.First(&sh, "id = ?", *w.ShiftID).Error == nil {
			end := shiftEndAt(w.Date, sh.StartMin, sh.EndMin)
			sched = &end
		}
	}
	s.CloseAt = leaveCloseAt(w.StartedAt, s.SameDay, sched, now)
	return s
}

// leaveCloseAt — каким моментом уход закрывает открытую смену (чистая, тест). Смена этих клубных суток — сейчас:
// работать он перестал в момент увольнения. Давняя — уход не отметил, а уволили потом — как closeForgotten: конец по
// графику, но не позже now; без графика — начало, ноль часов. Раньше начала — никогда.
func leaveCloseAt(start time.Time, sameDay bool, sched *time.Time, now time.Time) time.Time {
	end := start
	switch {
	case sameDay:
		end = now
	case sched != nil:
		end = *sched
		if end.After(now) {
			end = now
		}
	}
	if end.Before(start) {
		return start
	}
	return end
}

// leaveCloseShift — в транзакции ухода закрыть открытую смену моментом s.CloseAt: запись табеля — автозакрытая с
// пометкой note, правил — by. Смены нет или её успели закрыть уходом — false.
func leaveCloseShift(tx *gorm.DB, s leaveShift, note string, by uuid.UUID, now time.Time) (bool, error) {
	w := s.Entry
	if w == nil {
		return false, nil
	}
	end := s.CloseAt
	mins := workMinutes(w.StartedAt, end)
	res := tx.Model(&models.WorkEntry{}).Where("id = ? AND ended_at IS NULL", w.ID).Updates(map[string]any{
		"ended_at": end, "minutes": mins, "auto_closed": true, "note": note, "edited_by": by, "updated_at": now,
	})
	if res.Error != nil || res.RowsAffected == 0 {
		return false, res.Error
	}
	w.EndedAt, w.Minutes, w.AutoClosed, w.Note = &end, mins, true, note
	return true, nil
}

// leaveSince — с какого момента была открыта смена (чистая, тест): «14:08», если в тот же день, что уход (at), иначе
// «12.10 14:08». Время — по часам клуба.
func leaveSince(start, at time.Time) string {
	s, a := start.In(models.ClubLocation), at.In(models.ClubLocation)
	if s.Format("2006-01-02") == a.Format("2006-01-02") {
		return s.Format("15:04")
	}
	return s.Format("02.01 15:04")
}

// leaveShiftText — что уход в момент at сделал со сменой, для строки журнала (чистая, тест).
func leaveShiftText(w *models.WorkEntry, at time.Time) string {
	return "смена с " + leaveSince(w.StartedAt, at) + " закрыта (" + hoursRU(int64(w.Minutes)) + " — на проверку), касса не сдана"
}

// leaveOwnerLine — строка владельцу, что осталось за ушедшим (чистая, тест): верб — «уволен» или «снят с прав»; касса
// не сдана — сколько должно быть в ящике (или что ящика ещё нет); часы — чем закрыта смена и сколько их на проверку.
func leaveOwnerLine(nick, verb string, w *models.WorkEntry, sameDay bool, at time.Time, drawer bool, expected int64) string {
	if nick == "" {
		nick = "Сотрудник"
	}
	since := leaveSince(w.StartedAt, at)
	line := fmt.Sprintf("⚠ %s %s посреди смены (с %s) — осталось за него: ", nick, verb, since)
	if !sameDay {
		line = fmt.Sprintf("⚠ %s %s, а смена с %s осталась незакрытой — осталось за него: ", nick, verb, since)
	}
	if drawer {
		line += fmt.Sprintf("касса не сдана — в ящике должно быть %.2f zł, пересчитайте (расхождение до пересчёта ляжет на "+
			"следующую сдачу)", models.PLNFromGrosz(expected))
	} else {
		line += "касса не сдана — ящик ещё не заведён: пересчитанное станет его начальным остатком"
	}
	switch {
	case sameDay:
		return line + "; часы — смена " + w.Note + ", " + hoursRU(int64(w.Minutes)) + " в табеле на проверку"
	case w.Minutes > 0:
		return line + "; часы — смена закрыта по графику, " + hoursRU(int64(w.Minutes)) + " в табеле на проверку"
	}
	return line + "; часы — у смены нет графика, закрыта без часов: поставь их в табеле"
}

// leaveNotify — владельцу в канал, что осталось за ушедшим посреди смены: касса — ящик на этот момент. by — кто
// уволил: его собственная вкладка не показывает лишний тост «служебное в канале».
func leaveNotify(nick, verb string, s leaveShift, by uuid.UUID, now time.Time) {
	st := drawerAt(db, now)
	postSystem(leaveOwnerLine(nick, verb, s.Entry, s.SameDay, now, st.Point != nil, st.Expected), true,
		map[string]any{"staff_leave": true, "by": by})
}

// GET /admin/staff/:id/shift — открытая смена сотрудника на сейчас (owner, З14-и1): с какого момента, каким её закроет
// уход из персонала и сколько часов ляжет в табель; касса не сдана — сколько должно быть в ящике; сколько будущих смен
// снимется. Шторка «Уволить» и карточка («Снять права») показывают это до нажатия.
func handleAdminStaffShift(c *gin.Context) {
	var user models.User
	if err := db.First(&user, "id = ?", c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "user_not_found", "message": "Пользователь не найден"})
		return
	}
	now := time.Now()
	var future int64
	db.Model(&models.ShiftAssignment{}).Where("user_id = ? AND date >= ?", user.ID,
		futureShiftsFrom(now, clubDayHour())).Count(&future)
	out := gin.H{"user_id": user.ID, "nickname": user.Nickname, "role": user.Role, "future_shifts": future, "open_shift": nil,
		"club_today": clubDayOf(now, clubDayHour()).Format("2006-01-02")} // З14-и2: позже — увольнение отложенное
	var lp []models.StaffProfile
	if db.Where("user_id = ? AND leave_on IS NOT NULL", user.ID).Limit(1).Find(&lp); len(lp) == 1 {
		out["leave_on"] = dateOut(lp[0].LeaveOn) // уже назначено — новая дата заменит
	}
	if s := leaveShiftOf(user.ID, now); s.Entry != nil {
		row := workOut(s.Entry)
		row["same_day"], row["close_at"], row["close_minutes"] = s.SameDay, s.CloseAt, workMinutes(s.Entry.StartedAt, s.CloseAt)
		out["open_shift"] = row
		st := drawerAt(db, now) // как «сколько должно быть в ящике» перед уходом (GET /admin/work/closeout)
		out["cash"] = gin.H{"drawer": st.Point != nil, "expected_pln": models.PLNFromGrosz(st.Expected)}
	}
	c.JSON(http.StatusOK, out)
}

// GET /admin/staff/archive — уволенные (owner): кто, кем был и когда ушёл.
func handleAdminStaffArchive(c *gin.Context) {
	var profiles []models.StaffProfile
	db.Where("dismissed_at IS NOT NULL").Order("dismissed_at DESC").Find(&profiles)
	ids := make([]string, 0, len(profiles))
	for _, p := range profiles {
		ids = append(ids, p.UserID.String())
	}
	nick := nicknamesByID(ids)
	uids := make([]uuid.UUID, 0, len(profiles))
	for _, p := range profiles {
		uids = append(uids, p.UserID)
	}
	periods := periodsOut(uids) // З15-и5: все периоды работы, не только последний
	out := make([]gin.H, 0, len(profiles))
	for i := range profiles {
		p := profiles[i]
		out = append(out, gin.H{
			"id": p.UserID, "nickname": nick[p.UserID.String()],
			"full_name": p.FullName, "position": p.Position,
			"hired_at": dateOut(p.HiredAt), "dismissed_at": dateOut(p.DismissedAt),
			"periods": periods[p.UserID],
		})
	}
	c.JSON(http.StatusOK, gin.H{"count": len(out), "staff": out})
}

// ── З15-и5 (OWNER.md, №30; З-Р20): периоды работы ───────────────────────────────────────────────────────────────
//
// Карточка хранила одну пару «нанят — уволен», и повторное назначение бывшего сотрудника стирало увольнение: человек
// уходил из «Уволенных», а то, что он уже работал и уходил (когда и почему), оставалось только в журнале. Теперь у
// каждого периода своя строка (staff_periods, миграция 089): наём и назначение открывают период, увольнение (сразу или
// отложенное) закрывает его датой и причиной, правка даты найма в карточке двигает начало последнего; «Уволенные» и
// карточка показывают все периоды. Карточка по-прежнему хранит последний — с неё читают список персонала и выплаты.

// openPeriod — начать период работы с дня day, если открытого нет (наём, назначение, возвращение).
func openPeriod(tx *gorm.DB, userID uuid.UUID, day *time.Time) error {
	var n int64
	if err := tx.Model(&models.StaffPeriod{}).Where("user_id = ? AND dismissed_on IS NULL", userID).Count(&n).Error; err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	return tx.Create(&models.StaffPeriod{UserID: userID, HiredOn: day}).Error
}

// closePeriod — закрыть открытый период датой увольнения и причиной. Открытого нет (работал без карточки до 089 или
// вернулся в обход «Назначить») — закрытый период с датой найма из карточки.
func closePeriod(tx *gorm.DB, userID uuid.UUID, hired, day *time.Time, reason string, now time.Time) error {
	res := tx.Model(&models.StaffPeriod{}).Where("user_id = ? AND dismissed_on IS NULL", userID).
		Updates(map[string]any{"dismissed_on": day, "reason": reason, "updated_at": now})
	if res.Error != nil || res.RowsAffected > 0 {
		return res.Error
	}
	return tx.Create(&models.StaffPeriod{UserID: userID, HiredOn: hired, DismissedOn: day, Reason: reason}).Error
}

// syncPeriodStart — дата найма из карточки — начало последнего периода: у работающего — открытого (нет — открыть), у
// уволенного — последнего закрытого (нет — завести закрытый).
func syncPeriodStart(tx *gorm.DB, userID uuid.UUID, hired, dismissed *time.Time, now time.Time) error {
	if dismissed == nil {
		res := tx.Model(&models.StaffPeriod{}).Where("user_id = ? AND dismissed_on IS NULL", userID).
			Updates(map[string]any{"hired_on": hired, "updated_at": now})
		if res.Error != nil || res.RowsAffected > 0 {
			return res.Error
		}
		return tx.Create(&models.StaffPeriod{UserID: userID, HiredOn: hired}).Error
	}
	var last []models.StaffPeriod
	if err := tx.Where("user_id = ? AND dismissed_on IS NOT NULL", userID).
		Order("dismissed_on DESC, created_at DESC").Limit(1).Find(&last).Error; err != nil {
		return err
	}
	if len(last) == 1 {
		return tx.Model(&models.StaffPeriod{}).Where("id = ?", last[0].ID).
			Updates(map[string]any{"hired_on": hired, "updated_at": now}).Error
	}
	return tx.Create(&models.StaffPeriod{UserID: userID, HiredOn: hired, DismissedOn: dismissed}).Error
}

// periodsOut — периоды работы для ответа, по порядку: с какого дня, по какой («» — работает сейчас), причина.
func periodsOut(userIDs []uuid.UUID) map[uuid.UUID][]gin.H {
	out := make(map[uuid.UUID][]gin.H, len(userIDs))
	for _, id := range userIDs {
		out[id] = []gin.H{}
	}
	if len(userIDs) == 0 {
		return out
	}
	var ps []models.StaffPeriod
	db.Where("user_id IN ?", userIDs).Order("created_at, hired_on").Find(&ps)
	for i := range ps {
		p := ps[i]
		out[p.UserID] = append(out[p.UserID], gin.H{
			"hired_at": dateOut(p.HiredOn), "dismissed_at": dateOut(p.DismissedOn), "reason": p.Reason,
		})
	}
	return out
}

// ── З14-и4 (OWNER.md, №26; З-Р19): личные данные уволенного ─────────────────────────────────────────────────────
//
// Карточка уволенного хранила ФИО, телефон, заметку и ставку вечно, журнал — причину увольнения, и стереть их было нельзя
// даже вручную: карточка из «Уволенных» не сохранялась (not_staff). Теперь её правят, а владелец может стереть личные
// данные разом: ФИО, телефон, заметка и ставка — пусто, причина в строках журнала об увольнении и уходе — «—».
// Автоочистки по сроку нет (решение основателя). Остаются ник и игровой аккаунт (это сам человек, гость клуба), даты
// найма и увольнения, должность, табель и выплаты — учёт клуба; строки журнала о ставке — тоже, это деньги клуба.

// eraseReasonRe — строка журнала об увольнении или уходе с причиной: «ник · уволен ДАТА[ (последний рабочий день
// прошёл)] · причина[, смена с … закрыта (… — на проверку), касса не сдана][, снят с N будущих смен]» и «ник · уходит
// ДАТА · причина[, снимется со смен после этой даты: N]» — как их пишут увольнение, отложенное увольнение и прежняя
// З14-и1 («касса без пересчёта»). Причина — всё между датой и хвостом, запятые в ней — её часть.
var eraseReasonRe = regexp.MustCompile(`^(.+? · (?:уволен \d{4}-\d{2}-\d{2}(?: \(последний рабочий день прошёл\))?|уходит \d{4}-\d{2}-\d{2})) · (.+?)` +
	`((?:, смена с [^,]+ закрыта \([^)]*\), касса (?:не сдана|без пересчёта))?(?:, снят с \d+ будущих смен)?(?:, снимется со смен после этой даты: \d+)?)$`)

// eraseReason — та же строка журнала с «—» вместо причины (чистая, тест). Причины нет, уже «—» или строка не наша —
// как была и false.
func eraseReason(details string) (string, bool) {
	m := eraseReasonRe.FindStringSubmatch(details)
	if m == nil || m[2] == "—" {
		return details, false
	}
	return m[1] + " · —" + m[3], true
}

// erasableFields — что из личного в карточке ещё не пусто (чистая, тест): по ним строка журнала и ответ.
func erasableFields(p *models.StaffProfile) []string {
	out := []string{}
	if p.FullName != "" {
		out = append(out, "ФИО")
	}
	if p.Phone != "" {
		out = append(out, "телефон")
	}
	if p.Note != "" {
		out = append(out, "заметка")
	}
	if p.RateType != "none" || p.RateAmount != 0 {
		out = append(out, "ставка")
	}
	return out
}

// POST /admin/staff/:id/erase — стереть личные данные уволенного (owner, вручную). Повтор — 200 «стирать нечего».
func handleAdminStaffErase(c *gin.Context) {
	var user models.User
	if err := db.First(&user, "id = ?", c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "user_not_found", "message": "Пользователь не найден"})
		return
	}
	var p models.StaffProfile
	if db.First(&p, "user_id = ?", user.ID).Error != nil || p.DismissedAt == nil || roleIsStaff(string(user.Role)) {
		c.JSON(http.StatusConflict, gin.H{"code": "not_dismissed", "message": "Стирают данные уволенного — в «Уволенных» его нет"})
		return
	}
	fields := erasableFields(&p)
	lines := 0
	var periods int64
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.StaffProfile{}).Where("user_id = ?", user.ID).Updates(map[string]any{"full_name": "", "phone": "",
			"note": "", "rate_type": "none", "rate_amount": 0, "leave_reason": "", "updated_at": time.Now()}).Error; err != nil {
			return err
		}
		// З15-и5: причина увольнения живёт и в периодах работы — стирается и там
		res := tx.Model(&models.StaffPeriod{}).Where("user_id = ? AND reason <> ''", user.ID).
			Updates(map[string]any{"reason": "", "updated_at": time.Now()})
		if res.Error != nil {
			return res.Error
		}
		periods = res.RowsAffected
		var rows []models.AdminAction
		if err := tx.Where("target_user_id = ? AND action IN ?", user.ID, []string{"staff_dismiss", "staff_leave"}).Find(&rows).Error; err != nil {
			return err
		}
		for _, r := range rows {
			if s, ok := eraseReason(r.Details); ok {
				if err := tx.Model(&models.AdminAction{}).Where("id = ?", r.ID).Update("details", s).Error; err != nil {
					return err
				}
				lines++
			}
		}
		return nil
	})
	if err != nil {
		respondDBError(c, err)
		return
	}
	out := gin.H{"user_id": user.ID, "nickname": user.Nickname, "erased": fields, "journal_lines": lines, "period_reasons": periods}
	if len(fields) == 0 && lines == 0 && periods == 0 {
		out["nothing"] = true // уже стёрто — второй строки журнала не пишем
		c.JSON(http.StatusOK, out)
		return
	}
	text := user.Nickname + " · личные данные стёрты"
	if len(fields) > 0 {
		text += ": " + strings.Join(fields, ", ")
	}
	if lines > 0 {
		text += fmt.Sprintf("; причина увольнения в журнале — «—» (строк: %d)", lines)
	} else if periods > 0 {
		text += "; причина увольнения стёрта"
	}
	target := user.ID
	logAdminAction(c, "staff_erase", &target, text)
	c.JSON(http.StatusOK, out)
}

// ── З14-и2 (OWNER.md, №25 «а»; З-Р18): отложенное увольнение ────────────────────────────────────────────────────
//
// «Последний рабочий день 30.12» снимал роль в ту же секунду, а в архиве стояло «работал до 30.12». Теперь дата позже
// сегодняшних клубных суток — увольнение отложенное: роль остаётся, дата ложится в карточку (leave_on, кто назначил —
// leave_by, причина — leave_reason), в списке персонала — «уходит 30.12» и «Отменить увольнение». Доступ — до конца
// того клубного дня; со следующего adminMiddleware не пускает (staffLeft), а фоновый прогон раз в 10 минут завершает
// увольнение (finishLeaves): роль — player, дата увольнения — leave_on, смены графика после неё сняты, открытая смена
// закрыта моментом ухода с уведомлением владельцу (как З14-и1а), live-каналы закрыты; строка журнала — от имени того,
// кто назначил.

// leaveDeferred — последний рабочий день позже сегодняшних клубных суток: увольнение отложенное (чистая, тест).
func leaveDeferred(day, today time.Time) bool {
	return day.Format("2006-01-02") > today.Format("2006-01-02")
}

// leaveExpired — последний рабочий день прошёл: сегодня — уже следующие клубные сутки или позже (чистая, тест).
func leaveExpired(leaveOn *time.Time, today time.Time) bool {
	return leaveOn != nil && leaveOn.Format("2006-01-02") < today.Format("2006-01-02")
}

// staffLeft — у сотрудника назначено увольнение и его последний день прошёл: доступа в админку больше нет, даже если
// фоновый прогон ещё не снял роль. Обычный админ без даты — одно чтение по ключу; Find, а не First: «не найдено» на
// каждом запросе админки — не ошибка и не строка в логе.
func staffLeft(userID string, now time.Time) bool {
	var ps []models.StaffProfile
	db.Select("user_id", "leave_on").Where("user_id = ? AND leave_on IS NOT NULL", userID).Limit(1).Find(&ps)
	return len(ps) == 1 && leaveExpired(ps[0].LeaveOn, clubDayOf(now, clubDayHour()))
}

// scheduleLeave — назначить отложенное увольнение: дата, кто и почему — в карточку; роль и смены — как есть до конца
// последнего дня. Повторное назначение меняет дату.
func scheduleLeave(c *gin.Context, user *models.User, hasCard bool, day time.Time, reason string, by uuid.UUID, now time.Time) {
	var err error
	if hasCard {
		err = db.Model(&models.StaffProfile{}).Where("user_id = ?", user.ID).
			Updates(map[string]any{"leave_on": day, "leave_by": by, "leave_reason": reason, "updated_at": now}).Error
	} else { // карточки не было — минимальная, с датой ухода
		err = db.Create(&models.StaffProfile{UserID: user.ID, RateType: "none", LeaveOn: &day, LeaveBy: &by, LeaveReason: reason}).Error
	}
	if err != nil {
		respondDBError(c, err)
		return
	}
	var after int64
	db.Model(&models.ShiftAssignment{}).Where("user_id = ? AND date > ?", user.ID, day.Format("2006-01-02")).Count(&after)
	target := user.ID
	details := user.Nickname + " · уходит " + dateOut(&day)
	if reason != "" {
		details += " · " + reason
	}
	if after > 0 {
		details += fmt.Sprintf(", снимется со смен после этой даты: %d", after)
	}
	logAdminAction(c, "staff_leave", &target, details)
	c.JSON(http.StatusOK, gin.H{"user_id": user.ID, "nickname": user.Nickname, "leave_on": dateOut(&day), "deferred": true, "shifts_after": after})
}

// DELETE /admin/staff/:id/leave — отменить отложенное увольнение (owner): человек остаётся в команде, дата снимается.
func handleAdminStaffLeaveCancel(c *gin.Context) {
	var user models.User
	if err := db.First(&user, "id = ?", c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "user_not_found", "message": "Пользователь не найден"})
		return
	}
	var p models.StaffProfile
	if db.First(&p, "user_id = ? AND leave_on IS NOT NULL", user.ID).Error != nil {
		c.JSON(http.StatusConflict, gin.H{"code": "not_leaving", "message": "Увольнение не назначено — отменять нечего"})
		return
	}
	res := db.Model(&models.StaffProfile{}).Where("user_id = ? AND leave_on IS NOT NULL", user.ID).
		Updates(map[string]any{"leave_on": nil, "leave_by": nil, "leave_reason": "", "updated_at": time.Now()})
	if res.Error != nil {
		respondDBError(c, res.Error)
		return
	}
	if res.RowsAffected == 0 { // завершил фоновый прогон или отменили в соседней вкладке
		c.JSON(http.StatusConflict, gin.H{"code": "not_leaving", "message": "Увольнение уже не назначено — обнови список"})
		return
	}
	target := user.ID
	logAdminAction(c, "staff_leave_cancel", &target, user.Nickname+" · увольнение "+dateOut(p.LeaveOn)+" отменено")
	c.JSON(http.StatusOK, gin.H{"user_id": user.ID, "nickname": user.Nickname, "cancelled": dateOut(p.LeaveOn)})
}

// leaveDoneText — строка журнала о завершённом отложенном увольнении (чистая, тест).
func leaveDoneText(nick string, day time.Time, reason, shift string, removed int64) string {
	s := nick + " · уволен " + day.Format("2006-01-02") + " (последний рабочий день прошёл)"
	if reason != "" {
		s += " · " + reason
	}
	if shift != "" {
		s += ", " + shift
	}
	if removed > 0 {
		s += fmt.Sprintf(", снят с %d будущих смен", removed)
	}
	return s
}

// clearShiftsAfter — снять сотрудника со смен графика после дня day: сколько записей убрано.
func clearShiftsAfter(userID uuid.UUID, day time.Time) int64 {
	return db.Where("user_id = ? AND date > ?", userID, day.Format("2006-01-02")).Delete(&models.ShiftAssignment{}).RowsAffected
}

// leaveActor — от чьего имени строка журнала о завершении: кто назначил, иначе — владелец клуба.
func leaveActor(p *models.StaffProfile) uuid.UUID {
	if p.LeaveBy != nil {
		return *p.LeaveBy
	}
	var owner models.User
	db.Select("id").Where("role = ?", models.UserRoleOwner).Order("created_at").First(&owner)
	return owner.ID
}

// finishLeave — завершить одно отложенное увольнение. Кто первым снял дату, тот и завершает: два прогона разом
// не уволят дважды, отмена в ту же секунду — тоже не потеряется.
func finishLeave(p *models.StaffProfile, now time.Time) bool {
	var user models.User
	if p.LeaveOn == nil || db.First(&user, "id = ?", p.UserID).Error != nil {
		return false
	}
	day, by := *p.LeaveOn, leaveActor(p)
	shift := leaveShiftOf(user.ID, now) // смена последнего дня — уже не этих суток: закроется по графику
	done, closed := false, false
	err := db.Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&models.StaffProfile{}).Where("user_id = ? AND leave_on IS NOT NULL", user.ID).Updates(map[string]any{
			"leave_on": nil, "leave_by": nil, "leave_reason": "", "dismissed_at": day, "updated_at": now})
		if res.Error != nil || res.RowsAffected == 0 {
			return res.Error
		}
		done = true
		if user.Role == models.UserRoleAdmin {
			if err := tx.Model(&user).Update("role", models.UserRolePlayer).Error; err != nil {
				return err
			}
		}
		if err := closePeriod(tx, user.ID, p.HiredAt, &day, p.LeaveReason, now); err != nil { // З15-и5
			return err
		}
		var err error
		closed, err = leaveCloseShift(tx, shift, dismissNote, by, now)
		return err
	})
	if err != nil {
		log.Printf("отложенное увольнение %s не завершилось: %v", user.Nickname, err)
		return false
	}
	if !done {
		return false
	}
	dropAdminWS(user.ID)
	removed := clearShiftsAfter(user.ID, day)
	text := ""
	if closed {
		text = leaveShiftText(shift.Entry, now)
	}
	target := user.ID
	logAdminActionAs(by, "staff_dismiss", &target, leaveDoneText(user.Nickname, day, p.LeaveReason, text, removed))
	if closed {
		leaveNotify(user.Nickname, "уволен", shift, by, now)
	}
	return true
}

// finishLeaves — завершить отложенные увольнения, у которых последний день прошёл: сколько завершено.
func finishLeaves(now time.Time) int {
	today := clubDayOf(now, clubDayHour())
	var ps []models.StaffProfile
	db.Where("leave_on IS NOT NULL AND leave_on < ?", today.Format("2006-01-02")).Find(&ps)
	n := 0
	for i := range ps {
		if finishLeave(&ps[i], now) {
			n++
		}
	}
	return n
}

// POST /admin/staff/leave/run — завершить прошедшие отложенные увольнения сейчас, не дожидаясь фона (owner): та же
// логика, что у прогона.
func handleAdminStaffLeaveRun(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"finished": finishLeaves(time.Now())})
}

// startStaffLeaveJob — фоновый прогон раз в 10 минут, первый через минуту после старта: последний день кончается
// вместе с клубными сутками (report_hour), увольнение завершается в первые 10 минут следующих.
func startStaffLeaveJob() {
	go func() {
		time.Sleep(time.Minute)
		for {
			safely("staffLeave", func() {
				if n := finishLeaves(time.Now()); n > 0 {
					log.Printf("отложенные увольнения: завершено %d", n)
				}
			})
			time.Sleep(10 * time.Minute)
		}
	}()
}
