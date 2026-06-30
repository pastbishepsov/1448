package main

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

const (
	caseTTLDays        = 30   // кейс сгорает через месяц бездействия
	maxPaymentIncrease = 50.0 // потолок кэшбек-бонуса, % (тюнинг-параметр)
)

// grantCase — выдать кейс игроку. db может быть обычным или транзакционным.
func grantCase(db *gorm.DB, userID uuid.UUID, clubID *uuid.UUID, tier models.CaseTier, source models.CaseSource) error {
	c := models.Case{
		UserID:    userID,
		ClubID:    clubID,
		Tier:      tier,
		Source:    source,
		ExpiresAt: time.Now().AddDate(0, 0, caseTTLDays),
	}
	return db.Create(&c).Error
}

// tierForLevel — кейс какого тира выдать за достигнутый уровень.
func tierForLevel(level int) models.CaseTier {
	switch {
	case level >= 50:
		return models.CaseTierGods
	case level >= 30:
		return models.CaseTierTitan
	case level >= 15:
		return models.CaseTierHeavy
	case level >= 5:
		return models.CaseTierMedium
	default:
		return models.CaseTierLight
	}
}

// GET /me/cases — список кейсов игрока (неоткрытые сверху).
func handleGetMyCases(c *gin.Context) {
	userID := c.GetString("user_id")

	var cases []models.Case
	db.Where("user_id = ?", userID).
		Order("(opened_at IS NULL) DESC").
		Order("created_at DESC").
		Find(&cases)

	unopened := 0
	for _, x := range cases {
		if x.OpenedAt == nil {
			unopened++
		}
	}

	c.JSON(http.StatusOK, gin.H{"count": len(cases), "unopened": unopened, "cases": cases})
}

// POST /me/cases/:id/open — открыть кейс. Дроп считается ТОЛЬКО на сервере (crypto/rand).
func handleOpenCase(c *gin.Context) {
	userID := c.GetString("user_id")
	caseID := c.Param("id")

	var box models.Case
	if err := db.Where("id = ? AND user_id = ?", caseID, userID).First(&box).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "case_not_found", "message": "Кейс не найден"})
		return
	}
	if box.OpenedAt != nil {
		c.JSON(http.StatusConflict, gin.H{"code": "already_opened", "message": "Кейс уже открыт"})
		return
	}
	if time.Now().After(box.ExpiresAt) {
		c.JSON(http.StatusGone, gin.H{"code": "expired", "message": "Кейс сгорел"})
		return
	}

	// RTP-модификатор клуба (если кейс выдан в клубе).
	rtp := 1.0
	if box.ClubID != nil {
		var club models.Club
		if err := db.First(&club, "id = ?", *box.ClubID).Error; err == nil && club.RTPModifier > 0 {
			rtp = club.RTPModifier
		}
	}

	dropType, amount, err := box.Roll(rtp)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "roll_error", "message": err.Error()})
		return
	}

	var user models.User
	if err := db.First(&user, "id = ?", userID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "user_missing", "message": "Пользователь не найден"})
		return
	}

	// Применяем дроп к игроку.
	switch dropType {
	case models.DropTypeCoins:
		user.CoinsBalance += amount
	case models.DropTypeBuster:
		// BusterAmount — в сотых процента (100 = 1%).
		user.PaymentIncreasePct += float64(amount) / 100.0
		if user.PaymentIncreasePct > maxPaymentIncrease {
			user.PaymentIncreasePct = maxPaymentIncrease
		}
	}

	now := time.Now()
	err = db.Transaction(func(tx *gorm.DB) error {
		box.OpenedAt = &now
		box.DropType = &dropType
		box.DropAmount = &amount
		if err := tx.Save(&box).Error; err != nil {
			return err
		}
		return tx.Save(&user).Error
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "db_error", "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"case_id":     box.ID,
		"tier":        box.Tier,
		"drop_type":   dropType,
		"drop_amount": amount,
		"user": gin.H{
			"coins_balance":        user.CoinsBalance,
			"payment_increase_pct": user.PaymentIncreasePct,
		},
	})
}
