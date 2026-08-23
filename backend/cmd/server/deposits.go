package main

// Депозиты (пополнение баланса) и связанная экономика.
// MVP: депозит оформляет администратор (наличные/карта на кассе клуба) —
// POST /admin/users/:id/deposit. Stripe/BLIK позже встанут на это же место.
//
// Эффекты (Г0-и2, трек Г: депозит наполняет денежный КОШЕЛЁК — Р1 GUEST.md):
//   - деньги: amount_pln → users.wallet_grosz через walletApply, строка в
//     журнале wallet_transactions; сессия спишет их поминутно (Г1), остаток
//     живёт между визитами — «выйти и не потерять деньги»;
//   - кэшбек-монеты: 1 zł = coinsPerPLN монет — прежний курс оставлен
//     сознательно, при кошельке он щедрый (вопрос владельцу №6 GUEST.md);
//   - талант coin_mint (Intellect) даёт бонусные монеты к депозиту;
//   - ачивка first_deposit (deposit_count) выдаётся через checkAchievements.
//
// Кейса за депозит НЕТ: убран 2026-07-23 решением основателя (RESEARCH §4) —
// «реальные деньги → случайная награда» было единственной тонкой связкой по
// польскому лутбокс-проекту; теперь кейсы никак не связаны с деньгами
// (только игра/уровни/ачивки/гранты). Старые кейсы source=deposit остаются
// в истории, enum case_source не трогаем.

import (
	"fmt"
	"math"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

const (
	coinsPerPLN    int64   = 10
	maxDepositPLN  float64 = 10000.0
	maxDiscountPct float64 = 30.0
)

// depositCoins — монеты за депозит: база по курсу + бонус таланта coin_mint.
// Курс rate приходит из настроек клуба (settings, спринт А5), дефолт —
// coinsPerPLN. Чистая функция (тест). Оба значения кратны 5 (правило цифр).
func depositCoins(amountPLN, mintEffect float64, rate int64) (base, bonus int64) {
	base = models.RoundToStep(int64(math.Round(amountPLN*float64(rate))), 5)
	if mintEffect > 0 {
		bonus = models.RoundToStep(int64(math.Round(float64(base)*mintEffect)), 5)
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
// Цель — только player и не сам себе (Б0-и1, targetPlayer); для роли admin
// действует дневной потолок из настроек owner (Б0-и4).
func handleAdminDeposit(c *gin.Context) {
	user := targetPlayer(c)
	if user == nil {
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

	// Б0-и4: дневной потолок депозитов для роли admin (0 = без лимита).
	if c.GetString("user_role") == string(models.UserRoleAdmin) {
		if cap := settingInt64("admin_day_deposit_cap_pln", 0); cap > 0 {
			var used float64
			db.Model(&models.Deposit{}).Select("COALESCE(SUM(amount_pln),0)").
				Where("created_by = ? AND created_at >= ?", c.GetString("user_id"), startOfToday()).
				Scan(&used)
			if adminDayCapExceeded(used, req.AmountPLN, float64(cap)) {
				c.JSON(http.StatusForbidden, gin.H{"code": "day_cap",
					"message": fmt.Sprintf("Дневной лимит депозитов админа: %d zł (уже оформлено %.0f zł)", cap, used)})
				return
			}
		}
	}

	base, bonus := depositCoins(req.AmountPLN, talentEffect(user.ID.String(), "coin_mint"),
		settingInt64("coins_per_pln", coinsPerPLN))

	var createdBy *uuid.UUID
	if adminID, err := uuid.Parse(c.GetString("user_id")); err == nil {
		createdBy = &adminID
	}
	dep := models.Deposit{
		UserID: user.ID, AmountPLN: req.AmountPLN,
		CoinsGranted: base, BonusCoins: bonus,
		Method: method, CreatedBy: createdBy,
	}

	amountGrosz := models.GroszFromPLN(req.AmountPLN)
	var walletAfter int64
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&dep).Error; err != nil {
			return err
		}
		// Г0-и2 (Р1 GUEST.md): деньги депозита — в кошелёк, единственной
		// дверью walletApply (журнал + инвариант баланса).
		var werr error
		walletAfter, werr = walletApply(tx, walletOp{
			UserID: user.ID, Kind: models.WalletTxDeposit, Amount: amountGrosz,
			RefType: "deposit", RefID: &dep.ID, CreatedBy: createdBy,
		})
		if werr != nil {
			return werr
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

	// Ачивки (first_deposit и будущие deposit_count-пороги).
	uid := user.ID.String()
	checkAchievements(user.ID, playerStats{
		HoursPlayed:  userHoursPlayed(uid),
		LoginCount:   1,
		DepositCount: userDepositCount(uid),
	})

	// Live-событие в админку (спринт А6).
	hub.AdminBroadcast("deposit", map[string]any{
		"nickname":   user.Nickname,
		"amount_pln": req.AmountPLN,
		"coins":      base + bonus,
	})

	db.First(user, "id = ?", user.ID) // свежий баланс

	// Б4: гостю — тост о пополнении (сумма, монеты, новый баланс + кошелёк)
	notifyUser(user.ID, "deposit", map[string]any{
		"amount_pln": req.AmountPLN, "coins": base + bonus, "balance": user.CoinsBalance,
		"wallet_pln": models.PLNFromGrosz(walletAfter),
	})

	c.JSON(http.StatusCreated, gin.H{
		"deposit_id":    dep.ID,
		"nickname":      user.Nickname,
		"amount_pln":    req.AmountPLN,
		"coins_granted": base,
		"bonus_coins":   bonus,
		"coins_balance": user.CoinsBalance,
		"wallet_grosz":  walletAfter,
		"wallet_pln":    models.PLNFromGrosz(walletAfter),
	})
}

// GET /me/deposits — история пополнений игрока.
func handleGetMyDeposits(c *gin.Context) {
	var deposits []models.Deposit
	db.Where("user_id = ?", c.GetString("user_id")).
		Order("created_at DESC").Limit(50).Find(&deposits)
	c.JSON(http.StatusOK, gin.H{"count": len(deposits), "deposits": deposits})
}
