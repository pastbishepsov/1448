package main

// Ручные начисления администратора (ТЗ 7.1): XP или кейс, причина обязательна,
// каждая операция пишется в журнал admin_grants (позже попадёт в Owner Stats).

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/pastbishepsov/1448/backend/internal/models"
	"github.com/pastbishepsov/1448/backend/internal/websocket"
)

const maxGrantXP int64 = 100000

var validGrantTiers = map[string]models.CaseTier{
	"light": models.CaseTierLight, "medium": models.CaseTierMedium,
	"heavy": models.CaseTierHeavy, "titan": models.CaseTierTitan,
	"gods": models.CaseTierGods,
}

// validateGrant — чистая проверка запроса начисления (тестируется отдельно).
func validateGrant(grantType string, amount int64, tier, reason string) (ok bool, code string) {
	if strings.TrimSpace(reason) == "" {
		return false, "reason_required"
	}
	switch grantType {
	case "xp":
		if amount < 1 || amount > maxGrantXP {
			return false, "bad_amount"
		}
	case "case":
		if _, ok := validGrantTiers[tier]; !ok {
			return false, "bad_tier"
		}
	default:
		return false, "bad_type"
	}
	return true, ""
}

type grantRequest struct {
	Type     string `json:"type" binding:"required"` // xp | case
	Amount   int64  `json:"amount"`
	CaseTier string `json:"case_tier"`
	Reason   string `json:"reason"`
}

// POST /admin/users/:id/grant — начислить XP или кейс вручную.
// Цель — только player и не сам себе (Б0-и1, targetPlayer); для роли admin
// действует дневной потолок XP из настроек owner (Б0-и4).
func handleAdminGrant(c *gin.Context) {
	user := targetPlayer(c)
	if user == nil {
		return
	}

	var req grantRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_request", "message": err.Error()})
		return
	}
	if ok, code := validateGrant(req.Type, req.Amount, req.CaseTier, req.Reason); !ok {
		msg := map[string]string{
			"reason_required": "Причина начисления обязательна",
			"bad_amount":      "XP: от 1 до 100000",
			"bad_tier":        "case_tier: light | medium | heavy | titan | gods",
			"bad_type":        "type: xp | case",
		}[code]
		c.JSON(http.StatusBadRequest, gin.H{"code": code, "message": msg})
		return
	}

	adminID, err := uuid.Parse(c.GetString("user_id"))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": "invalid_user", "message": "Некорректный администратор"})
		return
	}

	// Б0-и4: дневной потолок XP-начислений для роли admin (0 = без лимита).
	if req.Type == "xp" && c.GetString("user_role") == string(models.UserRoleAdmin) {
		if cap := settingInt64("admin_day_xp_cap", 0); cap > 0 {
			var used int64
			db.Model(&models.AdminGrant{}).Select("COALESCE(SUM(amount),0)").
				Where("admin_id = ? AND grant_type = 'xp' AND created_at >= ?", adminID, startOfToday()).
				Scan(&used)
			if adminDayCapExceeded(float64(used), float64(req.Amount), float64(cap)) {
				c.JSON(http.StatusForbidden, gin.H{"code": "day_cap",
					"message": fmt.Sprintf("Дневной лимит XP-начислений админа: %d (уже начислено %d)", cap, used)})
				return
			}
		}
	}

	entry := models.AdminGrant{
		UserID: user.ID, AdminID: adminID,
		GrantType: req.Type, Reason: strings.TrimSpace(req.Reason),
	}

	levelsGained := 0
	var grantedTier models.CaseTier

	switch req.Type {
	case "xp":
		levelsGained = applyXP(user, req.Amount)
		entry.Amount = &req.Amount
		err = db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Save(user).Error; err != nil {
				return err
			}
			return tx.Create(&entry).Error
		})
		if err == nil && levelsGained > 0 {
			for i := 0; i < levelsGained; i++ {
				_ = grantCase(db, user.ID, nil, tierForLevel(user.Level), models.CaseSourceLevelUp)
			}
		}
		// Если игрок сейчас за ПК — послать xp_update на его Shell в реальном времени.
		if err == nil {
			var s models.Session
			if db.Where("user_id = ? AND status = ?", user.ID, models.SessionStatusActive).
				First(&s).Error == nil {
				notifyShell(s.ComputerID.String(), websocket.MsgXPUpdate, gin.H{
					"granted": req.Amount, "xp_total": user.XPTotal, "level": user.Level,
				})
			}
		}
	case "case":
		grantedTier = validGrantTiers[req.CaseTier]
		entry.CaseTier = &grantedTier
		err = db.Transaction(func(tx *gorm.DB) error {
			if err := grantCase(tx, user.ID, nil, grantedTier, models.CaseSourceAdminGrant); err != nil {
				return err
			}
			return tx.Create(&entry).Error
		})
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "db_error", "message": err.Error()})
		return
	}

	// Б4: гостю — тост о начислении, админкам — событие «grant» в ленту
	// (раньше грант был виден только в аудите поллингом).
	if req.Type == "xp" {
		notifyUser(user.ID, "grant_xp", map[string]any{
			"amount": req.Amount, "levels_gained": levelsGained, "level": user.Level,
		})
		hub.AdminBroadcast("grant", map[string]any{
			"nickname": user.Nickname, "type": "xp", "amount": req.Amount,
		})
	} else {
		notifyUser(user.ID, "grant_case", map[string]any{"tier": grantedTier})
		hub.AdminBroadcast("grant", map[string]any{
			"nickname": user.Nickname, "type": "case", "tier": grantedTier,
		})
	}

	c.JSON(http.StatusCreated, gin.H{
		"grant_id": entry.ID, "nickname": user.Nickname, "type": req.Type,
		"amount": req.Amount, "case_tier": grantedTier,
		"levels_gained": levelsGained, "level": user.Level,
	})
}

// GET /admin/grants — журнал ручных начислений (свежие сверху).
func handleAdminGrants(c *gin.Context) {
	var grants []models.AdminGrant
	db.Order("created_at DESC").Limit(100).Find(&grants)

	// ники одним запросом (получатель + админ)
	idSet := map[string]bool{}
	for _, g := range grants {
		idSet[g.UserID.String()] = true
		idSet[g.AdminID.String()] = true
	}
	ids := make([]string, 0, len(idSet))
	for id := range idSet {
		ids = append(ids, id)
	}
	nick := map[string]string{}
	if len(ids) > 0 {
		var users []models.User
		db.Where("id IN ?", ids).Find(&users)
		for _, u := range users {
			nick[u.ID.String()] = u.Nickname
		}
	}

	out := make([]gin.H, 0, len(grants))
	for _, g := range grants {
		out = append(out, gin.H{
			"id": g.ID, "created_at": g.CreatedAt,
			"nickname": nick[g.UserID.String()], "admin": nick[g.AdminID.String()],
			"type": g.GrantType, "amount": g.Amount, "case_tier": g.CaseTier,
			"reason": g.Reason,
		})
	}
	c.JSON(http.StatusOK, gin.H{"count": len(out), "grants": out})
}
