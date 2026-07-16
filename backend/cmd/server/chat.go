package main

// Чат гость ↔ админ (спринты Б2–Б3, ADMIN.md; миграция 019).
// Б2: вызов админа = сообщение kind=call. Гость шлёт POST /me/chat из
// браузера (работает без агента); агентский admin_call приходит через
// hub.OnAdminCall. Каждое сообщение уходит админкам событием "chat".
// Канал гостя — поллинг (решение №8), появится в Б3/Б4.

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

// validateChatMessage — чистая проверка сообщения (тест в chat_test.go).
// call: текст не обязателен; text: 1–500 символов после трима.
func validateChatMessage(kind, text string) (ok bool, code string) {
	if kind != models.ChatKindCall && kind != models.ChatKindText {
		return false, "bad_kind"
	}
	if kind == models.ChatKindText && strings.TrimSpace(text) == "" {
		return false, "empty_text"
	}
	if len([]rune(text)) > 500 {
		return false, "too_long"
	}
	return true, ""
}

// chatCooldown — минимальная пауза между сообщениями одного вида
// (тест в chat_test.go): вызов — 30с (не спамить звонком), текст — 2с.
func chatCooldown(kind string) time.Duration {
	if kind == models.ChatKindCall {
		return 30 * time.Second
	}
	return 2 * time.Second
}

// cooldownPassed — прошла ли пауза с последнего сообщения (чистая, тест).
func cooldownPassed(last, now time.Time, cd time.Duration) bool {
	return now.Sub(last) >= cd
}

// broadcastChat — событие "chat" всем админкам (ник и ПК уже разрезолвлены).
func broadcastChat(m *models.ChatMessage, nickname, computer string) {
	data := map[string]any{
		"id": m.ID.String(), "kind": m.Kind, "sender": m.Sender, "text": m.Text,
	}
	if nickname != "" {
		data["nickname"] = nickname
	}
	if computer != "" {
		data["computer"] = computer
	}
	if m.ComputerID != nil {
		data["computer_id"] = m.ComputerID.String()
	}
	if m.UserID != nil {
		data["user_id"] = m.UserID.String()
	}
	hub.AdminBroadcast("chat", data)
}

// POST /me/chat — сообщение или вызов от гостя (kind=call|text).
// Если у гостя активная сессия — сообщение привязывается к его ПК.
func handleGuestChatPost(c *gin.Context) {
	userID, err := uuid.Parse(c.GetString("user_id"))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": "invalid_user", "message": "Некорректный пользователь"})
		return
	}

	var req struct {
		Kind string `json:"kind"`
		Text string `json:"text"`
	}
	_ = c.ShouldBindJSON(&req)
	if req.Kind == "" {
		req.Kind = models.ChatKindText
	}
	if ok, code := validateChatMessage(req.Kind, req.Text); !ok {
		msg := map[string]string{
			"bad_kind":   "kind: call или text",
			"empty_text": "Пустое сообщение",
			"too_long":   "Сообщение длиннее 500 символов",
		}[code]
		c.JSON(http.StatusBadRequest, gin.H{"code": code, "message": msg})
		return
	}

	// пауза между сообщениями одного вида
	var last models.ChatMessage
	if db.Where("user_id = ? AND sender = ? AND kind = ?", userID, models.ChatSenderGuest, req.Kind).
		Order("created_at DESC").First(&last).Error == nil {
		if !cooldownPassed(last.CreatedAt, time.Now(), chatCooldown(req.Kind)) {
			msg := "Слишком часто — подожди пару секунд"
			if req.Kind == models.ChatKindCall {
				msg = "Администратор уже позван — идёт"
			}
			c.JSON(http.StatusTooManyRequests, gin.H{"code": "too_fast", "message": msg})
			return
		}
	}

	var user models.User
	if err := db.First(&user, "id = ?", userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "user_not_found", "message": "Пользователь не найден"})
		return
	}

	m := models.ChatMessage{
		UserID: &user.ID,
		Sender: models.ChatSenderGuest,
		Kind:   req.Kind,
		Text:   strings.TrimSpace(req.Text),
	}
	computerName := ""
	var s models.Session
	if db.Preload("Computer").
		Where("user_id = ? AND status = ?", userID, models.SessionStatusActive).
		First(&s).Error == nil {
		m.ComputerID = &s.ComputerID
		if s.Computer != nil {
			computerName = s.Computer.Name
		}
	}
	if err := db.Create(&m).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "db_error", "message": err.Error()})
		return
	}
	broadcastChat(&m, user.Nickname, computerName)
	c.JSON(http.StatusCreated, gin.H{"id": m.ID, "kind": m.Kind, "created_at": m.CreatedAt})
}

