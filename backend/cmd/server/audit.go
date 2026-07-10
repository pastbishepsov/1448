package main

// Журнал-аудит (спринт А4, ADMIN.md): единая лента действий персонала.
// Источники: admin_grants (ручные начисления), deposits (пополнения),
// admin_actions (остальное: баны, форс-энды, ремонт ПК, брони, каталог).

import (
	"fmt"
	"log"
	"net/http"
	"sort"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

// logAdminAction — запись действия в admin_actions. Сбой записи не роняет
// основное действие — только строка в лог сервера.
func logAdminAction(c *gin.Context, action string, targetUserID *uuid.UUID, details string) {
	adminID, err := uuid.Parse(c.GetString("user_id"))
	if err != nil {
		return
	}
	entry := models.AdminAction{AdminID: adminID, Action: action, TargetUserID: targetUserID, Details: details}
	if err := db.Create(&entry).Error; err != nil {
		log.Printf("audit: действие %s не записалось: %v", action, err)
	}
}

// GET /admin/audit — лента событий: три источника сливаются по времени,
// в ответ уходит максимум 100 свежих. Ники резолвятся одним запросом.
func handleAdminAudit(c *gin.Context) {
	type item struct {
		CreatedAt time.Time
		Kind      string
		AdminID   uuid.UUID
		UserID    *uuid.UUID
		Text      string
	}
	items := []item{}

	var grants []models.AdminGrant
	db.Order("created_at DESC").Limit(100).Find(&grants)
	for _, g := range grants {
		text := g.Reason
		if g.GrantType == "xp" && g.Amount != nil {
			text = fmt.Sprintf("+%d XP — %s", *g.Amount, g.Reason)
		} else if g.CaseTier != nil {
			text = fmt.Sprintf("кейс %s — %s", *g.CaseTier, g.Reason)
		}
		uid := g.UserID
		items = append(items, item{g.CreatedAt, "grant", g.AdminID, &uid, text})
	}

	var deposits []models.Deposit
	db.Order("created_at DESC").Limit(100).Find(&deposits)
	for _, d := range deposits {
		adminID := uuid.Nil
		if d.CreatedBy != nil {
			adminID = *d.CreatedBy
		}
		uid := d.UserID
		items = append(items, item{d.CreatedAt, "deposit", adminID, &uid,
			fmt.Sprintf("%.0f zł → +%d монет (%s)", d.AmountPLN, d.CoinsGranted+d.BonusCoins, d.Method)})
	}

	var actions []models.AdminAction
	db.Order("created_at DESC").Limit(100).Find(&actions)
	for _, a := range actions {
		items = append(items, item{a.CreatedAt, a.Action, a.AdminID, a.TargetUserID, a.Details})
	}

	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	if len(items) > 100 {
		items = items[:100]
	}

	idSet := map[string]bool{}
	for _, it := range items {
		if it.AdminID != uuid.Nil {
			idSet[it.AdminID.String()] = true
		}
		if it.UserID != nil {
			idSet[it.UserID.String()] = true
		}
	}
	nick := map[string]string{}
	if len(idSet) > 0 {
		ids := make([]string, 0, len(idSet))
		for id := range idSet {
			ids = append(ids, id)
		}
		var users []models.User
		db.Where("id IN ?", ids).Find(&users)
		for _, u := range users {
			nick[u.ID.String()] = u.Nickname
		}
	}

	out := make([]gin.H, 0, len(items))
	for _, it := range items {
		row := gin.H{"created_at": it.CreatedAt, "kind": it.Kind, "text": it.Text}
		if it.AdminID != uuid.Nil {
			row["admin"] = nick[it.AdminID.String()]
		}
		if it.UserID != nil {
			row["nickname"] = nick[it.UserID.String()]
		}
		out = append(out, row)
	}
	c.JSON(http.StatusOK, gin.H{"count": len(out), "items": out})
}
