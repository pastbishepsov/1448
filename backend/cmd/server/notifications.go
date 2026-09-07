package main

// Уведомления гостю о действиях админа (спринт Б4, ADMIN.md; миграция 020).
// Канал гостя — поллинг (решение №8): GET /me/notifications отдаёт
// непрочитанные и сразу помечает их — тост показывается один раз.
// Продюсеры живут в admin.go / deposits.go / grants.go (notifyUser).

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

// notifyUser — записать уведомление гостю. Сбой не роняет действие
// (как logAdminAction) — только строка в лог сервера.
func notifyUser(userID uuid.UUID, ntype string, payload map[string]any) {
	b, err := json.Marshal(payload)
	if err != nil {
		b = []byte("{}")
	}
	n := models.Notification{UserID: userID, Type: ntype, Payload: string(b)}
	if err := db.Create(&n).Error; err != nil {
		log.Printf("notify: %s не записалось: %v", ntype, err)
	}
}

// GET /me/notifications?client=kiosk|app — непрочитанные уведомления гостя
// (старые сверху, до 50); выдача помечает прочитанными.
//
// К3-и2 (№16/№17): у каждого клиента своя отметка — read_kiosk_at у киоска,
// read_app_at у PWA — и один тост доходит и до ПК, и до телефона. Без
// ?client= (текущая PWA до своей итерации в треке М) — как раньше: отдаём
// непрочитанное «вообще» (read_at IS NULL) и ставим все три отметки.
// read_at = «прочитано вообще»: ставится нетипизированным клиентом или
// вторым из типизированных.
func handleGetMyNotifications(c *gin.Context) {
	userID := c.GetString("user_id")
	client := notifClient(c.Query("client"))
	var items []models.Notification
	db.Where("user_id = ? AND "+notifUnreadCond(client), userID).
		Order("created_at ASC").Limit(50).Find(&items)

	if len(items) > 0 {
		ids := make([]uuid.UUID, 0, len(items))
		for _, n := range items {
			ids = append(ids, n.ID)
		}
		markNotificationsRead(ids, client, time.Now())
	}

	out := make([]gin.H, 0, len(items))
	for _, n := range items {
		out = append(out, gin.H{
			"id": n.ID, "type": n.Type,
			"payload": json.RawMessage(n.Payload), "created_at": n.CreatedAt,
		})
	}
	c.JSON(http.StatusOK, gin.H{"count": len(out), "items": out})
}

// notifClient — нормализованный клиент из ?client=: kiosk | app | "" (все).
func notifClient(q string) string {
	switch q {
	case "kiosk", "app":
		return q
	}
	return ""
}

// notifUnreadCond — условие «непрочитано этим клиентом» (чистая, тест).
// Типизированный клиент не видит и то, что прочитано «вообще» (read_at):
// это уведомления до миграции 055 или показанные старой PWA.
func notifUnreadCond(client string) string {
	switch client {
	case "kiosk":
		return "read_kiosk_at IS NULL AND read_at IS NULL"
	case "app":
		return "read_app_at IS NULL AND read_at IS NULL"
	}
	return "read_at IS NULL"
}

// markNotificationsRead — поставить отметки выдачи. Свою — всегда; read_at —
// нетипизированный клиент сразу, типизированный — когда прочитал и второй.
func markNotificationsRead(ids []uuid.UUID, client string, now time.Time) {
	q := db.Model(&models.Notification{}).Where("id IN ?", ids)
	switch client {
	case "kiosk":
		q.Updates(map[string]any{
			"read_kiosk_at": now,
			"read_at":       gorm.Expr("CASE WHEN read_app_at IS NULL THEN read_at ELSE ? END", now),
		})
	case "app":
		q.Updates(map[string]any{
			"read_app_at": now,
			"read_at":     gorm.Expr("CASE WHEN read_kiosk_at IS NULL THEN read_at ELSE ? END", now),
		})
	default:
		q.Updates(map[string]any{
			"read_at":       now,
			"read_kiosk_at": gorm.Expr("COALESCE(read_kiosk_at, ?)", now),
			"read_app_at":   gorm.Expr("COALESCE(read_app_at, ?)", now),
		})
	}
}
