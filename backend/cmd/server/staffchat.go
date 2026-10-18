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
//
// З1-и2 (OWNER.md, №59, Д4): служебные сообщения. Их пишет сервер (автора
// нет), и часть из них — «только владельцу»: деньги сдачи кассы, дальше —
// напоминания бухгалтерии и алерты. Админ такие строки не видит ни в ленте,
// ни в счётчике, ни в live-канале — фильтр стоит в запросе, а не в браузере.
//
// З15-и2 (OWNER.md, №27, З-Р20): канал страницами. Лента отдавала последние
// 100, а открытие шторки ставило «прочитано» на «сейчас»: две недели без
// канала — 120 строк, 20 старших (среди них недостачи) не достать, счётчик
// сброшен. Теперь открытие отдаёт все непрочитанные (до staffChatMax), но не
// меньше страницы; «Ещё» — следующая страница старше (?before=, как у
// журнала); «прочитано» встаёт на последнее показанное сообщение и назад не
// ходит — пришедшее, пока лента открывалась, остаётся непрочитанным.

import (
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

const maxStaffMessage = 1000 // символов: вопрос владельцу бывает подробным

// Страницы канала (З15-и2).
const (
	staffChatPage = 100 // страница «Ещё» и меньше на открытии не бывает
	staffChatMax  = 500 // больше на открытии не отдаём — дальше «Ещё»
)

// staffPageSize — сколько сообщений отдать на открытии ленты (чистая, тест):
// все, что после отметки «прочитано» (after), но не меньше страницы и не
// больше staffChatMax.
func staffPageSize(after int64) int {
	if after <= staffChatPage {
		return staffChatPage
	}
	if after >= staffChatMax {
		return staffChatMax
	}
	return int(after)
}

// Виды и видимость сообщений канала (миграция 067).
const (
	staffKindText, staffKindSystem = "text", "system"
	staffForAll, staffForOwner     = "all", "owner"
)

// staffMsgOut — сообщение наружу вместе с ником автора. Ник резолвится по
// карте, а не запросом на строку: канал читают часто. У служебного автора нет.
func staffMsgOut(m *models.StaffMessage, names map[string]string) gin.H {
	out := gin.H{
		"id": m.ID, "role": m.Role, "kind": m.Kind, "visible_to": m.VisibleTo,
		"text": m.Text, "created_at": m.CreatedAt,
	}
	if m.AuthorID != nil {
		out["author_id"], out["nickname"] = m.AuthorID, names[m.AuthorID.String()]
	}
	return out
}

// staffVisible — только то, что эта роль вправе видеть: строки «только
// владельцу» админу не отдаются и не считаются.
func staffVisible(q *gorm.DB, role string) *gorm.DB {
	if role != string(models.UserRoleOwner) {
		q = q.Where("visible_to = ?", staffForAll)
	}
	return q
}

// staffUnread — сколько сообщений человек ещё не видел. Чужие и служебные
// считаем, свои нет: собственное письмо непрочитанным не бывает.
func staffUnread(userID uuid.UUID, role string, readAt *time.Time) int64 {
	q := staffVisible(db.Model(&models.StaffMessage{}), role).
		Where("(author_id IS NULL OR author_id <> ?)", userID)
	if readAt != nil {
		q = q.Where("created_at > ?", *readAt)
	}
	var n int64
	q.Count(&n)
	return n
}

// GET /admin/staff-chat — страница канала (старые сверху) и счётчик непрочитанных.
// ?before=RFC3339 — сообщения строго раньше момента («Ещё»); has_more и
// next_before — есть ли старше и откуда брать следующую страницу (как у
// журнала). Без before — свежие: все после отметки «прочитано» (до
// staffChatMax), но не меньше staffChatPage (З15-и2).
func handleStaffChatList(c *gin.Context) {
	var me models.User
	if err := db.First(&me, "id = ?", c.GetString("user_id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "user_not_found", "message": "Пользователь не найден"})
		return
	}
	role := c.GetString("user_role") // З0-и1: роль из базы, не из токена
	var before time.Time
	if b := c.Query("before"); b != "" {
		t, err := time.Parse(time.RFC3339Nano, b)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": "bad_before", "message": "before — момент в формате RFC3339"})
			return
		}
		before = t
	}
	size, q := staffChatPage, staffVisible(db, role)
	if !before.IsZero() {
		q = q.Where("created_at < ?", before)
	} else if me.StaffChatReadAt != nil {
		var after int64
		staffVisible(db.Model(&models.StaffMessage{}), role).Where("created_at > ?", *me.StaffChatReadAt).Count(&after)
		size = staffPageSize(after)
	}
	var msgs []models.StaffMessage
	q.Order("created_at DESC").Order("id DESC").Limit(size + 1).Find(&msgs)
	hasMore := len(msgs) > size
	if hasMore {
		msgs = msgs[:size]
	}
	// Разворачиваем: клиенту удобнее старые сверху, а лимит нужен по свежим.
	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}
	ids := make([]string, 0, len(msgs))
	for i := range msgs {
		if msgs[i].AuthorID != nil {
			ids = append(ids, msgs[i].AuthorID.String())
		}
	}
	names := nicknamesByID(ids)
	out := make([]gin.H, 0, len(msgs))
	for i := range msgs {
		out = append(out, staffMsgOut(&msgs[i], names))
	}
	resp := gin.H{
		"messages": out,
		"unread":   staffUnread(me.ID, role, me.StaffChatReadAt),
		"read_at":  me.StaffChatReadAt,
		"has_more": hasMore,
	}
	if hasMore && len(msgs) > 0 {
		resp["next_before"] = msgs[0].CreatedAt.Format(time.RFC3339Nano)
	}
	c.JSON(http.StatusOK, resp)
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
	// вид и видимость — только сервер: из тела берётся один текст (З1-и2)
	m := models.StaffMessage{ClubID: club.ID, AuthorID: &authorID, Role: c.GetString("user_role"), Text: text}
	if err := postStaffMessage(&m, nil); err != nil {
		respondDBError(c, err)
		return
	}
	names := nicknamesByID([]string{authorID.String()})
	c.JSON(http.StatusCreated, gin.H{"message": staffMsgOut(&m, names)})
}

