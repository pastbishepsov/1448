package main

// Очередь-вейтлист, когда все ПК заняты (спринт Б9, ADMIN.md; миграция 022).
// Только зарегистрированные гости: админ ставит по нику у стойки, гость сам —
// через /me/waitlist (задел под PWA; гостевой UI — трек М). Посадка гостя
// любым путём закрывает его место сама (resolveWaitlistOnSeat в
// startSessionFor); освободившийся ПК разово зовёт голову очереди через
// уведомления Б4 (checkWaitlistNotify). Каждое движение — событие "waitlist"
// в live-канал админки.

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

// canJoinWaitlist — чистое правило постановки в очередь (тест в
// waitlist_test.go): только незабаненный гость (role=player) без активной
// сессии, ещё не в очереди — и только когда свободных ПК нет: иначе очередь
// не нужна, сажаем сразу.
func canJoinWaitlist(status models.UserStatus, role models.UserRole,
	hasActiveSession, alreadyWaiting bool, freeComputers int64) (ok bool, code string) {
	if role != models.UserRolePlayer {
		return false, "not_player"
	}
	if status == models.UserStatusBanned {
		return false, "banned"
	}
	if hasActiveSession {
		return false, "session_active"
	}
	if alreadyWaiting {
		return false, "already_waiting"
	}
	if freeComputers > 0 {
		return false, "computers_free"
	}
	return true, ""
}

var waitlistJoinErrors = map[string]struct {
	status  int
	message string
}{
	"not_player":      {http.StatusForbidden, "В очередь встают только гости"},
	"banned":          {http.StatusConflict, "Аккаунт заблокирован — сначала разбань"},
	"session_active":  {http.StatusConflict, "У гостя уже идёт сессия"},
	"already_waiting": {http.StatusConflict, "Гость уже в очереди"},
	"computers_free":  {http.StatusConflict, "Есть свободные ПК — очередь не нужна, сажай сразу"},
}

// waitlistChecks — собирает факты для canJoinWaitlist по конкретному гостю.
func waitlistChecks(userID uuid.UUID) (hasActiveSession, alreadyWaiting bool, freeComputers int64) {
	var activeCount int64
	db.Model(&models.Session{}).
		Where("user_id = ? AND status = ?", userID, models.SessionStatusActive).
		Count(&activeCount)
	var waitingCount int64
	db.Model(&models.WaitlistEntry{}).
		Where("user_id = ? AND status = ?", userID, models.WaitlistStatusWaiting).
		Count(&waitingCount)
	db.Model(&models.Computer{}).
		Where("status = ?", models.ComputerStatusAvailable).
		Count(&freeComputers)
	return activeCount > 0, waitingCount > 0, freeComputers
}

// waitlistPosition — позиция записи в очереди её клуба (1 = голова).
func waitlistPosition(e *models.WaitlistEntry) int64 {
	var ahead int64
	db.Model(&models.WaitlistEntry{}).
		Where("club_id = ? AND status = ? AND created_at < ?",
			e.ClubID, models.WaitlistStatusWaiting, e.CreatedAt).
		Count(&ahead)
	return ahead + 1
}

// defaultClub — клуб v1 (одноклубное решение №8: первый активный по имени).
func defaultClub() (*models.Club, bool) {
	var club models.Club
	if err := db.Where("is_active = ?", true).Order("name").First(&club).Error; err != nil {
		return nil, false
	}
	return &club, true
}

// joinWaitlist — общая постановка в очередь (админ у стойки или гость сам).
// Ответ об ошибке пишет сам; вернул nil — обработчику выходить.
func joinWaitlist(c *gin.Context, user *models.User, addedBy *uuid.UUID) *models.WaitlistEntry {
	hasSession, waiting, free := waitlistChecks(user.ID)
	if ok, code := canJoinWaitlist(user.Status, user.Role, hasSession, waiting, free); !ok {
		e := waitlistJoinErrors[code]
		c.JSON(e.status, gin.H{"code": code, "message": e.message})
		return nil
	}
	club, ok := defaultClub()
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "club_missing", "message": "Клуб не найден"})
		return nil
	}
	entry := models.WaitlistEntry{ClubID: club.ID, UserID: user.ID, Status: models.WaitlistStatusWaiting, AddedBy: addedBy}
	if err := db.Create(&entry).Error; err != nil {
		if isDuplicate(err) { // гонка с уникальным индексом активной записи
			c.JSON(http.StatusConflict, gin.H{"code": "already_waiting", "message": "Гость уже в очереди"})
			return nil
		}
		c.JSON(http.StatusInternalServerError, gin.H{"code": "db_error", "message": err.Error()})
		return nil
	}
	kind := map[bool]string{true: "add", false: "join"}[addedBy != nil]
	hub.AdminBroadcast("waitlist", map[string]any{"kind": kind, "nickname": user.Nickname})
	return &entry
}