// agentAdminCall — admin_call от shell-agent (hub.OnAdminCall, Б2):
// вызов с ПК; гость — по активной сессии, без неё пишем только ПК.
func agentAdminCall(computerID string) {
	cid, err := uuid.Parse(computerID)
	if err != nil {
		return
	}
	var pc models.Computer
	if err := db.First(&pc, "id = ?", cid).Error; err != nil {
		return
	}

	m := models.ChatMessage{
		ComputerID: &pc.ID,
		Sender:     models.ChatSenderGuest,
		Kind:       models.ChatKindCall,
	}
	nickname := ""
	var s models.Session
	if db.Preload("User").
		Where("computer_id = ? AND status = ?", pc.ID, models.SessionStatusActive).
		First(&s).Error == nil {
		m.UserID = &s.UserID
		nickname = nicknameOf(s.User)
	}
	// пауза как у браузерного вызова: не чаще раза в 30с с одного ПК
	var last models.ChatMessage
	if db.Where("computer_id = ? AND kind = ?", pc.ID, models.ChatKindCall).
		Order("created_at DESC").First(&last).Error == nil {
		if !cooldownPassed(last.CreatedAt, time.Now(), chatCooldown(models.ChatKindCall)) {
			return
		}
	}
	if err := db.Create(&m).Error; err != nil {
		return
	}
	broadcastChat(&m, nickname, pc.Name)
}

// GET /admin/chat/pending — непрочитанные сообщения/вызовы для колокола
// (свежие сверху, максимум 50; ники и ПК резолвятся пачкой).
func handleAdminChatPending(c *gin.Context) {
	var msgs []models.ChatMessage
	db.Where("read_staff = ? AND sender = ?", false, models.ChatSenderGuest).
		Order("created_at DESC").Limit(50).Find(&msgs)

	userIDs, pcIDs := map[string]bool{}, map[string]bool{}
	for _, m := range msgs {
		if m.UserID != nil {
			userIDs[m.UserID.String()] = true
		}
		if m.ComputerID != nil {
			pcIDs[m.ComputerID.String()] = true
		}
	}
	nick := map[string]string{}
	if len(userIDs) > 0 {
		ids := make([]string, 0, len(userIDs))
		for id := range userIDs {
			ids = append(ids, id)
		}
		var users []models.User
		db.Where("id IN ?", ids).Find(&users)
		for _, u := range users {
			nick[u.ID.String()] = u.Nickname
		}
	}
	pcName := map[string]string{}
	if len(pcIDs) > 0 {
		ids := make([]string, 0, len(pcIDs))
		for id := range pcIDs {
			ids = append(ids, id)
		}
		var pcs []models.Computer
		db.Where("id IN ?", ids).Find(&pcs)
		for _, p := range pcs {
			pcName[p.ID.String()] = p.Name
		}
	}

	out := make([]gin.H, 0, len(msgs))
	for _, m := range msgs {
		row := gin.H{"id": m.ID, "kind": m.Kind, "text": m.Text, "created_at": m.CreatedAt}
		if m.UserID != nil {
			row["user_id"] = m.UserID
			row["nickname"] = nick[m.UserID.String()]
		}
		if m.ComputerID != nil {
			row["computer_id"] = m.ComputerID
			row["computer"] = pcName[m.ComputerID.String()]
		}
		out = append(out, row)
	}
	c.JSON(http.StatusOK, gin.H{"count": len(out), "items": out})
}

// POST /admin/chat/:id/ack — «принял»: сообщение прочитано персоналом.
func handleAdminChatAck(c *gin.Context) {
	var m models.ChatMessage
	if err := db.First(&m, "id = ?", c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "message_not_found", "message": "Сообщение не найдено"})
		return
	}
	updates := map[string]any{"read_staff": true}
	if adminID, err := uuid.Parse(c.GetString("user_id")); err == nil {
		updates["admin_id"] = adminID
	}
	if err := db.Model(&m).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "db_error", "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": m.ID, "read_staff": true})
}