// postStaffMessage — записать сообщение канала и разослать событие. Своё
// сообщение автору сразу прочитано: иначе его счётчик вырос бы от
// собственного письма. Живой канал админки (А6): у остальных счётчик обязан
// обновиться сам — вопрос ночью ждать до утра не должен; строка «только
// владельцу» уходит только в каналы владельцев. extra — поля события.
func postStaffMessage(m *models.StaffMessage, extra map[string]any) error {
	if m.Kind == "" {
		m.Kind = staffKindText
	}
	if m.VisibleTo == "" {
		m.VisibleTo = staffForAll
	}
	if err := db.Create(m).Error; err != nil {
		return err
	}
	ev := map[string]any{"kind": "message", "role": m.Role, "visible_to": m.VisibleTo}
	if m.AuthorID != nil {
		id := m.AuthorID.String()
		db.Model(&models.User{}).Where("id = ?", id).Update("staff_chat_read_at", m.CreatedAt)
		ev["author_id"], ev["nickname"] = id, nicknamesByID([]string{id})[id] // ник — для строки ленты зала
	}
	if m.Kind == staffKindSystem {
		ev["kind"] = staffKindSystem
	}
	for k, v := range extra {
		ev[k] = v
	}
	var only map[string]bool
	if m.VisibleTo == staffForOwner {
		only = ownerIDs()
	}
	hub.AdminBroadcastTo("staff_chat", ev, only)
	return nil
}

// postSystem — служебное сообщение от сервера (без автора); ownerOnly — видит
// только владелец. Сбой записи не роняет основное действие — строка в лог.
func postSystem(text string, ownerOnly bool, extra map[string]any) {
	club, ok := defaultClub()
	if !ok {
		return
	}
	m := models.StaffMessage{ClubID: club.ID, Role: "system", Kind: staffKindSystem, VisibleTo: staffForAll, Text: text}
	if ownerOnly {
		m.VisibleTo = staffForOwner
	}
	if err := postStaffMessage(&m, extra); err != nil {
		log.Printf("канал персонала: служебное сообщение не записалось: %v", err)
	}
}

// ownerIDs — действующие владельцы: кому уходят события строк «только владельцу».
func ownerIDs() map[string]bool {
	var ids []uuid.UUID
	db.Model(&models.User{}).Where("role = ? AND status <> ?", models.UserRoleOwner, models.UserStatusBanned).Pluck("id", &ids)
	out := make(map[string]bool, len(ids))
	for _, id := range ids {
		out[id.String()] = true
	}
	return out
}

type staffReadRequest struct {
	Until string `json:"until"` // З15-и2: id последнего показанного сообщения
}

// POST /admin/staff-chat/read — «дочитал до сих пор». З15-и2: до последнего
// показанного — until, id сообщения, которое человек видел последним:
// отметка встаёт на его время и назад не ходит, пришедшее позже остаётся
// непрочитанным. Без until (страница, открытая до обновления) — как раньше,
// «до сейчас». В ответе — отметка и сколько непрочитанных осталось.
func handleStaffChatRead(c *gin.Context) {
	var req staffReadRequest
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_request", "message": "Тело — {\"until\": id сообщения}"})
		return
	}
	userID, err := uuid.Parse(c.GetString("user_id"))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": "bad_token", "message": "Не разобрал токен"})
		return
	}
	role := c.GetString("user_role")
	at := time.Now()
	if req.Until != "" {
		id, err := uuid.Parse(req.Until)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": "bad_until", "message": "until — id сообщения канала"})
			return
		}
		var m models.StaffMessage
		if staffVisible(db, role).Select("id", "created_at").First(&m, "id = ?", id).Error != nil {
			c.JSON(http.StatusNotFound, gin.H{"code": "message_not_found", "message": "Такого сообщения в канале нет"})
			return
		}
		at = m.CreatedAt
	}
	if err := db.Model(&models.User{}).Where("id = ? AND (staff_chat_read_at IS NULL OR staff_chat_read_at < ?)", userID, at).
		Update("staff_chat_read_at", at).Error; err != nil {
		respondDBError(c, err)
		return
	}
	var me models.User
	db.Select("id", "staff_chat_read_at").First(&me, "id = ?", userID)
	c.JSON(http.StatusOK, gin.H{"read_at": me.StaffChatReadAt, "unread": staffUnread(userID, role, me.StaffChatReadAt)})
}