// closeWaitlistEntry — снять запись из очереди (removed) с меткой времени.
func closeWaitlistEntry(entry *models.WaitlistEntry) error {
	now := time.Now()
	return db.Model(entry).Updates(map[string]any{
		"status": models.WaitlistStatusRemoved, "resolved_at": now,
	}).Error
}

// resolveWaitlistOnSeat — Б9: гость сел за ПК — его место в очереди
// закрывается само, каким бы путём ни началась сессия (сам с киоска,
// посадка у стойки, из очереди). Вернёт true, если гость был в очереди.
func resolveWaitlistOnSeat(userID uuid.UUID, nickname string) bool {
	res := db.Model(&models.WaitlistEntry{}).
		Where("user_id = ? AND status = ?", userID, models.WaitlistStatusWaiting).
		Updates(map[string]any{"status": models.WaitlistStatusSeated, "resolved_at": time.Now()})
	if res.Error != nil || res.RowsAffected == 0 {
		return false
	}
	hub.AdminBroadcast("waitlist", map[string]any{"kind": "seated", "nickname": nickname})
	return true
}

// checkWaitlistNotify — Б9: если в клубе есть свободный ПК и голова очереди
// ещё не звана — разовое уведомление «ПК свободен — подойди к стойке» (шина
// Б4). Зовётся при освобождении ПК (finishSession, выход из ремонта) и при
// движении очереди. notified_at защищает от спама при серии освобождений.
func checkWaitlistNotify(clubID uuid.UUID) {
	var pc models.Computer
	if db.Where("club_id = ? AND status = ?", clubID, models.ComputerStatusAvailable).
		Order("name").First(&pc).Error != nil {
		return
	}
	var entry models.WaitlistEntry
	if db.Preload("User").
		Where("club_id = ? AND status = ?", clubID, models.WaitlistStatusWaiting).
		Order("created_at ASC").First(&entry).Error != nil {
		return
	}
	if entry.NotifiedAt != nil {
		return
	}
	if err := db.Model(&entry).Update("notified_at", time.Now()).Error; err != nil {
		return
	}
	notifyUser(entry.UserID, "waitlist_ready", map[string]any{"computer": pc.Name})
	hub.AdminBroadcast("waitlist", map[string]any{"kind": "ready", "nickname": nicknameOf(entry.User), "computer": pc.Name})
}

// ───────────────────────────── админка ─────────────────────────────

// GET /admin/waitlist — очередь: голова сверху, с временем ожидания.
func handleAdminWaitlist(c *gin.Context) {
	var entries []models.WaitlistEntry
	db.Preload("User").
		Where("status = ?", models.WaitlistStatusWaiting).
		Order("created_at ASC").Limit(100).Find(&entries)

	// ники поставивших админов — одним запросом
	idSet := map[string]bool{}
	for _, e := range entries {
		if e.AddedBy != nil {
			idSet[e.AddedBy.String()] = true
		}
	}
	nick := map[string]string{}
	if len(idSet) > 0 {
		ids := make([]string, 0, len(idSet))
		for id := range idSet {
			ids = append(ids, id)
		}
		var admins []models.User
		db.Where("id IN ?", ids).Find(&admins)
		for _, a := range admins {
			nick[a.ID.String()] = a.Nickname
		}
	}

	now := time.Now()
	out := make([]gin.H, 0, len(entries))
	for i, e := range entries {
		row := gin.H{
			"id": e.ID, "user_id": e.UserID, "nickname": nicknameOf(e.User),
			"position": i + 1, "created_at": e.CreatedAt,
			"waited_min": int(now.Sub(e.CreatedAt).Minutes()),
			"notified":   e.NotifiedAt != nil,
			"source":     map[bool]string{true: "admin", false: "self"}[e.AddedBy != nil],
		}
		if e.AddedBy != nil {
			row["added_by"] = nick[e.AddedBy.String()]
		}
		out = append(out, row)
	}
	c.JSON(http.StatusOK, gin.H{"count": len(out), "items": out})
}

