package main

// Рабочий канал персонала: админ ↔ владелец (спринт Е6, OPERATOR.md; Р7).
//
// «Чат нужен в обе стороны». Гость ↔ админ работает с Б2–Б3, а у самого
// админа спросить некого: ночью вопрос «гость требует вернуть деньги за
// пакет, что делать» упирается в тишину — админ решает наугад, а разбор
// начинается утром с фразы «а почему ты…».
//
// Канал ОДИН на клуб. Вопрос у стойки почти всегда операционный, и ответ
// владельца полезен всей смене; заодно из переписки сам собой получается
// справочник частых случаев. Приватного «только со мной» здесь нет
// сознательно: личные разговоры — не задача рабочего канала.

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

const maxStaffMessage = 1000 // символов: вопрос владельцу бывает подробным

// staffMsgOut — сообщение наружу вместе с ником автора. Ник резолвится по
// карте, а не запросом на строку: канал читают часто.
func staffMsgOut(m *models.StaffMessage, names map[string]string) gin.H {
	return gin.H{
		"id": m.ID, "author_id": m.AuthorID, "nickname": names[m.AuthorID.String()],
		"role": m.Role, "text": m.Text, "created_at": m.CreatedAt,
	}
}

// staffUnread — сколько сообщений человек ещё не видел. Чужие считаем, свои
// нет: собственное письмо непрочитанным не бывает.
func staffUnread(userID uuid.UUID, readAt *time.Time) int64 {
	q := db.Model(&models.StaffMessage{}).Where("author_id <> ?", userID)
	if readAt != nil {
		q = q.Where("created_at > ?", *readAt)
	}
	var n int64
	q.Count(&n)
	return n
}

// GET /admin/staff-chat — последние сообщения канала и счётчик непрочитанных.
func handleStaffChatList(c *gin.Context) {
	var me models.User
	if err := db.First(&me, "id = ?", c.GetString("user_id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "user_not_found", "message": "Пользователь не найден"})
		return
	}
	var msgs []models.StaffMessage
	db.Order("created_at DESC").Limit(100).Find(&msgs)
	// Разворачиваем: клиенту удобнее старые сверху, а лимит нужен по свежим.
	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}
	ids := make([]string, 0, len(msgs))
	for i := range msgs {
		ids = append(ids, msgs[i].AuthorID.String())
	}
	names := nicknamesByID(ids)
	out := make([]gin.H, 0, len(msgs))
	for i := range msgs {
		out = append(out, staffMsgOut(&msgs[i], names))
	}
	c.JSON(http.StatusOK, gin.H{
		"messages": out,
		"unread":   staffUnread(me.ID, me.StaffChatReadAt),
		"read_at":  me.StaffChatReadAt,
	})
}

type staffChatRequest struct {
	Text string `json:"text"`
}

// POST /admin/staff-chat — написать в канал (staff).
func handleStaffChatPost(c *gin.Context) {
	var req staffChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_request", "message": "Нужен текст"})
		return
	}
	text := strings.TrimSpace(req.Text)
	if text == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": "empty_text", "message": "Пустое сообщение отправлять некуда"})
		return
	}
	if len([]rune(text)) > maxStaffMessage {
		c.JSON(http.StatusBadRequest, gin.H{"code": "too_long",
			"message": "Сообщение длиннее 1000 символов — сократи или разбей на два"})
		return
	}
	club, ok := defaultClub()
	if !ok {
		c.JSON(http.StatusConflict, gin.H{"code": "no_club", "message": "Клуб не настроен"})
		return
	}
	authorID, err := uuid.Parse(c.GetString("user_id"))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": "bad_token", "message": "Не разобрал токен"})
		return
	}
	m := models.StaffMessage{
		ClubID: club.ID, AuthorID: authorID,
		Role: c.GetString("user_role"), Text: text,
	}
	if err := db.Create(&m).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "db_error", "message": err.Error()})
		return
	}
	// Своё сообщение прочитанным считается сразу: иначе счётчик у автора
	// вырос бы от собственного письма.
	db.Model(&models.User{}).Where("id = ?", authorID).
		Update("staff_chat_read_at", m.CreatedAt)

	// Живой канал админки (А6): у остальных счётчик обязан обновиться сам, а
	// не по заходу в раздел — вопрос ночью ждать до утра не должен.
	hub.AdminBroadcast("staff_chat", map[string]any{
		"kind": "message", "author_id": authorID.String(), "role": m.Role,
	})
	names := nicknamesByID([]string{authorID.String()})
	c.JSON(http.StatusCreated, gin.H{"message": staffMsgOut(&m, names)})
}

// POST /admin/staff-chat/read — «дочитал до сих пор».
func handleStaffChatRead(c *gin.Context) {
	now := time.Now()
	if err := db.Model(&models.User{}).Where("id = ?", c.GetString("user_id")).
		Update("staff_chat_read_at", now).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "db_error", "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"read_at": now, "unread": 0})
}
