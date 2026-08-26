package main

import (
	"crypto/rand"
	"errors"
	"math"
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

// errCaseAlreadyOpened — кейс успел открыться параллельным запросом (ревью 26.08).
var errCaseAlreadyOpened = errors.New("case_already_opened")

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

// ── Прозрачность кейсов (RESEARCH.md §4): открытая таблица шансов ────────

// tierOdds — шансы одного тира для публичной выдачи. Проценты БАЗОВЫЕ:
// таланты (luck_grade) и ранг повышают шанс редких тиров — только в пользу
// игрока, поэтому публикуем нижнюю границу.
type tierOdds struct {
	Tier           models.CaseTier `json:"tier"`
	BonusRollPct   float64         `json:"bonus_roll_pct"` // шанс тира при ролле бонусного кейса
	CoinsPct       float64         `json:"coins_pct"`
	CoinsMin       int64           `json:"coins_min"`
	CoinsMax       int64           `json:"coins_max"`
	BusterPct      float64         `json:"buster_pct"`
	BusterBoostPct float64         `json:"buster_boost_pct"` // размер бустера кэшбека, %
	JackpotPct     float64         `json:"jackpot_pct"`
	JackpotAmount  int64           `json:"jackpot_amount"`
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }

// caseOddsTiers — проценты по тирам. Пороги зеркалят Roll (models/case.go):
// roll из 100000, сначала джекпот, затем бустер, остаток — монеты; RTP клуба
// множит пороги джекпота и бустера, клэмп на 100000 — как в Roll.
func caseOddsTiers(rtp float64) []tierOdds {
	out := make([]tierOdds, 0, len(bonusTierWeights))
	for _, w := range bonusTierWeights {
		cfg := models.DropConfigFor(w.tier)
		jt := float64(cfg.JackpotChance) * rtp
		bt := jt + float64(cfg.BusterChance)*rtp
		if jt > 100000 {
			jt = 100000
		}
		if bt > 100000 {
			bt = 100000
		}
		jackpotPct := jt / 1000
		busterPct := (bt - jt) / 1000
		out = append(out, tierOdds{
			Tier:           w.tier,
			BonusRollPct:   round2(w.weight / 100),
			CoinsPct:       round2(100 - jackpotPct - busterPct),
			CoinsMin:       cfg.CoinsMin,
			CoinsMax:       cfg.CoinsMax,
			BusterPct:      round2(busterPct),
			BusterBoostPct: float64(cfg.BusterAmount) / 100,
			JackpotPct:     round2(jackpotPct),
			JackpotAmount:  cfg.JackpotAmount,
		})
	}
	return out
}

// GET /cases/odds?club_id= — публичная таблица шансов кейсов (без JWT, как
// /catalog): версия и дата, проценты по тирам, распределение тиров бонусного
// кейса. club_id учитывает RTP-модификатор клуба, без него rtp = 1.
func handleGetCaseOdds(c *gin.Context) {
	rtp := 1.0
	if clubID := c.Query("club_id"); clubID != "" {
		var club models.Club
		if err := db.First(&club, "id = ?", clubID).Error; err == nil && club.RTPModifier > 0 {
			rtp = club.RTPModifier
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"version":      models.CaseOddsVersion,
		"updated_at":   models.CaseOddsDate,
		"rtp_modifier": rtp,
		"tiers":        caseOddsTiers(rtp),
	})
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

	now := time.Now()
	err = db.Transaction(func(tx *gorm.DB) error {
		// CAS по opened_at: два параллельных запроса (двойной тап в PWA) не
		// должны открыть один кейс дважды — второй уйдёт в already_opened.
		res := tx.Model(&models.Case{}).
			Where("id = ? AND opened_at IS NULL", box.ID).
			Updates(map[string]any{
				"opened_at":   now,
				"drop_type":   dropType,
				"drop_amount": amount,
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return errCaseAlreadyOpened
		}
		box.OpenedAt = &now
		box.DropType = &dropType
		box.DropAmount = &amount

		// ВАЖНО: только колонки награды. Полный Save(&user) писал бы и
		// wallet_grosz/coin_minutes значениями на момент чтения и затирал бы
		// параллельное списание биллинга или депозит у стойки (ревью 26.08).
		switch dropType {
		case models.DropTypeCoins:
			return tx.Model(&models.User{}).Where("id = ?", user.ID).
				UpdateColumn("coins_balance", gorm.Expr("coins_balance + ?", amount)).Error
		case models.DropTypeBuster:
			return tx.Model(&models.User{}).Where("id = ?", user.ID).
				UpdateColumn("payment_increase_pct",
					gorm.Expr("LEAST(payment_increase_pct + ?, ?)",
						float64(amount)/100.0, maxPaymentIncrease)).Error
		}
		return nil
	})
	if errors.Is(err, errCaseAlreadyOpened) {
		c.JSON(http.StatusConflict, gin.H{"code": "already_opened", "message": "Кейс уже открыт"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "db_error", "message": err.Error()})
		return
	}

	// Балансы перечитываем после коммита: в памяти они устарели бы на любое
	// параллельное начисление, а гость видит их сразу на экране.
	_ = db.First(&user, "id = ?", userID).Error

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
