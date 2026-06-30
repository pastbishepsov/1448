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

// Параметры начисления. Простые на старте — позже вынесем в конфиг/Admin Panel.
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
// XP для перехода на следующий уровень: XP(n) = 1000 * n^1.4 (см. models.XPForNextLevel).
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

	session := models.Session{
		UserID:           userID,
		ComputerID:       computer.ID,
		ClubID:           club.ID,
		Status:           models.SessionStatusActive,
		StartedAt:        time.Now(),
		BaseRatePLN:      club.BaseRatePLN,
		EffectiveRatePLN: club.BaseRatePLN,
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

	c.JSON(http.StatusCreated, gin.H{
		"session_id": session.ID,
		"started_at": session.StartedAt,
		"computer":   computer.Name,
		"club":       club.Name,
		"rate_pln":   club.BaseRatePLN,
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

	now := time.Now()
	minutes := int(math.Ceil(now.Sub(session.StartedAt).Minutes()))
	if minutes < 0 {
		minutes = 0
	}
	// dev-оверрайд: позволяет тестировать начисление без ожидания реального времени
	if req.Minutes != nil && os.Getenv("SERVER_ENV") != "production" {
		minutes = *req.Minutes
		if minutes < 0 {
			minutes = 0
		}
	}

	xpGained := int64(minutes) * xpPerMinute
	coinsGained := int64(minutes) * coinsPerMinute

	var user models.User
	if err := db.First(&user, "id = ?", userID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "user_missing", "message": "Пользователь не найден"})
		return
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
		if err := tx.Omit("User", "Computer").Save(&session).Error; err != nil {
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
		c.JSON(http.StatusInternalServerError, gin.H{"code": "db_error", "message": err.Error()})
		return
	}

	// Кейс за каждый новый уровень.
	if levelsGained > 0 {
		for i := 0; i < levelsGained; i++ {
			_ = grantCase(db, user.ID, &session.ClubID, tierForLevel(user.Level), models.CaseSourceLevelUp)
		}
	}

	// Команда на ПК: завершить и заблокировать (если Shell подключён).
	notifyShell(session.ComputerID.String(), websocket.MsgSessionEnd, gin.H{
		"session_id":   session.ID,
		"xp_earned":    xpGained,
		"coins_earned": coinsGained,
	})

	c.JSON(http.StatusOK, gin.H{
		"session_id":    session.ID,
		"minutes":       minutes,
		"xp_earned":     xpGained,
		"coins_earned":  coinsGained,
		"levels_gained": levelsGained,
		"user": gin.H{
			"level":                 user.Level,
			"xp_current":            user.XPCurrent,
			"xp_total":              user.XPTotal,
			"xp_for_next_level":     models.XPForNextLevel(user.Level),
			"coins_balance":         user.CoinsBalance,
			"skillpoints_available": user.SkillpointsAvailable,
		},
	})
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
