package main

import (
	"crypto/rand"
	"math/big"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

const (
	caseTTLDays           = 30   // кейс сгорает через месяц бездействия
	maxPaymentIncrease    = 50.0 // потолок кэшбек-бонуса, % (тюнинг-параметр)
	baseSessionCaseChance = 0.20 // базовый шанс выпадения кейса за сессию (тюнинг)
	maxSessionCaseChance  = 0.85 // потолок шанса с учётом талантов
)

// chance — произошло ли событие с вероятностью p. crypto/rand, только на сервере.
func chance(p float64) bool {
	if p <= 0 {
		return false
	}
	if p >= 1 {
		return true
	}
	n, err := rand.Int(rand.Reader, big.NewInt(10000))
	if err != nil {
		return false
	}
	return float64(n.Int64()) < p*10000
}

// cryptoIntn — случайное целое в [0, n). crypto/rand.
func cryptoIntn(n int64) int64 {
	if n <= 0 {
		return 0
	}
	v, err := rand.Int(rand.Reader, big.NewInt(n))
	if err != nil {
		return 0
	}
	return v.Int64()
}

// Базовые веса тиров для бонусного кейса (сумма 10000): Light доминирует.
var bonusTierWeights = []struct {
	tier   models.CaseTier
	weight float64
	rare   bool // Heavy+ — на них действует talent luck_grade
}{
	{models.CaseTierLight, 6900, false},
	{models.CaseTierMedium, 2000, false},
	{models.CaseTierHeavy, 800, true},
	{models.CaseTierTitan, 250, true},
	{models.CaseTierGods, 50, true},
}

// rollCaseTier — взвешенный выбор тира бонусного кейса.
// luckBoost (talent luck_grade) множит веса Heavy+ на (1 + luckBoost).
func rollCaseTier(luckBoost float64) models.CaseTier {
	total := 0.0
	weighted := make([]float64, len(bonusTierWeights))
	for i, w := range bonusTierWeights {
		ww := w.weight
		if w.rare && luckBoost > 0 {
			ww *= 1 + luckBoost
		}
		weighted[i] = ww
		total += ww
	}
	r := float64(cryptoIntn(int64(total)))
	acc := 0.0
	for i, w := range bonusTierWeights {
		acc += weighted[i]
		if r < acc {
			return w.tier
		}
	}
	return models.CaseTierLight
}

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

	// Неоткрытые считаем без сгоревших (expires_at в прошлом).
	now := time.Now()
	unopened := 0
	for _, x := range cases {
		if x.OpenedAt == nil && x.ExpiresAt.After(now) {
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
