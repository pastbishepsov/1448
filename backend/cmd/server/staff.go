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
	// человек мог у нас уже работать — снимаем отметку об увольнении,
	// иначе он останется в архиве уволенных, работая на смене
	var p models.StaffProfile
	returned := false
	if db.First(&p, "user_id = ? AND dismissed_at IS NOT NULL", user.ID).Error == nil {
		db.Model(&models.StaffProfile{}).Where("user_id = ?", user.ID).
			Updates(map[string]any{"dismissed_at": nil, "updated_at": time.Now()})
		returned = true
	}

	target := user.ID
	note := " → admin"
	if returned {
		note = " → admin (вернулся)"
	}
	logAdminAction(c, "staff_promote", &target, user.Nickname+note)
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
	phone := strings.TrimSpace(req.Phone)
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
	user := models.User{
		Nickname: strings.TrimSpace(req.Nickname), PasswordHash: string(hash),
		Status: models.UserStatusActive, Role: models.UserRoleAdmin, Level: 1,
		RegisteredAt: now, LastActiveAt: now,
	}
	if hired == nil {
		today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		hired = &today
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&user).Error; err != nil {
			return err
		}
		return tx.Create(&models.StaffProfile{
			UserID: user.ID, FullName: name, Phone: phone, Position: position,
			HiredAt: hired, RateType: rateType, RateAmount: rate, Note: note,
		}).Error
	})
	if err != nil {
		if isDuplicate(err) {
			c.JSON(http.StatusConflict, gin.H{"code": "already_exists", "message": hireErrors["already_exists"]})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"code": "db_error", "message": err.Error()})
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
	if date == nil {
		now := time.Now()
		today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
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

	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&user).Update("role", models.UserRolePlayer).Error; err != nil {
			return err
		}
		if hasCard {
			return tx.Model(&models.StaffProfile{}).Where("user_id = ?", user.ID).
				Updates(map[string]any{"dismissed_at": date, "updated_at": time.Now()}).Error
		}
		// карточки не было — заводим минимальную, чтобы человек попал в архив
		return tx.Create(&models.StaffProfile{UserID: user.ID, DismissedAt: date, RateType: "none"}).Error
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "db_error", "message": err.Error()})
		return
	}
	removed := clearFutureShifts(user.ID)

	target := user.ID
	details := user.Nickname + " · уволен " + dateOut(date)
	if reason != "" {
		details += " · " + reason
	}
	if removed > 0 {
		details += fmt.Sprintf(", снят с %d будущих смен", removed)
	}
	logAdminAction(c, "staff_dismiss", &target, details)
	c.JSON(http.StatusOK, gin.H{"user_id": user.ID, "nickname": user.Nickname,
		"dismissed_at": dateOut(date), "removed_shifts": removed})
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
	out := make([]gin.H, 0, len(profiles))
	for i := range profiles {
		p := profiles[i]
		out = append(out, gin.H{
			"id": p.UserID, "nickname": nick[p.UserID.String()],
			"full_name": p.FullName, "position": p.Position,
			"hired_at": dateOut(p.HiredAt), "dismissed_at": dateOut(p.DismissedAt),
		})
	}
	c.JSON(http.StatusOK, gin.H{"count": len(out), "staff": out})
}