// POST /admin/waitlist {nickname} — поставить гостя в очередь у стойки.
// Инвариант цели тот же, что у депозита/гранта (canTargetUser).
func handleAdminWaitlistAdd(c *gin.Context) {
	var req struct {
		Nickname string `json:"nickname" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_request", "message": "Нужен ник гостя"})
		return
	}
	var user models.User
	if err := db.First(&user, "nickname = ?", req.Nickname).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "user_not_found", "message": "Гость с таким ником не найден"})
		return
	}
	if ok, code := canTargetUser(user.Role, user.ID.String(), c.GetString("user_id")); !ok {
		msg := map[string]string{
			"cannot_touch_self":  "Нельзя применить действие к самому себе",
			"cannot_touch_staff": "Действия персонала применимы только к гостям",
		}[code]
		c.JSON(http.StatusForbidden, gin.H{"code": code, "message": msg})
		return
	}
	adminID, err := uuid.Parse(c.GetString("user_id"))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": "invalid_user", "message": "Некорректный пользователь"})
		return
	}
	entry := joinWaitlist(c, &user, &adminID)
	if entry == nil {
		return
	}
	target := user.ID
	logAdminAction(c, "waitlist_add", &target, user.Nickname)
	c.JSON(http.StatusCreated, gin.H{
		"id": entry.ID, "nickname": user.Nickname,
		"position": waitlistPosition(entry), "created_at": entry.CreatedAt,
	})
}

// DELETE /admin/waitlist/:id — снять из очереди (не пришёл / передумал).
// Гость узнаёт тостом через /me/notifications (Б4).
func handleAdminWaitlistRemove(c *gin.Context) {
	var entry models.WaitlistEntry
	if err := db.Preload("User").
		First(&entry, "id = ? AND status = ?", c.Param("id"), models.WaitlistStatusWaiting).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "entry_not_found", "message": "Запись в очереди не найдена"})
		return
	}
	if err := closeWaitlistEntry(&entry); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "db_error", "message": err.Error()})
		return
	}
	target := entry.UserID
	logAdminAction(c, "waitlist_remove", &target, nicknameOf(entry.User))
	notifyUser(entry.UserID, "waitlist_remove", map[string]any{})
	hub.AdminBroadcast("waitlist", map[string]any{"kind": "remove", "nickname": nicknameOf(entry.User)})
	checkWaitlistNotify(entry.ClubID) // очередь сдвинулась — новая голова
	c.JSON(http.StatusOK, gin.H{"id": entry.ID, "status": models.WaitlistStatusRemoved})
}

// ─────────────────────── гость (задел под PWA) ───────────────────────

// POST /me/waitlist — встать в очередь самому (гостевой UI — трек М).
func handleJoinWaitlist(c *gin.Context) {
	userID, err := uuid.Parse(c.GetString("user_id"))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": "invalid_user", "message": "Некорректный пользователь"})
		return
	}
	var user models.User
	if err := db.First(&user, "id = ?", userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "user_not_found", "message": "Пользователь не найден"})
		return
	}
	entry := joinWaitlist(c, &user, nil)
	if entry == nil {
		return
	}
	pos := waitlistPosition(entry)
	c.JSON(http.StatusCreated, gin.H{
		"id": entry.ID, "position": pos, "ahead": pos - 1, "created_at": entry.CreatedAt,
	})
}

// GET /me/waitlist — своя позиция (поллинг из PWA). Не в очереди — waiting:false.
func handleGetMyWaitlist(c *gin.Context) {
	userID := c.GetString("user_id")
	var entry models.WaitlistEntry
	if db.Where("user_id = ? AND status = ?", userID, models.WaitlistStatusWaiting).
		First(&entry).Error != nil {
		c.JSON(http.StatusOK, gin.H{"waiting": false})
		return
	}
	var total int64
	db.Model(&models.WaitlistEntry{}).
		Where("club_id = ? AND status = ?", entry.ClubID, models.WaitlistStatusWaiting).
		Count(&total)
	c.JSON(http.StatusOK, gin.H{
		"waiting": true, "id": entry.ID,
		"position": waitlistPosition(&entry), "total": total,
		"notified": entry.NotifiedAt != nil, "created_at": entry.CreatedAt,
	})
}

// DELETE /me/waitlist — выйти из очереди самому.
func handleLeaveWaitlist(c *gin.Context) {
	userID := c.GetString("user_id")
	var entry models.WaitlistEntry
	if err := db.Preload("User").
		Where("user_id = ? AND status = ?", userID, models.WaitlistStatusWaiting).
		First(&entry).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "not_waiting", "message": "Ты не в очереди"})
		return
	}
	if err := closeWaitlistEntry(&entry); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "db_error", "message": err.Error()})
		return
	}
	hub.AdminBroadcast("waitlist", map[string]any{"kind": "leave", "nickname": nicknameOf(entry.User)})
	checkWaitlistNotify(entry.ClubID) // очередь сдвинулась — новая голова
	c.JSON(http.StatusOK, gin.H{"id": entry.ID, "status": models.WaitlistStatusRemoved})
}
