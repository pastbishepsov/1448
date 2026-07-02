package main

// Депозиты (пополнение баланса) и связанная экономика.
// MVP: депозит оформляет администратор (наличные/карта на кассе клуба) —
// POST /admin/users/:id/deposit. Stripe/BLIK позже встанут на это же место.
//
// Эффекты:
//   - курс: 1 zł = coinsPerPLN монет;
//   - талант coin_mint (Intellect) даёт бонусные монеты к депозиту;
//   - депозит от depositCaseMinPLN злотых — Light-кейс (source=deposit);
//   - ачивка first_deposit (deposit_count) выдаётся через checkAchievements.

import (
	"math"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

const (
	coinsPerPLN       int64   = 10
	depositCaseMinPLN float64 = 20.0
	maxDepositPLN     float64 = 10000.0
	maxDiscountPct    float64 = 30.0
)

// depositCoins — монеты за депозит: база по курсу + бонус таланта coin_mint.
// Чистая функция (тестируется отдельно).
func depositCoins(amountPLN, mintEffect float64) (base, bonus int64) {
	base = int64(math.Round(amountPLN * float64(coinsPerPLN)))
	if mintEffect > 0 {
		bonus = int64(math.Round(float64(base) * mintEffect))
	}
	return base, bonus
}

// isNightHour — ночное время клуба (22:00–07:59): действует талант night_mode.
func isNightHour(hour int) bool {
	return hour >= 22 || hour < 8
}

// effectiveRate — тариф с учётом скидки (кэшбек игрока + талант cashback_master,
// ночью + night_mode), скидка ограничена maxDiscountPct. Чистая функция (тест).
func effectiveRate(basePLN, discountPct float64) float64 {
	if discountPct < 0 {
		discountPct = 0
	}
	if discountPct > maxDiscountPct {
		discountPct = maxDiscountPct
	}
	return math.Round(basePLN*(1-discountPct/100)*100) / 100
}

func userDepositCount(userID string) int {
	var n int64
	db.Model(&models.Deposit{}).Where("user_id = ?", userID).Count(&n)
	return int(n)
}

type depositRequest struct {
	AmountPLN float64 `json:"amount_pln" binding:"required,gt=0"`
	Method    string  `json:"method"` // cash (по умолчанию) | card | blik
}

// POST /admin/users/:id/deposit — оформить пополнение гостю.
func handleAdminDeposit(c *gin.Context) {
	var user models.User
	if err := db.First(&user, "id = ?", c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "user_not_found", "message": "Пользователь не найден"})
		return
	}
	if user.Status == models.UserStatusBanned {
		c.JSON(http.StatusConflict, gin.H{"code": "banned", "message": "Аккаунт заблокирован — сначала разбань"})
		return
	}

	var req depositRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_request", "message": "нужна сумма amount_pln > 0"})
		return
	}
	if req.AmountPLN > maxDepositPLN {
		c.JSON(http.StatusBadRequest, gin.H{"code": "too_much", "message": "Слишком большая сумма за раз"})
		return
	}
	method := req.Method
	if method == "" {
		method = "cash"
	}
	if method != "cash" && method != "card" && method != "blik" {
		c.JSON(http.StatusBadRequest, gin.H{"code": "bad_method", "message": "method: cash | card | blik"})
		return
	}

	base, bonus := depositCoins(req.AmountPLN, talentEffect(user.ID.String(), "coin_mint"))

	var createdBy *uuid.UUID
	if adminID, err := uuid.Parse(c.GetString("user_id")); err == nil {
		createdBy = &adminID
	}
	dep := models.Deposit{
		UserID: user.ID, AmountPLN: req.AmountPLN,
		CoinsGranted: base, BonusCoins: bonus,
		Method: method, CreatedBy: createdBy,
	}

	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&dep).Error; err != nil {
			return err
		}
		return tx.Model(&models.User{}).Where("id = ?", user.ID).
			Updates(map[string]any{
				"coins_balance":  gorm.Expr("coins_balance + ?", base+bonus),
				"last_active_at": time.Now(),
			}).Error
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "db_error", "message": err.Error()})
		return
	}

	// Кейс за ощутимый депозит.
	caseGranted := false
	if req.AmountPLN >= depositCaseMinPLN {
		if grantCase(db, user.ID, nil, models.CaseTierLight, models.CaseSourceDeposit) == nil {
			caseGranted = true
		}
	}

	// Ачивки (first_deposit и будущие deposit_count-пороги).
	uid := user.ID.String()
	checkAchievements(user.ID, playerStats{
		HoursPlayed:  userHoursPlayed(uid),
		LoginCount:   1,
		DepositCount: userDepositCount(uid),
	})

	db.First(&user, "id = ?", user.ID) // свежий баланс
	c.JSON(http.StatusCreated, gin.H{
		"deposit_id":    dep.ID,
		"nickname":      user.Nickname,
		"amount_pln":    req.AmountPLN,
		"coins_granted": base,
		"bonus_coins":   bonus,
		"case_granted":  caseGranted,
		"coins_balance": user.CoinsBalance,
	})
}

// GET /me/deposits — история пополнений игрока.
func handleGetMyDeposits(c *gin.Context) {
	var deposits []models.Deposit
	db.Where("user_id = ?", c.GetString("user_id")).
		Order("created_at DESC").Limit(50).Find(&deposits)
	c.JSON(http.StatusOK, gin.H{"count": len(deposits), "deposits": deposits})
}
