package main

// Профиль сенсы игрока (S3): DPI + сенсы по играм, синхронизируются с аккаунтом
// и подставляются на любом клубном ПК. GET/PUT /me/sensitivity.

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm/clause"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

const (
	minDPI      = 100
	maxDPI      = 32000
	maxSensVal  = 1000.0
	maxSensGame = 64 // ключей в профиле
)

// validateSensitivity — чистая проверка профиля (тестируется отдельно).
func validateSensitivity(dpi int, games map[string]float64) (ok bool, code string) {
	if dpi < minDPI || dpi > maxDPI {
		return false, "bad_dpi"
	}
	if len(games) > maxSensGame {
		return false, "too_many"
	}
	for id, s := range games {
		if id == "" || len(id) > 32 {
			return false, "bad_game"
		}
		if s <= 0 || s > maxSensVal {
			return false, "bad_sens"
		}
	}
	return true, ""
}

// GET /me/sensitivity — профиль сенсы игрока (дефолт, если ещё не сохранял).
func handleGetSensitivity(c *gin.Context) {
	userID := c.GetString("user_id")

	var p models.SensitivityProfile
	if err := db.First(&p, "user_id = ?", userID).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"dpi": 800, "games": gin.H{}, "saved": false})
		return
	}
	games := map[string]float64{}
	_ = json.Unmarshal([]byte(p.Games), &games)
	c.JSON(http.StatusOK, gin.H{
		"dpi": p.DPI, "games": games, "saved": true, "updated_at": p.UpdatedAt,
	})
}

type sensitivityRequest struct {
	DPI   int                `json:"dpi" binding:"required"`
	Games map[string]float64 `json:"games"`
}

// PUT /me/sensitivity — сохранить/обновить профиль сенсы (upsert по user_id).
func handlePutSensitivity(c *gin.Context) {
	userID, err := uuid.Parse(c.GetString("user_id"))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": "invalid_user", "message": "Некорректный пользователь"})
		return
	}

	var req sensitivityRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_request", "message": err.Error()})
		return
	}
	if req.Games == nil {
		req.Games = map[string]float64{}
	}
	if ok, code := validateSensitivity(req.DPI, req.Games); !ok {
		msg := map[string]string{
			"bad_dpi":  "DPI должен быть от 100 до 32000",
			"too_many": "Слишком много игр в профиле",
			"bad_game": "Некорректный идентификатор игры",
			"bad_sens": "Сенса должна быть больше 0",
		}[code]
		c.JSON(http.StatusBadRequest, gin.H{"code": code, "message": msg})
		return
	}

	gamesJSON, _ := json.Marshal(req.Games)
	profile := models.SensitivityProfile{
		UserID: userID, DPI: req.DPI, Games: string(gamesJSON),
	}
	// upsert: создать или обновить dpi/games/updated_at
	if err := db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"dpi", "games", "updated_at"}),
	}).Create(&profile).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "db_error", "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"dpi": req.DPI, "games": req.Games, "saved": true})
}
