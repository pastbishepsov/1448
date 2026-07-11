package main

import (
	"math"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/pastbishepsov/1448/backend/internal/models"
	"github.com/pastbishepsov/1448/backend/internal/websocket"
)

// Дефолты начисления. Рабочие значения владелец задаёт в админке —
// таблица settings (миграция 017), чтение через settingInt64 (спринт А5).
const (
	xpPerMinute    int64 = 10
	coinsPerMinute int64 = 2
)

type startSessionRequest struct {
	ComputerID *string `json:"computer_id"` // необязательно: иначе берём первый свободный
}

type endSessionRequest struct {
	Minutes *int `json:"minutes"` // dev-оверрайд; в production время берётся из факта
}

// applyXP — начисляет опыт и обрабатывает повышение уровня.
// XP для перехода на следующий уровень: XP(n) = 1000 * n^1.4, округлено до 100
// (см. models.XPForNextLevel; правило цифр — DESIGN.md).
func applyXP(u *models.User, gained int64) int {
	if gained <= 0 {
		return 0
	}
	startLevel := u.Level
	u.XPTotal += gained
	u.XPCurrent += gained
	for u.XPCurrent >= models.XPForNextLevel(u.Level) {
		u.XPCurrent -= models.XPForNextLevel(u.Level)
		u.Level++
		u.SkillpointsAvailable++ // одно очко навыка за уровень
	}
	return u.Level - startLevel
}

// boostedXP — базовый опыт с учётом таланта (effect — доля прибавки, напр. 0.30 = +30%).
func boostedXP(baseXP int64, effect float64) int64 {
	if effect <= 0 {
		return baseXP
	}
	return int64(math.Round(float64(baseXP) * (1 + effect)))
}

// POST /me/sessions/start — начать игровую сессию за компьютером.
func handleStartSession(c *gin.Context) {
	userID, err := uuid.Parse(c.GetString("user_id"))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": "invalid_user", "message": "Некорректный пользователь"})
		return
	}

	var req startSessionRequest
	_ = c.ShouldBindJSON(&req) // тело необязательно

	// Уже есть активная сессия?
	var activeCount int64
	db.Model(&models.Session{}).
		Where("user_id = ? AND status = ?", userID, models.SessionStatusActive).
		Count(&activeCount)
	if activeCount > 0 {
		c.JSON(http.StatusConflict, gin.H{"code": "session_active", "message": "У вас уже есть активная сессия"})
		return
	}

	// Выбрать компьютер: по id или первый свободный.
	var computer models.Computer
	query := db.Where("status = ?", models.ComputerStatusAvailable)
	if req.ComputerID != nil && *req.ComputerID != "" {
		query = db.Where("id = ?", *req.ComputerID)
	}
	if err := query.First(&computer).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "no_computer", "message": "Нет доступного компьютера"})
		return
	}
	if computer.Status != models.ComputerStatusAvailable {
		c.JSON(http.StatusConflict, gin.H{"code": "computer_busy", "message": "Компьютер занят"})
		return
	}

	var club models.Club
	if err := db.First(&club, "id = ?", computer.ClubID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "club_missing", "message": "Клуб не найден"})
		return
	}

	// Скидка на тариф: кэшбек игрока (бустеры кейсов) + талант cashback_master;
	// ночью (22:00–07:59) дополнительно действует талант night_mode.
	var user models.User
	_ = db.First(&user, "id = ?", userID).Error
	discountPct := user.PaymentIncreasePct + talentEffect(userID.String(), "cashback_master")*100
	if isNightHour(time.Now().Hour()) {
		discountPct += talentEffect(userID.String(), "night_mode") * 100
	}
	if discountPct > maxDiscountPct {
		discountPct = maxDiscountPct
	}
	rate := effectiveRate(club.BaseRatePLN, discountPct)

	session := models.Session{
		UserID:           userID,
		ComputerID:       computer.ID,
		ClubID:           club.ID,
		Status:           models.SessionStatusActive,
		StartedAt:        time.Now(),
		BaseRatePLN:      club.BaseRatePLN,
		EffectiveRatePLN: rate,
	}

	err = db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Omit("User", "Computer").Create(&session).Error; err != nil {
			return err
		}
		return tx.Model(&models.Computer{}).
			Where("id = ?", computer.ID).
			Update("status", models.ComputerStatusInSession).Error
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "db_error", "message": err.Error()})
		return
	}

	// Команда на ПК в реальном времени (если Shell подключён).
	notifyShell(computer.ID.String(), websocket.MsgSessionStart, gin.H{
		"session_id": session.ID,
		"started_at": session.StartedAt,
	})

	// Live-событие в админку (спринт А6).
	hub.AdminBroadcast("session_start", map[string]any{
		"computer_id": computer.ID.String(),
		"computer":    computer.Name,
		"nickname":    user.Nickname,
	})

	c.JSON(http.StatusCreated, gin.H{
		"session_id":         session.ID,
		"started_at":         session.StartedAt,
		"computer":           computer.Name,
		"club":               club.Name,
		"rate_pln":           club.BaseRatePLN,
		"effective_rate_pln": rate,
		"discount_pct":       discountPct,
	})
}

