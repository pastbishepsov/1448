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

// GET /me/notifications — непрочитанные уведомления гостя (старые сверху,
// до 50); выдача помечает прочитанными.
func handleGetMyNotifications(c *gin.Context) {
	userID := c.GetString("user_id")
	var items []models.Notification
	db.Where("user_id = ? AND read_at IS NULL", userID).
		Order("created_at ASC").Limit(50).Find(&items)

	if len(items) > 0 {
		ids := make([]uuid.UUID, 0, len(items))
		for _, n := range items {
			ids = append(ids, n.ID)
		}
		db.Model(&models.Notification{}).Where("id IN ?", ids).Update("read_at", time.Now())
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
