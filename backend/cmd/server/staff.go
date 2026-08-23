package main

// Персонал (спринт Б5, ADMIN.md; решение №9 — на своём Go+Postgres).
// Owner назначает/снимает админов из админки вместо SQL. Роль вступает
// в силу после перелогина сотрудника (роль едет в JWT-клейме).
// Все роуты — за ownerMiddleware (main.go); события — в аудит staff_*
// (от роли admin они скрыты фильтром Б1 в audit.go).

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

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
	today := startOfToday()
	acts := map[string]int64{}
	type cnt struct {
		AdminID string
		N       int64
	}
	var rows []cnt
	db.Model(&models.AdminAction{}).Select("admin_id, COUNT(*) AS n").
		Where("created_at >= ?", today).Group("admin_id").Scan(&rows)
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
		}
		out = append(out, row)
	}
	c.JSON(http.StatusOK, gin.H{"count": len(out), "staff": out})
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
	if err := db.Model(&user).Update("role", models.UserRoleAdmin).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "db_error", "message": err.Error()})
		return
	}
	target := user.ID
	logAdminAction(c, "staff_promote", &target, user.Nickname+" → admin")
	c.JSON(http.StatusOK, gin.H{"user_id": user.ID, "nickname": user.Nickname, "role": models.UserRoleAdmin})
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
	from := futureShiftsFrom(time.Now(), int(settingInt64("report_hour", 8)))
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
	if err := db.Model(&user).Update("role", models.UserRolePlayer).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "db_error", "message": err.Error()})
		return
	}
	// БАГ до этого фикса: снятый админ оставался висеть в будущих сменах —
	// назначение проверяет роль только в момент постановки, а снятие график
	// не трогало, и в сетке недели стоял человек, который тут больше не работает.
	removed := clearFutureShifts(user.ID)

	target := user.ID
	details := user.Nickname + " → player"
	if removed > 0 {
		details += fmt.Sprintf(", снят с %d будущих смен", removed)
	}
	logAdminAction(c, "staff_demote", &target, details)
	c.JSON(http.StatusOK, gin.H{"user_id": user.ID, "nickname": user.Nickname,
		"role": models.UserRolePlayer, "removed_shifts": removed})
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
	"bad_phone":     "Телефон — до 32 символов",
	"bad_position":  "Должность — до 64 символов",
	"bad_note":      "Заметка — до 2000 символов",
	"bad_rate_type": "Ставка: не задана, за час, за смену или оклад",
	"bad_rate":      "Сумма ставки — от 0 до 1 000 000",
	"rate_needed":   "Выбран тип ставки — нужна сумма",
	"rate_extra":    "Ставка не задана — сумма должна быть пустой",
	"bad_dates":     "Дата увольнения раньше даты найма",
	"bad_date":      "Дата — в формате ГГГГ-ММ-ДД",
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
	if !roleIsStaff(string(user.Role)) {
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
	rateType := strings.TrimSpace(req.RateType)
	if rateType == "" {
		rateType = "none"
	}
	rate := 0.0
	if req.RateAmount != nil {
		rate = *req.RateAmount
	}
	name := strings.TrimSpace(req.FullName)
	phone := strings.TrimSpace(req.Phone)
	position := strings.TrimSpace(req.Position)
	note := strings.TrimSpace(req.Note)
	if valid, code := validateStaffProfile(name, phone, position, note, rateType, rate, hired, dismissed); !valid {
		c.JSON(http.StatusBadRequest, gin.H{"code": code, "message": profileErrors[code]})
		return
	}

	var old models.StaffProfile
	existed := db.First(&old, "user_id = ?", user.ID).Error == nil

	p := models.StaffProfile{
		UserID: user.ID, FullName: name, Phone: phone, Position: position,
		HiredAt: hired, DismissedAt: dismissed, RateType: rateType, RateAmount: rate, Note: note,
		UpdatedAt: time.Now(),
	}
	if existed {
		p.CreatedAt = old.CreatedAt
		if err := db.Model(&models.StaffProfile{}).Where("user_id = ?", user.ID).
			Updates(map[string]any{
				"full_name": name, "phone": phone, "position": position,
				"hired_at": hired, "dismissed_at": dismissed,
				"rate_type": rateType, "rate_amount": rate, "note": note, "updated_at": p.UpdatedAt,
			}).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": "db_error", "message": err.Error()})
			return
		}
	} else if err := db.Create(&p).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "db_error", "message": err.Error()})
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