// POST /me/sessions/end — завершить активную сессию и начислить XP/coins.
func handleEndSession(c *gin.Context) {
	userID := c.GetString("user_id")

	var req endSessionRequest
	_ = c.ShouldBindJSON(&req)

	var session models.Session
	if err := db.Where("user_id = ? AND status = ?", userID, models.SessionStatusActive).
		First(&session).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "no_active_session", "message": "Активная сессия не найдена"})
		return
	}

	res, err := finishSession(&session, req.Minutes)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "db_error", "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, finishResponse(res))
}

// finishResult — итог завершения сессии (общий для игрока и админа).
type finishResult struct {
	Session        *models.Session
	Minutes        int
	XPGained       int64
	CoinsGained    int64
	DailyBonusXP   int64
	LevelsGained   int
	BonusCase      bool
	BonusCaseTier  models.CaseTier
	BonusCaseTier2 models.CaseTier
	Rank           AccountRank
	User           models.User
}

func finishResponse(r *finishResult) gin.H {
	return gin.H{
		"session_id":    r.Session.ID,
		"minutes":       r.Minutes,
		"xp_earned":     r.XPGained,
		"coins_earned":  r.CoinsGained,
		"daily_bonus_xp": r.DailyBonusXP,
		"levels_gained":   r.LevelsGained,
		"bonus_case":      r.BonusCase,
		"bonus_case_tier": r.BonusCaseTier,
		"bonus_case_tier_2": r.BonusCaseTier2,
		"rank":            gin.H{"level": r.Rank.Level, "name": r.Rank.Name, "xp_mult": r.Rank.XPMult, "coin_mult": r.Rank.CoinMult},
		"user": gin.H{
			"level":                 r.User.Level,
			"xp_current":            r.User.XPCurrent,
			"xp_total":              r.User.XPTotal,
			"xp_for_next_level":     models.XPForNextLevel(r.User.Level),
			"coins_balance":         r.User.CoinsBalance,
			"skillpoints_available": r.User.SkillpointsAvailable,
		},
	}
}

