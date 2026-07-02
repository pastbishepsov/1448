package main

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

// playerStats — статистика игрока для проверки условий достижений.
type playerStats struct {
	HoursPlayed  int
	LoginCount   int
	DepositCount int
}

// conditionMet — выполнено ли условие достижения при данной статистике игрока.
// Чистая функция (тестируется в achievements_test.go).
// Поддержаны типы: hours_played, login_count, deposit_count. Остальные — по мере появления механик.
func conditionMet(condType, condValueJSON string, s playerStats) bool {
	var cv struct {
		Min *int `json:"min"`
	}
	_ = json.Unmarshal([]byte(condValueJSON), &cv)
	min := 0
	if cv.Min != nil {
		min = *cv.Min
	}
	switch condType {
	case "hours_played":
		return s.HoursPlayed >= min
	case "login_count":
		return s.LoginCount >= min
	case "deposit_count":
		return s.DepositCount >= min
	default:
		return false
	}
}

// userHoursPlayed — суммарные часы игрока по завершённым сессиям.
func userHoursPlayed(userID string) int {
	var totalMin int64
	db.Model(&models.Session{}).
		Where("user_id = ? AND status = ?", userID, models.SessionStatusCompleted).
		Select("COALESCE(SUM(minutes_used), 0)").
		Scan(&totalMin)
	return int(totalMin / 60)
}

// checkAchievements — выдать новые lifetime-достижения и их награды (best-effort).
func checkAchievements(userID uuid.UUID, stats playerStats) {
	var earned []models.UserAchievement
	db.Where("user_id = ?", userID).Find(&earned)
	have := map[string]bool{}
	for _, e := range earned {
		have[e.AchievementID] = true
	}

	var defs []models.Achievement
	db.Where("category = ? AND is_active = ?", "lifetime", true).Find(&defs)

	for _, a := range defs {
		if have[a.ID] {
			continue
		}
		if !conditionMet(a.ConditionType, a.ConditionValue, stats) {
			continue
		}
		tier := a.RewardCaseTier
		sp := a.RewardSkillpoints
		_ = db.Transaction(func(tx *gorm.DB) error {
			ua := models.UserAchievement{UserID: userID, AchievementID: a.ID, EarnedAt: time.Now()}
			if err := tx.Create(&ua).Error; err != nil {
				return err
			}
			if sp > 0 {
				if err := tx.Model(&models.User{}).Where("id = ?", userID).
					Update("skillpoints_available", gorm.Expr("skillpoints_available + ?", sp)).Error; err != nil {
					return err
				}
			}
			if tier != nil {
				if err := grantCase(tx, userID, nil, *tier, models.CaseSourceAchievement); err != nil {
					return err
				}
			}
			return nil
		})
	}
}

// GET /me/achievements — список достижений с отметкой о получении.
func handleGetMyAchievements(c *gin.Context) {
	userID := c.GetString("user_id")

	var defs []models.Achievement
	db.Where("is_active = ?", true).Order("category").Find(&defs)

	var earned []models.UserAchievement
	db.Where("user_id = ?", userID).Find(&earned)
	earnedAt := map[string]time.Time{}
	for _, e := range earned {
		earnedAt[e.AchievementID] = e.EarnedAt
	}

	out := make([]gin.H, 0, len(defs))
	for _, a := range defs {
		item := gin.H{
			"id":                 a.ID,
			"title":              a.Title,
			"description":        a.Description,
			"category":           a.Category,
			"reward_skillpoints": a.RewardSkillpoints,
			"reward_case_tier":   a.RewardCaseTier,
			"earned":             false,
		}
		if t, ok := earnedAt[a.ID]; ok {
			item["earned"] = true
			item["earned_at"] = t
		}
		out = append(out, item)
	}

	c.JSON(http.StatusOK, gin.H{
		"hours_played": userHoursPlayed(userID),
		"earned_count": len(earned),
		"achievements": out,
	})
}