// finishSession — единая логика завершения: начисление XP/coins, уровни, кейсы,
// достижения, освобождение ПК, команда Shell. Используется игроком (/me/sessions/end)
// и админом (/admin/sessions/:id/end).
func finishSession(session *models.Session, minutesOverride *int) (*finishResult, error) {
	userID := session.UserID.String()

	now := time.Now()
	minutes := int(math.Ceil(now.Sub(session.StartedAt).Minutes()))
	if minutes < 0 {
		minutes = 0
	}
	// dev-оверрайд: позволяет тестировать начисление без ожидания реального времени
	if minutesOverride != nil && os.Getenv("SERVER_ENV") != "production" {
		minutes = *minutesOverride
		if minutes < 0 {
			minutes = 0
		}
	}

	// Ранг аккаунта (по наигранным часам) множит XP/coins и бустит кейсы.
	rank, _ := accountRankFor(userHoursPlayed(userID))

	// Ставки начисления — из настроек клуба (settings, спринт А5).
	xpRate := settingInt64("xp_per_min", xpPerMinute)
	coinRate := settingInt64("coins_per_min", coinsPerMinute)

	// XP: базовый × (1 + xp_boost) × множитель ранга. Итог кратен 5 (правило цифр).
	xpGained := models.RoundToStep(int64(math.Round(float64(boostedXP(int64(minutes)*xpRate, talentEffect(userID, "xp_boost")))*rank.XPMult)), 5)
	// coins: базовый × множитель ранга. Итог кратен 5.
	coinsGained := models.RoundToStep(int64(math.Round(float64(int64(minutes)*coinRate)*rank.CoinMult)), 5)

	// Первый визит за день: +50 XP (фиксировано, без модификаторов — ТЗ 4.1).
	dailyBonusXP := int64(0)
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	var todayCount int64
	db.Model(&models.Session{}).
		Where("user_id = ? AND status = ? AND ended_at >= ? AND id <> ?",
			userID, models.SessionStatusCompleted, startOfDay, session.ID).
		Count(&todayCount)
	if todayCount == 0 {
		dailyBonusXP = 50
		xpGained += dailyBonusXP
	}

	var user models.User
	if err := db.First(&user, "id = ?", userID).Error; err != nil {
		return nil, err
	}

	levelsGained := applyXP(&user, xpGained)
	user.CoinsBalance += coinsGained
	user.LastActiveAt = now

	err := db.Transaction(func(tx *gorm.DB) error {
		session.Status = models.SessionStatusCompleted
		session.EndedAt = &now
		session.MinutesUsed = minutes
		session.XPEarned = xpGained
		session.CoinsEarned = coinsGained
		if err := tx.Omit("User", "Computer").Save(session).Error; err != nil {
			return err
		}
		if err := tx.Model(&models.Computer{}).
			Where("id = ?", session.ComputerID).
			Update("status", models.ComputerStatusAvailable).Error; err != nil {
			return err
		}
		return tx.Save(&user).Error
	})
	if err != nil {
		return nil, err
	}

	// Кейс за каждый новый уровень.
	if levelsGained > 0 {
		for i := 0; i < levelsGained; i++ {
			_ = grantCase(db, user.ID, &session.ClubID, tierForLevel(user.Level), models.CaseSourceLevelUp)
		}
	}

	// Бонусный кейс за сессию. Шанс: case_hunter + ранг; тир: luck_grade + ранг.
	bonusCase := false
	var bonusCaseTier models.CaseTier
	dropChance := sessionCaseChance(talentEffect(userID, "case_hunter"), rank.CaseChanceBonus)
	tierBoost := talentEffect(userID, "luck_grade") + rank.TierBoost
	if chance(dropChance) {
		bonusCaseTier = rollCaseTier(tierBoost)
		if grantCase(db, user.ID, &session.ClubID, bonusCaseTier, models.CaseSourceDailyVisit) == nil {
			bonusCase = true
		}
	}

	// Талант double_drop (Strength): шанс второго бонусного кейса (тир роллится заново).
	var bonusCaseTier2 models.CaseTier
	if bonusCase && chance(talentEffect(userID, "double_drop")) {
		bonusCaseTier2 = rollCaseTier(tierBoost)
		if grantCase(db, user.ID, &session.ClubID, bonusCaseTier2, models.CaseSourceDailyVisit) != nil {
			bonusCaseTier2 = ""
		}
	}

	// Достижения: часы, входы, депозиты.
	checkAchievements(session.UserID, playerStats{
		HoursPlayed:  userHoursPlayed(userID),
		LoginCount:   1,
		DepositCount: userDepositCount(userID),
	})

	// Команда на ПК: завершить и заблокировать (если Shell подключён).
	notifyShell(session.ComputerID.String(), websocket.MsgSessionEnd, gin.H{
		"session_id":   session.ID,
		"xp_earned":    xpGained,
		"coins_earned": coinsGained,
	})

	// Live-событие в админку (спринт А6).
	hub.AdminBroadcast("session_end", map[string]any{
		"computer_id": session.ComputerID.String(),
		"nickname":    user.Nickname,
		"minutes":     minutes,
		"xp":          xpGained,
		"coins":       coinsGained,
	})

	return &finishResult{
		Session: session, Minutes: minutes,
		XPGained: xpGained, CoinsGained: coinsGained,
		DailyBonusXP: dailyBonusXP,
		LevelsGained: levelsGained,
		BonusCase:    bonusCase, BonusCaseTier: bonusCaseTier, BonusCaseTier2: bonusCaseTier2,
		Rank: rank,
		User: user,
	}, nil
}

// GET /me/sessions — последние сессии текущего игрока.
func handleGetMySessions(c *gin.Context) {
	userID := c.GetString("user_id")

	var sessions []models.Session
	db.Where("user_id = ?", userID).
		Order("started_at DESC").
		Limit(50).
		Find(&sessions)

	c.JSON(http.StatusOK, gin.H{"count": len(sessions), "sessions": sessions})
}
